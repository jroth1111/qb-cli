package cli

import "github.com/spf13/cobra"

func newAdvancedCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "advanced",
		Short: "Advanced-only tools",
		Long:  "Advanced-only tools. qb advanced <entity> <verb>. Stubs return not-wired.",
	}
	backupEnt := &cobra.Command{Use: "backup", Short: "backup"}
	backupEnt.AddCommand(newStubCmd(flags, "advanced backup", "create", "backup create (not wired)"))

	cmd.AddCommand(backupEnt)
	custom_rolesEnt := &cobra.Command{Use: "custom-roles", Short: "custom-roles"}
	custom_rolesEnt.AddCommand(newStubCmd(flags, "advanced custom-roles", "create", "custom-roles create (not wired)"))
	custom_rolesEnt.AddCommand(newStubCmd(flags, "advanced custom-roles", "update", "custom-roles edit (not wired)"))
	custom_rolesEnt.AddCommand(newStubCmd(flags, "advanced custom-roles", "get", "custom-roles read (not wired)"))
	cmd.AddCommand(custom_rolesEnt)
	expense_claimsEnt := &cobra.Command{Use: "expense-claims", Short: "expense-claims"}
	expense_claimsEnt.AddCommand(newStubCmd(flags, "advanced expense-claims", "create", "expense-claims create (not wired)"))
	expense_claimsEnt.AddCommand(newStubCmd(flags, "advanced expense-claims", "get", "expense-claims read (not wired)"))
	expense_claimsEnt.AddCommand(newStubCmd(flags, "advanced expense-claims", "verify", "expense-claims review (not wired)"))
	cmd.AddCommand(expense_claimsEnt)
	reclassifyEnt := &cobra.Command{Use: "reclassify", Short: "reclassify"}
	reclassifyEnt.AddCommand(newStubCmd(flags, "advanced reclassify", "run", "reclassify run (not wired)"))
	cmd.AddCommand(reclassifyEnt)
	revenue_recognitionEnt := &cobra.Command{Use: "revenue-recognition", Short: "revenue-recognition"}
	revenue_recognitionEnt.AddCommand(newStubCmd(flags, "advanced revenue-recognition", "update", "revenue-recognition edit (not wired)"))
	revenue_recognitionEnt.AddCommand(newStubCmd(flags, "advanced revenue-recognition", "run setup", "revenue-recognition setup (not wired)"))
	cmd.AddCommand(revenue_recognitionEnt)
	spreadsheet_syncEnt := &cobra.Command{Use: "spreadsheet-sync", Short: "spreadsheet-sync"}
	spreadsheet_syncEnt.AddCommand(newStubCmd(flags, "advanced spreadsheet-sync", "import", "spreadsheet-sync import (not wired)"))
	cmd.AddCommand(spreadsheet_syncEnt)
	tasksEnt := &cobra.Command{Use: "tasks", Short: "tasks"}
	tasksEnt.AddCommand(newStubCmd(flags, "advanced tasks", "create", "tasks create (not wired)"))
	tasksEnt.AddCommand(newStubCmd(flags, "advanced tasks", "delete", "tasks delete (not wired)"))
	tasksEnt.AddCommand(newStubCmd(flags, "advanced tasks", "update", "tasks edit (not wired)"))
	tasksEnt.AddCommand(newStubCmd(flags, "advanced tasks", "get", "tasks read (not wired)"))
	cmd.AddCommand(tasksEnt)
	workflowsEnt := &cobra.Command{Use: "workflows", Short: "workflows"}
	workflowsEnt.AddCommand(newStubCmd(flags, "advanced workflows", "create", "workflows create (not wired)"))
	workflowsEnt.AddCommand(newStubCmd(flags, "advanced workflows", "update", "workflows edit (not wired)"))
	workflowsEnt.AddCommand(newStubCmd(flags, "advanced workflows", "get", "workflows read (not wired)"))
	cmd.AddCommand(workflowsEnt)
	// Service map v2 (2026-08-24): never-triggered production services, blocked.
	erp_builderEnt := &cobra.Command{Use: "erp-builder", Short: "erp-builder"}
	erp_builderEnt.AddCommand(newStubCmd(flags, "advanced erp-builder", "list", "erp-builder-svc.api.intuit.com/v1 list (service map; not wired)"))
	cmd.AddCommand(erp_builderEnt)
	acsEnt := &cobra.Command{Use: "acs", Short: "acs"}
	acsEnt.AddCommand(newStubCmd(flags, "advanced acs", "get", "accountantclientservices.api.intuit.com/v4/graphql get (service map; not wired)"))
	cmd.AddCommand(acsEnt)
	return cmd
}

// newAccountantServiceMapCmd wires `qb accountant` (service map, 2026-08-24):
// accountant-workflow surfaces the CLI never triggers. Blocked stubs only —
// see .audit/qb/2026-08-24/SERVICE_MAP.md.
func newAccountantServiceMapCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accountant",
		Short: "Accountant-workflow services (service map)",
		Long:  "Accountant-workflow services documented from the capture corpus. Stubs return not-wired.",
	}
	portfolio_storeEnt := &cobra.Command{Use: "portfolio-store", Short: "portfolio-store"}
	portfolio_storeEnt.AddCommand(newStubCmd(flags, "accountant portfolio-store", "get", "client portfolio read (service map; not wired)"))
	cmd.AddCommand(portfolio_storeEnt)
	return cmd
}

// newIntegrationsServiceMapCmd wires `qb integrations` (service map,
// 2026-08-24): app-flow synchronisation the CLI never triggers. Blocked stub.
func newIntegrationsServiceMapCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "integrations",
		Short: "Third-party integrations (service map)",
		Long:  "Third-party integration services documented from the capture corpus. Stubs return not-wired.",
	}
	appflowEnt := &cobra.Command{Use: "appflow", Short: "appflow"}
	appflowEnt.AddCommand(newStubCmd(flags, "integrations appflow", "run", "app connection sync (service map; not wired)"))
	cmd.AddCommand(appflowEnt)
	// Service map v2 (2026-08-24): never-triggered production services, blocked.
	idxEnt := &cobra.Command{Use: "idx", Short: "idx"}
	idxEnt.AddCommand(newStubCmd(flags, "integrations idx", "get", "idx.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(idxEnt)
	v4_awsEnt := &cobra.Command{Use: "v4-aws", Short: "v4-aws"}
	v4_awsEnt.AddCommand(newStubCmd(flags, "integrations v4-aws", "post", "v4-aws.api.intuit.com post (service map; not wired)"))
	cmd.AddCommand(v4_awsEnt)
	workflowautomationEnt := &cobra.Command{Use: "workflowautomation", Short: "workflowautomation"}
	workflowautomationEnt.AddCommand(newStubCmd(flags, "integrations workflowautomation", "get", "workflowautomation.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(workflowautomationEnt)
	return cmd
}
