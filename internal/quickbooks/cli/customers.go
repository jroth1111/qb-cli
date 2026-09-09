package cli

import "github.com/spf13/cobra"

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
	proposalEnt.AddCommand(newStubCmd(flags, "customers proposal", "create", "proposal create (not wired)"))
	proposalEnt.AddCommand(newStubCmd(flags, "customers proposal", "get", "proposal read (not wired)"))
	proposalEnt.AddCommand(newStubCmd(flags, "customers proposal", "search", "proposal search (not wired)"))
	cmd.AddCommand(proposalEnt)
	reviewEnt := &cobra.Command{Use: "review", Short: "review"}
	reviewEnt.AddCommand(newStubCmd(flags, "customers review", "get", "review read (not wired)"))
	reviewEnt.AddCommand(newStubCmd(flags, "customers review", "search", "review search (not wired)"))
	cmd.AddCommand(reviewEnt)
	// Service map (2026-08-24): never-triggered production services, blocked.
	proposal_svcEnt := &cobra.Command{Use: "proposal-svc", Short: "proposal-svc"}
	proposal_svcEnt.AddCommand(newStubCmd(flags, "customers proposal-svc", "create", "crm-proposal-svc proposal (service map; not wired)"))
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
