package cli

import (
	"context"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

// newSalesSettingsGetCmd implements `sales salesettings get`: the captured
// readSettings call the Account-and-settings > Sales tab issues against
// salesettings.api.intuit.com/v4/graphql (qbo-settings-ui plugin). On TC2
// the resolver returns a null salesSettings row — reported honestly.
func newSalesSettingsGetCmd(flags *rootFlags) *cobra.Command {
	var query string
	var limit int
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Read sales-form settings (readSettings)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "sales salesettings get",
					ID:      "QBO.SALES.SALES_SETTINGS_GET",
					Mode:    modeRead,
					Method:  "POST",
					URL:     "https://salesettings.api.intuit.com/v4/graphql?intuit-company-id={realm}",
					Flags:   localFlagMap(cmd),
					Note:    "captured readSettings; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplaySalesSettings(ctx, query, limit)
			if err != nil {
				return feedErr(flags, err)
			}
			return printQueryResult(cmd.OutOrStdout(), flags, res)
		},
	}
	cmd.Flags().StringVar(&query, "form", "", "sales form type filter (informational)")
	cmd.Flags().IntVar(&limit, "limit", 0, "max rows (0 = all)")
	applyCatalogHelp(cmd, "QBO.SALES.SALES_SETTINGS_GET")
	return cmd
}

func newSalesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sales",
		Short: "Invoices and receivables",
		Long:  "Invoices and receivables. qb sales <entity> <verb>. Stubs return not-wired.",
	}
	credit_card_creditEnt := &cobra.Command{Use: "credit-card-credit", Short: "credit-card-credit"}
	credit_card_creditEnt.AddCommand(newStubCmd(flags, "sales credit-card-credit", "create", "credit-card-credit create (not wired)"))
	cmd.AddCommand(credit_card_creditEnt)
	credit_memoEnt := &cobra.Command{Use: "credit-memo", Short: "credit-memo"}
	credit_memoEnt.AddCommand(newStubCmd(flags, "sales credit-memo", "create", "credit-memo create (not wired)"))
	credit_memoEnt.AddCommand(newStubCmd(flags, "sales credit-memo", "delete", "credit-memo delete (not wired)"))
	credit_memoEnt.AddCommand(newStubCmd(flags, "sales credit-memo", "update", "credit-memo edit (not wired)"))
	credit_memoEnt.AddCommand(newStubCmd(flags, "sales credit-memo", "get", "credit-memo read (not wired)"))
	credit_memoRunEnt := &cobra.Command{Use: "run", Short: "run"}
	credit_memoRunEnt.AddCommand(newSalesSendCmd(flags, "CreditMemo", "QBO.SALES.CREDIT_MEMO_SEND"))
	credit_memoEnt.AddCommand(credit_memoRunEnt)
	credit_memoEnt.AddCommand(newSalesFormPDFCmd(flags, "CreditMemo", "QBO.SALES.CREDIT_MEMO_PDF"))
	cmd.AddCommand(credit_memoEnt)
	delayed_chargeEnt := &cobra.Command{Use: "delayed-charge", Short: "delayed-charge"}
	delayed_chargeEnt.AddCommand(newStubCmd(flags, "sales delayed-charge", "create", "delayed-charge create (not wired)"))
	cmd.AddCommand(delayed_chargeEnt)
	delayed_creditEnt := &cobra.Command{Use: "delayed-credit", Short: "delayed-credit"}
	delayed_creditEnt.AddCommand(newStubCmd(flags, "sales delayed-credit", "create", "delayed-credit create (not wired)"))
	cmd.AddCommand(delayed_creditEnt)
	estimateEnt := &cobra.Command{Use: "estimate", Short: "estimate"}
	estimateEnt.AddCommand(newStubCmd(flags, "sales estimate", "copy", "estimate copy (not wired)"))
	estimateEnt.AddCommand(newStubCmd(flags, "sales estimate", "create", "estimate create (not wired)"))
	estimateEnt.AddCommand(newStubCmd(flags, "sales estimate", "delete", "estimate delete (not wired)"))
	estimateEnt.AddCommand(newStubCmd(flags, "sales estimate", "update", "estimate edit (not wired)"))
	estimateEnt.AddCommand(newStubCmd(flags, "sales estimate", "get", "estimate read (not wired)"))
	estimateEnt.AddCommand(newStubCmd(flags, "sales estimate", "search", "estimate search (not wired)"))
	estimateRunEnt := &cobra.Command{Use: "run", Short: "run"}
	estimateRunEnt.AddCommand(newSalesSendCmd(flags, "Estimate", "QBO.SALES.ESTIMATE_SEND"))
	estimateEnt.AddCommand(estimateRunEnt)
	estimateEnt.AddCommand(newSalesFormPDFCmd(flags, "Estimate", "QBO.SALES.ESTIMATE_PDF"))
	cmd.AddCommand(estimateEnt)
	hubEnt := &cobra.Command{Use: "hub", Short: "hub"}
	hubEnt.AddCommand(newStubCmd(flags, "sales hub", "get", "hub read (not wired)"))
	hubEnt.AddCommand(newStubCmd(flags, "sales hub", "search", "hub search (not wired)"))
	cmd.AddCommand(hubEnt)
	invoiceEnt := &cobra.Command{Use: "invoice", Short: "invoice"}
	invoiceEnt.AddCommand(newStubCmd(flags, "sales invoice", "copy", "invoice copy (not wired)"))
	invoiceEnt.AddCommand(newStubCmd(flags, "sales invoice", "create", "invoice create (not wired)"))
	invoiceEnt.AddCommand(newStubCmd(flags, "sales invoice", "delete", "invoice delete (not wired)"))
	invoiceEnt.AddCommand(newStubCmd(flags, "sales invoice", "get", "invoice read (not wired)"))
	invoiceEnt.AddCommand(newStubCmd(flags, "sales invoice", "search", "invoice search (not wired)"))
	invoiceEnt.AddCommand(newSalesInvoicePDFCmd(flags))
	invoiceFormEnt := &cobra.Command{Use: "form", Short: "form"}
	invoiceFormEnt.AddCommand(newStubCmd(flags, "sales invoice form", "delete", "invoice-form delete (not wired)"))
	invoiceEnt.AddCommand(invoiceFormEnt)
	invoiceListEnt := &cobra.Command{Use: "list", Short: "list"}
	invoiceListEnt.AddCommand(newStubCmd(flags, "sales invoice list", "create", "invoice-list create (not wired)"))
	invoiceListEnt.AddCommand(newStubCmd(flags, "sales invoice list", "edit", "invoice-list edit (not wired)"))
	invoiceEnt.AddCommand(invoiceListEnt)
	invoiceRunEnt := &cobra.Command{Use: "run", Short: "run"}
	invoiceRunEnt.AddCommand(newStubCmd(flags, "sales invoice run", "remind", "invoice remind (not wired)"))
	invoiceRunEnt.AddCommand(newSalesSendCmd(flags, "Invoice", "QBO.SALES.INVOICE_SEND"))
	invoiceEnt.AddCommand(invoiceRunEnt)
	invoiceUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	invoiceUpdateEnt.AddCommand(newStubCmd(flags, "sales invoice update", "edit", "invoice edit (not wired)"))
	invoiceUpdateEnt.AddCommand(newStubCmd(flags, "sales invoice update", "customise", "invoice customise (not wired)"))
	invoiceUpdateEnt.AddCommand(newStubCmd(flags, "sales invoice update", "discount", "invoice discount (not wired)"))
	invoiceUpdateEnt.AddCommand(newStubCmd(flags, "sales invoice update", "progress", "invoice progress (not wired)"))
	invoiceUpdateEnt.AddCommand(newStubCmd(flags, "sales invoice update", "write-off", "invoice write-off (not wired)"))
	invoiceEnt.AddCommand(invoiceUpdateEnt)
	late_feeEnt := &cobra.Command{Use: "late-fee", Short: "late-fee"}
	late_feeEnt.AddCommand(newStubCmd(flags, "sales late-fee", "create", "automatic late fee (service map; not wired)"))
	cmd.AddCommand(late_feeEnt)
	knowledge_baseEnt := &cobra.Command{Use: "knowledge-base", Short: "knowledge-base"}
	knowledge_baseEnt.AddCommand(newStubCmd(flags, "sales knowledge-base", "search", "c1-kb-svc search (service map; not wired)"))
	cmd.AddCommand(knowledge_baseEnt)
	commerce_controlEnt := &cobra.Command{Use: "commerce-control", Short: "commerce-control"}
	commerce_controlEnt.AddCommand(newStubCmd(flags, "sales commerce-control", "get", "commercecontrol service read (service map; not wired)"))
	cmd.AddCommand(commerce_controlEnt)
	cmd.AddCommand(invoiceEnt)
	overviewEnt := &cobra.Command{Use: "overview", Short: "overview"}
	overviewEnt.AddCommand(newStubCmd(flags, "sales overview", "get", "universalreportsgraphql allSales read"))
	cmd.AddCommand(overviewEnt)
	paymentEnt := &cobra.Command{Use: "payment", Short: "payment"}
	paymentEnt.AddCommand(newStubCmd(flags, "sales payment", "create", "payment create (not wired)"))
	paymentEnt.AddCommand(newStubCmd(flags, "sales payment", "delete", "payment delete (not wired)"))
	paymentEnt.AddCommand(newStubCmd(flags, "sales payment", "update", "payment edit (not wired)"))
	paymentEnt.AddCommand(newStubCmd(flags, "sales payment", "get", "payment read (not wired)"))
	cmd.AddCommand(paymentEnt)
	receiptEnt := &cobra.Command{Use: "receipt", Short: "receipt"}
	receiptEnt.AddCommand(newStubCmd(flags, "sales receipt", "create", "receipt create (not wired)"))
	receiptEnt.AddCommand(newStubCmd(flags, "sales receipt", "delete", "receipt delete (not wired)"))
	receiptEnt.AddCommand(newStubCmd(flags, "sales receipt", "update", "receipt edit (not wired)"))
	receiptEnt.AddCommand(newStubCmd(flags, "sales receipt", "get", "receipt read (not wired)"))
	receiptRunEnt := &cobra.Command{Use: "run", Short: "run"}
	receiptRunEnt.AddCommand(newSalesSendCmd(flags, "SalesReceipt", "QBO.SALES.RECEIPT_SEND"))
	receiptEnt.AddCommand(receiptRunEnt)
	receiptEnt.AddCommand(newSalesFormPDFCmd(flags, "SalesReceipt", "QBO.SALES.RECEIPT_PDF"))
	cmd.AddCommand(receiptEnt)
	refund_receiptEnt := &cobra.Command{Use: "refund-receipt", Short: "refund-receipt"}
	refund_receiptEnt.AddCommand(newStubCmd(flags, "sales refund-receipt", "create", "refund-receipt create (not wired)"))
	refund_receiptEnt.AddCommand(newStubCmd(flags, "sales refund-receipt", "delete", "refund-receipt delete (not wired)"))
	refund_receiptEnt.AddCommand(newStubCmd(flags, "sales refund-receipt", "update", "refund-receipt edit (not wired)"))
	refund_receiptEnt.AddCommand(newStubCmd(flags, "sales refund-receipt", "get", "refund-receipt read (not wired)"))
	cmd.AddCommand(refund_receiptEnt)
	sales_orderEnt := &cobra.Command{Use: "sales-order", Short: "sales-order"}
	sales_orderEnt.AddCommand(newStubCmd(flags, "sales sales-order", "create", "sales-order create (not wired)"))
	sales_orderEnt.AddCommand(newStubCmd(flags, "sales sales-order", "delete", "sales-order delete (not wired)"))
	sales_orderEnt.AddCommand(newStubCmd(flags, "sales sales-order", "update", "sales-order edit (not wired)"))
	sales_orderEnt.AddCommand(newStubCmd(flags, "sales sales-order", "get", "sales-order read (not wired)"))
	sales_orderEnt.AddCommand(newStubCmd(flags, "sales sales-order", "search", "sales-order search (not wired)"))
	cmd.AddCommand(sales_orderEnt)
	statementEnt := &cobra.Command{Use: "statement", Short: "statement"}
	statementEnt.AddCommand(newStubCmd(flags, "sales statement", "create", "statement create"))
	cmd.AddCommand(statementEnt)
	time_invoiceEnt := &cobra.Command{Use: "time-invoice", Short: "time-invoice"}
	time_invoiceEnt.AddCommand(newStubCmd(flags, "sales time-invoice", "create", "time-invoice create"))
	cmd.AddCommand(time_invoiceEnt)
	// Service map v2 (2026-08-24): never-triggered production services, blocked.
	salestransactions_riskEnt := &cobra.Command{Use: "salestransactions-risk", Short: "salestransactions-risk"}
	salestransactions_riskEnt.AddCommand(newStubCmd(flags, "sales salestransactions-risk", "get", "salestxnrisksvc risk/eligibility read"))
	cmd.AddCommand(salestransactions_riskEnt)
	sales_ai_callEnt := &cobra.Command{Use: "sales-ai-call", Short: "sales-ai-call"}
	sales_ai_callEnt.AddCommand(newStubCmd(flags, "sales sales-ai-call", "post", "sales-ai-svc.api.intuit.com/api/v1 post (service map; not wired)"))
	cmd.AddCommand(sales_ai_callEnt)
	traffic_routerEnt := &cobra.Command{Use: "traffic-router", Short: "traffic-router"}
	traffic_routerEnt.AddCommand(newStubCmd(flags, "sales traffic-router", "post", "sales-traffic-router.api.intuit.com[/v4/graphql] post (service map; not wired)"))
	cmd.AddCommand(traffic_routerEnt)
	checkoutEnt := &cobra.Command{Use: "checkout", Short: "checkout"}
	checkoutEnt.AddCommand(newStubCmd(flags, "sales checkout", "get", "salescheckout.api.intuit.com/v2/sale/{id}/activities get (service map; not wired)"))
	cmd.AddCommand(checkoutEnt)
	salesettingsEnt := &cobra.Command{Use: "salesettings", Short: "salesettings"}
	salesettingsEnt.AddCommand(newSalesSettingsGetCmd(flags))
	cmd.AddCommand(salesettingsEnt)
	txns_renderingEnt := &cobra.Command{Use: "txns-rendering", Short: "txns-rendering"}
	txns_renderingEnt.AddCommand(newStubCmd(flags, "sales txns-rendering", "post", "txnsrendering.api.intuit.com post (service map; not wired)"))
	cmd.AddCommand(txns_renderingEnt)
	qbonline_gatewayEnt := &cobra.Command{Use: "qbonline-gateway", Short: "qbonline-gateway"}
	qbonline_gatewayEnt.AddCommand(newStubCmd(flags, "sales qbonline-gateway", "get", "qbonline-aws /v4/graphql read"))
	cmd.AddCommand(qbonline_gatewayEnt)
	return cmd
}
