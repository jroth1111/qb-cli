package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

type registerFlags struct {
	accountID string
	limit     int
}

func newAccountingRegisterCmd(flags *rootFlags) *cobra.Command {
	ff := &registerFlags{}
	cmd := &cobra.Command{
		Use:   "get",
		Short: "List posted transactions for an account (register ground truth)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunGET(flags, cmd, "accounting register get", "QBO.ACCOUNTING.REGISTER_READ", client.PlannedRegisterURL(ff.accountID), "captured GET; not sent"); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayRegister(ctx, ff.accountID, ff.limit)
			if err != nil {
				return feedErr(flags, err)
			}
			printRegister(cmd.OutOrStdout(), cmd.ErrOrStderr(), flags, res, ff.accountID)
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.accountID, "account-id", client.DefaultAccountID, "account id")
	cmd.Flags().IntVar(&ff.limit, "limit", 20, "max transactions to fetch")
	applyCatalogHelp(cmd, "QBO.ACCOUNTING.REGISTER_READ")
	return cmd

}

func printRegister(stdout, stderr io.Writer, flags *rootFlags, res *client.RegisterResult, accountID string) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	if res == nil || len(res.Transactions) == 0 {
		fmt.Fprintf(stdout, "No register transactions for account %s.\n", accountID)
		return
	}
	total := res.Counts["total"]
	if total > 0 {
		fmt.Fprintf(stdout, "Register transactions for account %s (%d shown, %d total):\n",
			accountID, len(res.Transactions), total)
	} else {
		fmt.Fprintf(stdout, "Register transactions for account %s (%d):\n",
			accountID, len(res.Transactions))
	}
	for _, t := range res.Transactions {
		date := t.Date
		if len(date) >= 10 {
			date = date[:10]
		}
		fmt.Fprintf(stdout, "  %-14s %12.2f  %s\n", date, t.Amount, truncate(t.Description, 50))
	}
}

func newAccountingAuditLogReadCmd(flags *rootFlags) *cobra.Command {
	var entityID string
	var includeSnapshots bool
	var fromDate, toDate string
	var offset int
	var (
		id    string
		limit int
	)
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Read audit log events (POST audit.api.intuit.com/v1/audit/logs)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "accounting audit-log get",
					ID:      "QBO.ACCOUNTING.AUDIT_LOG_READ",
					Mode:    modeRead,
					Method:  "POST",
					URL:     client.PlannedAuditURL(),
					Flags:   localFlagMap(cmd),
					Note:    "captured POST; not sent",
				})
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayAuditLogFiltered(ctx, client.AuditFilter{Limit: limit, FromDate: fromDate, ToDate: toDate, Offset: offset, EntityID: entityID, IncludeSnapshots: includeSnapshots})
			if err != nil {
				return feedErr(flags, err)
			}
			if id != "" {
				filtered := res.Events[:0]
				for _, e := range res.Events {
					if e.ID == id {
						filtered = append(filtered, e)
					}
				}
				res.Events = filtered
				res.Counts = map[string]int{"events": len(filtered)}
			}
			printAudit(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "target audit event id")
	cmd.Flags().StringVar(&entityID, "entity-id", "", "server-side accounting entity ID filter (not a bank-feed ID)")
	cmd.Flags().BoolVar(&includeSnapshots, "include-snapshots", false, "include historical transaction snapshots in JSON output")
	cmd.Flags().IntVar(&limit, "limit", 20, "max events to return")
	cmd.Flags().StringVar(&fromDate, "from", "", "inclusive start timestamp (RFC3339, with timezone; requires --to)")
	cmd.Flags().StringVar(&toDate, "to", "", "inclusive end timestamp (RFC3339, with timezone; requires --from)")
	cmd.Flags().IntVar(&offset, "offset", 0, "server event offset; use pageInfo for subsequent pages")
	applyCatalogHelp(cmd, "QBO.ACCOUNTING.AUDIT_LOG_READ")
	return cmd
}

