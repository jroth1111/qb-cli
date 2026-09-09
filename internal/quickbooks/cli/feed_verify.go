package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

// newFeedVerifyCmd implements `qb feed txn verify`: walk the pending
// population and report completeness against numTxnToReview. A mismatch is
// reported (exit 1) — never silently ignored.
func newFeedVerifyCmd(flags *rootFlags) *cobra.Command {
	ff := &feedFlags{}
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Audit feed completeness against the server count",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunGET(flags, cmd, "feed txn verify", "QBO.FEED.TXN_VERIFY", client.PlannedVerifyURL(ff.accountID), "captured GET; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.VerifyFeed(ctx, ff.accountID)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			state := "COMPLETE"
			if !res.Complete {
				state = "INCOMPLETE"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "account %s: expected=%d fetched=%d %s\n",
				res.AccountID, res.Expected, res.Fetched, state)
			if !res.Complete {
				return &ExitError{Code: ExitInputError,
					Err:    errors.New("feed verification failed"),
					Silent: flags.asJSON}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_VERIFY")
	return cmd
}

// newFeedPopulationAllCmd implements `qb feed txn population-all`: full
// pending walk for every connected account, producing a per-account
// completeness manifest.
func newFeedPopulationAllCmd(flags *rootFlags) *cobra.Command {
	var maxPages int
	cmd := &cobra.Command{
		Use:   "population-all",
		Short: "Full pending walk for ALL connected accounts (completeness manifest)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunGET(flags, cmd, "feed txn population-all", "QBO.FEED.TXN_POPULATION_ALL", client.PlannedAccountsURL(), "walks every account; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags)*4)
			defer cancel()
			res, err := client.ReplayPopulationAll(ctx, maxPages)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			for _, a := range res.Accounts {
				state := "COMPLETE"
				if !a.Complete {
					state = "INCOMPLETE"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-10s expected=%-6d fetched=%-6d %s\n",
					a.AccountID, a.Expected, a.Fetched, state)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\n%d/%d accounts complete\n",
				countComplete(res), len(res.Accounts))
			if !res.AllComplete {
				return &ExitError{Code: ExitInputError,
					Err:    errors.New("one or more accounts incomplete"),
					Silent: flags.asJSON}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&maxPages, "max-pages-per-account", 40,
		"page ceiling per account (300 rows per page)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_POPULATION_ALL")
	return cmd
}

func countComplete(res *client.PopulationAllResult) int {
	n := 0
	for _, a := range res.Accounts {
		if a.Complete {
			n++
		}
	}
	return n
}
