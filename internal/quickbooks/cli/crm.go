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

// crm.go wires the CRM domain: qb crm lead|opportunity|association <verb>
// against the qbo.intuit.com/api/core leads service (client/crm_leads.go).
//
// Closed verb grammar:
//	qb crm lead list|get|search|create|update|delete|convert
//	qb crm opportunity list|create
//	qb crm association list

type crmFlags struct {
	id         string
	query      string
	name       string
	org        string
	email      string
	phone      string
	status     string
	notes      string
	customerID string
	page       int
	size       int
	inactive   bool
}

// newCrmCmd builds the `qb crm` domain group.
func newCrmCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "crm",
		Short: "CRM leads and opportunities",
		Long: "CRM leads and opportunities. qb crm <entity> <verb> against qbo.intuit.com/api/core (crm-leads-ui).\n\n" +
			"qb crm lead list|get|search|create|update|delete|convert\n" +
			"qb crm opportunity list|create\n" +
			"qb crm association list",
	}
	leadEnt := &cobra.Command{Use: "lead", Short: "leads"}
	leadEnt.AddCommand(
		newCrmLeadListCmd(flags),
		newCrmLeadGetCmd(flags),
		newCrmLeadSearchCmd(flags),
		newCrmLeadCreateCmd(flags),
		newCrmLeadUpdateCmd(flags),
		newCrmLeadDeleteCmd(flags),
		newCrmLeadConvertCmd(flags),
	)
	cmd.AddCommand(leadEnt)
	opportunityEnt := &cobra.Command{Use: "opportunity", Short: "opportunities"}
	opportunityEnt.AddCommand(
		newCrmOpportunityListCmd(flags),
		newCrmOpportunityCreateCmd(flags),
	)
	cmd.AddCommand(opportunityEnt)
	associationEnt := &cobra.Command{Use: "association", Short: "lead-customer associations"}
	associationEnt.AddCommand(newCrmAssociationListCmd(flags))
	cmd.AddCommand(associationEnt)
	return cmd
}

// --- shared helpers ---------------------------------------------------------

func (f *crmFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringVar(&f.id, "id", "", "lead/opportunity id")
	cmd.Flags().StringVar(&f.query, "query", "", "search text")
	cmd.Flags().StringVar(&f.name, "name", "", "display name")
	cmd.Flags().StringVar(&f.org, "org", "", "organization name")
	cmd.Flags().StringVar(&f.email, "email", "", "email address")
	cmd.Flags().StringVar(&f.phone, "phone", "", "phone number")
	cmd.Flags().StringVar(&f.status, "status", "", "lead status (e.g. HOT)")
	cmd.Flags().StringVar(&f.notes, "notes", "", "free-text note")
	cmd.Flags().StringVar(&f.customerID, "customer-id", "", "customer id")
	cmd.Flags().IntVar(&f.page, "page", 0, "zero-based page number")
	cmd.Flags().IntVar(&f.size, "size", 25, "page size")
	cmd.Flags().BoolVar(&f.inactive, "inactive", false, "include inactive leads")
}

func (f *crmFlags) leadInput() client.CrmLeadInput {
	return client.CrmLeadInput{
		DisplayName:      strings.TrimSpace(f.name),
		OrganizationName: strings.TrimSpace(f.org),
		Email:            strings.TrimSpace(f.email),
		Phone:            strings.TrimSpace(f.phone),
		Status:           strings.TrimSpace(f.status),
		Notes:            strings.TrimSpace(f.notes),
	}
}

func newCrmLeadListCmd(flags *rootFlags) *cobra.Command {
	ff := &crmFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List CRM leads (GET /api/core/leads)",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan := client.PlanLeadList(ff.page, ff.size, ff.inactive)
			if handled, err := dryRunGET(flags, cmd, "crm lead list", "QBO.CRM.LEAD_LIST", plan.URL, plan.Note); handled {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayLeadList(ctx, ff.page, ff.size, ff.inactive)
			if err != nil {
				return feedErr(flags, err)
			}
			return printCrmLeads(cmd.OutOrStdout(), flags, res)
		},
	}
	ff.bind(cmd)
	return cmd
}

