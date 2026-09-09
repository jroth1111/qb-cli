package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

// customobjects.go wires two top-level domains onto client/customobjects.go:
//
//	qb customobjects list|get|create|update|delete   (appFoundations definitions;
//	    list/get wired to the captured GetCustomObjectDefinitions query,
//	    create/update/delete stay not-wired stubs — no captured definition API)
//	qb costgroups list|create|update|delete          (cost-group-svc graphql;
//	    all four verbs fully captured from cost-groups-ui)
//
// Both services are GraphQL POSTs scoped by session cookie (no realm URL
// segment), so dry-run plans carry the service URL and never a company id.
// Writes honour the production-realm guard inside the client.

type coFlags struct {
	id          string
	name        string
	description string
	typ         string
	version     int
	status      string
	limit       int
	activate    bool
}

func (f *coFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.id, "id", "", "record id")
	cmd.Flags().StringVar(&f.name, "name", "", "name")
	cmd.Flags().StringVar(&f.description, "description", "", "description")
	cmd.Flags().StringVar(&f.typ, "type", "", "cost group type (default ITEM)")
	cmd.Flags().IntVar(&f.version, "version", 0, "optimistic-concurrency version of the row being changed")
	cmd.Flags().StringVar(&f.status, "status", "active", "list filter: active|inactive|all")
	cmd.Flags().IntVar(&f.limit, "limit", 25, "max rows (1-100)")
	cmd.Flags().BoolVar(&f.activate, "activate", false, "with costgroups delete: re-activate instead of deleting")
}

// newCustomObjectsCmd builds the `qb customobjects` domain group.
func newCustomObjectsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "customobjects",
		Short: "Custom object definitions",
		Long: "Custom object definitions (/app/customobjects, cos-service appFoundations).\n\n" +
			"qb customobjects list\n" +
			"qb customobjects get --id <id>\n" +
			"qb customobjects create|update|delete   (not wired: no captured definition API)",
	}
	cmd.AddCommand(newCustomObjectsListCmd(flags))
	cmd.AddCommand(newCustomObjectsGetCmd(flags))
	for _, verb := range []struct{ use, short string }{
		{"create", "create a definition (not wired)"},
		{"update", "edit a definition (not wired)"},
		{"delete", "delete a definition (not wired)"},
	} {
		v := verb
		cmd.AddCommand(newNotWiredMutationCmd(flags, "customobjects", v.use, v.short))
	}
	return cmd
}

// newCostGroupsCmd builds the `qb costgroups` domain group.
func newCostGroupsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "costgroups",
		Short: "Cost groups",
		Long: "Cost groups (/app/cost-groups, cost-group-svc.api.intuit.com graphql).\n\n" +
			"qb costgroups list [--status active|inactive|all] [--limit N]\n" +
			"qb costgroups create --name <name> [--description d] [--type ITEM]\n" +
			"qb costgroups update --id <id> --name <name> [--description d] --version <v>\n" +
			"qb costgroups delete --id <id> --version <v> [--activate]",
	}
	cmd.AddCommand(newCostGroupsListCmd(flags))
	cmd.AddCommand(newCostGroupsCreateCmd(flags))
	cmd.AddCommand(newCostGroupsUpdateCmd(flags))
	cmd.AddCommand(newCostGroupsDeleteCmd(flags))
	return cmd
}

// --- custom objects ----------------------------------------------------------

func newCustomObjectsListCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List custom object definitions (appFoundationsCustomObjectDefinitions query)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunPOST(flags, cmd, "customobjects object list", "QBO.CUSTOMOBJECTS.OBJECT_LIST", client.PlanCustomObjectList()); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCustomObjectList(ctx)
			if err != nil {
				return feedErr(flags, err)
			}
			return printCustomObjects(cmd.OutOrStdout(), flags, res)
		},
	}
	return cmd
}

func newCustomObjectsGetCmd(flags *rootFlags) *cobra.Command {
	ff := &coFlags{}
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one custom object definition by id",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := client.PlanCustomObjectGet(ff.id)
			if err != nil {
				return feedErr(flags, err)
			}
			if handled, derr := dryRunPOST(flags, cmd, "customobjects object get", "", plan); handled {
				return derr
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			def, err := client.ReplayCustomObjectGet(ctx, ff.id)
			if err != nil {
				return feedErr(flags, err)
			}
			return printCustomObjectOne(cmd.OutOrStdout(), flags, def)
		},
	}
	cmd.Flags().StringVar(&ff.id, "id", "", "definition id")
	return cmd
}

// --- cost groups -------------------------------------------------------------

func newCostGroupsListCmd(flags *rootFlags) *cobra.Command {
	ff := &coFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List cost groups (GetCostGroupsWithFilter query)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if handled, err := dryRunPOST(flags, cmd, "costgroups group list", "QBO.COSTGROUPS.GROUP_LIST", client.PlanCostGroupList(ff.limit, ff.status, ff.typ)); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCostGroupList(ctx, ff.limit, ff.status, ff.typ)
			if err != nil {
				return feedErr(flags, err)
			}
			return printCostGroups(cmd.OutOrStdout(), flags, res)
		},
	}
	cmd.Flags().StringVar(&ff.status, "status", "active", "active|inactive|all")
	cmd.Flags().IntVar(&ff.limit, "limit", 25, "max rows (1-100)")
	cmd.Flags().StringVar(&ff.typ, "type", "", "cost group type (default ITEM)")
	return cmd
}

