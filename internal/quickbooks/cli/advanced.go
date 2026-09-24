package cli

import (
	"context"
	"fmt"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

func newAdvancedCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "advanced",
		Short: "Advanced-only tools",
		Long:  "Advanced-only tools. qb advanced <entity> <verb>. Stubs return not-wired.",
	}
	backupEnt := &cobra.Command{Use: "backup", Short: "backup"}
	backupEnt.AddCommand(newStubCmd(flags, "advanced backup", "create", "backup create"))

	batchEnt := &cobra.Command{Use: "batch", Short: "batch"}
	batchEnt.AddCommand(newAdvancedBatchRunCmd(flags))
	cmd.AddCommand(batchEnt)
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
	reclassifyEnt.AddCommand(newStubCmd(flags, "advanced reclassify", "run", "reclassify run"))
	cmd.AddCommand(reclassifyEnt)
	revenue_recognitionEnt := &cobra.Command{Use: "revenue-recognition", Short: "revenue-recognition"}
	revenue_recognitionEnt.AddCommand(newStubCmd(flags, "advanced revenue-recognition", "update", "revenue-recognition edit (not wired)"))
	revenue_recognitionEnt.AddCommand(newStubCmd(flags, "advanced revenue-recognition", "run setup", "revenue-recognition setup (not wired)"))
	cmd.AddCommand(revenue_recognitionEnt)
	spreadsheet_syncEnt := &cobra.Command{Use: "spreadsheet-sync", Short: "spreadsheet-sync"}
	spreadsheet_syncEnt.AddCommand(newStubCmd(flags, "advanced spreadsheet-sync", "import", "spreadsheet-sync import (not wired)"))
	cmd.AddCommand(spreadsheet_syncEnt)
	tasksEnt := &cobra.Command{Use: "tasks", Short: "tasks"}
	tasksEnt.AddCommand(newTaskCreateCmd(flags))
	tasksEnt.AddCommand(newTaskDeleteCmd(flags))
	tasksEnt.AddCommand(newTaskUpdateCmd(flags))
	tasksEnt.AddCommand(newStubCmd(flags, "advanced tasks", "get", "tasks read (not wired)"))
	cmd.AddCommand(tasksEnt)
	workflowsEnt := &cobra.Command{Use: "workflows", Short: "workflows"}
	workflowsEnt.AddCommand(newWorkflowCreateCmd(flags))
	workflowsEnt.AddCommand(newWorkflowUpdateCmd(flags))
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
	workflowautomationEnt.AddCommand(newStubCmd(flags, "integrations workflowautomation", "get", "workflowautomation WASAllRules"))
	cmd.AddCommand(workflowautomationEnt)
	return cmd
}

// newWorkflowCreateCmd implements `advanced workflows create`: POST one rule
// onto the company's activated Workflow AppConnect subscription
// (api.appconnect.intuit.com/api/v1/subscriptions/{sub}/workflows/).
func newWorkflowCreateCmd(flags *rootFlags) *cobra.Command {
	var name, desc, template string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a workflow rule (appconnect POST workflows/)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if template != "" {
				return feedErr(flags, fmt.Errorf("--template is not supported by the captured workflow-create contract; no blank rule created"))
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "advanced workflows create", ID: "QBO.ADVANCED.WORKFLOWS_CREATE",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedWorkflowsURL(),
					Flags: localFlagMap(cmd), Note: "subscription lookup + POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayWorkflowMutate(ctx, "create", "", name, desc, "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "workflow name (required)")
	cmd.Flags().StringVar(&desc, "description", "", "workflow description")
	cmd.Flags().StringVar(&template, "template", "", "unsupported template selector; explicit values are rejected, omission creates a blank rule")
	_ = cmd.MarkFlagRequired("name")
	applyCatalogHelp(cmd, "QBO.ADVANCED.WORKFLOWS_CREATE")
	return cmd
}

// newWorkflowUpdateCmd implements `advanced workflows update`: PUT a rename/
// description onto an appconnect rule, or POST ?action=activate|deactivate.
func newWorkflowUpdateCmd(flags *rootFlags) *cobra.Command {
	var id, name, desc, action string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a workflow rule (appconnect PUT workflows/{id} or POST ?action=)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "advanced workflows update", ID: "QBO.ADVANCED.WORKFLOWS_EDIT",
					Mode: modeWired, Method: "PUT",
					URL:   client.PlannedWorkflowsURL(),
					Flags: localFlagMap(cmd), Note: "subscription lookup + PUT/POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayWorkflowMutate(ctx, "update", id, name, desc, action)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "appconnect workflow id (required)")
	cmd.Flags().StringVar(&name, "name", "", "new workflow name")
	cmd.Flags().StringVar(&desc, "description", "", "new workflow description")
	cmd.Flags().StringVar(&action, "action", "", "status action: activate or deactivate")
	_ = cmd.MarkFlagRequired("id")
	applyCatalogHelp(cmd, "QBO.ADVANCED.WORKFLOWS_EDIT")
	return cmd
}

// newTaskCreateCmd implements `advanced tasks create`:
// taskManagementCreateTask on smallbusiness.api.intuit.com/graphql. The
// service requires name, status, assignee (identity profile id — resolved
// from the session when --assignee is omitted), and an RFC3339 dueDate.
func newTaskCreateCmd(flags *rootFlags) *cobra.Command {
	var name, due, assignee string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a task (taskManagementCreateTask)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "advanced tasks create", ID: "QBO.ADVANCED.TASKS_CREATE",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedTaskURL(),
					Flags: localFlagMap(cmd), Note: "identity assignee lookup + POST graphql; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayTaskMutate(ctx, "create", "", name, due, assignee)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "task name (required)")
	cmd.Flags().StringVar(&due, "due", "", "due date YYYY-MM-DD or RFC3339 (required)")
	cmd.Flags().StringVar(&assignee, "assignee", "", "assignee identity profile id (defaults to the session user)")
	_ = cmd.MarkFlagRequired("name")
	_ = cmd.MarkFlagRequired("due")
	applyCatalogHelp(cmd, "QBO.ADVANCED.TASKS_CREATE")
	return cmd
}

// newTaskUpdateCmd implements `advanced tasks update`:
// taskManagementUpdateTask — rename a task by numeric id.
func newTaskUpdateCmd(flags *rootFlags) *cobra.Command {
	var id, name string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Rename a task (taskManagementUpdateTask)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "advanced tasks update", ID: "QBO.ADVANCED.TASKS_EDIT",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedTaskURL(),
					Flags: localFlagMap(cmd), Note: "POST graphql; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayTaskMutate(ctx, "update", id, name, "", "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "numeric task id (required)")
	cmd.Flags().StringVar(&name, "name", "", "new task name (required)")
	_ = cmd.MarkFlagRequired("id")
	_ = cmd.MarkFlagRequired("name")
	applyCatalogHelp(cmd, "QBO.ADVANCED.TASKS_EDIT")
	return cmd
}

// newTaskDeleteCmd implements `advanced tasks delete`:
// taskManagementDeleteTask — hard delete by numeric id.
func newTaskDeleteCmd(flags *rootFlags) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a task (taskManagementDeleteTask)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "advanced tasks delete", ID: "QBO.ADVANCED.TASKS_DELETE",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedTaskURL(),
					Flags: localFlagMap(cmd), Note: "POST graphql; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayTaskMutate(ctx, "delete", id, "", "", "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "numeric task id (required)")
	_ = cmd.MarkFlagRequired("id")
	applyCatalogHelp(cmd, "QBO.ADVANCED.TASKS_DELETE")
	return cmd
}