func newCrmLeadGetCmd(flags *rootFlags) *cobra.Command {
	ff := &crmFlags{}
	cmd := &cobra.Command{
		Use:   "get",
		Short: "Get one CRM lead by id (GET /api/core/leads/{id})",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := client.PlanLeadGet(ff.id)
			if err != nil {
				return feedErr(flags, err)
			}
			if handled, derr := dryRunGET(flags, cmd, "crm lead get", "QBO.CRM.LEAD_GET", plan.URL, plan.Note); handled {
				return derr
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			l, err := client.ReplayLeadGet(ctx, ff.id)
			if err != nil {
				return feedErr(flags, err)
			}
			return printCrmOne(cmd.OutOrStdout(), flags, l)
		},
	}
	ff.bind(cmd)
	return cmd
}

func newCrmLeadSearchCmd(flags *rootFlags) *cobra.Command {
	ff := &crmFlags{}
	cmd := &cobra.Command{
		Use:   "search",
		Short: "Search CRM leads (GET /api/core/leads/search?q=)",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := client.PlanLeadSearch(ff.query, ff.page, ff.size)
			if err != nil {
				return feedErr(flags, err)
			}
			if handled, derr := dryRunGET(flags, cmd, "crm lead search", "QBO.CRM.LEAD_SEARCH", plan.URL, plan.Note); handled {
				return derr
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayLeadSearch(ctx, ff.query, ff.page, ff.size)
			if err != nil {
				return feedErr(flags, err)
			}
			return printCrmLeads(cmd.OutOrStdout(), flags, res)
		},
	}
	ff.bind(cmd)
	return cmd
}

func newCrmLeadCreateCmd(flags *rootFlags) *cobra.Command {
	ff := &crmFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a CRM lead (POST /api/core/leads)",
		RunE: func(cmd *cobra.Command, args []string) error {
			in := ff.leadInput()
			plan, err := client.PlanLeadCreate(in)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planFromClient(cmd, "crm lead create", "QBO.CRM.LEAD_CREATE", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			l, err := client.ReplayLeadCreate(ctx, in)
			if err != nil {
				return feedErr(flags, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created lead %s (%s)\n", l.ID, l.DisplayName)
			return nil
		},
	}
	ff.bind(cmd)
	return cmd
}

func newCrmLeadUpdateCmd(flags *rootFlags) *cobra.Command {
	ff := &crmFlags{}
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a CRM lead (PUT /api/core/leads/{id})",
		RunE: func(cmd *cobra.Command, args []string) error {
			in := ff.leadInput()
			plan, err := client.PlanLeadUpdate(ff.id, in)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planFromClient(cmd, "crm lead update", "QBO.CRM.LEAD_UPDATE", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			l, err := client.ReplayLeadUpdate(ctx, ff.id, in)
			if err != nil {
				return feedErr(flags, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "updated lead %s (%s)\n", l.ID, l.DisplayName)
			return nil
		},
	}
	ff.bind(cmd)
	return cmd
}

func newCrmLeadDeleteCmd(flags *rootFlags) *cobra.Command {
	ff := &crmFlags{}
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a CRM lead (DELETE /api/core/leads/{id})",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := client.PlanLeadDelete(ff.id)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planFromClient(cmd, "crm lead delete", "QBO.CRM.LEAD_DELETE", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			status, err := client.ReplayLeadDelete(ctx, ff.id)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"ok": true, "status": status})
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted lead %s (status %s)\n", ff.id, strconv.Itoa(status))
			return nil
		},
	}
	ff.bind(cmd)
	return cmd
}

func newCrmLeadConvertCmd(flags *rootFlags) *cobra.Command {
	ff := &crmFlags{}
	cmd := &cobra.Command{
		Use:   "convert",
		Short: "Convert a lead to a customer (POST /api/core/leads/convert-to-customer)",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan, err := client.PlanLeadConvert(ff.id)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planFromClient(cmd, "crm lead convert", "QBO.CRM.LEAD_CONVERT", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayLeadConvert(ctx, ff.id)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "converted lead %s -> %s (status %d)\n", ff.id, res.State, res.Status)
			return nil
		},
	}
	ff.bind(cmd)
	return cmd
}

