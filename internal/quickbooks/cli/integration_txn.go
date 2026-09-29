package cli

import (
	"context"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

func newAccountingIntegrationTxnReadCmd(flags *rootFlags) *cobra.Command {
	var id string
	var limit int
	cmd := &cobra.Command{
		Use:   "get",
		Short: "accounting integration-txn read (v4 connections GraphQL)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "accounting integration-txn get",
					ID:      "QBO.ACCOUNTING.INTEGRATION_TXN_READ",
					Mode:    modeRead,
					Method:  "POST",
					URL:     client.PlannedConnectionsURL(),
					Flags:   localFlagMap(cmd),
					Note:    "captured v4 connections GraphQL; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayConnections(ctx, id, limit)
			if err != nil {
				return feedErr(flags, err)
			}
			printQuery(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "target connection id")
	cmd.Flags().IntVar(&limit, "limit", 0, "max rows (0 = all)")
	applyCatalogHelp(cmd, "QBO.ACCOUNTING.INTEGRATION_TXN_READ")
	return cmd
}
