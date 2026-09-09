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

// documentsFlags holds flags local to the `qb documents` subcommands.
// Persistent flags (--json, --home, --timeout, --dry-run) live on rootFlags.
type documentsFlags struct {
	txnID  string
	email  string
	entity string
	id     string
	output string
	ids    []string
	data   string
}

// newDocumentsCmd wires `qb documents`: the document print/email surface
// (emailingestion / txnsrendering / financialdocument hosts).
//
//	qb documents send    --txn-id X --email Y   POST /v2/destinationEmail
//	qb documents print   --txn-id X             POST /v2/print
//	qb documents pdf     --txn-id X --out FILE  GET  /v3/documents/pdf
//	qb documents list    --entity E --ids …     POST /v2/documents/list
//	qb documents update  --id X --data '{…}'    PUT  /v2/documents/{id}
//	qb documents email get                      GET  /v2/destinationEmail
//	qb documents email check --email Y          POST /v2/destinationEmail/checkAvailability
//	qb documents email history                  GET  /v2/destinationEmails
func newDocumentsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "documents",
		Short: "Document print/email service",
		Long: "qb documents <verb>.\n" +
			"Wired: send, print, pdf, list, update, email get|check|history.\n" +
			"send/print/pdf/list/update require --txn-id/--ids/--id; validation fails before any dial.",
	}
	df := &documentsFlags{}

	send := &cobra.Command{
		Use:   "send",
		Short: "Email a document to a destination address (POST /v2/destinationEmail)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: cmd.CommandPath(), Mode: modeWired,
					Method: "POST", URL: client.PlannedDocumentSendURL(),
					Flags: localFlagMap(cmd), Note: "emailingestion POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayDestinationEmailSend(ctx, df.txnID, df.email)
			if err != nil {
				return feedErr(flags, err)
			}
			printDestinationEmail(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	send.Flags().StringVar(&df.txnID, "txn-id", "", "target QBO transaction id (required)")
	send.Flags().StringVar(&df.email, "email", "", "destination email address (required)")
	_ = send.MarkFlagRequired("txn-id")
	_ = send.MarkFlagRequired("email")

	printCmd := &cobra.Command{
		Use:   "print",
		Short: "Render a document PDF via the print service (POST /v2/print)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: cmd.CommandPath(), Mode: modeWired,
					Method: "POST", URL: client.PlannedDocumentPrintURL(),
					Flags: localFlagMap(cmd), Note: "txnsrendering POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayDocumentPrint(ctx, df.txnID)
			if err != nil {
				return feedErr(flags, err)
			}
			printPrintResult(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	printCmd.Flags().StringVar(&df.txnID, "txn-id", "", "target QBO transaction id (required)")
	_ = printCmd.MarkFlagRequired("txn-id")

	pdf := &cobra.Command{
		Use:   "pdf",
		Short: "Download a rendered document (GET /v3/documents/pdf)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: cmd.CommandPath(), Mode: modeRead,
					Method: "GET", URL: client.PlannedDocumentPDFURL(df.txnID),
					Flags: localFlagMap(cmd), Note: "txnsrendering GET; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayDocumentPDF(ctx, df.txnID, df.output)
			if err != nil {
				return feedErr(flags, err)
			}
			printPDFResult(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	pdf.Flags().StringVar(&df.txnID, "txn-id", "", "target QBO transaction id (required)")
	pdf.Flags().StringVar(&df.output, "output", "", "output .pdf path (required)")
	_ = pdf.MarkFlagRequired("txn-id")
	_ = pdf.MarkFlagRequired("output")

	list := &cobra.Command{
		Use:   "list",
		Short: "List uploaded documents by id (POST /v2/documents/list)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: cmd.CommandPath(), Mode: modeRead,
					Method: "POST", URL: client.PlannedDocumentListURL(),
					Flags: localFlagMap(cmd), Note: "financialdocumentpci POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayDocumentsList(ctx, df.entity, df.ids)
			if err != nil {
				return feedErr(flags, err)
			}
			printDocuments(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	list.Flags().StringVar(&df.entity, "entity", "invoice", "entity label for the envelope (invoice, receipt, …)")
	list.Flags().StringVar(&df.id, "id", "", "single document id (shorthand for --ids)")
	list.Flags().StringSliceVar(&df.ids, "ids", nil, "document ids to fetch (comma-separated)")
	list.PreRunE = func(cmd *cobra.Command, args []string) error {
		if len(df.ids) == 0 && strings.TrimSpace(df.id) != "" {
			df.ids = []string{df.id}
		}
		return nil
	}

	update := &cobra.Command{
		Use:   "update",
		Short: "Update a stored document object (PUT /v2/documents/{id})",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: cmd.CommandPath(), Mode: modeWired,
					Method: "PUT", URL: client.PlannedDocumentUpdateURL(df.id),
					Flags: localFlagMap(cmd), Note: "financialdocumentpci PUT; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayDocumentUpdate(ctx, df.id, df.data)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(res)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "update document %s\n", res.Item.ID)
			return nil
		},
	}
	update.Flags().StringVar(&df.id, "id", "", "document id (required)")
	update.Flags().StringVar(&df.data, "data", "", "JSON document body to PUT (object)")
	_ = update.MarkFlagRequired("id")
	_ = update.MarkFlagRequired("data")

	email := &cobra.Command{Use: "email", Short: "Unique inbound destination email"}
	get := &cobra.Command{
		Use:   "get",
		Short: "Show the company's inbound destination email (GET /v2/destinationEmail)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunGET(flags, cmd, cmd.CommandPath(), "QBO.DOCUMENTS.EMAIL_GET", client.PlannedDocumentSendURL(), "captured GET; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayDestinationEmailGet(ctx)
			if err != nil {
				return feedErr(flags, err)
			}
			printDestinationEmail(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	check := &cobra.Command{
		Use:   "check",
		Short: "Check availability of a custom destination email (POST /v2/destinationEmail/checkAvailability)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: cmd.CommandPath(), Mode: modeRead,
					Method: "POST", URL: client.PlannedDocumentCheckURL(),
					Flags: localFlagMap(cmd), Note: "availability POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayDestinationEmailCheck(ctx, df.email)
			if err != nil {
				return feedErr(flags, err)
			}
			printDestinationEmail(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	check.Flags().StringVar(&df.email, "email", "", "candidate custom email address (required)")
	_ = check.MarkFlagRequired("email")
	history := &cobra.Command{
		Use:   "history",
		Short: "List past destination emails (GET /v2/destinationEmails)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunGET(flags, cmd, cmd.CommandPath(), "QBO.DOCUMENTS.EMAIL_HISTORY", client.PlannedDocumentEmailsURL(), "captured GET; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayDestinationEmailsList(ctx)
			if err != nil {
				return feedErr(flags, err)
			}
			printDestinationHistory(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	email.AddCommand(get, check, history)

	cmd.AddCommand(send, printCmd, pdf, list, update, email)
	return cmd
}

// --- output (secret-free) ---------------------------------------------------

func printDestinationEmail(stdout io.Writer, flags *rootFlags, res *client.DestinationEmailResult) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	switch res.Op {
	case "check":
		state := "unavailable"
		if res.Available != nil && *res.Available {
			state = "available"
		}
		fmt.Fprintf(stdout, "%s is %s\n", res.Email, state)
	case "list":
		fmt.Fprintln(stdout, "No destination email history.")
	default:
		fmt.Fprintf(stdout, "Destination email: %s\n", res.Email)
	}
}

func printDestinationHistory(stdout io.Writer, flags *rootFlags, res *client.DestinationEmailResult) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	if len(res.History) == 0 {
		fmt.Fprintln(stdout, "No destination email history.")
		return
	}
	fmt.Fprintf(stdout, "Destination email history (%d):\n", len(res.History))
	for _, it := range res.History {
		fmt.Fprintf(stdout, "  %-10s %-40s %s\n", it.ID, it.Name, it.Date)
	}
}

func printPrintResult(stdout io.Writer, flags *rootFlags, res *client.PrintResult) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	fmt.Fprintf(stdout, "printed txn %s (%d bytes %s)\n", res.TxnID, res.Bytes, res.ContentType)
}

func printPDFResult(stdout io.Writer, flags *rootFlags, res *client.PDFResult) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	fmt.Fprintf(stdout, "wrote %s (%d bytes)\n", res.Path, res.Bytes)
}

func printDocuments(stdout io.Writer, flags *rootFlags, res *client.DocumentsResult) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	if len(res.Items) == 0 {
		fmt.Fprintln(stdout, "No documents found.")
		return
	}
	fmt.Fprintf(stdout, "Documents (%d):\n", len(res.Items))
	for _, d := range res.Items {
		fmt.Fprintf(stdout, "  %-20s %-16s %-32s %s\n", d.ID, d.Type, truncate(d.Name, 32), d.Date)
	}
}
