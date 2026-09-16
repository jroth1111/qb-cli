package cli

// accounting_deep.go wires the accounting-domain deep CRUD verbs discovered
// in the JS capture corpus onto `qb accounting`. The client layer
// (client/accounting_deep.go) owns transport + validation; this file only
// binds flags, plans dry-runs and renders results.
//
//	Wired here: budget list, journal list|create|delete (bulk),
//	            class delete|update, location delete|update,
//	            deferredrevenue get|create, prepaid get|create,
//	            openingbalance create.
//	(Other verbs on these entities stay as previously wired commands in
//	cli/accounting.go — v3-backed or stubs.)

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

// printQueryResult is the shared secret-free renderer for QueryResult
// envelopes across the deep-accounting commands.
func printAccountingQueryResult(cmd *cobra.Command, flags *rootFlags, res *client.QueryResult) {
	if flags.asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	out := cmd.OutOrStdout()
	if res == nil || len(res.Items) == 0 {
		fmt.Fprintf(out, "no %s rows (%s)\n", res.Entity, res.Note)
		return
	}
	fmt.Fprintf(out, "%s (%d rows) %s\n", res.Entity, len(res.Items), res.Note)
	for _, it := range res.Items {
		line := fmt.Sprintf("  %-14s", it.ID)
		if it.Name != "" {
			line += " " + truncate(it.Name, 36)
		}
		if it.Date != "" {
			line += "  " + it.Date
		}
		if it.Amount != 0 {
			line += fmt.Sprintf("  %12.2f", it.Amount)
		}
		if it.Account != "" {
			line += "  " + it.Account
		}
		fmt.Fprintln(out, line)
	}
}

// printMutateResult renders a MutateResult in text/JSON.
func printMutateResult(cmd *cobra.Command, flags *rootFlags, res *client.MutateResult) {
	if flags.asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	note := ""
	if res.Note != "" {
		note = " (" + res.Note + ")"
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s%s\n", res.Op, res.Entity, res.Item.ID, note)
}

func newAccountingBudgetListCmd(flags *rootFlags) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List budgets (budgeting.api GetSupergraphBudgetsByIds)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunGET(flags, cmd, "accounting budget list", "QBO.ACCOUNTING.BUDGET_LIST", client.PlannedBudgetListURL(), "captured supergraph POST; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayBudgetList(ctx, limit)
			if err != nil {
				return feedErr(flags, err)
			}
			printAccountingQueryResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "max budgets to fetch")
	applyCatalogHelp(cmd, "QBO.ACCOUNTING.BUDGET_READ")
	return cmd
}

