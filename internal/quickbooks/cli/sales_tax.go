package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

// sales_tax.go wires the `qb salestx` domain (client/sales_tax.go):
//
//	v3 company-API transactions — invoice|estimate|payment|creditmemo|
//	  delayedcharge|salesreceipt with list/get/create/update/delete and the
//	  invoice extras void/send/pdf — all via ReplayMutate/ReplayQuery on the
//	  existing v3Path entities.
//	tax readers — taxcode list (GetTaxCodes_qbo), taxrate list|get
//	  (TaxRates__indirect_tax_ui_qbo).
//	indirect-tax AU surface — gst list|file, bas list|lodge, tpar
//	  list|generate over taxconfig/taxfilingsvc graphql plus the shared
//	  universalreportinsights instances read.
//
// Reads are mode-read; every write passes the production-realm guard inside
// the client. Dry-run plans carry {realm}-free URLs only.

type salestxFlags struct {
	id       string
	to       string
	email    string
	customer string
	amount   float64
	date     string
	item     string
	memo     string
	account  string // payment account id (gst file)
	agency   string // tax agency node id (bas list / gst file)
	status   string
	version  int
}

func (f *salestxFlags) bindWrite(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.id, "id", "", "target QBO id")
	cmd.Flags().StringVar(&f.to, "to", "", "override recipient email")
	cmd.Flags().StringVar(&f.email, "email", "", "email address (alias of --to)")
	cmd.Flags().StringVar(&f.customer, "customer", "", "customer id (required for create)")
	cmd.Flags().Float64Var(&f.amount, "amount", 0, "line amount (AUD)")
	cmd.Flags().StringVar(&f.date, "date", "", "dd/MM/yyyy")
	cmd.Flags().StringVar(&f.item, "item", "", "item id for sales lines")
	cmd.Flags().StringVar(&f.memo, "memo", "", "memo / private note")
	_ = cmd.Flags().MarkHidden("email")
}

// newSalesTxV3Cmd builds one v3-backed leaf: verb over a CLI entity word.
// op is the ReplayMutate operation; empty op means a query read.
func newSalesTxV3Cmd(flags *rootFlags, entity, use, op string) *cobra.Command {
	ff := &salestxFlags{}
	command := "salestx " + entity + " " + use
	isRead := op == "" || op == "list" || op == "get"
	var id, query string
	var limit int
	cmd := &cobra.Command{
		Use: use,
		RunE: func(cmd *cobra.Command, args []string) error {
			if isRead {
				if !client.V3Queryable(v3Word(entity)) {
					if flags.dryRun {
						return writePlan(cmd, flags, planEnvelope{
							Command: command,
							ID:      eFor(command),
							Mode:    modeBlocked,
							Flags:   localFlagMap(cmd),
							Note:    "unsupported contract: v3 query has no " + v3Word(entity) + " context; not sent",
						})
					}
					return feedErr(flags, fmt.Errorf("%w: %s", client.ErrUnsupportedQueryContext, v3Word(entity)))
				}
				if handled, err := dryRunGET(flags, cmd, command, eFor(command), client.PlannedQueryURL(v3Word(entity)), "v3 query GET; not sent"); handled {
					return err
				}
				ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
				defer cancel()
				res, err := client.ReplaySalestxQuery(ctx, entity, id, query, limit)
				if err != nil {
					return feedErr(flags, err)
				}
				printQuery(cmd.OutOrStdout(), flags, res)
				return nil
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command,
					ID:      eFor(command),
					Mode:    modeWired,
					Method:  "POST",
					URL:     client.PlannedSalestxMutateURL(entity, op),
					Flags:   localFlagMap(cmd),
					Note:    "v3 POST; not sent",
				})
			}
			fm := collectFlags(cmd)
			if op == "send" && fm["to"] == "" {
				fm["to"] = fm["email"]
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplaySalestxMutate(ctx, entity, op, fm["id"], fm)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutated(cmd.OutOrStdout(), flags, res, strings.Title(op)+" "+entity) //nolint:staticcheck // display only
			return nil
		},
	}
	if isRead {
		cmd.Annotations = map[string]string{"qb:read-only": "true"}
		cmd.Short = command + " (v3 query)"
		if !client.V3Queryable(v3Word(entity)) {
			cmd.Short = command + " (unsupported: no v3 query context)"
		}
		cmd.Flags().StringVar(&id, "id", "", "v3 entity id (omit to list)")
		cmd.Flags().StringVar(&query, "query", "", "substring match on doc number")
		cmd.Flags().IntVar(&limit, "limit", 0, "max rows (0 = all)")
	} else {
		if op != "send" {
			cmd.Annotations = map[string]string{readbackAnnotation: "true"}
		}
		ff.bindWrite(cmd)
	}
	return cmd
}

