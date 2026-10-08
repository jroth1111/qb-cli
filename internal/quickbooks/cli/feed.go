package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

// feedFlags holds flags local to the `qb feed` subcommands. Persistent
// flags (--json, --home, --timeout) live on rootFlags.
type feedFlags struct {
	accountID   string
	reviewState string
	limit       int
	ids         []string
}

func newFeedCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feed",
		Short: "Replay QuickBooks banking feeds",
		Long: "qb feed <entity> <verb>.\n" +
			"Wired: account list; rule get|search|create|update|delete; txn list|get|population|import|update exclude|undo-excluded|categorise|match|split|batch-accept|unpost; run transfer|attach.\n" +
			"olbTxnIds only. Test mutations: account 44 (CR-Test-Account). --dry-run never dials.",
	}
	account := &cobra.Command{Use: "account", Short: "Bank accounts"}
	account.AddCommand(newFeedAccountsCmd(flags))
	account.AddCommand(newFeedMutationCmd(flags, "account", "run", "link account (not wired)"))
	account.AddCommand(newFeedMutationCmd(flags, "account", "refresh", "refresh account (manualUpdate; not wired)"))
	txn := &cobra.Command{Use: "txn", Short: "Feed transactions"}
	listEnt := newFeedPendingCmd(flags)
	listEnt.Use = "list"
	listEnt.Short = "List feed rows by review state (default: pending)"
	txn.AddCommand(listEnt)
	listEnt.AddCommand(newFeedStateCmd(flags, "posted", "List posted (accepted) transactions for an account", "ACCEPTED", 0))
	listEnt.AddCommand(newFeedStateCmd(flags, "excluded", "List excluded transactions for an account", "EXCLUDED", 0))
	txn.AddCommand(newFeedLookupCmd(flags))
	txn.AddCommand(newFeedStateCmd(flags, "population", "List the full pending population for an account", "PENDING", 0))
	txn.AddCommand(newFeedVerifyCmd(flags))
	txn.AddCommand(newFeedPopulationAllCmd(flags))
	updateEnt := &cobra.Command{Use: "update", Short: "Mutate pending feed rows"}
	updateEnt.AddCommand(newFeedExcludeCmd(flags))
	updateEnt.AddCommand(newFeedUndoCmd(flags))
	updateEnt.AddCommand(newFeedCategoriseCmd(flags))
	updateEnt.AddCommand(newFeedMatchCmd(flags))
	updateEnt.AddCommand(newFeedSplitCmd(flags))
	updateEnt.AddCommand(newFeedBatchAcceptCmd(flags))
	runEnt := &cobra.Command{Use: "run", Short: "Run feed operations"}
	runEnt.AddCommand(newFeedTxnAttachCmd(flags))
	runEnt.AddCommand(newFeedTransferCmd(flags))
	runEnt.AddCommand(newFeedMutationCmd(flags, "txn run", "tag", "tag (blocked: tag writes ride the retired tag service)"))
	updateEnt.AddCommand(newFeedUnpostCmd(flags))
	txn.AddCommand(updateEnt, runEnt)
	txn.AddCommand(newFeedImportCmd(flags))
	rule := &cobra.Command{Use: "rule", Short: "Bank rules"}
	rule.AddCommand(newFeedRuleReadCmd(flags))
	rule.AddCommand(newFeedRuleSearchCmd(flags))
	rule.AddCommand(newFeedRuleSaveCmd(flags, "create", "QBO.FEED.RULE_CREATE"))
	rule.AddCommand(newFeedRuleSaveCmd(flags, "update", "QBO.FEED.RULE_EDIT"))
	rule.AddCommand(newFeedRuleDeleteCmd(flags))

	rec := &cobra.Command{Use: "rec", Short: "Reconcile helpers"}
	rec.AddCommand(newFeedRecRunCmd(flags))
	if e, ok := catalogByID("QBO.FEED.REC_REPORT"); ok && e.Mode != modeBlocked && e.Mode != modeExcluded {
		rec.AddCommand(newFeedRecReportCmd(flags, e))
	} else {
		rec.AddCommand(newFeedMutationCmd(flags, "rec", "get", "rec report (not wired)"))
	}

	// Service map (2026-08-24): never-triggered production services, blocked.
	bank_account_lookupEnt := &cobra.Command{Use: "bank-account-lookup", Short: "bank-account-lookup"}
	bank_account_lookupEnt.AddCommand(newFeedMutationCmd(flags, "bank-account-lookup", "get", "bankaccountlookup service (service map; not wired)"))
	ach_transferEnt := &cobra.Command{Use: "ach-transfer", Short: "ach-transfer"}
	ach_transferEnt.AddCommand(newFeedMutationCmd(flags, "ach-transfer", "create", "ach-aws money movement (service map; not wired)"))
	cmd.AddCommand(account, txn, rule, rec, bank_account_lookupEnt, ach_transferEnt)
	cmd.AddCommand(newFeedServiceMapCmds(flags)...)
	validateFeedArguments(cmd)
	return cmd
}

// Feed leaves are flag-only. Without this guard, `--ids 1 2 3` silently
// submitted only 1 while discarding the other positional arguments.
func validateFeedArguments(cmd *cobra.Command) {
	if cmd.Args == nil && (cmd.RunE != nil || cmd.Run != nil) {
		cmd.Args = func(cmd *cobra.Command, args []string) error {
			if len(args) != 0 {
				return fmt.Errorf("unexpected positional arguments; pass IDs comma-separated (--ids 1,2,3) or repeat --ids")
			}
			if cmd.Flags().Lookup("ids") != nil {
				ids, err := cmd.Flags().GetStringSlice("ids")
				if err != nil {
					return err
				}
				for _, id := range ids {
					if strings.ContainsAny(id, " \t\r\n") {
						return fmt.Errorf("--ids must be comma-separated or repeated, not whitespace-separated")
					}
				}
			}
			return nil
		}
	}
	for _, child := range cmd.Commands() {
		validateFeedArguments(child)
	}
}

func newFeedAccountsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List connected banking accounts",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunGET(flags, cmd, "feed account list", "QBO.FEED.ACCOUNT_LIST", client.PlannedAccountsURL(), "captured GET; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayAccounts(ctx)
			if err != nil {
				return feedErr(flags, err)
			}
			printAccounts(cmd.OutOrStdout(), cmd.ErrOrStderr(), flags, res)
			return nil
		},
	}
	applyCatalogHelp(cmd, "QBO.FEED.ACCOUNT_LIST")
	return cmd
}

// newFeedPendingCmd implements `qb feed txn list`: list pending review
// transactions for an account. Defaults to account 204
// when --account-id is omitted.
func newFeedPendingCmd(flags *rootFlags) *cobra.Command {
	ff := &feedFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List pending review transactions for an account",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunGET(flags, cmd, "feed txn list", "QBO.FEED.TXN_PENDING", client.PlannedFeedURL(ff.accountID, "PENDING"), "captured GET; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayPending(ctx, ff.accountID, ff.reviewState, ff.limit)
			if err != nil {
				return feedErr(flags, err)
			}
			printPending(cmd.OutOrStdout(), cmd.ErrOrStderr(), flags, res, ff.accountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id (defaults to "+client.DefaultAccountID+")")
	cmd.Flags().StringVar(&ff.reviewState, "review-state", "pending",
		"review state filter (QBO enum; sent uppercased, e.g. PENDING)")
	cmd.Flags().IntVar(&ff.limit, "limit", 0, "max transactions to fetch (0 = all)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_PENDING")
	return cmd
}

// newFeedStateCmd implements a read-only `qb feed <use>` that replays a
// fixed review state. population runs the full walk (client.ReplayFeedComplete):
// paged startIndex/chunkSize + x-range with a completeness check against
// numTxnToReview from getInitialData. The other states use single capped
// requests via client.ReplayFeed.
//
// Date filtering: the ATS getTransactions endpoint ignores all date
// parameters server-side, so the CLI offers no date flags here; slice the
// complete population dump client-side by Transaction.Date instead.
func newFeedStateCmd(flags *rootFlags, use, short, reviewState string, defaultLimit int) *cobra.Command {
	ff := &feedFlags{}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			id := "QBO.FEED.TXN_POSTED"
			switch use {
			case "excluded":
				id = "QBO.FEED.TXN_EXCLUDED"
			case "population":
				id = "QBO.FEED.TXN_POPULATION"
			}
			if handled, err := dryRunGET(flags, cmd, feedStateCommand(use), id, client.PlannedFeedURL(ff.accountID, reviewState), "captured GET; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			var res *client.PendingResult
			var err error
			if use == "population" {
				res, err = client.ReplayFeedComplete(ctx, ff.accountID, reviewState, 0)
				if errors.Is(err, client.ErrIncomplete) {
					// Surface partial rows plus the mismatch; exit non-zero
					// so pipelines cannot mistake this for a complete dump.
					printPending(cmd.OutOrStdout(), cmd.ErrOrStderr(), flags, res, ff.accountID)
					fmt.Fprintf(cmd.ErrOrStderr(), "WARNING: %v\n", err)
					return &ExitError{Code: ExitInputError, Err: err, Silent: true}
				}
			} else {
				res, err = client.ReplayFeed(ctx, ff.accountID, reviewState, ff.limit)
			}
			if err != nil {
				return feedErr(flags, err)
			}
			printPending(cmd.OutOrStdout(), cmd.ErrOrStderr(), flags, res, ff.accountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id (defaults to "+client.DefaultAccountID+")")
	if use == "population" {
		cmd.Short = short + " — full paged walk, completeness-checked"
	} else {
		cmd.Flags().IntVar(&ff.limit, "limit", defaultLimit, "max transactions to fetch")
	}
	id := "QBO.FEED.TXN_POSTED"
	switch use {
	case "excluded":
		id = "QBO.FEED.TXN_EXCLUDED"
	case "population":
		id = "QBO.FEED.TXN_POPULATION"
	}
	applyCatalogHelp(cmd, id)
	return cmd
}

// newFeedLookupCmd implements `qb feed txn get`: search pending, accepted,
// and excluded transactions for an account by id or description
// (case-insensitive) via client.ReplayLookup. --query is required; an empty
// query is rejected at the CLI before any replay so the failure is instant
// and never reaches the network.
func newFeedLookupCmd(flags *rootFlags) *cobra.Command {
	ff := &feedFlags{}
	var query string
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Search pending/posted/excluded transactions by id, description or cheque number",
		RunE: func(cmd *cobra.Command, args []string) error {
			if query == "" {
				return &ExitError{
					Code:   ExitInputError,
					Err:    errors.New("lookup requires --query"),
					Silent: flags.asJSON,
				}
			}
			if handled, err := dryRunGET(flags, cmd, "feed txn get", "QBO.FEED.TXN_LOOKUP", client.PlannedFeedURL(ff.accountID, "PENDING"), "lookup walks pending+posted+excluded to empty terminal pages; limit caps returned matches; not sent"); handled {
				return err
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayLookup(ctx, ff.accountID, query, ff.limit)
			if err != nil {
				return feedErr(flags, err)
			}
			printPending(cmd.OutOrStdout(), cmd.ErrOrStderr(), flags, res, ff.accountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id (defaults to "+client.DefaultAccountID+")")
	cmd.Flags().StringVar(&query, "query", "", "search query (matches id, description or cheque number, case-insensitive)")
	cmd.Flags().IntVar(&ff.limit, "limit", 0, "max matching rows returned; all three review states are fully searched (0 = all)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_LOOKUP")
	return cmd

}

// newFeedExcludeCmd POSTs excludeTransactions (olb-bank-feed.yaml).
// --ids are olbTxnIds. Empty ids fail before network. Do not pass :ofx display ids.
func newFeedExcludeCmd(flags *rootFlags) *cobra.Command {
	ff := &feedFlags{}
	cmd := &cobra.Command{
		Use:   "exclude",
		Short: "Exclude pending feed rows (POST excludeTransactions)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				plan, err := client.PlanExclude(ff.accountID, ff.ids)
				if err != nil {
					return feedErr(flags, err)
				}
				return writePlan(cmd, flags, planFromClient(cmd, "feed txn update exclude", "QBO.FEED.TXN_EXCLUDE", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			if err := client.ReplayExclude(ctx, ff.accountID, ff.ids); err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"ok": true, "excluded": len(ff.ids)})
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "excluded %d olbTxnId(s) on account %s\n", len(ff.ids), ff.accountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id")
	cmd.Flags().StringSliceVar(&ff.ids, "ids", nil, "olbTxnIds to exclude (not :ofx display ids)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_EXCLUDE")
	return cmd

}

// newFeedUndoCmd POSTs undoTransactions (olb-bank-feed.yaml).
// --ids are olbTxnIds. Empty ids fail before network.
func newFeedUndoCmd(flags *rootFlags) *cobra.Command {
	ff := &feedFlags{}
	cmd := &cobra.Command{
		Use:   "undo-excluded",
		Short: "Restore excluded feed rows to pending (POST undoTransactions)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				plan, err := client.PlanUndo(ff.accountID, ff.ids)
				if err != nil {
					return feedErr(flags, err)
				}
				return writePlan(cmd, flags, planFromClient(cmd, "feed txn update undo-excluded", "QBO.FEED.TXN_UNDO_EXCLUDED", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			if err := client.ReplayUndo(ctx, ff.accountID, ff.ids); err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"ok": true, "undone": len(ff.ids)})
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "restored %d olbTxnId(s) on account %s\n", len(ff.ids), ff.accountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id")
	cmd.Flags().StringSliceVar(&ff.ids, "ids", nil, "olbTxnIds to restore (not :ofx display ids)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_UNDO_EXCLUDED")
	return cmd

}

type importFlags struct {
	accountID   string
	date        string
	description string
	amount      string
	file        string
}

// newFeedImportCmd POSTs processCsvFile (captured 2026-08-17).
// Creates PENDING rows on a CSV-import account. Refuses 204/93.
// --account-id is required (no default).
func newFeedImportCmd(flags *rootFlags) *cobra.Command {
	ff := &importFlags{}
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Add pending feed rows via CSV file upload, or a single row via flags",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				plan, _, err := client.PlanImportCSV(ff.accountID, ff.date, ff.description, ff.amount)
				if err != nil && ff.file == "" {
					return feedErr(flags, err)
				}
				return writePlan(cmd, flags, planFromClient(cmd, "feed txn import", "QBO.FEED.TXN_IMPORT", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			if ff.file != "" {
				return runFileImport(cmd, flags, ctx, ff)
			}
			res, err := client.ReplayImportCSV(ctx, ff.accountID, ff.date, ff.description, ff.amount)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(res)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "imported %d row(s) on account %s\n", res.Imported, res.AccountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", "",
		"CSV-import banking account id (44 works on TC2; 209 is inactive; refuses 204 and 93)")
	cmd.Flags().StringVar(&ff.date, "date", "",
		"row date as dd/MM/yyyy (default today in Australia/Sydney)")
	cmd.Flags().StringVar(&ff.description, "description", "",
		"bank description (required)")
	cmd.Flags().StringVar(&ff.amount, "amount", "",
		"amount as a number; income positive, expense negative (required)")
	cmd.Flags().StringVar(&ff.file, "file", "",
		"CSV bank-statement file to stage and process (alternative to --description/--amount)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_IMPORT")
	return cmd
}

// runFileImport stages a CSV bank file via uploadCsvFile, then processes
// each data row through the single-row processCsvFile replay. The header
// row names Date, Description, Amount (case-insensitive); a headerless file
// is read positionally in the same order.
func runFileImport(cmd *cobra.Command, flags *rootFlags, ctx context.Context, ff *importFlags) error {
	raw, err := os.ReadFile(ff.file)
	if err != nil {
		return feedErr(flags, fmt.Errorf("reading import file: %w", err))
	}
	rows, err := parseImportCSV(raw)
	if err != nil {
		return feedErr(flags, err)
	}
	if _, err := client.UploadCSVFile(ctx, filepath.Base(ff.file), raw); err != nil {
		return feedErr(flags, err)
	}
	done := 0
	for _, r := range rows {
		res, err := client.ReplayImportCSV(ctx, ff.accountID, r.date, r.description, r.amount)
		if err != nil {
			return feedErr(flags, fmt.Errorf("row %d (%s): %w", done+1, r.description, err))
		}
		done += res.Imported
	}
	fmt.Fprintf(cmd.OutOrStdout(), "imported %d row(s) on account %s from %s\n", done, ff.accountID, ff.file)
	return nil
}

type importRow struct {
	date        string
	description string
	amount      string
}

// parseImportCSV reads Date,Description,Amount rows, tolerating a header.
func parseImportCSV(raw []byte) ([]importRow, error) {
	rdr := csv.NewReader(strings.NewReader(string(raw)))
	rdr.FieldsPerRecord = -1
	recs, err := rdr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parsing import CSV: %w", err)
	}
	start := 0
	if len(recs) > 0 && isImportHeader(recs[0]) {
		start = 1
	}
	var out []importRow
	for _, rec := range recs[start:] {
		if len(rec) < 3 {
			continue
		}
		out = append(out, importRow{
			date:        strings.TrimSpace(rec[0]),
			description: strings.TrimSpace(rec[1]),
			amount:      strings.TrimSpace(rec[2]),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("import CSV holds no data rows")
	}
	return out, nil
}

// isImportHeader reports a Date,Description,Amount header row.
func isImportHeader(rec []string) bool {
	if len(rec) < 3 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(rec[0]), "date") &&
		strings.EqualFold(strings.TrimSpace(rec[1]), "description") &&
		strings.EqualFold(strings.TrimSpace(rec[2]), "amount")
}

// newFeedCategoriseCmd POSTs batchAcceptTransactions?acceptOnly=true — the
// current SPA folds categorise into the accept call, setting
// addAsQboTxn.details[0].categoryId on each full feed row. --ids are
// olbTxnIds; --category is the QBO account/category id to post to; --class
// sets the class, with "none" explicitly clearing it. Empty ids or category
// fail before network.
func newFeedCategoriseCmd(flags *rootFlags) *cobra.Command {
	ff := &feedFlags{}
	var (
		category   string
		classRef   string
		memo       string
		entityType string
	)
	cmd := &cobra.Command{
		Use: "categorise",
		RunE: func(cmd *cobra.Command, args []string) error {
			detail := client.TransactionDetail{
				EntityType:  entityType,
				CategoryRef: &client.RefValue{Value: category},
			}
			if cmd.Flags().Changed("class") {
				value := classRef
				if strings.EqualFold(strings.TrimSpace(value), "none") {
					value = ""
				}
				detail.ClassRef = &client.RefValue{Value: value}
			}
			if cmd.Flags().Changed("memo") {
				detail.Memo = &memo
			}
			if flags.dryRun {
				plan, err := client.PlanCategorise(ff.accountID, ff.ids, detail)
				if err != nil {
					return feedErr(flags, err)
				}
				return writePlan(cmd, flags, planFromClient(cmd, "feed txn update categorise", "QBO.FEED.TXN_CATEGORISE", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			status, err := client.ReplayCategorise(ctx, ff.accountID, ff.ids, detail)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"ok": true, "status": status, "categorised": len(ff.ids)})
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "categorised %d olbTxnId(s) on account %s\n", len(ff.ids), ff.accountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id")
	cmd.Flags().StringSliceVar(&ff.ids, "ids", nil, "olbTxnIds to categorise (not :ofx display ids)")
	cmd.Flags().StringVar(&category, "category", "", "account/category id to post to (e.g. 7)")
	cmd.Flags().StringVar(&classRef, "class", "", "class id; none clears the existing suggestion")
	cmd.Flags().StringVar(&memo, "memo", "", "memo to persist on the posted transaction; omitted preserves existing text")
	cmd.Flags().StringVar(&entityType, "entity-type", "Expense", "QBO entity type (Expense, Deposit)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_CATEGORISE")
	return cmd
}

// newFeedMatchCmd POSTs acceptTransactions — the current SPA's match call.
// --ids are olbTxnIds; --match-id are existing QBO record ids to match
// against, resolved against the feed account's register so txnTypeId,
// sequence, sync token and payment amount all come from a live read.
// --txn-type is advisory only (the register's recorded type is sent).
// Empty ids or match-ids fail before network.
func newFeedMatchCmd(flags *rootFlags) *cobra.Command {
	ff := &feedFlags{}
	var (
		matchIDs []string
		txnType  string
	)
	cmd := &cobra.Command{
		Use:   "match",
		Short: "Match pending feed rows to existing QBO records (POST acceptTransactions)",
		RunE: func(cmd *cobra.Command, args []string) error {
			matchTxns := make([]client.MatchTxn, 0, len(matchIDs))
			for _, id := range matchIDs {
				id = strings.TrimSpace(id)
				if id == "" {
					continue
				}
				matchTxns = append(matchTxns, client.MatchTxn{TxnID: id, TxnType: txnType})
			}
			if flags.dryRun {
				plan, err := client.PlanMatch(ff.accountID, ff.ids, matchTxns)
				if err != nil {
					return feedErr(flags, err)
				}
				return writePlan(cmd, flags, planFromClient(cmd, "feed txn update match", "QBO.FEED.TXN_MATCH", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			status, err := client.ReplayMatch(ctx, ff.accountID, ff.ids, matchTxns)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"ok": true, "status": status, "matched": len(ff.ids)})
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "matched %d olbTxnId(s) on account %s\n", len(ff.ids), ff.accountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id")
	cmd.Flags().StringSliceVar(&ff.ids, "ids", nil, "olbTxnIds to match (not :ofx display ids)")
	cmd.Flags().StringSliceVar(&matchIDs, "match-id", nil, "existing QBO record ids to match against")
	cmd.Flags().StringVar(&txnType, "txn-type", "", "advisory record type hint (the register's recorded type is sent)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_MATCH")
	return cmd
}

// newFeedSplitCmd POSTs batchAcceptTransactions?acceptOnly=true — the
// current SPA folds split into the accept call, writing the lines to
// addAsQboTxn.details on each full feed row. --ids are olbTxnIds; --lines is
// a comma-separated list of categoryId=amount pairs (e.g. 7=-6,8=-4).
// The wire contract sends unsigned line magnitudes; signed or unsigned input
// is accepted. Empty ids or fewer than two lines fail before network.
func newFeedSplitCmd(flags *rootFlags) *cobra.Command {
	ff := &feedFlags{}
	var (
		linesStr   string
		entityType string
	)
	cmd := &cobra.Command{
		Use:   "split",
		Short: "Split pending feed rows across multiple accounts (POST batchAcceptTransactions)",
		RunE: func(cmd *cobra.Command, args []string) error {
			lines, perr := parseSplitLines(linesStr)
			if perr != nil {
				return &ExitError{Code: ExitInputError, Err: perr, Silent: flags.asJSON}
			}
			if flags.dryRun {
				plan, err := client.PlanSplit(ff.accountID, ff.ids, entityType, lines)
				if err != nil {
					return feedErr(flags, err)
				}
				return writePlan(cmd, flags, planFromClient(cmd, "feed txn update split", "QBO.FEED.TXN_SPLIT", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			status, err := client.ReplaySplit(ctx, ff.accountID, ff.ids, entityType, lines)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"ok": true, "status": status, "split": len(ff.ids)})
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "split %d olbTxnId(s) on account %s\n", len(ff.ids), ff.accountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id")
	cmd.Flags().StringSliceVar(&ff.ids, "ids", nil, "olbTxnIds to split (not :ofx display ids)")
	cmd.Flags().StringVar(&linesStr, "lines", "", "comma-separated categoryId=amount pairs; sent as positive magnitudes (e.g. 7=-6,8=-4)")
	cmd.Flags().StringVar(&entityType, "entity-type", "Expense", "QBO entity type (Expense, Deposit)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_SPLIT")
	return cmd
}

// newFeedBatchAcceptCmd POSTs batchAcceptTransactions?acceptOnly=true
// (captured live SPA). --ids are olbTxnIds to accept (post) in bulk, sent as
// full feed rows with acceptType ADD. Empty ids fail before network.
func newFeedBatchAcceptCmd(flags *rootFlags) *cobra.Command {
	ff := &feedFlags{}
	cmd := &cobra.Command{
		Use:   "batch-accept",
		Short: "Accept pending feed rows in bulk (POST batchAcceptTransactions)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				plan, err := client.PlanBatchAccept(ff.accountID, ff.ids)
				if err != nil {
					return feedErr(flags, err)
				}
				return writePlan(cmd, flags, planFromClient(cmd, "feed txn update batch-accept", "QBO.FEED.TXN_BATCH_ACCEPT", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			status, err := client.ReplayBatchAccept(ctx, ff.accountID, ff.ids)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"ok": true, "status": status, "accepted": len(ff.ids)})
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "accepted %d olbTxnId(s) on account %s\n", len(ff.ids), ff.accountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id")
	cmd.Flags().StringSliceVar(&ff.ids, "ids", nil, "olbTxnIds to accept in bulk (not :ofx display ids)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_BATCH_ACCEPT")
	return cmd
}

// parseSplitLines parses a comma-separated list of "categoryId=amount" pairs
// into SplitLine values. Returns an error if the string is empty or any pair
// is malformed.
func parseSplitLines(s string) ([]client.SplitLine, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("split requires --lines (comma-separated categoryId=amount pairs)")
	}
	pairs := strings.Split(s, ",")
	lines := make([]client.SplitLine, 0, len(pairs))
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		eq := strings.IndexByte(pair, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("invalid --lines pair %q: expected categoryId=amount", pair)
		}
		cat := strings.TrimSpace(pair[:eq])
		amtStr := strings.TrimSpace(pair[eq+1:])
		amt, err := strconv.ParseFloat(amtStr, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid --lines amount %q: %w", amtStr, err)
		}
		lines = append(lines, client.SplitLine{
			CategoryRef: client.RefValue{Value: cat},
			Amount:      amt,
		})
	}
	return lines, nil
}

// newFeedMutationCmd implements a `qb feed <prefix> <use>` mutation stub. It calls
// client.RequireSession (newAPIClient only; no network) so a missing-creds
// failure is still a fast auth exit, then surfaces client.ErrMutationNotWired
// as an input error. It never constructs a mutation body or POSTs to QBO.
func newFeedMutationCmd(flags *rootFlags, prefix, use, short string) *cobra.Command {
	command := "feed " + prefix + " " + use
	e, ok := catalogByCommand(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				var entry primitiveEntry
				if ok {
					entry = e
				}
				return writePlan(cmd, flags, uncapturedPlan(cmd, command, entry))
			}
			if err := client.RequireSession(); err != nil {
				return feedErr(flags, err)
			}
			return feedErr(flags, client.ErrMutationNotWired)
		},
	}
	if ok {
		cmd.Long = longHelp(e)
		cmd.Example = usageFor(e)
		attachParamFlags(cmd, e.ID)
	}
	return cmd
}

func newFeedRuleReadCmd(flags *rootFlags) *cobra.Command {
	var (
		id    string
		limit int
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "List bank rules (GET getRules)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunGET(flags, cmd, "feed rule get", "QBO.FEED.RULE_READ", client.PlannedRulesURL(), "captured GET; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayRules(ctx, limit)
			if err != nil {
				return feedErr(flags, err)
			}
			if id != "" {
				filtered := res.Rules[:0]
				for _, r := range res.Rules {
					if r.ID == id {
						filtered = append(filtered, r)
					}
				}
				res.Rules = filtered
				res.Counts = map[string]int{"rules": len(filtered)}
			}
			printRules(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "specific bank rule id")
	cmd.Flags().IntVar(&limit, "limit", 0, "max rules to return (0 = all)")
	applyCatalogHelp(cmd, "QBO.FEED.RULE_READ")
	return cmd
}

func newFeedRuleSearchCmd(flags *rootFlags) *cobra.Command {
	var (
		query string
		limit int
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search bank rules by name (GET getRules, client filter)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunGET(flags, cmd, "feed rule search", "QBO.FEED.RULE_SEARCH", client.PlannedRulesURL(), "captured GET; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayRules(ctx, 300)
			if err != nil {
				return feedErr(flags, err)
			}
			q := strings.ToLower(strings.TrimSpace(query))
			if q != "" {
				filtered := res.Rules[:0]
				for _, r := range res.Rules {
					if strings.Contains(strings.ToLower(r.Name), q) || strings.Contains(strings.ToLower(r.ID), q) {
						filtered = append(filtered, r)
					}
				}
				res.Rules = filtered
			}
			if limit > 0 && len(res.Rules) > limit {
				res.Rules = res.Rules[:limit]
			}
			res.Counts = map[string]int{"rules": len(res.Rules)}
			printRules(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "substring match on rule name")
	cmd.Flags().IntVar(&limit, "limit", 0, "max rules to return (0 = all)")
	applyCatalogHelp(cmd, "QBO.FEED.RULE_SEARCH")
	return cmd
}

// newFeedRuleSaveCmd wires `feed rule create|update` to the neo
// lists/olbrules/save upsert the olbrules drawer uses. Update resolves the
// rule's editSequence/ruleOrder through getRules before saving.
func newFeedRuleSaveCmd(flags *rootFlags, use, id string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   use,
		Short: "Save a bank rule (POST lists/olbrules/save)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "feed rule " + use,
					ID:      id,
					Method:  "POST",
					URL:     client.PlannedRuleSaveURL(),
					Note:    "neo lists/olbrules/save upsert; not sent",
					Flags:   fm,
				})
			}
			if err := client.RequireSession(); err != nil {
				return feedErr(flags, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayRuleSave(ctx, fm)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	for _, fd := range paramsFor(id) {
		cmd.Flags().String(fd.Name, fd.Default, fd.Help)
	}
	applyCatalogHelp(cmd, id)
	return cmd
}

// newFeedRuleDeleteCmd wires `feed rule delete` to the UI's batchDelete route.
func newFeedRuleDeleteCmd(flags *rootFlags) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a bank rule (POST lists/olbrules/batchDelete)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "feed rule delete",
					ID:      "QBO.FEED.RULE_DELETE",
					Method:  "POST",
					URL:     client.PlannedRuleDeleteURL(),
					Note:    "neo lists/olbrules/batchDelete; not sent",
					Flags:   fm,
				})
			}
			if err := client.RequireSession(); err != nil {
				return feedErr(flags, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayRuleDelete(ctx, fm)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "bank rule id to delete (required)")
	applyCatalogHelp(cmd, "QBO.FEED.RULE_DELETE")
	return cmd
}

// newFeedServiceMapCmds returns the Service map v2 (2026-08-24)
// never-triggered production services as feed-level entities, matching
// their catalog commands. Previously nested under rule search, which made
// the cataloged feed-level paths unresolvable.
func newFeedServiceMapCmds(flags *rootFlags) []*cobra.Command {
	financial_profileEnt := &cobra.Command{Use: "financial-profile", Short: "financial-profile"}
	financial_profileEnt.AddCommand(newStubCmd(flags, "feed financial-profile", "get", "financialprofile-aws.api.intuit.com get (service map; not wired)"))
	financial_providerEnt := &cobra.Command{Use: "financial-provider", Short: "financial-provider"}
	financial_providerEnt.AddCommand(newStubCmd(flags, "feed financial-provider", "get", "financialprovider-aws.api.intuit.com get (service map; not wired)"))
	financial_data_interactionEnt := &cobra.Command{Use: "financial-data-interaction", Short: "financial-data-interaction"}
	financial_data_interactionEnt.AddCommand(newStubCmd(flags, "feed financial-data-interaction", "post", "financialdatainteraction-aws.api.intuit.com post (service map; not wired)"))
	financial_documentEnt := &cobra.Command{Use: "financial-document", Short: "financial-document"}
	financial_documentEnt.AddCommand(newStubCmd(flags, "feed financial-document", "get", "financialdocumentpci.api.intuit.com get (service map; not wired)"))
	financial_profile_orchEnt := &cobra.Command{Use: "financial-profile-orch", Short: "financial-profile-orch"}
	financial_profile_orchEnt.AddCommand(newStubCmd(flags, "feed financial-profile-orch", "post", "financialprofileorchestration-aws.api.intuit.com post (service map; not wired)"))
	txns_orchestrationEnt := &cobra.Command{Use: "txns-orchestration", Short: "txns-orchestration"}
	txns_orchestrationEnt.AddCommand(newStubCmd(flags, "feed txns-orchestration", "post", "txnsorchestrationsvc.api.intuit.com/graphql post (service map; not wired)"))
	vaultEnt := &cobra.Command{Use: "vault", Short: "vault"}
	vaultEnt.AddCommand(newStubCmd(flags, "feed vault", "get", "vault*.api.intuit.com/v1[/oauth] get (service map; not wired)"))
	return []*cobra.Command{financial_profileEnt, financial_providerEnt, financial_data_interactionEnt, financial_documentEnt, financial_profile_orchEnt, txns_orchestrationEnt, vaultEnt}
}

func printRules(stdout io.Writer, flags *rootFlags, res *client.RulesResult) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	if res == nil || len(res.Rules) == 0 {
		fmt.Fprintln(stdout, "no rules")
		return
	}
	fmt.Fprintf(stdout, "%d rule(s)\n", len(res.Rules))
	for _, r := range res.Rules {
		fmt.Fprintf(stdout, "%s\t%s\n", r.ID, r.Name)
	}
}

// feedTimeout derives the per-call timeout from the shared --timeout flag,
// clamped to a sane minimum so a misconfigured default cannot starve the
// in-process HTTP client of the time it needs for the TLS handshake + request.
func feedTimeout(flags *rootFlags) time.Duration {
	if flags.timeout <= 0 {
		return 5 * time.Minute
	}
	return flags.timeout
}

// feedErr maps a client error to a structured ExitError. Missing
// credentials surface as ExitAuthError (2) so scripts can branch; any
// other transport/API failure is ExitInputError (1), matching the
// non-200 response path.
func feedErr(flags *rootFlags, err error) error {
	if errors.Is(err, client.ErrNoCredentials) {
		return &ExitError{Code: ExitAuthError, Err: err, Silent: flags.asJSON}
	}
	return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
}

// --- output (secret-free) -------------------------------------------------

func printAccounts(stdout, stderr io.Writer, flags *rootFlags, res *client.AccountsResult) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	if res == nil || len(res.Accounts) == 0 {
		fmt.Fprintln(stdout, "No connected banking accounts.")
		return
	}
	fmt.Fprintf(stdout, "Connected banking accounts (%d):\n", len(res.Accounts))
	for _, a := range res.Accounts {
		fmt.Fprintf(stdout, "  %-6s %-32s %-28s %12.2f\n", a.ID, truncate(a.Name, 32), truncate(a.Type, 28), a.Balance)
	}
}

func printPending(stdout, stderr io.Writer, flags *rootFlags, res *client.PendingResult, accountID string) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	if res == nil || len(res.Transactions) == 0 {
		fmt.Fprintf(stdout, "No pending transactions for account %s.\n", accountID)
		return
	}
	total := res.Counts["total"]
	if total > 0 {
		fmt.Fprintf(stdout, "Pending transactions for account %s (%d shown, %d total):\n",
			accountID, len(res.Transactions), total)
	} else {
		fmt.Fprintf(stdout, "Pending transactions for account %s (%d):\n",
			accountID, len(res.Transactions))
	}
	for _, t := range res.Transactions {
		date := t.Date
		if len(date) >= 10 {
			date = date[:10]
		}
		fmt.Fprintf(stdout, "  %-14s %12.2f  %-10s  %s\n", date, t.Amount, t.AcceptType, truncate(t.Description, 50))
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// feedStateCommand maps a newFeedStateCmd use to its catalog command.
// posted/excluded live under `qb feed txn list`; population stays bare.
func feedStateCommand(use string) string {
	if use == "population" {
		return "feed txn population"
	}
	return "feed txn list " + use
}

// newFeedRecReportCmd implements `qb feed rec get`: the per-account
// reconciliation session report via the captured Integration_Reconciliation
// node read (replaces the former BalanceSheet stand-in).
func newFeedRecReportCmd(flags *rootFlags, e primitiveEntry) *cobra.Command {
	ff := &feedFlags{}
	cmd := &cobra.Command{
		Use:   "get",
		Short: "reconciliation session report for an account",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunGET(flags, cmd, "feed rec get", "QBO.FEED.REC_REPORT", client.PlannedReconcileURL(ff.accountID), "v4 Integration_Reconciliation node read; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayReconciliation(ctx, ff.accountID, "", ff.limit)
			if err != nil {
				return feedErr(flags, err)
			}
			printQuery(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id (defaults to "+client.DefaultAccountID+")")
	cmd.Flags().IntVar(&ff.limit, "limit", 0, "max rows (0 = all)")
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newFeedRecRunCmd implements `qb feed rec run`: reconcileAction FINISH on
// the account's open reconcile session — the UI's "accept the difference
// and book an adjusting entry" path (verified live TC2 2026-09-19: session
// closes and a DocNumber ADJ "Reconcile Adjustment" txn posts to the
// adjusting account). Requires an open session (accounting reconcile
// create); empty flags default from the session node read.
func newFeedRecRunCmd(flags *rootFlags) *cobra.Command {
	var (
		accountID string
		adjDate   string
		adjAcct   string
		rate      string
		endDate   string
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Finish open reconcile session with an adjusting entry (FINISH)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				plan, err := client.PlanReconcileFinish(accountID, adjDate, adjAcct, rate, endDate)
				if err != nil {
					return feedErr(flags, err)
				}
				return writePlan(cmd, flags, planFromClient(cmd, "feed rec run", "QBO.FEED.REC_AUTO_ADJUST", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayReconcileFinish(ctx, accountID, adjDate, adjAcct, rate, endDate)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&accountID, "account-id", "",
		"bank account id with an open reconcile session (required)")
	cmd.Flags().StringVar(&adjDate, "date", "",
		"adjusting entry date yyyy-MM-dd (defaults to the statement ending date)")
	cmd.Flags().StringVar(&adjAcct, "adjust-account", "",
		"adjusting entry account — v3 account id or Relay node id (defaults to the account's Reconciliation Discrepancies account)")
	cmd.Flags().StringVar(&rate, "exchange-rate", "",
		"adjusting entry exchange rate (defaults to 1, the home-currency rate)")
	cmd.Flags().StringVar(&endDate, "ending-date", "",
		"statement ending date yyyy-MM-dd (defaults to the open session's date)")
	applyCatalogHelp(cmd, "QBO.FEED.REC_AUTO_ADJUST")
	return cmd
}

// newFeedUnpostCmd POSTs undoTransactions with reviewState ACCEPTED — the
// Posted tab's Undo contract (verified live TC2 2026-09-19). Moves posted
// feed rows back to pending and deletes the transaction the accept
// created. --ids are olbTxnIds. Empty ids fail before network.
func newFeedUnpostCmd(flags *rootFlags) *cobra.Command {
	ff := &feedFlags{}
	cmd := &cobra.Command{
		Use:   "unpost",
		Short: "Undo posted feed rows back to pending (POST undoTransactions)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				plan, err := client.PlanUnpost(ff.accountID, ff.ids)
				if err != nil {
					return feedErr(flags, err)
				}
				return writePlan(cmd, flags, planFromClient(cmd, "feed txn update unpost", "QBO.FEED.TXN_UNPOST", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			if err := client.ReplayUnpost(ctx, ff.accountID, ff.ids); err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"ok": true, "unposted": len(ff.ids)})
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "unposted %d olbTxnId(s) on account %s\n", len(ff.ids), ff.accountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id")
	cmd.Flags().StringSliceVar(&ff.ids, "ids", nil, "olbTxnIds to unpost (not :ofx display ids)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_UNPOST")
	return cmd
}

// newFeedTransferCmd POSTs batchAcceptTransactions?acceptOnly=true with each
// pending row shaped as a transfer — the Transaction type=Transfer + Post
// contract (verified live TC2 2026-09-19: acceptType TRANSFER, transfer:true,
// addAsQboTxn.txnTypeId 26, details[0].categoryId = destination account).
// --ids are olbTxnIds; --to-account-id is the destination bank/credit-card
// account. Empty ids or destination fail before network.
func newFeedTransferCmd(flags *rootFlags) *cobra.Command {
	ff := &feedFlags{}
	var toAccount string
	cmd := &cobra.Command{
		Use:   "transfer",
		Short: "Post pending feed rows as transfers to another account (POST batchAcceptTransactions)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				plan, err := client.PlanTransfer(ff.accountID, ff.ids, toAccount)
				if err != nil {
					return feedErr(flags, err)
				}
				return writePlan(cmd, flags, planFromClient(cmd, "feed txn run transfer", "QBO.FEED.TXN_TRANSFER", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			status, err := client.ReplayTransfer(ctx, ff.accountID, ff.ids, toAccount)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"ok": true, "status": status, "transferred": len(ff.ids)})
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "transferred %d olbTxnId(s) on account %s to account %s\n", len(ff.ids), ff.accountID, toAccount)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID,
		"banking account id")
	cmd.Flags().StringSliceVar(&ff.ids, "ids", nil, "olbTxnIds to post as transfers (not :ofx display ids)")
	cmd.Flags().StringVar(&toAccount, "to-account-id", "", "destination bank/credit card account id (required)")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_TRANSFER")
	return cmd
}

// newFeedTxnAttachCmd attaches a file and/or note to a posted QBO
// transaction via the v3 Attachable entity (verified live TC2 2026-09-19):
// --file uploads via POST /upload then sparse-links with AttachableRef;
// --note alone creates a note-only attachable already linked. --id is the
// posted transaction id and --txn-type its v3 entity type (Invoice, Bill,
// Purchase, Transfer, ...). At least one of --note/--file is required.
func newFeedTxnAttachCmd(flags *rootFlags) *cobra.Command {
	var (
		id      string
		txnType string
		note    string
		file    string
	)
	cmd := &cobra.Command{
		Use:   "attach",
		Short: "Attach a file and/or note to a posted transaction (v3 upload + attachable link)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				plan, err := client.PlanTxnAttach(txnType, id, note, file)
				if err != nil {
					return feedErr(flags, err)
				}
				return writePlan(cmd, flags, planFromClient(cmd, "feed txn run attach", "QBO.FEED.TXN_ATTACH", modeWired, plan))
			}
			var content []byte
			var filename string
			if strings.TrimSpace(file) != "" {
				raw, err := os.ReadFile(file)
				if err != nil {
					return feedErr(flags, fmt.Errorf("reading attach file: %w", err))
				}
				content = raw
				filename = filepath.Base(file)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayTxnAttach(ctx, txnType, id, note, filename, content)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(res)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "attached %s %s to %s %s (attachable %s)\n",
				res.FileName, res.Note, res.TxnType, res.TxnID, res.AttachableID)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "posted transaction id to attach to (required)")
	cmd.Flags().StringVar(&txnType, "txn-type", "", "v3 entity type of --id: Invoice, Bill, Purchase, Transfer, ... (required)")
	cmd.Flags().StringVar(&note, "note", "", "note text to attach (optional when --file is given)")
	cmd.Flags().StringVar(&file, "file", "", "file to upload and link — PDF/PNG/JPG/CSV/TXT")
	applyCatalogHelp(cmd, "QBO.FEED.TXN_ATTACH")
	return cmd
}
