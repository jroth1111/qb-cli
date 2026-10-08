package cli

import (
	"encoding/json"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql"
	"github.com/spf13/cobra"
)

func newGqlInspectTransactionCmd(flags *rootFlags) *cobra.Command {
	var id string
	var credit bool
	cmd := &cobra.Command{Use: "inspect-transaction", Short: "Read native purchase tax traits and bank-match provenance without changing the transaction",
		Long: "Reads the transaction model used by the expense form. Preserves nullable tax fields and bank-match metadata. Match mode is not clearing state; a taxable flag is not proof of posted tax. No metadata restoration or global accounting certification.",
		Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{"dry_run": true, "dial": false, "operation": "TxnQuery_qbo", "localId": id, "creditCardCredit": credit})
			}
			token, err := auth.Load()
			if err != nil {
				return err
			}
			ctx, cancel := contextWithTimeout(cmd, flags.timeout)
			defer cancel()
			result, err := gql.InspectTransaction(ctx, token.RealmID, id, credit)
			if err != nil {
				return err
			}
			encoder := json.NewEncoder(cmd.OutOrStdout())
			if !flags.asJSON {
				encoder.SetIndent("", "  ")
			}
			return encoder.Encode(result)
		}}
	cmd.Flags().StringVar(&id, "id", "", "local Purchase ID in the signed-in company")
	cmd.Flags().BoolVar(&credit, "credit-card-credit", false, "use the captured credit-card-credit form filter")
	_ = cmd.MarkFlagRequired("id")
	return cmd
}
