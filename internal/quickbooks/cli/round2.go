package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
	"os"
	"strings"
)

// newAccountingCDCCmd implements `accounting cdc get`: v3 change-data
// capture (verified live TC2). Dry-run plans.
func newAccountingCDCCmd(flags *rootFlags) *cobra.Command {
	var entities, since string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "List entities changed since a timestamp (v3 CDC)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "accounting cdc get", ID: "QBO.ACCOUNTING.CDC_READ",
					Mode: modeRead, Method: "GET",
					URL:   "https://qbo.intuit.com/api/v3/company/{realm}/cdc",
					Flags: localFlagMap(cmd), Note: "change-data-capture query; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			out, err := client.ReplayCDC(ctx, entities, since, 100)
			if err != nil {
				return feedErr(flags, err)
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return nil
		},
	}
	cmd.Flags().StringVar(&entities, "entities", "", "comma-separated v3 names, e.g. Invoice,Bill (required)")
	cmd.Flags().StringVar(&since, "since", "", "changed-since timestamp RFC3339 or yyyy-MM-dd (required)")
	_ = cmd.MarkFlagRequired("entities")
	_ = cmd.MarkFlagRequired("since")
	return cmd
}

// newSalesSendCmd builds `sales {entity} run send` for invoice/estimate/
// credit-memo/receipt (verified live TC2: endpoint+auth proven; a 200 means
// mail went out, so --to is explicit and dry-run plans first).
func newSalesSendCmd(flags *rootFlags, entity, opID string) *cobra.Command {
	var id, to string
	cmd := &cobra.Command{
		Use:   "send",
		Short: "Email a " + entity + " (POST /send)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: cmd.CommandPath(), ID: opID,
					Mode: modeWired, Method: "POST",
					URL:   "https://qbo.intuit.com/api/v3/company/{realm}/" + entity + "/{id}/send",
					Flags: localFlagMap(cmd), Note: "sends customer email; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			out, err := client.SendSalesForm(ctx, entity, id, to)
			if err != nil {
				return feedErr(flags, err)
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", entity+" id (required)")
	cmd.Flags().StringVar(&to, "to", "", "override recipient email (default: entity email)")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}

// newSalesFormPDFCmd builds `sales {entity} pdf` for estimate/credit-memo/
// receipt (same %PDF-verified path as invoice pdf).
func newSalesFormPDFCmd(flags *rootFlags, entity, opID string) *cobra.Command {
	var id, out string
	cmd := &cobra.Command{
		Use:   "pdf",
		Short: "Download a " + entity + " as PDF",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: cmd.CommandPath(), ID: opID,
					Mode: modeRead, Method: "GET",
					URL:   "https://qbo.intuit.com/api/v3/company/{realm}/" + entity + "/{id}/pdf",
					Flags: localFlagMap(cmd), Note: "binary PDF download; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplaySalesFormPDF(ctx, entity, id, out)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				fmt.Fprintf(cmd.OutOrStdout(), "{\"path\":%q,\"bytes\":%d,\"content_type\":%q}\n",
					res.Path, res.Bytes, res.ContentType)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s PDF %d bytes -> %s\n", entity, res.Bytes, res.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", entity+" id (required)")
	cmd.Flags().StringVar(&out, "out", "", "output .pdf path (required)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

// newCompanyWebhookVerifyCmd implements `company webhook verify`: local
// HMAC-SHA256 check of an Intuit webhook payload (no dial). Unit-tested;
// no live call exists by design.
func newCompanyWebhookVerifyCmd(flags *rootFlags) *cobra.Command {
	var file, sig, token string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify an Intuit webhook signature (local HMAC)",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := os.ReadFile(file)
			if err != nil {
				return exitInput(fmt.Errorf("reading payload: %w", err))
			}
			if err := client.VerifyWebhookSignature(raw, sig, token); err != nil {
				return feedErr(flags, err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "webhook signature valid")
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "payload", "", "raw webhook body file (required)")
	cmd.Flags().StringVar(&sig, "signature", "", "intuit-signature header value (required)")
	cmd.Flags().StringVar(&token, "token", "", "webhook verifier token (required)")
	_ = cmd.MarkFlagRequired("payload")
	_ = cmd.MarkFlagRequired("signature")
	_ = cmd.MarkFlagRequired("token")
	return cmd
}

// newTaxCodeCreateCmd implements `tax code create` via the v3 taxservice
// (endpoint + validation proven live TC2; full success needs a real
// TaxAgency, absent on the test company). Dry-run plans.
func newTaxCodeCreateCmd(flags *rootFlags) *cobra.Command {
	var name, rates string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a tax code via the v3 taxservice",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "tax code create", ID: "QBO.TAX.CODE_CREATE",
					Mode: modeWired, Method: "POST",
					URL:   "https://qbo.intuit.com/api/v3/company/{realm}/taxservice/taxcode",
					Flags: localFlagMap(cmd), Note: "taxservice POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			out, err := client.CreateTaxCode(ctx, name, rates)
			if err != nil {
				return feedErr(flags, err)
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "tax-code name (required)")
	cmd.Flags().StringVar(&rates, "rates-json", "", "TaxRateDetails JSON array, @file allowed (needs a real tax agency)")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

// newAdvancedBatchRunCmd implements `advanced batch run`: native v3 batch
// (up to 25 mixed read/write ops per call, verified live TC2). Items pass
// through verbatim: {"bId":"1","Query":"select ..."} or
// {"bId":"2","operation":"create","Invoice":{...}}. R4: a batch can mutate.
func newAdvancedBatchRunCmd(flags *rootFlags) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "run",
		Short: "POST a native v3 batch (up to 25 ops)",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw := file
			if raw == "" {
				return exitInput(fmt.Errorf("batch run requires --file (@path allowed)"))
			}
			if !strings.HasPrefix(strings.TrimSpace(raw), "@") && !strings.HasPrefix(strings.TrimSpace(raw), "[") {
				raw = "@" + raw
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "advanced batch run", ID: "QBO.ADVANCED.BATCH_RUN",
					Mode: modeWired, Method: "POST",
					URL:   "https://qbo.intuit.com/api/v3/company/{realm}/batch",
					Flags: localFlagMap(cmd), Note: "native batch POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			items, err := client.ReplayBatch(ctx, raw)
			if err != nil {
				if items != nil {
					_ = json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"items": items, "success": false, "readback_required": true})
				}
				return feedErr(flags, err)
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{"items": items})
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "BatchItemRequest JSON array, @path or inline (required)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}
