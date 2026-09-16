package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func salesOrderParamDocs(id string) ([]paramDoc, bool) {
	if !strings.HasPrefix(id, "QBO.SALES.SALES_ORDER_") {
		return nil, false
	}
	if strings.HasSuffix(id, "_READ") || strings.HasSuffix(id, "_SEARCH") {
		return []paramDoc{
			p("id", "string", "native sales-order ID; cannot be combined with --query", false, ""),
			p("query", "string", "case-insensitive name, order number or ID search across all native pages", strings.HasSuffix(id, "_SEARCH"), ""),
			p("limit", "int", "maximum returned rows; larger limits fetch successive native pages", false, "20"),
		}, true
	}
	create := strings.HasSuffix(id, "_CREATE")
	ps := []paramDoc{}
	if !create {
		ps = append(ps, p("id", "string", "native sales-order ID to update or permanently delete", true, ""))
	}
	if strings.HasSuffix(id, "_DELETE") {
		return ps, true
	}
	ps = append(ps,
		p("customer", "string", "native customer ID used by the sales-order service", create, ""),
		p("order-date", "string", "sales-order transaction date as dd/MM/yyyy or yyyy-MM-dd", create, ""),
		p("currency", "string", "three-letter order currency code, for example AUD", create, ""),
		p("line-items", "string", "JSON v3 SalesItemLineDetail array; SERVICE items only; updates preserve omitted fields and require IDs for multiple lines", create, ""),
		p("order-number", "string", "sales-order reference number; independent of invoice numbers", false, ""),
		p("due-date", "string", "due date as dd/MM/yyyy or yyyy-MM-dd; omitted updates preserve it", false, ""),
		p("memo", "string", "customer-facing note on the sales order; empty clears the note", false, ""),
		p("term", "string", "native payment-term ID; omitted updates preserve the existing term", false, ""),
		p("issued-at", "string", "explicit RFC3339 issued timestamp; otherwise a changed order-date uses UTC midnight", false, ""),
		p("billing-address", "string", "free-form billing address; omitted updates preserve the address", false, ""),
		p("shipping-address", "string", "free-form shipping address; omitted updates preserve the address", false, ""),
		p("print-template-id", "string", "native sales-form print-template ID, when required by the account", false, ""),
	)
	return ps, true
}

func newSalesOrderMutateCmd(flags *rootFlags, e primitiveEntry, command, op string) *cobra.Command {
	cmd := &cobra.Command{Use: verbOf(command), Short: command + " (native sales-order API)", RunE: func(cmd *cobra.Command, args []string) error {
		fm := map[string]string{}
		cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
			if f.Changed {
				fm[f.Name] = f.Value.String()
			}
		})
		if flags.dryRun {
			return writePlan(cmd, flags, planEnvelope{Command: command, ID: e.ID, Mode: modeWired, Method: "POST", URL: client.PlannedSalesOrderURL(), Flags: fm, Note: "native sales-order GraphQL; not sent"})
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
		defer cancel()
		result, err := client.ReplaySalesOrderMutate(ctx, op, fm["id"], fm)
		if err != nil {
			return feedErr(flags, err)
		}
		if flags.asJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", result.Op, result.Entity, result.Item.ID)
		return nil
	}}
	attachParamFlags(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}
