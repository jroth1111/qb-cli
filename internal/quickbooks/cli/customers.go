package cli

import (
	"context"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

func newCustomersCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "customers",
		Short: "Customers and hub",
		Long:  "Customers and hub. qb customers <entity> <verb>. Stubs return not-wired.",
	}
	appointmentEnt := &cobra.Command{Use: "appointment", Short: "appointment"}
	appointmentEnt.AddCommand(newStubCmd(flags, "customers appointment", "get", "appointment read (not wired)"))
	appointmentEnt.AddCommand(newStubCmd(flags, "customers appointment", "search", "appointment search (not wired)"))
	cmd.AddCommand(appointmentEnt)
	contractEnt := &cobra.Command{Use: "contract", Short: "contract"}
	contractEnt.AddCommand(newStubCmd(flags, "customers contract", "get", "contract read (not wired)"))
	contractEnt.AddCommand(newStubCmd(flags, "customers contract", "search", "contract search (not wired)"))
	cmd.AddCommand(contractEnt)
	customerEnt := &cobra.Command{Use: "customer", Short: "customer"}
	customerEnt.AddCommand(newStubCmd(flags, "customers customer", "create", "customer create (not wired)"))
	customerEnt.AddCommand(newStubCmd(flags, "customers customer", "run", "customer credit-transfer (not wired)"))
	customerEnt.AddCommand(newStubCmd(flags, "customers customer", "delete", "customer delete (not wired)"))
	customerUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	customerUpdateEnt.AddCommand(newStubCmd(flags, "customers customer update", "edit", "customer edit (not wired)"))
	customerUpdateEnt.AddCommand(newStubCmd(flags, "customers customer update", "merge", "customer merge (not wired)"))
	customerEnt.AddCommand(customerUpdateEnt)
	customerEnt.AddCommand(newStubCmd(flags, "customers customer", "get", "customer read (not wired)"))
	customerEnt.AddCommand(newStubCmd(flags, "customers customer", "search", "customer search (not wired)"))
	cmd.AddCommand(customerEnt)
	customerTypeEnt := &cobra.Command{Use: "customer-type", Short: "customer-type"}
	customerTypeEnt.AddCommand(newStubCmd(flags, "customers customer-type", "get", "customer-type read"))
	customerTypeEnt.AddCommand(newStubCmd(flags, "customers customer-type", "search", "customer-type search"))
	cmd.AddCommand(customerTypeEnt)
	opportunityEnt := &cobra.Command{Use: "opportunity", Short: "opportunity"}
	opportunityEnt.AddCommand(newStubCmd(flags, "customers opportunity", "create", "opportunity create (not wired)"))
	opportunityEnt.AddCommand(newStubCmd(flags, "customers opportunity", "get", "opportunity read (not wired)"))
	opportunityEnt.AddCommand(newStubCmd(flags, "customers opportunity", "search", "opportunity search (not wired)"))
	cmd.AddCommand(opportunityEnt)
	overviewEnt := &cobra.Command{Use: "overview", Short: "overview"}
	overviewEnt.AddCommand(newStubCmd(flags, "customers overview", "get", "overview read (not wired)"))
	cmd.AddCommand(overviewEnt)
	proposalEnt := &cobra.Command{Use: "proposal", Short: "proposal"}
	proposalEnt.AddCommand(newProposalCreateCmd(flags))
	proposalEnt.AddCommand(newProposalUpdateCmd(flags))
	proposalEnt.AddCommand(newProposalDeleteCmd(flags))
	proposalEnt.AddCommand(newStubCmd(flags, "customers proposal", "get", "proposal read (not wired)"))
	proposalEnt.AddCommand(newStubCmd(flags, "customers proposal", "search", "proposal search (not wired)"))
	cmd.AddCommand(proposalEnt)
	reviewEnt := &cobra.Command{Use: "review", Short: "review"}
	reviewEnt.AddCommand(newStubCmd(flags, "customers review", "get", "review read (not wired)"))
	reviewEnt.AddCommand(newStubCmd(flags, "customers review", "search", "review search (not wired)"))
	cmd.AddCommand(reviewEnt)
	// Service map (2026-08-24): never-triggered production services, blocked.
	proposal_svcEnt := &cobra.Command{Use: "proposal-svc", Short: "proposal-svc"}
	proposal_svcEnt.AddCommand(newProposalSvcCreateCmd(flags))
	cmd.AddCommand(proposal_svcEnt)
	sales_agentEnt := &cobra.Command{Use: "sales-agent", Short: "sales-agent"}
	sales_agentEnt.AddCommand(newStubCmd(flags, "customers sales-agent", "get", "AI sales-agent conversations (service map; not wired)"))
	sales_agentPromptEnt := &cobra.Command{Use: "prompt", Short: "prompt"}
	sales_agentPromptEnt.AddCommand(newStubCmd(flags, "customers sales-agent prompt", "post", "sales-agent prompt evaluation (service map; not wired)"))
	sales_agentEnt.AddCommand(sales_agentPromptEnt)
	cmd.AddCommand(sales_agentEnt)
	// Service map v2 (2026-08-24): never-triggered production services, blocked.
	realtime_entity_resolutionEnt := &cobra.Command{Use: "realtime-entity-resolution", Short: "realtime-entity-resolution"}
	realtime_entity_resolutionEnt.AddCommand(newStubCmd(flags, "customers realtime-entity-resolution", "get", "realtimeentityresolution.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(realtime_entity_resolutionEnt)
	prompt_evaluatorEnt := &cobra.Command{Use: "prompt-evaluator", Short: "prompt-evaluator"}
	prompt_evaluatorEnt.AddCommand(newStubCmd(flags, "customers prompt-evaluator", "post", "prompt-evaluator-svc.api.intuit.com post (service map; not wired)"))
	cmd.AddCommand(prompt_evaluatorEnt)
	mcf_backendEnt := &cobra.Command{Use: "mcf-backend", Short: "mcf-backend"}
	mcf_backendEnt.AddCommand(newStubCmd(flags, "customers mcf-backend", "get", "mcfbackend.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(mcf_backendEnt)
	mc_appfabricEnt := &cobra.Command{Use: "mc-appfabric", Short: "mc-appfabric"}
	mc_appfabricEnt.AddCommand(newStubCmd(flags, "customers mc-appfabric", "get", "mc-appfabricproxy.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(mc_appfabricEnt)
	mailchimp_notificationsEnt := &cobra.Command{Use: "mailchimp-notifications", Short: "mailchimp-notifications"}
	mailchimp_notificationsEnt.AddCommand(newStubCmd(flags, "customers mailchimp-notifications", "post", "mailchimpnotifications.api.intuit.com post (service map; not wired)"))
	cmd.AddCommand(mailchimp_notificationsEnt)
	reviewsEnt := &cobra.Command{Use: "reviews", Short: "reviews"}
	reviewsEnt.AddCommand(newStubCmd(flags, "customers reviews", "get", "reviews.api.intuit.com/v4/graphql get (service map; not wired)"))
	cmd.AddCommand(reviewsEnt)
	return cmd
}

// newProposalCreateCmd implements `customers proposal create`: POST
// /v1/proposals on crm-proposal-svc. Live-proven on TC2 — the service
// auto-names drafts "Proposal {id}"; --title is forwarded but the
// service-assigned display name wins.
func newProposalCreateCmd(flags *rootFlags) *cobra.Command {
	var customer, contactType, title string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a proposal draft (crm-proposal-svc POST /v1/proposals)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "customers proposal create", ID: "QBO.CUSTOMERS.PROPOSAL_CREATE",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedProposalURL(),
					Flags: localFlagMap(cmd), Note: "POST {contactId,contactType}; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayProposalCreate(ctx, customer, contactType, title)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "customer/lead contact id the proposal is addressed to (required)")
	cmd.Flags().StringVar(&contactType, "contact-type", "CUSTOMER", "contact type (CUSTOMER or LEAD)")
	cmd.Flags().StringVar(&title, "title", "", "proposal title (service assigns 'Proposal {id}' as the display name)")
	_ = cmd.MarkFlagRequired("customer")
	applyCatalogHelp(cmd, "QBO.CUSTOMERS.PROPOSAL_CREATE")
	return cmd
}

// newProposalUpdateCmd implements `customers proposal update`: fetch by
// refId UUID, apply overrides, PUT the full object back (the service
// requires a complete body). Asymmetric ids: update keys by refId.
func newProposalUpdateCmd(flags *rootFlags) *cobra.Command {
	var id, title string
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a proposal (crm-proposal-svc GET+PUT /v1/proposals/{refId})",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "customers proposal update", ID: "QBO.CUSTOMERS.PROPOSAL_EDIT",
					Mode: modeWired, Method: "PUT",
					URL:   client.PlannedProposalURL(),
					Flags: localFlagMap(cmd), Note: "GET /v1/proposals/{refId} then PUT full body; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayProposalUpdate(ctx, id, title)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "proposal refId UUID (required — not the numeric id)")
	cmd.Flags().StringVar(&title, "title", "", "new title (service keeps auto 'Proposal {id}' display name)")
	_ = cmd.MarkFlagRequired("id")
	applyCatalogHelp(cmd, "QBO.CUSTOMERS.PROPOSAL_EDIT")
	return cmd
}

// newProposalDeleteCmd implements `customers proposal delete`: DELETE
// /v1/proposals/{numericId} — the service expects the numeric id here
// (refId is rejected), and the delete is a soft flag (isDeleted:true).
func newProposalDeleteCmd(flags *rootFlags) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a proposal (crm-proposal-svc DELETE /v1/proposals/{numericId})",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "customers proposal delete", ID: "QBO.CUSTOMERS.PROPOSAL_DELETE",
					Mode: modeWired, Method: "DELETE",
					URL:   client.PlannedProposalURL(),
					Flags: localFlagMap(cmd), Note: "DELETE /v1/proposals/{numericId}; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayProposalDelete(ctx, id)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "numeric proposal id (required — not the refId UUID)")
	_ = cmd.MarkFlagRequired("id")
	applyCatalogHelp(cmd, "QBO.CUSTOMERS.PROPOSAL_DELETE")
	return cmd
}

// newProposalSvcCreateCmd implements `customers proposal-svc create` — the
// service-map row for the same crm-proposal-svc surface.
func newProposalSvcCreateCmd(flags *rootFlags) *cobra.Command {
	var customer, contactType string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "crm-proposal-svc proposal create (service map; same POST /v1/proposals)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "customers proposal-svc create", ID: "QBO.CUSTOMERS.PROPOSAL_SVC_CREATE",
					Mode: modeWired, Method: "POST",
					URL:   client.PlannedProposalURL(),
					Flags: localFlagMap(cmd), Note: "POST {contactId,contactType}; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayProposalCreate(ctx, customer, contactType, "")
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&customer, "customer", "", "customer/lead contact id (required)")
	cmd.Flags().StringVar(&contactType, "contact-type", "CUSTOMER", "contact type (CUSTOMER or LEAD)")
	_ = cmd.MarkFlagRequired("customer")
	applyCatalogHelp(cmd, "QBO.CUSTOMERS.PROPOSAL_SVC_CREATE")
	return cmd
}