func newCostGroupsCreateCmd(flags *rootFlags) *cobra.Command {
	ff := &coFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a cost group (CreateCostGroup mutation)",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := client.PlanCostGroupCreate(ff.name, ff.description, ff.typ)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planFromClient(cmd, "costgroups group create", "QBO.COSTGROUPS.GROUP_CREATE", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCostGroupCreate(ctx, ff.name, ff.description, ff.typ)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutated(cmd.OutOrStdout(), flags, res, "created cost group")
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.name, "name", "", "cost group name")
	cmd.Flags().StringVar(&ff.description, "description", "", "description")
	cmd.Flags().StringVar(&ff.typ, "type", "", "cost group type (default ITEM)")
	return cmd
}

func newCostGroupsUpdateCmd(flags *rootFlags) *cobra.Command {
	ff := &coFlags{}
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a cost group (UpdateCostGroup mutation)",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := client.PlanCostGroupUpdate(ff.id, ff.name, ff.description, ff.version)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planFromClient(cmd, "costgroups group update", "QBO.COSTGROUPS.GROUP_UPDATE", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCostGroupUpdate(ctx, ff.id, ff.name, ff.description, ff.version)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutated(cmd.OutOrStdout(), flags, res, "updated cost group")
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.id, "id", "", "cost group id")
	cmd.Flags().StringVar(&ff.name, "name", "", "new name")
	cmd.Flags().StringVar(&ff.description, "description", "", "new description")
	cmd.Flags().IntVar(&ff.version, "version", 0, "optimistic-concurrency version of the row being changed")
	return cmd
}

func newCostGroupsDeleteCmd(flags *rootFlags) *cobra.Command {
	ff := &coFlags{}
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete (soft-toggle) a cost group; pass --activate to undo",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := client.PlanCostGroupDelete(ff.id, ff.version, ff.activate)
			if err != nil {
				return feedErr(flags, err)
			}
			command := "costgroups group delete"
			if ff.activate {
				command = "costgroups group activate"
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planFromClient(cmd, command, "QBO.COSTGROUPS.GROUP_DELETE", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayCostGroupDelete(ctx, ff.id, ff.version, ff.activate)
			if err != nil {
				return feedErr(flags, err)
			}
			verb := "deleted"
			if ff.activate {
				verb = "re-activated"
			}
			printMutated(cmd.OutOrStdout(), flags, res, verb+" cost group")
			return nil
		},
	}
	cmd.Flags().StringVar(&ff.id, "id", "", "cost group id")
	cmd.Flags().IntVar(&ff.version, "version", 0, "optimistic-concurrency version of the row being changed")
	cmd.Flags().BoolVar(&ff.activate, "activate", false, "re-activate (toggle deleted=false) instead of deleting")
	return cmd
}

// newNotWiredMutationCmd is the shared not-wired stub for verbs whose API was
// never captured. It requires a session first so missing credentials still
// fail as an auth exit, then surfaces ErrMutationNotWired.
func newNotWiredMutationCmd(flags *rootFlags, domain, use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, uncapturedPlan(cmd, domain+" "+use, primitiveEntry{}))
			}
			if err := client.RequireSession(); err != nil {
				return feedErr(flags, err)
			}
			return feedErr(flags, client.ErrMutationNotWired)
		},
	}
}

// dryRunPOST emits the secret-free plan for a read that is a POST under the
// hood (both domains are GraphQL).
func dryRunPOST(flags *rootFlags, cmd *cobra.Command, command, id string, p *client.RequestPlan) (bool, error) {
	if !flags.dryRun {
		return false, nil
	}
	mode := modeRead
	return true, writePlan(cmd, flags, planFromClient(cmd, command, id, mode, p))
}

func printMutated(stdout io.Writer, flags *rootFlags, res *client.MutateResult, prefix string) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return
	}
	fmt.Fprintf(stdout, "%s %s (%s)\n", prefix, res.Item.ID, res.Item.Name)
}

// --- output ------------------------------------------------------------------

func printCustomObjects(stdout io.Writer, flags *rootFlags, res *client.CustomObjectDefinitionResult) error {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if res.Count == 0 {
		fmt.Fprintln(stdout, "no custom object definitions")
		return nil
	}
	fmt.Fprintf(stdout, "%d definition(s)\n", res.Count)
	for _, d := range res.Definitions {
		state := "inactive"
		if d.Active {
			state = "active"
		}
		applies := make([]string, 0, len(d.AppliesTo))
		for _, a := range d.AppliesTo {
			applies = append(applies, a.Entity)
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", d.ID, truncate(d.Label, 40), state, strings.Join(applies, ","))
	}
	return nil
}

func printCustomObjectOne(stdout io.Writer, flags *rootFlags, d *client.CustomObjectDefinition) error {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(d)
	}
	state := "inactive"
	if d.Active {
		state = "active"
	}
	fmt.Fprintf(stdout, "id %s\nlabel %s\nstate %s\nappliesTo %d\n", d.ID, d.Label, state, len(d.AppliesTo))
	for _, a := range d.AppliesTo {
		fmt.Fprintf(stdout, "  entity=%s scope=%s attribute=%s active=%v\n", a.Entity, a.Scope, a.Attribute, a.Active)
	}
	return nil
}

func printCostGroups(stdout io.Writer, flags *rootFlags, res *client.CostGroupListResult) error {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if res.Count == 0 {
		fmt.Fprintln(stdout, "no cost groups")
		return nil
	}
	fmt.Fprintf(stdout, "%d cost group(s)\n", res.Count)
	for _, g := range res.Groups {
		state := "active"
		if g.Deleted {
			state = "inactive"
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\tv%s\n", g.ID, truncate(g.Name, 40), truncate(g.Description, 30), state, strconv.Itoa(g.Version))
	}
	return nil
}