// eFor derives a stable pseudo catalog id so plans identify their command.
func eFor(command string) string {
	return "QBO.SALESTX." + strings.ToUpper(strings.ReplaceAll(strings.Join(strings.Fields(command), "_"), " ", "_"))
}

// v3Word maps a salestx CLI entity word onto the v3 entity name used in
// planned query URLs ("invoice" → "Invoice"). Unknown words pass through.
func v3Word(entity string) string {
	if v, ok := client.SalestxV3Entity(entity); ok {
		return v
	}
	return entity
}

// newInvoicePDFCmd wires `qb salestx invoice pdf` onto the captured
// txnsrendering document renderer (documents.go).
func newInvoicePDFCmd(flags *rootFlags) *cobra.Command {
	var txnID, output string
	cmd := &cobra.Command{
		Use:   "pdf",
		Short: "salestx invoice pdf (txnsrendering GET /v3/documents/pdf)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "salestx invoice pdf",
					ID:      "QBO.SALESTX.INVOICE_PDF",
					Mode:    modeRead,
					Method:  "GET",
					URL:     client.PlannedDocumentPDFURL(txnID),
					Flags:   localFlagMap(cmd),
					Note:    "txnsrendering GET; not sent",
				})
			}
			if strings.TrimSpace(output) == "" {
				return feedErr(flags, fmt.Errorf("invoice pdf requires --output"))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayDocumentPDF(ctx, txnID, output)
			if err != nil {
				return feedErr(flags, err)
			}
			printSalestxPDF(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&txnID, "id", "", "invoice/transaction id (required)")
	cmd.Flags().StringVar(&output, "output", "", "output .pdf path (required)")
	return cmd
}

func printSalestxPDF(stdout io.Writer, flags *rootFlags, res *client.PDFResult) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", res.Path, res.Bytes)
}

// newSalesTaxCodeListCmd / rate / gst / bas / tpar builders follow.

func newTaxCodeListCmd(flags *rootFlags) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "salestx taxcode list (GetTaxCodes_qbo)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunPOST(flags, cmd, "salestx taxcode list", "QBO.SALESTX.TAXCODE_LIST", client.PlanTaxCodeList(limit)); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayTaxCodeList(ctx, limit)
			if err != nil {
				return feedErr(flags, err)
			}
			return printQueryResult(cmd.OutOrStdout(), flags, res)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "max rows (0 = all; the service applies its own page cap)")
	return cmd
}

