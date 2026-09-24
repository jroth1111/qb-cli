package cli

import (
	"context"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

// newCompanySearchCmd implements `company search run`: the nav-searchbox
// transaction search captured from /app/fullSearch — POST
// ceressos.api.intuit.com/graphql GetTransactionGlobalSearchEntities under
// the base-list-ui plugin (live-proven TC2 2026-09-19).
func newCompanySearchCmd(flags *rootFlags) *cobra.Command {
	var (
		query string
		limit int
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Search transactions, contacts, and more (global search)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "company search run",
					ID:      "QBO.COMPANY.SEARCH",
					Mode:    modeRead,
					Method:  "POST",
					URL:     "https://ceressos.api.intuit.com/graphql",
					Flags:   localFlagMap(cmd),
					Note:    "captured GetTransactionGlobalSearchEntities; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayGlobalSearch(ctx, query, limit)
			if err != nil {
				return feedErr(flags, err)
			}
			return printQueryResult(cmd.OutOrStdout(), flags, res)
		},
	}
	cmd.Flags().StringVar(&query, "query", "", "search text — name, memo, number, or id (required)")
	cmd.Flags().IntVar(&limit, "limit", 20, "max rows to return")
	_ = cmd.MarkFlagRequired("query")
	applyCatalogHelp(cmd, "QBO.COMPANY.SEARCH")
	return cmd
}

func newCompanyCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "company",
		Short: "Settings and users",
		Long:  "Settings and users. qb company <entity> <verb>. Stubs return not-wired.",
	}
	accountantEnt := &cobra.Command{Use: "accountant", Short: "accountant"}
	accountantEnt.AddCommand(newStubCmd(flags, "company accountant", "run invite", "accountant invite (not wired)"))
	cmd.AddCommand(accountantEnt)
	attachableEnt := &cobra.Command{Use: "attachable", Short: "attachable"}
	attachableEnt.AddCommand(newCompanyAttachableUploadCmd(flags))
	attachableEnt.AddCommand(newCompanyAttachableDownloadCmd(flags))
	attachableEnt.AddCommand(newStubCmd(flags, "company attachable", "get", "attachable read"))
	attachableEnt.AddCommand(newStubCmd(flags, "company attachable", "search", "attachable search"))
	attachableEnt.AddCommand(newStubCmd(flags, "company attachable", "update", "attachable edit"))
	attachableEnt.AddCommand(newStubCmd(flags, "company attachable", "delete", "attachable delete"))
	cmd.AddCommand(attachableEnt)
	attachmentEnt := &cobra.Command{Use: "attachment", Short: "attachment"}
	attachmentEnt.AddCommand(newStubCmd(flags, "company attachment", "get", "attachment read (not wired)"))
	cmd.AddCommand(attachmentEnt)
	billsEnt := &cobra.Command{Use: "bills", Short: "bills"}
	billsEnt.AddCommand(newStubCmd(flags, "company bills", "import", "bills import"))
	cmd.AddCommand(billsEnt)

	business_feedEnt := &cobra.Command{Use: "business-feed", Short: "business-feed"}
	business_feedEnt.AddCommand(newStubCmd(flags, "company business-feed", "get", "business-feed read (not wired)"))
	cmd.AddCommand(business_feedEnt)
	coaEnt := &cobra.Command{Use: "coa", Short: "coa"}
	coaEnt.AddCommand(newStubCmd(flags, "company coa", "import", "coa import"))
	cmd.AddCommand(coaEnt)
	company_infoEnt := &cobra.Command{Use: "company-info", Short: "company-info"}
	company_infoEnt.AddCommand(newStubCmd(flags, "company company-info", "get", "company-info read"))
	company_infoUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	company_infoUpdateEnt.AddCommand(newStubCmd(flags, "company company-info update", "edit", "company-info edit"))
	company_infoEnt.AddCommand(company_infoUpdateEnt)
	cmd.AddCommand(company_infoEnt)
	contact_formEnt := &cobra.Command{Use: "contact-form", Short: "contact-form"}
	contact_formEnt.AddCommand(newStubCmd(flags, "company contact-form", "create", "contact-form create (not wired)"))
	cmd.AddCommand(contact_formEnt)
	currencyEnt := &cobra.Command{Use: "currency", Short: "currency"}
	currencyEnt.AddCommand(newStubCmd(flags, "company currency", "create", "currency create"))
	currencyEnt.AddCommand(newStubCmd(flags, "company currency", "delete", "currency delete"))

	currencyEnt.AddCommand(newStubCmd(flags, "company currency", "get", "currency read"))
	currencyUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	currencyUpdateEnt.AddCommand(newStubCmd(flags, "company currency update", "edit", "currency edit"))
	currencyUpdateEnt.AddCommand(newStubCmd(flags, "company currency update", "revalue", "currency revalue (not wired)"))
	currencyEnt.AddCommand(currencyUpdateEnt)
	cmd.AddCommand(currencyEnt)
	custom_fieldEnt := &cobra.Command{Use: "custom-field", Short: "custom-field"}
	custom_fieldEnt.AddCommand(newStubCmd(flags, "company custom-field", "create", "custom-field create (not wired)"))
	custom_fieldEnt.AddCommand(newStubCmd(flags, "company custom-field", "delete", "custom-field delete (not wired)"))
	custom_fieldEnt.AddCommand(newStubCmd(flags, "company custom-field", "update", "custom-field edit (not wired)"))
	custom_fieldEnt.AddCommand(newStubCmd(flags, "company custom-field", "get", "custom-field read (not wired)"))
	cmd.AddCommand(custom_fieldEnt)
	exchangeRateEnt := &cobra.Command{Use: "exchange-rate", Short: "exchange-rate"}
	exchangeRateEnt.AddCommand(newStubCmd(flags, "company exchange-rate", "get", "exchange-rate read"))
	exchangeRateEnt.AddCommand(newStubCmd(flags, "company exchange-rate", "search", "exchange-rate search"))
	cmd.AddCommand(exchangeRateEnt)
	customersEnt := &cobra.Command{Use: "customers", Short: "customers"}
	customersEnt.AddCommand(newStubCmd(flags, "company customers", "import", "customers import"))
	cmd.AddCommand(customersEnt)
	form_styleEnt := &cobra.Command{Use: "form-style", Short: "form-style"}
	form_styleEnt.AddCommand(newStubCmd(flags, "company form-style", "update", "form-style update"))
	form_styleEnt.AddCommand(newStubCmd(flags, "company form-style", "get", "form-style get"))
	cmd.AddCommand(form_styleEnt)
	itemsEnt := &cobra.Command{Use: "items", Short: "items"}
	itemsEnt.AddCommand(newStubCmd(flags, "company items", "import", "items import"))
	cmd.AddCommand(itemsEnt)
	listEnt := &cobra.Command{Use: "list", Short: "list"}
	listEnt.AddCommand(newStubCmd(flags, "company list", "get", "list read (not wired)"))
	cmd.AddCommand(listEnt)
	marketingEnt := &cobra.Command{Use: "marketing", Short: "marketing"}
	marketingEnt.AddCommand(newStubCmd(flags, "company marketing", "get", "marketing read (not wired)"))
	cmd.AddCommand(marketingEnt)
	payment_methodEnt := &cobra.Command{Use: "payment-method", Short: "payment-method"}
	payment_methodEnt.AddCommand(newStubCmd(flags, "company payment-method", "create", "payment-method create"))
	payment_methodEnt.AddCommand(newStubCmd(flags, "company payment-method", "delete", "payment-method delete"))
	payment_methodEnt.AddCommand(newStubCmd(flags, "company payment-method", "get", "payment-method read"))
	payment_methodUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	payment_methodUpdateEnt.AddCommand(newStubCmd(flags, "company payment-method update", "edit", "payment-method edit"))
	payment_methodEnt.AddCommand(payment_methodUpdateEnt)
	cmd.AddCommand(payment_methodEnt)
	preferencesEnt := &cobra.Command{Use: "preferences", Short: "preferences"}
	preferencesEnt.AddCommand(newStubCmd(flags, "company preferences", "get", "preferences read (not wired)"))
	cmd.AddCommand(preferencesEnt)
	roleEnt := &cobra.Command{Use: "role", Short: "role"}
	roleEnt.AddCommand(newStubCmd(flags, "company role", "create", "role create (not wired)"))
	roleEnt.AddCommand(newStubCmd(flags, "company role", "delete", "role delete (not wired)"))
	roleEnt.AddCommand(newStubCmd(flags, "company role", "update", "role edit (not wired)"))
	roleEnt.AddCommand(newStubCmd(flags, "company role", "get", "role read (not wired)"))
	cmd.AddCommand(roleEnt)
	searchEnt := &cobra.Command{Use: "search", Short: "search"}
	searchEnt.AddCommand(newCompanySearchCmd(flags))
	cmd.AddCommand(searchEnt)
	settingsEnt := &cobra.Command{Use: "settings", Short: "settings"}
	settingsEnt.AddCommand(newStubCmd(flags, "company settings", "update", "settings edit (not wired)"))
	settingsEnt.AddCommand(newStubCmd(flags, "company settings", "get", "settings read (not wired)"))
	cmd.AddCommand(settingsEnt)
	suppliersEnt := &cobra.Command{Use: "suppliers", Short: "suppliers"}
	suppliersEnt.AddCommand(newStubCmd(flags, "company suppliers", "import", "suppliers import"))
	cmd.AddCommand(suppliersEnt)
	tagEnt := &cobra.Command{Use: "tag", Short: "tag"}
	tagEnt.AddCommand(newStubCmd(flags, "company tag", "create", "tag create (not wired)"))
	tagEnt.AddCommand(newStubCmd(flags, "company tag", "delete", "tag delete (not wired)"))
	tagEnt.AddCommand(newStubCmd(flags, "company tag", "update", "tag edit (not wired)"))
	tagEnt.AddCommand(newStubCmd(flags, "company tag", "get", "tag read (not wired)"))
	cmd.AddCommand(tagEnt)
	termEnt := &cobra.Command{Use: "term", Short: "term"}
	termEnt.AddCommand(newStubCmd(flags, "company term", "create", "term create"))
	termEnt.AddCommand(newStubCmd(flags, "company term", "delete", "term delete"))
	termEnt.AddCommand(newStubCmd(flags, "company term", "get", "term read"))
	termUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	termUpdateEnt.AddCommand(newStubCmd(flags, "company term update", "edit", "term edit"))
	termEnt.AddCommand(termUpdateEnt)
	cmd.AddCommand(termEnt)
	userEnt := &cobra.Command{Use: "user", Short: "user"}
	userEnt.AddCommand(newStubCmd(flags, "company user", "create", "user create (not wired)"))
	userEnt.AddCommand(newStubCmd(flags, "company user", "delete", "user delete (not wired)"))
	userEnt.AddCommand(newStubCmd(flags, "company user", "update", "user edit (not wired)"))
	userEnt.AddCommand(newStubCmd(flags, "company user", "get", "identity identityUsers"))
	cmd.AddCommand(userEnt)
	webhookEnt := &cobra.Command{Use: "webhook", Short: "webhook"}
	webhookEnt.AddCommand(newCompanyWebhookVerifyCmd(flags))
	cmd.AddCommand(webhookEnt)
	// Service map (2026-08-24): never-triggered production services, blocked.
	dimensionsEnt := &cobra.Command{Use: "dimensions-graphql", Short: "dimensions-graphql"}
	dimensionsEnt.AddCommand(newStubCmd(flags, "company dimensions-graphql", "get", "custom dimensions read (service map; not wired)"))
	cmd.AddCommand(dimensionsEnt)
	custom_extensionsEnt := &cobra.Command{Use: "custom-extensions", Short: "custom-extensions"}
	custom_extensionsEnt.AddCommand(newStubCmd(flags, "company custom-extensions", "list", "customextensions CustomFieldsQueryCES"))
	cmd.AddCommand(custom_extensionsEnt)
	company_managementEnt := &cobra.Command{Use: "company-management", Short: "company-management"}
	company_managementEnt.AddCommand(newStubCmd(flags, "company company-management", "get", "multi-company group read (service map; not wired)"))
	cmd.AddCommand(company_managementEnt)
	consentEnt := &cobra.Command{Use: "consent", Short: "consent"}
	consentEnt.AddCommand(newStubCmd(flags, "company consent", "get", "consent grants read (service map; not wired)"))
	cmd.AddCommand(consentEnt)
	consumer_notificationEnt := &cobra.Command{Use: "consumer-notification", Short: "consumer-notification"}
	consumer_notificationEnt.AddCommand(newStubCmd(flags, "company consumer-notification", "post", "browser notification event (service map; not wired)"))
	cmd.AddCommand(consumer_notificationEnt)
	content_accessEnt := &cobra.Command{Use: "content-access", Short: "content-access"}
	content_accessEnt.AddCommand(newStubCmd(flags, "company content-access", "get", "help content entitlement search (service map; not wired)"))
	cmd.AddCommand(content_accessEnt)
	customization_agentEnt := &cobra.Command{Use: "customization-agent", Short: "customization-agent"}
	customization_agentEnt.AddCommand(newStubCmd(flags, "company customization-agent", "post", "natural-language customization request (service map; not wired)"))
	cmd.AddCommand(customization_agentEnt)
	data_exportEnt := &cobra.Command{Use: "data-export", Short: "data-export"}
	data_exportEnt.AddCommand(newCompanyDataExportCmd(flags))
	cmd.AddCommand(data_exportEnt)
	data_mirrorEnt := &cobra.Command{Use: "data-mirror", Short: "data-mirror"}
	data_mirrorEnt.AddCommand(newStubCmd(flags, "company data-mirror", "get", "data-dev mirror dataset read (service map; not wired)"))
	cmd.AddCommand(data_mirrorEnt)
	// Service map v2 (2026-08-24): never-triggered production services, blocked.
	chorusEnt := &cobra.Command{Use: "chorus", Short: "chorus"}
	chorusEnt.AddCommand(newStubCmd(flags, "company chorus", "get", "chorus.api.intuit.com/graphql get (service map; not wired)"))
	cmd.AddCommand(chorusEnt)
	customization_agent_qaEnt := &cobra.Command{Use: "customization-agent-qa", Short: "customization-agent-qa"}
	customization_agent_qaEnt.AddCommand(newStubCmd(flags, "company customization-agent-qa", "post", "customization-agent-qa.api.intuit.com/v1 post (service map; not wired)"))
	cmd.AddCommand(customization_agent_qaEnt)
	envelopeEnt := &cobra.Command{Use: "envelope", Short: "envelope"}
	envelopeEnt.AddCommand(newStubCmd(flags, "company envelope", "post", "envelope.api.intuit.com/v4/graphql post (service map; not wired)"))
	cmd.AddCommand(envelopeEnt)
	input_to_actionEnt := &cobra.Command{Use: "input-to-action", Short: "input-to-action"}
	input_to_actionEnt.AddCommand(newStubCmd(flags, "company input-to-action", "post", "inputtoaction.api.intuit.com/v1/business_feed post (service map; not wired)"))
	cmd.AddCommand(input_to_actionEnt)
	qbdata_migration_statusEnt := &cobra.Command{Use: "qbdata-migration-status", Short: "qbdata-migration-status"}
	qbdata_migration_statusEnt.AddCommand(newStubCmd(flags, "company qbdata-migration-status", "get", "qbdatamigration.api.intuit.com/v1 get (service map; not wired)"))
	cmd.AddCommand(qbdata_migration_statusEnt)
	qblive_experienceEnt := &cobra.Command{Use: "qblive-experience", Short: "qblive-experience"}
	qblive_experienceEnt.AddCommand(newStubCmd(flags, "company qblive-experience", "get", "qbliveexperience.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(qblive_experienceEnt)
	genplugin_registryEnt := &cobra.Command{Use: "genplugin-registry", Short: "genplugin-registry"}
	genplugin_registryEnt.AddCommand(newStubCmd(flags, "company genplugin-registry", "get", "genpluginregistry.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(genplugin_registryEnt)
	modelexecutionEnt := &cobra.Command{Use: "modelexecution", Short: "modelexecution"}
	modelexecutionEnt.AddCommand(newStubCmd(flags, "company modelexecution", "post", "modelexecution.api.intuit.com/v2/smart-compose-*/predict post (service map; not wired)"))
	cmd.AddCommand(modelexecutionEnt)
	experiment_assignmentEnt := &cobra.Command{Use: "experiment-assignment", Short: "experiment-assignment"}
	experiment_assignmentEnt.AddCommand(newStubCmd(flags, "company experiment-assignment", "get", "experimentassignment assignments"))
	cmd.AddCommand(experiment_assignmentEnt)
	identity_qaEnt := &cobra.Command{Use: "identity-qa", Short: "identity-qa"}
	identity_qaEnt.AddCommand(newStubCmd(flags, "company identity-qa", "get", "identity-qa.api.intuit.com/v2/graphql get (service map; not wired)"))
	cmd.AddCommand(identity_qaEnt)
	identityEnt := &cobra.Command{Use: "identity", Short: "identity"}
	identityEnt.AddCommand(newStubCmd(flags, "company identity", "get", "identity account query"))
	cmd.AddCommand(identityEnt)
	settings_facadeEnt := &cobra.Command{Use: "settings-facade", Short: "settings-facade"}
	settings_facadeEnt.AddCommand(newStubCmd(flags, "company settings-facade", "get", "settingsfacade qbAppFoundationQbSettings"))
	cmd.AddCommand(settings_facadeEnt)
	franchise_orchEnt := &cobra.Command{Use: "franchise-orch", Short: "franchise-orch"}
	franchise_orchEnt.AddCommand(newStubCmd(flags, "company franchise-orch", "post", "franchise-orch-svc.api.intuit.com/v1 post (service map; not wired)"))
	cmd.AddCommand(franchise_orchEnt)
	swift_noncat_requestEnt := &cobra.Command{Use: "swift-noncat-request", Short: "swift-noncat-request"}
	swift_noncat_requestEnt.AddCommand(newStubCmd(flags, "company swift-noncat-request", "post", "swift-non-cat-request.api.intuit.com/graphql post (service map; not wired)"))
	cmd.AddCommand(swift_noncat_requestEnt)
	expert_credentialsEnt := &cobra.Command{Use: "expert-credentials", Short: "expert-credentials"}
	expert_credentialsEnt.AddCommand(newStubCmd(flags, "company expert-credentials", "get", "expertcredentials.api.intuit.com/graphql get (service map; not wired)"))
	cmd.AddCommand(expert_credentialsEnt)
	expert_engagementEnt := &cobra.Command{Use: "expert-engagement", Short: "expert-engagement"}
	expert_engagementEnt.AddCommand(newStubCmd(flags, "company expert-engagement", "post", "expertengagement.api.intuit.com/graphql post (service map; not wired)"))
	cmd.AddCommand(expert_engagementEnt)
	geocodeEnt := &cobra.Command{Use: "geocode", Short: "geocode"}
	geocodeEnt.AddCommand(newStubCmd(flags, "company geocode", "get", "geocode.api.intuit.com/geocode/v1 get (service map; not wired)"))
	cmd.AddCommand(geocodeEnt)
	return cmd
}