func newCrmOpportunityListCmd(flags *rootFlags) *cobra.Command {
	ff := &crmFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List opportunities (GET /api/core/opportunities)",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan := client.PlanOpportunityList(ff.page, ff.size)
			if handled, derr := dryRunGET(flags, cmd, "crm opportunity list", "QBO.CRM.OPPORTUNITY_LIST", plan.URL, plan.Note); handled {
				return derr
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayOpportunityList(ctx, ff.page, ff.size)
			if err != nil {
				return feedErr(flags, err)
			}
			return printCrmOpportunities(cmd.OutOrStdout(), flags, res)
		},
	}
	ff.bind(cmd)
	return cmd
}

func newCrmOpportunityCreateCmd(flags *rootFlags) *cobra.Command {
	ff := &crmFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an opportunity (POST /api/core/opportunities)",
		RunE: func(cmd *cobra.Command, args []string) error {
			in := client.CrmOpportunityInput{
				Name:       strings.TrimSpace(ff.name),
				CustomerID: strings.TrimSpace(ff.customerID),
			}
			plan, err := client.PlanOpportunityCreate(in)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planFromClient(cmd, "crm opportunity create", "QBO.CRM.OPPORTUNITY_CREATE", modeWired, plan))
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			o, err := client.ReplayOpportunityCreate(ctx, in)
			if err != nil {
				return feedErr(flags, err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "created opportunity %s (%s)\n", o.ID, o.Name)
			return nil
		},
	}
	ff.bind(cmd)
	return cmd
}

func newCrmAssociationListCmd(flags *rootFlags) *cobra.Command {
	ff := &crmFlags{}
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List lead-customer associations (GET /api/core/lead-customer-associations)",
		RunE: func(cmd *cobra.Command, args []string) error {
			plan := client.PlanAssociationList(ff.customerID)
			if handled, derr := dryRunGET(flags, cmd, "crm association list", "QBO.CRM.ASSOCIATION_LIST", plan.URL, plan.Note); handled {
				return derr
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayAssociationList(ctx, ff.customerID)
			if err != nil {
				return feedErr(flags, err)
			}
			return printCrmAssociations(cmd.OutOrStdout(), flags, res)
		},
	}
	ff.bind(cmd)
	return cmd
}

// --- output -----------------------------------------------------------------

func printCrmLeads(stdout io.Writer, flags *rootFlags, res *client.CrmLeadListResult) error {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if len(res.Leads) == 0 {
		fmt.Fprintln(stdout, "no leads")
		return nil
	}
	fmt.Fprintf(stdout, "%d lead(s), total %d\n", res.Count, res.Total)
	for _, l := range res.Leads {
		name := l.DisplayName
		if name == "" {
			name = l.OrganizationName
		}
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\n", l.ID, truncate(name, 40), truncate(l.Email, 30), l.Status)
	}
	return nil
}

func printCrmOne(stdout io.Writer, flags *rootFlags, l *client.CrmLead) error {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(l)
	}
	fmt.Fprintf(stdout, "id %s\nname %s\norg %s\nemail %s\nphone %s\nstatus %s\n", l.ID, l.DisplayName, l.OrganizationName, l.Email, l.Phone, l.Status)
	return nil
}

func printCrmOpportunities(stdout io.Writer, flags *rootFlags, res *client.CrmOpportunityListResult) error {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if len(res.Items) == 0 {
		fmt.Fprintln(stdout, "no opportunities")
		return nil
	}
	fmt.Fprintf(stdout, "%d opportunit(y/ies), total %d\n", res.Count, res.Total)
	for _, o := range res.Items {
		fmt.Fprintf(stdout, "%s\t%s\t%s\t%.2f\n", o.ID, truncate(o.Name, 40), o.Stage, o.Amount)
	}
	return nil
}

func printCrmAssociations(stdout io.Writer, flags *rootFlags, res *client.CrmAssociationResult) error {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}
	if len(res.Associations) == 0 {
		fmt.Fprintln(stdout, "no lead-customer associations")
		return nil
	}
	fmt.Fprintf(stdout, "%d association(s)\n", res.Count)
	for _, a := range res.Associations {
		fmt.Fprintf(stdout, "%s -> %s\n", a.LeadID, a.CustomerID)
	}
	return nil
}