func newTaxRateCmd(flags *rootFlags, get bool) *cobra.Command {
	var id string
	use := "list"
	short := "salestx taxrate list (TaxRates__indirect_tax_ui_qbo)"
	run := func(cmd *cobra.Command, args []string) error {
		if handled, err := dryRunPOST(flags, cmd, "salestx taxrate "+use, "QBO.SALESTX.TAXRATE_"+strings.ToUpper(use), client.PlanTaxRateList()); handled {
			return err
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
		defer cancel()
		res, err := client.ReplayTaxRateList(ctx)
		if err != nil {
			return feedErr(flags, err)
		}
		res = filterByID(res, id)
		return printQueryResult(cmd.OutOrStdout(), flags, res)
	}
	if get {
		use = "get"
		short = "salestx taxrate get (TaxRates__indirect_tax_ui_qbo filtered by --id)"
	}
	cmd := &cobra.Command{Use: use, Short: short, RunE: run}
	if get {
		cmd.Flags().StringVar(&id, "id", "", "tax rate id (required)")
		_ = cmd.MarkFlagRequired("id")
	}
	return cmd
}

func filterByID(res *client.QueryResult, id string) *client.QueryResult {
	id = strings.TrimSpace(id)
	if res == nil || id == "" {
		return res
	}
	kept := res.Items[:0:0]
	for _, it := range res.Items {
		if it.ID == id {
			kept = append(kept, it)
		}
	}
	out := *res
	out.Items = kept
	out.Counts = map[string]int{"items": len(kept)}
	return &out
}

func newGSTCmd(flags *rootFlags, file bool) *cobra.Command {
	ff := &salestxFlags{}
	if file {
		cmd := &cobra.Command{
			Use:   "file",
			Short: "Record an ATO GST payment (CreateTaxPayment mutation)",
			RunE: func(cmd *cobra.Command, args []string) error {
				plan, perr := client.PlanGSTFile(ff.amount, ff.date, ff.account, ff.agency, ff.memo)
				if perr != nil {
					return feedErr(flags, perr)
				}
				if flags.dryRun {
					return writePlan(cmd, flags, planFromClient(cmd, "salestx gst file", "QBO.SALESTX.GST_FILE", modeWired, plan))
				}
				ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
				defer cancel()
				res, err := client.ReplayGSTFile(ctx, ff.amount, ff.date, ff.account, ff.agency, ff.memo)
				if err != nil {
					return feedErr(flags, err)
				}
				printMutated(cmd.OutOrStdout(), flags, res, "recorded GST payment")
				return nil
			},
		}
		cmd.Flags().Float64Var(&ff.amount, "amount", 0, "payment amount (AUD)")
		cmd.Flags().StringVar(&ff.date, "date", "", "dd/MM/yyyy (defaults to today)")
		cmd.Flags().StringVar(&ff.account, "account", "", "bank account id paid from (required)")
		cmd.Flags().StringVar(&ff.agency, "agency", "", "tax agency node id (required)")
		cmd.Flags().StringVar(&ff.memo, "memo", "", "description")
		return cmd
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "GST agencies/groups/categories (IndirectTaxConfigQuery_qbo)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunPOST(flags, cmd, "salestx gst list", "QBO.SALESTX.GST_LIST", client.PlanGSTList()); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayGSTList(ctx)
			if err != nil {
				return feedErr(flags, err)
			}
			return printQueryResult(cmd.OutOrStdout(), flags, res)
		},
	}
	return cmd
}

func newBASCmd(flags *rootFlags, lodge bool) *cobra.Command {
	ff := &salestxFlags{}
	if lodge {
		cmd := &cobra.Command{
			Use:   "lodge",
			Short: "Mark a BAS return lodged (EfileTaxReturn mutation)",
			RunE: func(cmd *cobra.Command, args []string) error {
				plan, perr := client.PlanBASLodge(ff.id, ff.status, ff.version)
				if perr != nil {
					return feedErr(flags, perr)
				}
				if flags.dryRun {
					return writePlan(cmd, flags, planFromClient(cmd, "salestx bas lodge", "QBO.SALESTX.BAS_LODGE", modeWired, plan))
				}
				ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
				defer cancel()
				res, err := client.ReplayBASLodge(ctx, ff.id, ff.status, ff.version)
				if err != nil {
					return feedErr(flags, err)
				}
				printMutated(cmd.OutOrStdout(), flags, res, "lodged BAS return")
				return nil
			},
		}
		cmd.Flags().StringVar(&ff.id, "id", "", "tax return node id (required)")
		cmd.Flags().IntVar(&ff.version, "version", 0, "optimistic-concurrency entityVersion of the return")
		cmd.Flags().StringVar(&ff.status, "status", "Filed", "eFilingStatus to set (default Filed)")
		return cmd
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "BAS returns for one agency (TaxReturns__indirect_tax_ui_qbo)",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, perr := client.PlanBASList(ff.agency)
			if perr != nil {
				return feedErr(flags, perr)
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planFromClient(cmd, "salestx bas list", "QBO.SALESTX.BAS_LIST", modeRead, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayBASList(ctx, ff.agency)
			if err != nil {
				return feedErr(flags, err)
			}
			return printQueryResult(cmd.OutOrStdout(), flags, res)
		},
	}
	cmd.Flags().StringVar(&ff.agency, "agency", "", "tax agency node id from `qb salestx gst list` (required)")
	return cmd
}

func newTPARCmd(flags *rootFlags, generate bool) *cobra.Command {
	var limit int
	if generate {
		return &cobra.Command{
			Use:   "generate",
			Short: "Generate TPAR lodgement (not wired: no captured API)",
			RunE: func(cmd *cobra.Command, args []string) error {
				if flags.dryRun {
					return writePlan(cmd, flags, uncapturedPlan(cmd, "salestx tpar generate", primitiveEntry{}))
				}
				if err := client.RequireSession(); err != nil {
					return feedErr(flags, err)
				}
				_, err := client.ReplayTPARGenerate(cmd.Context())
				return feedErr(flags, err)
			},
		}
	}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "salestx tpar list (universalreportinsights TAXABLE_PAYMENTS/instances)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunPOST(flags, cmd, "salestx tpar list", "QBO.SALESTX.TPAR_LIST", client.PlanTPARList(limit)); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayTPARList(ctx, limit)
			if err != nil {
				return feedErr(flags, err)
			}
			return printQueryResult(cmd.OutOrStdout(), flags, res)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 0, "page size (0 = all)")
	return cmd
}

