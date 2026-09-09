package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

// newCompanyAttachableUploadCmd implements `company attachable upload`:
// multipart file upload as a v3 Attachable (verified live TC2). Dry-run
// plans without sending; --file also accepts @path indirection.
func newCompanyAttachableUploadCmd(flags *rootFlags) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload a file as a v3 Attachable",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := file
			if len(path) > 1 && path[0] == '@' {
				path = path[1:]
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return exitInput(fmt.Errorf("reading --file: %w", err))
			}
			name := path
			for i := len(name) - 1; i >= 0; i-- {
				if name[i] == '/' {
					name = name[i+1:]
					break
				}
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "company attachable upload", ID: "QBO.COMPANY.ATTACHABLE_CREATE",
					Mode: modeWired, Method: "POST",
					URL:   "https://qbo.intuit.com/api/v3/company/{realm}/upload",
					Flags: localFlagMap(cmd), Note: fmt.Sprintf("multipart upload %d bytes as %s; not sent", len(content), name),
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.UploadAttachable(ctx, name, content)
			if err != nil {
				return feedErr(flags, err)
			}
			printAttachResult(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "local file to upload (@path allowed, required)")
	_ = cmd.MarkFlagRequired("file")
	return cmd
}

// printAttachResult prints the secret-free upload outcome (id + name only;
// TempDownloadUri embeds a live apikey and is never surfaced).
func printAttachResult(stdout io.Writer, flags *rootFlags, res *client.AttachResult) {
	if flags.asJSON {
		fmt.Fprintf(stdout, "{\"id\":%q,\"file_name\":%q,\"bytes\":%d}\n", res.ID, res.FileName, res.Bytes)
		return
	}
	fmt.Fprintf(stdout, "Attachable %s (%s, %d bytes)\n", res.ID, res.FileName, res.Bytes)
}

// newSalesInvoicePDFCmd implements `sales invoice pdf`: download the sales
// form as PDF (verified live TC2, 21KB) to --out. Dry-run plans.
func newSalesInvoicePDFCmd(flags *rootFlags) *cobra.Command {
	var id, out string
	cmd := &cobra.Command{
		Use:   "pdf",
		Short: "Download a sales invoice as PDF",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "sales invoice pdf", ID: "QBO.SALES.INVOICE_PDF",
					Mode: modeRead, Method: "GET",
					URL:   "https://qbo.intuit.com/api/v3/company/{realm}/invoice/{id}/pdf",
					Flags: localFlagMap(cmd), Note: "binary PDF download; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayInvoicePDF(ctx, id, out)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				fmt.Fprintf(cmd.OutOrStdout(), "{\"path\":%q,\"bytes\":%d,\"content_type\":%q}\n",
					res.Path, res.Bytes, res.ContentType)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Invoice PDF %d bytes -> %s\n", res.Bytes, res.Path)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "invoice id (required)")
	cmd.Flags().StringVar(&out, "out", "", "output .pdf path (required)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}