func newAccountingAuditLogSearchCmd(flags *rootFlags) *cobra.Command {
	var entityID string
	var includeSnapshots bool
	var fromDate, toDate string
	var offset int
	var (
		eventType string
		user      string
		query     string
		limit     int
	)
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search audit log events (POST audit.api.intuit.com/v1/audit/logs)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "accounting audit-log search",
					ID:      "QBO.ACCOUNTING.AUDIT_LOG_SEARCH",
					Mode:    modeRead,
					Method:  "POST",
					URL:     client.PlannedAuditURL(),
					Flags:   localFlagMap(cmd),
					Note:    "captured POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayAuditLogFiltered(ctx, client.AuditFilter{
				EntityID: entityID, IncludeSnapshots: includeSnapshots,
				Limit:    limit,
				FromDate: fromDate, ToDate: toDate, Offset: offset,
				EventType: eventType,
				User:      user,
				Query:     query,
			})
			if err != nil {
				return feedErr(flags, err)
			}
			printAudit(cmd.OutOrStdout(), flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&eventType, "event-type", "", "filter by event type (LOGIN, VOID, CREATED)")
	cmd.Flags().StringVar(&entityID, "entity-id", "", "server-side accounting entity ID filter (not a bank-feed ID)")
	cmd.Flags().BoolVar(&includeSnapshots, "include-snapshots", false, "include historical transaction snapshots in JSON output")
	cmd.Flags().StringVar(&user, "user", "", "filter by user id")
	cmd.Flags().StringVar(&query, "query", "", "substring match on type/name/user")
	cmd.Flags().IntVar(&limit, "limit", 20, "max events to return")
	cmd.Flags().StringVar(&fromDate, "from", "", "inclusive start timestamp (RFC3339, with timezone; requires --to)")
	cmd.Flags().StringVar(&toDate, "to", "", "inclusive end timestamp (RFC3339, with timezone; requires --from)")
	cmd.Flags().IntVar(&offset, "offset", 0, "server event offset; use pageInfo for subsequent pages")
	applyCatalogHelp(cmd, "QBO.ACCOUNTING.AUDIT_LOG_SEARCH")
	return cmd
}

func printAudit(stdout io.Writer, flags *rootFlags, res *client.AuditResult) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	if res == nil || len(res.Events) == 0 {
		if res != nil && res.Counts["bytes"] > 0 {
			fmt.Fprintf(stdout, "audit surface %d bytes (no event list in payload)\n", res.Counts["bytes"])
			return
		}
		fmt.Fprintln(stdout, "no audit events")
		return
	}
	fmt.Fprintf(stdout, "%d event(s)\n", len(res.Events))
	for _, e := range res.Events {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", e.Date, e.User, e.EventType, e.Name)
	}
}

func newAccountingCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "accounting", Short: "Register and books", Long: "qb accounting <entity> <verb>."}
	audit_logEnt := &cobra.Command{Use: "audit-log", Short: "audit-log"}
	audit_logEnt.AddCommand(newAccountingAuditLogReadCmd(flags))

	audit_logEnt.AddCommand(newAccountingAuditLogSearchCmd(flags))

	cmd.AddCommand(audit_logEnt)
	budgetEnt := &cobra.Command{Use: "budget", Short: "budget"}
	deep := newAccountingDeepCmds(flags)
	budgetEnt.AddCommand(newAccountingBudgetListCmd(flags))
	budgetEnt.AddCommand(newStubCmd(flags, "accounting budget", "create", "budget create (not wired)"))
	budgetEnt.AddCommand(newStubCmd(flags, "accounting budget", "search", "budget search"))
	budgetEnt.AddCommand(newStubCmd(flags, "accounting budget", "delete", "budget delete (v3 delete)"))
	budgetEnt.AddCommand(newStubCmd(flags, "accounting budget", "update", "budget edit (not wired)"))
	budgetEnt.AddCommand(newStubCmd(flags, "accounting budget", "get", "budget read (not wired)"))
	cmd.AddCommand(budgetEnt)
	cdcEnt := &cobra.Command{Use: "cdc", Short: "cdc"}
	cdcEnt.AddCommand(newAccountingCDCCmd(flags))
	cmd.AddCommand(cdcEnt)
	classEnt := &cobra.Command{Use: "class", Short: "class"}
	classEnt.AddCommand(newStubCmd(flags, "accounting class", "create", "class create (not wired)"))
	classEnt.AddCommand(deep["class"]...)
	classEnt.AddCommand(newStubCmd(flags, "accounting class", "get", "class read (not wired)"))
	classEnt.AddCommand(newStubCmd(flags, "accounting class", "search", "class search"))
	cmd.AddCommand(classEnt)
	coaEnt := &cobra.Command{Use: "coa", Short: "coa"}
	coaEnt.AddCommand(newStubCmd(flags, "accounting coa", "create", "coa create"))
	coaEnt.AddCommand(newStubCmd(flags, "accounting coa", "delete", "coa deactivate"))
	coaEnt.AddCommand(newStubCmd(flags, "accounting coa", "get", "coa read (not wired)"))
	coaEnt.AddCommand(newStubCmd(flags, "accounting coa", "search", "coa search (not wired)"))
	cmd.AddCommand(coaEnt)
	departmentEnt := &cobra.Command{Use: "department", Short: "department"}
	departmentEnt.AddCommand(newStubCmd(flags, "accounting department", "create", "department create (not wired)"))
	departmentEnt.AddCommand(newStubCmd(flags, "accounting department", "delete", "department delete (not wired)"))
	departmentEnt.AddCommand(newStubCmd(flags, "accounting department", "update", "department edit (not wired)"))
	departmentEnt.AddCommand(newStubCmd(flags, "accounting department", "get", "department read (not wired)"))
	departmentEnt.AddCommand(newStubCmd(flags, "accounting department", "search", "department search (not wired)"))
	cmd.AddCommand(departmentEnt)

	depositEnt := &cobra.Command{Use: "deposit", Short: "deposit"}
	depositEnt.AddCommand(newStubCmd(flags, "accounting deposit", "create", "deposit create (not wired)"))
	depositEnt.AddCommand(newStubCmd(flags, "accounting deposit", "delete", "deposit delete (not wired)"))
	depositEnt.AddCommand(newStubCmd(flags, "accounting deposit", "update", "deposit edit (not wired)"))
	depositEnt.AddCommand(newStubCmd(flags, "accounting deposit", "get", "deposit read (not wired)"))
	cmd.AddCommand(depositEnt)
	fixed_assetEnt := &cobra.Command{Use: "fixed-asset", Short: "fixed-asset"}
	fixed_assetEnt.AddCommand(newStubCmd(flags, "accounting fixed-asset", "create", "fixed-asset create (not wired)"))
	fixed_assetEnt.AddCommand(newStubCmd(flags, "accounting fixed-asset", "delete", "fixed-asset delete (not wired)"))
	fixed_assetEnt.AddCommand(newStubCmd(flags, "accounting fixed-asset", "update", "fixed-asset edit (not wired)"))
	fixed_assetEnt.AddCommand(newStubCmd(flags, "accounting fixed-asset", "get", "fixed-asset read (not wired)"))
	fixed_assetEnt.AddCommand(newStubCmd(flags, "accounting fixed-asset", "search", "fixed-asset search (not wired)"))
	cmd.AddCommand(fixed_assetEnt)
	integration_txnEnt := &cobra.Command{Use: "integration-txn", Short: "integration-txn"}
	integration_txnEnt.AddCommand(newStubCmd(flags, "accounting integration-txn", "get", "integration-txn read (not wired)"))
	cmd.AddCommand(integration_txnEnt)
	journalEnt := &cobra.Command{Use: "journal", Short: "journal"}
	journalEnt.AddCommand(newAccountingJournalBulkCreateCmd(flags))
	journalEnt.AddCommand(newAccountingJournalBulkDeleteCmd(flags))
	journalEnt.AddCommand(newStubCmd(flags, "accounting journal", "update", "journal edit (not wired)"))
	journalEnt.AddCommand(newStubCmd(flags, "accounting journal", "get", "journal read (not wired)"))
	cmd.AddCommand(journalEnt)
	locationEnt := &cobra.Command{Use: "location", Short: "location"}
	locationEnt.AddCommand(newStubCmd(flags, "accounting location", "create", "location create (not wired)"))
	locationEnt.AddCommand(deep["location"]...)
	locationEnt.AddCommand(newStubCmd(flags, "accounting location", "get", "location read (not wired)"))
	cmd.AddCommand(locationEnt)
	my_accountantEnt := &cobra.Command{Use: "my-accountant", Short: "my-accountant"}
	my_accountantEnt.AddCommand(newStubCmd(flags, "accounting my-accountant", "get", "my-accountant read (not wired)"))
	cmd.AddCommand(my_accountantEnt)
	opening_balanceEnt := &cobra.Command{Use: "opening-balance", Short: "opening-balance"}
	opening_balanceEnt.AddCommand(newStubCmd(flags, "accounting opening-balance", "update", "opening-balance set (not wired)"))
	opening_balanceEnt.AddCommand(deep["opening-balance"]...)
	cmd.AddCommand(opening_balanceEnt)
	prepaidEnt := &cobra.Command{Use: "prepaid", Short: "prepaid"}
	prepaidEnt.AddCommand(deep["prepaid"]...)
	cmd.AddCommand(prepaidEnt)
	projectEnt := &cobra.Command{Use: "project", Short: "project"}
	projectEnt.AddCommand(newStubCmd(flags, "accounting project", "create", "project create"))
	projectEnt.AddCommand(newStubCmd(flags, "accounting project", "update", "project edit"))
	projectEnt.AddCommand(newStubCmd(flags, "accounting project", "get", "project read"))
	projectEnt.AddCommand(newStubCmd(flags, "accounting project", "search", "project search"))
	cmd.AddCommand(projectEnt)
	reconcileEnt := &cobra.Command{Use: "reconcile", Short: "reconcile"}
	reconcileEnt.AddCommand(newStubCmd(flags, "accounting reconcile", "create", "begin reconcile session (qbonline-aws StartReconcile)"))
	reconcileEnt.AddCommand(newStubCmd(flags, "accounting reconcile", "get", "Integration_Reconciliation node read"))
	cmd.AddCommand(reconcileEnt)
	recurringEnt := &cobra.Command{Use: "recurring", Short: "recurring"}
	recurringEnt.AddCommand(newStubCmd(flags, "accounting recurring", "create", "recurring create (not wired)"))
	recurringEnt.AddCommand(newStubCmd(flags, "accounting recurring", "delete", "recurring delete (not wired)"))
	recurringEnt.AddCommand(newStubCmd(flags, "accounting recurring", "update", "recurring edit (not wired)"))
	recurringEnt.AddCommand(newStubCmd(flags, "accounting recurring", "get", "recurring read (not wired)"))
	recurringEnt.AddCommand(newStubCmd(flags, "accounting recurring", "search", "recurring search (not wired)"))
	cmd.AddCommand(recurringEnt)
	registerEnt := &cobra.Command{Use: "register", Short: "register"}
	registerEnt.AddCommand(newAccountingRegisterCmd(flags))
	cmd.AddCommand(registerEnt)
	revenue_recognitionEnt := &cobra.Command{Use: "revenue-recognition", Short: "revenue-recognition"}
	revenue_recognitionEnt.AddCommand(newStubCmd(flags, "accounting revenue-recognition", "get", "revenue-recognition read (not wired)"))
	cmd.AddCommand(revenue_recognitionEnt)
	transferEnt := &cobra.Command{Use: "transfer", Short: "transfer"}
	transferEnt.AddCommand(newStubCmd(flags, "accounting transfer", "create", "transfer create (not wired)"))
	transferEnt.AddCommand(newStubCmd(flags, "accounting transfer", "delete", "transfer delete (not wired)"))
	transferEnt.AddCommand(newStubCmd(flags, "accounting transfer", "update", "transfer edit (not wired)"))
	transferEnt.AddCommand(newStubCmd(flags, "accounting transfer", "get", "transfer read (not wired)"))
	cmd.AddCommand(transferEnt)
	txnEnt := &cobra.Command{Use: "txn", Short: "txn"}
	txnEnt.AddCommand(newStubCmd(flags, "accounting txn", "update", "txn void (not wired)"))
	cmd.AddCommand(txnEnt)
	// Service map (2026-08-24): never-triggered production services, blocked.
	reconcile_serviceEnt := &cobra.Command{Use: "reconcile-service", Short: "reconcile-service"}
	reconcile_serviceEnt.AddCommand(newStubCmd(flags, "accounting reconcile-service", "get", "accountreconcile service read (service map; not wired)"))
	cmd.AddCommand(reconcile_serviceEnt)
	budget_planningEnt := &cobra.Command{Use: "budget-planning", Short: "budget-planning"}
	budget_planningEnt.AddCommand(newStubCmd(flags, "accounting budget-planning", "get", "budgeting fetchAllBudgets read"))
	cmd.AddCommand(budget_planningEnt)
	coa_templatesEnt := &cobra.Command{Use: "coa-templates", Short: "coa-templates"}
	coa_templatesEnt.AddCommand(newStubCmd(flags, "accounting coa-templates", "get", "coa-core templates read (service map; not wired)"))
	cmd.AddCommand(coa_templatesEnt)
	deferred_recognitionEnt := &cobra.Command{Use: "deferred-recognition", Short: "deferred-recognition"}
	deferred_recognitionEnt.AddCommand(newStubCmd(flags, "accounting deferred-recognition", "get", "deferredrecognition schedules read (service map; not wired)"))
	cmd.AddCommand(deferred_recognitionEnt)
	deferredrevenueEnt := &cobra.Command{Use: "deferredrevenue", Short: "deferredrevenue schedules (deferredrecognition.api)"}
	deferredrevenueEnt.AddCommand(deep["deferredrevenue"]...)
	cmd.AddCommand(deferredrevenueEnt)
	// Service map v2 (2026-08-24): never-triggered production services, blocked.
	fit_transactionsEnt := &cobra.Command{Use: "fit-transactions", Short: "fit-transactions"}
	fit_transactionsEnt.AddCommand(newStubCmd(flags, "accounting fit-transactions", "get", "fitransactions.api.intuit.com/graphql get (service map; not wired)"))
	cmd.AddCommand(fit_transactionsEnt)
	midmarket_settingsEnt := &cobra.Command{Use: "midmarket-settings", Short: "midmarket-settings"}
	midmarket_settingsEnt.AddCommand(newStubCmd(flags, "accounting midmarket-settings", "get", "mid-market-settings-svc.api.intuit.com/graphql get (service map; not wired)"))
	cmd.AddCommand(midmarket_settingsEnt)
	multientityEnt := &cobra.Command{Use: "multientity", Short: "multientity"}
	multientityEnt.AddCommand(newStubCmd(flags, "accounting multientity", "get", "multientityorchs.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(multientityEnt)
	libroEnt := &cobra.Command{Use: "libro", Short: "libro"}
	libroEnt.AddCommand(newStubCmd(flags, "accounting libro", "get", "libro-svc.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(libroEnt)
	taxconfig_mirrorEnt := &cobra.Command{Use: "taxconfig-mirror", Short: "taxconfig-mirror"}
	taxconfig_mirrorEnt.AddCommand(newStubCmd(flags, "accounting taxconfig-mirror", "get", "taxconfig IndirectTaxTaxGropups read"))
	cmd.AddCommand(taxconfig_mirrorEnt)
	qbdata_migrationEnt := &cobra.Command{Use: "qbdata-migration", Short: "qbdata-migration"}
	qbdata_migrationEnt.AddCommand(newStubCmd(flags, "accounting qbdata-migration", "post", "qbdatamigration.api.intuit.com/v1 post (service map; not wired)"))
	cmd.AddCommand(qbdata_migrationEnt)
	return cmd
}