// newSalesTaxCmd builds the whole `qb salestx` domain group.
func newSalesTaxCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "salestx",
		Short: "Sales transactions and indirect tax (v3 + tax services)",
		Long: "Sales transactions and indirect tax. qb salestx <entity> <verb>.\n\n" +
			"v3 transactions: invoice list|get|create|update|delete|void|send|pdf,\n" +
			"estimate list|get|create|update|delete|send, payment list|get|create|delete,\n" +
			"creditmemo list|get|create|delete, delayedcharge create (list unsupported: no v3 query context),\n" +
			"salesreceipt list|get|create|delete.\n" +
			"tax: taxcode list, taxrate list|get, gst list|file, bas list|lodge,\n" +
			"tpar list|generate.",
	}
	add := func(entity string, verbs []struct {
		use, op string
	}) {
		ent := &cobra.Command{Use: entity, Short: entity}
		for _, v := range verbs {
			if v.use == "pdf" && entity == "invoice" {
				ent.AddCommand(newInvoicePDFCmd(flags))
				continue
			}
			ent.AddCommand(newSalesTxV3Cmd(flags, entity, v.use, v.op))
		}
		cmd.AddCommand(ent)
	}
	readWrite := []struct{ use, op string }{
		{"list", "list"}, {"get", "get"},
		{"create", "create"}, {"update", "update"}, {"delete", "delete"},
	}
	add("invoice", []struct{ use, op string }{
		{"list", "list"}, {"get", "get"},
		{"create", "create"}, {"update", "update"}, {"delete", "delete"},
		{"void", "void"}, {"send", "send"}, {"pdf", "pdf"},
	})
	add("estimate", []struct{ use, op string }{
		{"list", "list"}, {"get", "get"},
		{"create", "create"}, {"update", "update"}, {"delete", "delete"},
		{"send", "send"},
	})
	add("payment", readWrite[:5])
	add("creditmemo", readWrite[:5])
	add("delayedcharge", []struct{ use, op string }{{"list", "list"}, {"create", "create"}})
	add("salesreceipt", readWrite[:5])

	taxEnt := &cobra.Command{Use: "taxcode", Short: "taxcode"}
	taxEnt.AddCommand(newTaxCodeListCmd(flags))
	cmd.AddCommand(taxEnt)

	rateEnt := &cobra.Command{Use: "taxrate", Short: "taxrate"}
	rateEnt.AddCommand(newTaxRateCmd(flags, false))
	rateEnt.AddCommand(newTaxRateCmd(flags, true))
	cmd.AddCommand(rateEnt)

	gstEnt := &cobra.Command{Use: "gst", Short: "gst"}
	gstEnt.AddCommand(newGSTCmd(flags, false))
	gstEnt.AddCommand(newGSTCmd(flags, true))
	cmd.AddCommand(gstEnt)

	basEnt := &cobra.Command{Use: "bas", Short: "bas"}
	basEnt.AddCommand(newBASCmd(flags, false))
	basEnt.AddCommand(newBASCmd(flags, true))
	cmd.AddCommand(basEnt)

	tparEnt := &cobra.Command{Use: "tpar", Short: "tpar"}
	tparEnt.AddCommand(newTPARCmd(flags, false))
	tparEnt.AddCommand(newTPARCmd(flags, true))
	cmd.AddCommand(tparEnt)
	return cmd
}

// printQueryResult renders QueryResult payloads from the tax GraphQL readers
// (printQuery handles the same shape but lives beside v3read's read flow;
// both are secret-free JSON/text renderers).
func printQueryResult(stdout io.Writer, flags *rootFlags, res *client.QueryResult) error {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if res == nil || len(res.Items) == 0 {
		if res != nil && res.Note != "" {
			fmt.Fprintln(stdout, res.Note)
			return nil
		}
		fmt.Fprintln(stdout, "no rows")
		return nil
	}
	fmt.Fprintf(stdout, "%s %d row(s)\n", res.Entity, len(res.Items))
	for _, it := range res.Items {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%v\n", it.ID, truncate(it.Name, 40), it.Date, it.Amount)
	}
	return nil
}