func newAccountingJournalBulkCreateCmd(flags *rootFlags) *cobra.Command {
	var itemsJSON string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Bulk-create balanced journal entries (v3 /batch)",
		Long: "Bulk-create journal entries in one v3 batch request.\n" +
			"--items-json must decode to [{\"date\":\"yyyy-MM-dd\",\"amount\":N,\"from-account\":\"ID\",\"to-account\":\"ID\",\"memo\":\"...\"}].\n" +
			"Every item books a two-line entry: debit from-account, credit to-account.",
		RunE: func(cmd *cobra.Command, args []string) error {
			items, perr := client.ParseJournalBulkItems(itemsJSON)
			if perr != nil {
				return exitInput(perr)
			}
			if flags.dryRun {
				body, _ := json.Marshal(items)
				return writePlan(cmd, flags, planEnvelope{
					Command: "accounting journal create",
					ID:      "QBO.ACCOUNTING.JOURNAL_CREATE",
					Mode:    modeWired,
					Method:  "POST",
					URL:     client.PlannedBatchURL(),
					Body:    body,
					Flags:   localFlagMap(cmd),
					Note:    "v3 BatchItemRequest; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayJournalBulkCreate(ctx, items)
			if err != nil {
				return feedErr(flags, err)
			}
			printAccountingQueryResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&itemsJSON, "items-json", "", "JSON array of {date, amount, from-account, to-account, memo}")
	_ = cmd.MarkFlagRequired("items-json")
	applyCatalogHelp(cmd, "QBO.ACCOUNTING.JOURNAL_CREATE")
	return cmd
}

func newAccountingJournalBulkDeleteCmd(flags *rootFlags) *cobra.Command {
	var ids []string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Bulk-delete journal entries (v3 /batch)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(ids) == 0 {
				return exitInput(client.ErrEmptyJournalIDs)
			}
			if flags.dryRun {
				body, _ := json.Marshal(ids)
				return writePlan(cmd, flags, planEnvelope{
					Command: "accounting journal delete",
					ID:      "QBO.ACCOUNTING.JOURNAL_DELETE",
					Mode:    modeWired,
					Method:  "POST",
					URL:     client.PlannedBatchURL(),
					Body:    body,
					Flags:   localFlagMap(cmd),
					Note:    "v3 batch delete (SyncTokens fetched per id); not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayJournalBulkDelete(ctx, ids)
			if err != nil {
				return feedErr(flags, err)
			}
			printAccountingQueryResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&ids, "ids", nil, "journal entry ids to delete")
	_ = cmd.MarkFlagRequired("ids")
	applyCatalogHelp(cmd, "QBO.ACCOUNTING.JOURNAL_DELETE")
	return cmd
}

// newAccountingDimensionMutateCmd builds class/location update+delete via v3.
func newAccountingDimensionMutateCmd(flags *rootFlags, entity, verb, catalogID string) *cobra.Command {
	var (
		id     string
		name   string
		active string
	)
	cmd := &cobra.Command{
		Use:   verb,
		Short: entity + " " + verb + " (v3)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if verb == "update" && id == "" {
				return exitInput(client.ErrMissingMutateID)
			}
			if verb == "delete" && id == "" {
				return exitInput(client.ErrMissingMutateID)
			}
			fm := localFlagMap(cmd)
			fm["id"] = id
			op := verb
			if verb == "delete" {
				// Class/Department have no v3 hard delete (operation=delete
				// returns 200 with an empty id and changes nothing);
				// deactivation is the honest delete.
				op = "deactivate"
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "accounting " + lower(entity) + " " + verb,
					ID:      catalogID,
					Mode:    modeWired,
					Method:  "POST",
					URL:     client.PlannedMutateURL(entity, op),
					Flags:   fm,
					Note:    "v3 " + op + "; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayMutate(ctx, entity, op, id, fm)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "target "+lower(entity)+" id")
	cmd.Flags().StringVar(&name, "name", "", "new display name (update)")
	cmd.Flags().StringVar(&active, "active", "", "set to false to deactivate")
	applyCatalogHelp(cmd, catalogID)
	return cmd
}

func lower(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out[i] = c
	}
	return string(out)
}

func newAccountingDeferredScheduleGetCmd(flags *rootFlags, prepaid bool) *cobra.Command {
	var sourceID string
	label := "deferredrevenue"
	entity := "DeferredRevenueSchedule"
	id := "QBO.ACCOUNTING.DEFERRED_RECOGNITION_GET"
	if prepaid {
		label = "prepaid"
		entity = "PrepaidSchedule"
		id = "QBO.ACCOUNTING.PREPAID_READ"
	}
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get the recognition schedule for a source txn (deferredrecognition GET_SCHEDULE)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if sourceID == "" {
				return exitInput(fmt.Errorf("%s get requires --source-id", label))
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "accounting " + label + " get",
					ID:      id,
					Mode:    modeRead,
					Method:  "POST",
					URL:     client.PlannedDeferredRecognitionURL(),
					Flags:   localFlagMap(cmd),
					Note:    "captured GET_SCHEDULE POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayDeferredScheduleGet(ctx, sourceID, prepaid)
			if err != nil {
				return feedErr(flags, err)
			}
			res.Entity = entity
			printAccountingQueryResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&sourceID, "source-id", "", "source transaction id carrying the schedule")
	applyCatalogHelp(cmd, id)
	return cmd
}

func newAccountingDeferredScheduleCreateCmd(flags *rootFlags, prepaid bool) *cobra.Command {
	var (
		data   string
		yes    bool
		draft  bool
		source string
	)
	label := "deferredrevenue"
	id := "QBO.ACCOUNTING.DEFERRED_RECOGNITION_CREATE"
	if prepaid {
		label = "prepaid"
		id = "QBO.ACCOUNTING.PREPAID_CREATE"
	}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Write a recognition schedule (deferredrecognition UpdateDeferredRecognitionScheduleOp)",
		Long: "Writes a deferred/prepaid recognition schedule via the captured\n" +
			"UpdateDeferredRecognitionScheduleOp mutation. The input schema was not\n" +
			"fully captured, so --data JSON is passed through verbatim — nothing is\n" +
			"invented. Use --draft to preview with GET_SCHEDULE_PREVIEW instead of\n" +
			"writing. Writes require --yes.",
		RunE: func(cmd *cobra.Command, args []string) error {
			in, perr := client.ParseScheduleInput(data)
			if perr != nil {
				return exitInput(perr)
			}
			if flags.dryRun {
				body, _ := json.Marshal(in)
				return writePlan(cmd, flags, planEnvelope{
					Command: "accounting " + label + " create",
					ID:      id,
					Mode:    modeWired,
					Method:  "POST",
					URL:     client.PlannedDeferredRecognitionURL(),
					Body:    body,
					Flags:   localFlagMap(cmd),
					Note:    "captured schedule mutation; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			if draft {
				res, err := client.ReplayDeferredSchedulePreview(ctx, in, prepaid)
				if err != nil {
					return feedErr(flags, err)
				}
				printAccountingQueryResult(cmd, flags, res)
				return nil
			}
			if !yes {
				return &ExitError{Code: ExitInputError, Err: fmt.Errorf(
					"%s create writes a schedule: pass --yes to execute or --dry-run/--draft to preview", label)}
			}
			res, err := client.ReplayDeferredScheduleCreate(ctx, in, prepaid)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&data, "data", "", "mutation input JSON object (passed through verbatim)")
	cmd.Flags().BoolVar(&draft, "draft", false, "preview via GET_SCHEDULE_PREVIEW instead of writing")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm the write")
	cmd.Flags().StringVar(&source, "source-id", "", "optional source txn id recorded in the plan")
	_ = cmd.MarkFlagRequired("data")
	return cmd
}

func newAccountingOpeningBalanceCmd(flags *rootFlags) *cobra.Command {
	var (
		date  string
		lines string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Book the trial balance as an opening-balance journal (v3)",
		Long: "Books one journal entry whose lines are the trial balance.\n" +
			"--lines must decode to [{\"account\":\"ID\",\"debit\":N}|{\"credit\":N}];\n" +
			"total debits MUST equal total credits (validated locally before any dial).",
		RunE: func(cmd *cobra.Command, args []string) error {
			tbl, perr := client.ParseOpeningBalanceLines(lines)
			if perr != nil {
				return exitInput(perr)
			}
			if flags.dryRun {
				body, _ := json.Marshal(map[string]any{"date": date, "lines": tbl})
				return writePlan(cmd, flags, planEnvelope{
					Command: "accounting opening-balance create",
					ID:      "QBO.ACCOUNTING.OPENING_BALANCE_CREATE",
					Mode:    modeWired,
					Method:  "POST",
					URL:     client.PlannedMutateURL("JournalEntry", "create"),
					Body:    body,
					Flags:   localFlagMap(cmd),
					Note:    "v3 journalentry create (trial balance); not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayOpeningBalanceCreate(ctx, date, tbl)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "opening balance date (dd/MM/yyyy or yyyy-MM-dd)")
	cmd.Flags().StringVar(&lines, "lines", "", `JSON array of {"account","debit"|"credit"} lines`)
	_ = cmd.MarkFlagRequired("lines")
	_ = cmd.MarkFlagRequired("date")
	applyCatalogHelp(cmd, "QBO.ACCOUNTING.OPENING_BALANCE_CREATE")
	return cmd
}

// newAccountingDeepCmds returns the deep-CRUD commands keyed by the entity
// group they extend. newAccountingCmd merges them into the existing tree.
func newAccountingDeepCmds(flags *rootFlags) map[string][]*cobra.Command {
	return map[string][]*cobra.Command{
		"budget": {newAccountingBudgetListCmd(flags)},
		"journal": {
			newAccountingJournalBulkCreateCmd(flags),
			newAccountingJournalBulkDeleteCmd(flags),
		},
		"class": {
			newAccountingDimensionMutateCmd(flags, "Class", "update", "QBO.ACCOUNTING.CLASS_EDIT"),
			newAccountingDimensionMutateCmd(flags, "Class", "delete", "QBO.ACCOUNTING.CLASS_DELETE"),
		},
		"location": {
			newAccountingDimensionMutateCmd(flags, "Department", "update", "QBO.ACCOUNTING.LOCATION_EDIT"),
			newAccountingDimensionMutateCmd(flags, "Department", "delete", "QBO.ACCOUNTING.LOCATION_DELETE"),
		},
		"deferredrevenue": {
			newAccountingDeferredScheduleGetCmd(flags, false),
			newAccountingDeferredScheduleCreateCmd(flags, false),
		},
		"prepaid": {
			newAccountingDeferredScheduleGetCmd(flags, true),
			newAccountingDeferredScheduleCreateCmd(flags, true),
		},
		"opening-balance": {newAccountingOpeningBalanceCmd(flags)},
	}
}
