package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/doctor"
	"github.com/spf13/cobra"
)

// primitiveMode is the wiring state of one catalog row.
type primitiveMode string

// Catalog modes describe wiring. Wired writes additionally require a reviewed
// readback adapter; the verification field describes runtime availability.
const (
	modeRead     primitiveMode = "read"
	modeWired    primitiveMode = "wired"
	modeBlocked  primitiveMode = "blocked"
	modeExcluded primitiveMode = "excluded"
)

// primitiveEntry is one row of the action catalog. Usage, Params, Notes
// and Globals are filled by documentedEntry, never stored in the table.
// JSON keys match the `qb actions --json` contract agents parse.
type primitiveEntry struct {
	ID           string        `json:"id"`
	Domain       string        `json:"domain"`
	Risk         string        `json:"risk"`
	Mode         primitiveMode `json:"mode"`
	Command      string        `json:"command,omitempty"`
	Usage        string        `json:"usage,omitempty"`
	Params       []paramDoc    `json:"params,omitempty"`
	Notes        string        `json:"notes,omitempty"`
	Globals      []paramDoc    `json:"globals,omitempty"`
	Verification string        `json:"verification,omitempty"`
}

// catalogPrimitives is the generated action catalog: every QBO primitive
// surfaced on the qb CLI with its mode. Rebuilt from the capture corpus;
// row edits go through the same review as wired commands.
var catalogPrimitives = []primitiveEntry{
	{ID: "QBO.ACCOUNTANT.PORTFOLIO_STORE_GET", Domain: "accountant", Risk: "R0", Mode: modeBlocked, Command: "accountant portfolio-store get"},
	{ID: "QBO.ACCOUNTING.AUDIT_LOG_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting audit-log get"},
	{ID: "QBO.ACCOUNTING.AUDIT_LOG_SEARCH", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting audit-log search"},
	{ID: "QBO.ACCOUNTING.BUDGET_CREATE", Domain: "accounting", Risk: "R3", Mode: modeWired, Command: "accounting budget create"},
	{ID: "QBO.ACCOUNTING.BUDGET_DELETE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting budget delete"},
	{ID: "QBO.ACCOUNTING.BUDGET_EDIT", Domain: "accounting", Risk: "R3", Mode: modeWired, Command: "accounting budget update"},
	{ID: "QBO.ACCOUNTING.BUDGET_LIST", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting budget list"},
	{ID: "QBO.ACCOUNTING.BUDGET_PLANNING_GET", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting budget-planning get"},
	{ID: "QBO.ACCOUNTING.BUDGET_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting budget get"},
	{ID: "QBO.ACCOUNTING.BUDGET_SEARCH", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting budget search"},
	{ID: "QBO.ACCOUNTING.CDC_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting cdc get"},
	{ID: "QBO.ACCOUNTING.CLASS_CREATE", Domain: "accounting", Risk: "R4", Mode: modeWired, Command: "accounting class create"},
	{ID: "QBO.ACCOUNTING.CLASS_DELETE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting class delete"},
	{ID: "QBO.ACCOUNTING.CLASS_EDIT", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting class update"},
	{ID: "QBO.ACCOUNTING.CLASS_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting class get"},
	{ID: "QBO.ACCOUNTING.CLASS_SEARCH", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting class search"},
	{ID: "QBO.ACCOUNTING.COA_CREATE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting coa create"},
	{ID: "QBO.ACCOUNTING.COA_DEACTIVATE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting coa delete"},
	{ID: "QBO.ACCOUNTING.COA_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting coa get"},
	{ID: "QBO.ACCOUNTING.COA_SEARCH", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting coa search"},
	{ID: "QBO.ACCOUNTING.COA_TEMPLATES_GET", Domain: "accounting", Risk: "R0", Mode: modeBlocked, Command: "accounting coa-templates get"},
	{ID: "QBO.ACCOUNTING.DEFERRED_RECOGNITION_CREATE", Domain: "accounting", Risk: "R3", Mode: modeWired, Command: "accounting deferredrevenue create"},
	{ID: "QBO.ACCOUNTING.DEFERRED_RECOGNITION_GET", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting deferredrevenue get"},
	{ID: "QBO.ACCOUNTING.DEPARTMENT_CREATE", Domain: "accounting", Risk: "R4", Mode: modeWired, Command: "accounting department create"},
	{ID: "QBO.ACCOUNTING.DEPARTMENT_DELETE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting department delete"},
	{ID: "QBO.ACCOUNTING.DEPARTMENT_EDIT", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting department update"},
	{ID: "QBO.ACCOUNTING.DEPARTMENT_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting department get"},
	{ID: "QBO.ACCOUNTING.DEPARTMENT_SEARCH", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting department search"},
	{ID: "QBO.ACCOUNTING.DEPOSIT_CREATE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting deposit create"},
	{ID: "QBO.ACCOUNTING.DEPOSIT_DELETE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting deposit delete"},
	{ID: "QBO.ACCOUNTING.DEPOSIT_EDIT", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting deposit update"},
	{ID: "QBO.ACCOUNTING.DEPOSIT_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting deposit get"},
	{ID: "QBO.ACCOUNTING.FIT_TRANSACTIONS_GET", Domain: "accounting", Risk: "R0", Mode: modeBlocked, Command: "accounting fit-transactions get"},
	{ID: "QBO.ACCOUNTING.FIXED_ASSET_CREATE", Domain: "accounting", Risk: "R4", Mode: modeWired, Command: "accounting fixed-asset create"},
	{ID: "QBO.ACCOUNTING.FIXED_ASSET_DELETE", Domain: "accounting", Risk: "R4", Mode: modeWired, Command: "accounting fixed-asset delete"},
	{ID: "QBO.ACCOUNTING.FIXED_ASSET_EDIT", Domain: "accounting", Risk: "R4", Mode: modeWired, Command: "accounting fixed-asset update"},
	{ID: "QBO.ACCOUNTING.FIXED_ASSET_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting fixed-asset get"},
	{ID: "QBO.ACCOUNTING.FIXED_ASSET_SEARCH", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting fixed-asset search"},
	{ID: "QBO.ACCOUNTING.INTEGRATION_TXN_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting integration-txn get"},
	{ID: "QBO.ACCOUNTING.JOURNAL_CREATE", Domain: "accounting", Risk: "R3", Mode: modeWired, Command: "accounting journal create"},
	{ID: "QBO.ACCOUNTING.JOURNAL_DELETE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting journal delete"},
	{ID: "QBO.ACCOUNTING.JOURNAL_EDIT", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting journal update"},
	{ID: "QBO.ACCOUNTING.JOURNAL_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting journal get"},
	{ID: "QBO.ACCOUNTING.LIBRO_GET", Domain: "accounting", Risk: "R0", Mode: modeBlocked, Command: "accounting libro get"},
	{ID: "QBO.ACCOUNTING.LOCATION_CREATE", Domain: "accounting", Risk: "R4", Mode: modeWired, Command: "accounting location create"},
	{ID: "QBO.ACCOUNTING.LOCATION_DELETE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting location delete"},
	{ID: "QBO.ACCOUNTING.LOCATION_EDIT", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting location update"},
	{ID: "QBO.ACCOUNTING.LOCATION_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting location get"},
	{ID: "QBO.ACCOUNTING.MIDMARKET_SETTINGS_GET", Domain: "accounting", Risk: "R0", Mode: modeBlocked, Command: "accounting midmarket-settings get"},
	{ID: "QBO.ACCOUNTING.MULTIENTITY_GET", Domain: "accounting", Risk: "R0", Mode: modeBlocked, Command: "accounting multientity get"},
	{ID: "QBO.ACCOUNTING.MY_ACCOUNTANT_READ", Domain: "accounting", Risk: "R0", Mode: modeBlocked, Command: "accounting my-accountant get"},
	{ID: "QBO.ACCOUNTING.OPENING_BALANCE_CREATE", Domain: "accounting", Risk: "R3", Mode: modeWired, Command: "accounting opening-balance create"},
	{ID: "QBO.ACCOUNTING.OPENING_BALANCE_SET", Domain: "accounting", Risk: "R3", Mode: modeWired, Command: "accounting opening-balance update"},
	{ID: "QBO.ACCOUNTING.PREPAID_CREATE", Domain: "accounting", Risk: "R3", Mode: modeWired, Command: "accounting prepaid create"},
	{ID: "QBO.ACCOUNTING.PREPAID_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting prepaid get"},
	{ID: "QBO.ACCOUNTING.PROJECT_CREATE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting project create"},
	{ID: "QBO.ACCOUNTING.PROJECT_EDIT", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting project update"},
	{ID: "QBO.ACCOUNTING.PROJECT_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting project get"},
	{ID: "QBO.ACCOUNTING.PROJECT_SEARCH", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting project search"},
	{ID: "QBO.ACCOUNTING.QBDATA_MIGRATION_POST", Domain: "accounting", Risk: "R2", Mode: modeBlocked, Command: "accounting qbdata-migration post"},
	{ID: "QBO.ACCOUNTING.RECONCILE_CREATE", Domain: "accounting", Risk: "R3", Mode: modeWired, Command: "accounting reconcile create"},
	{ID: "QBO.ACCOUNTING.RECONCILE_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting reconcile get"},
	{ID: "QBO.ACCOUNTING.RECONCILE_SERVICE_GET", Domain: "accounting", Risk: "R0", Mode: modeBlocked, Command: "accounting reconcile-service get"},
	{ID: "QBO.ACCOUNTING.RECURRING_CREATE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting recurring create"},
	{ID: "QBO.ACCOUNTING.RECURRING_DELETE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting recurring delete"},
	{ID: "QBO.ACCOUNTING.RECURRING_EDIT", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting recurring update"},
	{ID: "QBO.ACCOUNTING.RECURRING_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting recurring get"},
	{ID: "QBO.ACCOUNTING.RECURRING_SEARCH", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting recurring search"},
	{ID: "QBO.ACCOUNTING.REGISTER_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting register get"},
	{ID: "QBO.ACCOUNTING.REVENUE_RECOGNITION_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting revenue-recognition get"},
	{ID: "QBO.ACCOUNTING.TAXCONFIG_MIRROR_GET", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting taxconfig-mirror get"},
	{ID: "QBO.ACCOUNTING.TRANSFER_CREATE", Domain: "accounting", Risk: "R3", Mode: modeWired, Command: "accounting transfer create"},
	{ID: "QBO.ACCOUNTING.TRANSFER_DELETE", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting transfer delete"},
	{ID: "QBO.ACCOUNTING.TRANSFER_EDIT", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting transfer update"},
	{ID: "QBO.ACCOUNTING.TRANSFER_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting transfer get"},
	{ID: "QBO.ACCOUNTING.TXN_VOID", Domain: "accounting", Risk: "R2", Mode: modeWired, Command: "accounting txn update"},
	{ID: "QBO.ADVANCED.ACS_GET", Domain: "advanced", Risk: "R0", Mode: modeBlocked, Command: "advanced acs get"},
	{ID: "QBO.ADVANCED.BACKUP_CREATE", Domain: "advanced", Risk: "R3", Mode: modeWired, Command: "advanced backup create"},
	{ID: "QBO.ADVANCED.BACKUP_RESTORE", Domain: "advanced", Risk: "R3", Mode: modeExcluded},
	{ID: "QBO.ADVANCED.BATCH_RUN", Domain: "advanced", Risk: "R4", Mode: modeWired, Command: "advanced batch run"},
	{ID: "QBO.ADVANCED.CUSTOM_ROLES_CREATE", Domain: "advanced", Risk: "R4", Mode: modeBlocked, Command: "advanced custom-roles create"},
	{ID: "QBO.ADVANCED.CUSTOM_ROLES_EDIT", Domain: "advanced", Risk: "R4", Mode: modeBlocked, Command: "advanced custom-roles update"},
	{ID: "QBO.ADVANCED.CUSTOM_ROLES_READ", Domain: "advanced", Risk: "R0", Mode: modeRead, Command: "advanced custom-roles get"},
	{ID: "QBO.ADVANCED.ERP_BUILDER_LIST", Domain: "advanced", Risk: "R0", Mode: modeBlocked, Command: "advanced erp-builder list"},
	{ID: "QBO.ADVANCED.EXPENSE_CLAIMS_CREATE", Domain: "advanced", Risk: "R2", Mode: modeBlocked, Command: "advanced expense-claims create"},
	{ID: "QBO.ADVANCED.EXPENSE_CLAIMS_READ", Domain: "advanced", Risk: "R0", Mode: modeBlocked, Command: "advanced expense-claims get"},
	{ID: "QBO.ADVANCED.EXPENSE_CLAIMS_REVIEW", Domain: "advanced", Risk: "R2", Mode: modeBlocked, Command: "advanced expense-claims verify"},
	{ID: "QBO.ADVANCED.RECLASSIFY_RUN", Domain: "advanced", Risk: "R3", Mode: modeWired, Command: "advanced reclassify run"},
	{ID: "QBO.ADVANCED.REVENUE_RECOGNITION_EDIT", Domain: "advanced", Risk: "R4", Mode: modeBlocked, Command: "advanced revenue-recognition update"},
	{ID: "QBO.ADVANCED.REVENUE_RECOGNITION_SETUP", Domain: "advanced", Risk: "R4", Mode: modeBlocked, Command: "advanced revenue-recognition run"},
	{ID: "QBO.ADVANCED.SPREADSHEET_SYNC_IMPORT", Domain: "advanced", Risk: "R3", Mode: modeBlocked, Command: "advanced spreadsheet-sync import"},
	{ID: "QBO.ADVANCED.TASKS_CREATE", Domain: "advanced", Risk: "R2", Mode: modeWired, Command: "advanced tasks create"},
	{ID: "QBO.ADVANCED.TASKS_DELETE", Domain: "advanced", Risk: "R2", Mode: modeWired, Command: "advanced tasks delete"},
	{ID: "QBO.ADVANCED.TASKS_EDIT", Domain: "advanced", Risk: "R2", Mode: modeWired, Command: "advanced tasks update"},
	{ID: "QBO.ADVANCED.TASKS_READ", Domain: "advanced", Risk: "R0", Mode: modeRead, Command: "advanced tasks get"},
	{ID: "QBO.ADVANCED.WORKFLOWS_CREATE", Domain: "advanced", Risk: "R2", Mode: modeWired, Command: "advanced workflows create"},
	{ID: "QBO.ADVANCED.WORKFLOWS_EDIT", Domain: "advanced", Risk: "R2", Mode: modeWired, Command: "advanced workflows update"},
	{ID: "QBO.ADVANCED.WORKFLOWS_READ", Domain: "advanced", Risk: "R0", Mode: modeRead, Command: "advanced workflows get"},
	{ID: "QBO.COMPANY.ACCOUNTANT_INVITE", Domain: "company", Risk: "R4", Mode: modeBlocked, Command: "company accountant run"},
	{ID: "QBO.COMPANY.ATTACHABLE_CREATE", Domain: "company", Risk: "R2", Mode: modeWired, Command: "company attachable upload"},
	{ID: "QBO.COMPANY.ATTACHABLE_DELETE", Domain: "company", Risk: "R2", Mode: modeWired, Command: "company attachable delete"},
	{ID: "QBO.COMPANY.ATTACHABLE_DOWNLOAD", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company attachable download"},
	{ID: "QBO.COMPANY.ATTACHABLE_EDIT", Domain: "company", Risk: "R2", Mode: modeWired, Command: "company attachable update"},
	{ID: "QBO.COMPANY.ATTACHABLE_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company attachable get"},
	{ID: "QBO.COMPANY.ATTACHABLE_SEARCH", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company attachable search"},
	{ID: "QBO.COMPANY.ATTACHMENT_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company attachment get"},
	{ID: "QBO.COMPANY.BILLS_IMPORT", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company bills import"},
	{ID: "QBO.COMPANY.BOOKS_CLOSE", Domain: "company", Risk: "R3", Mode: modeExcluded},
	{ID: "QBO.COMPANY.BUSINESS_FEED_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company business-feed get"},
	{ID: "QBO.COMPANY.CHORUS_GET", Domain: "company", Risk: "R0", Mode: modeBlocked, Command: "company chorus get"},
	{ID: "QBO.COMPANY.COA_IMPORT", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company coa import"},
	{ID: "QBO.COMPANY.COMPANY_INFO_EDIT", Domain: "company", Risk: "R2", Mode: modeWired, Command: "company company-info update edit"},
	{ID: "QBO.COMPANY.COMPANY_INFO_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company company-info get"},
	{ID: "QBO.COMPANY.COMPANY_MANAGEMENT_GET", Domain: "company", Risk: "R0", Mode: modeBlocked, Command: "company company-management get"},
	{ID: "QBO.COMPANY.CONSENT_GET", Domain: "company", Risk: "R1", Mode: modeBlocked, Command: "company consent get"},
	{ID: "QBO.COMPANY.CONSUMER_NOTIFICATION_POST", Domain: "company", Risk: "R1", Mode: modeBlocked, Command: "company consumer-notification post"},
	{ID: "QBO.COMPANY.CONTACT_FORM_CREATE", Domain: "company", Risk: "R2", Mode: modeBlocked, Command: "company contact-form create"},
	{ID: "QBO.COMPANY.CONTENT_ACCESS_GET", Domain: "company", Risk: "R0", Mode: modeBlocked, Command: "company content-access get"},
	{ID: "QBO.COMPANY.CURRENCY_CREATE", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company currency create"},
	{ID: "QBO.COMPANY.CURRENCY_DELETE", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company currency delete"},
	{ID: "QBO.COMPANY.CURRENCY_EDIT", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company currency update edit"},
	{ID: "QBO.COMPANY.CURRENCY_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company currency get"},
	{ID: "QBO.COMPANY.CURRENCY_REVALUE", Domain: "company", Risk: "R3", Mode: modeWired, Command: "company currency update revalue"},
	{ID: "QBO.COMPANY.CUSTOMERS_IMPORT", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company customers import"},
	{ID: "QBO.COMPANY.CUSTOMIZATION_AGENT_POST", Domain: "company", Risk: "R1", Mode: modeBlocked, Command: "company customization-agent post"},
	{ID: "QBO.COMPANY.CUSTOMIZATION_AGENT_QA_POST", Domain: "company", Risk: "R1", Mode: modeBlocked, Command: "company customization-agent-qa post"},
	{ID: "QBO.COMPANY.CUSTOM_EXTENSIONS_LIST", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company custom-extensions list"},
	{ID: "QBO.COMPANY.CUSTOM_FIELD_CREATE", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company custom-field create"},
	{ID: "QBO.COMPANY.CUSTOM_FIELD_DELETE", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company custom-field delete"},
	{ID: "QBO.COMPANY.CUSTOM_FIELD_EDIT", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company custom-field update"},
	{ID: "QBO.COMPANY.CUSTOM_FIELD_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company custom-field get"},
	{ID: "QBO.COMPANY.DATA_EXPORT_GET", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company data-export get"},
	{ID: "QBO.COMPANY.DATA_MIRROR_GET", Domain: "company", Risk: "R0", Mode: modeBlocked, Command: "company data-mirror get"},
	{ID: "QBO.COMPANY.DIMENSIONS_GRAPHQL_GET", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company dimensions-graphql get"},
	{ID: "QBO.COMPANY.ENVELOPE_POST", Domain: "company", Risk: "R1", Mode: modeBlocked, Command: "company envelope post"},
	{ID: "QBO.COMPANY.EXCHANGE_RATE_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company exchange-rate get"},
	{ID: "QBO.COMPANY.EXCHANGE_RATE_SEARCH", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company exchange-rate search"},
	{ID: "QBO.COMPANY.EXPERIMENT_ASSIGNMENT_GET", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company experiment-assignment get"},
	{ID: "QBO.COMPANY.EXPERT_CREDENTIALS_GET", Domain: "company", Risk: "R0", Mode: modeBlocked, Command: "company expert-credentials get"},
	{ID: "QBO.COMPANY.EXPERT_ENGAGEMENT_POST", Domain: "company", Risk: "R1", Mode: modeBlocked, Command: "company expert-engagement post"},
	{ID: "QBO.COMPANY.FORM_STYLE_EDIT", Domain: "company", Risk: "R2", Mode: modeWired, Command: "company form-style update"},
	{ID: "QBO.COMPANY.FORM_STYLE_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company form-style get"},
	{ID: "QBO.COMPANY.FRANCHISE_ORCH_POST", Domain: "company", Risk: "R1", Mode: modeBlocked, Command: "company franchise-orch post"},
	{ID: "QBO.COMPANY.GENPLUGIN_REGISTRY_GET", Domain: "company", Risk: "R0", Mode: modeBlocked, Command: "company genplugin-registry get"},
	{ID: "QBO.COMPANY.GEOCODE_GET", Domain: "company", Risk: "R0", Mode: modeBlocked, Command: "company geocode get"},
	{ID: "QBO.COMPANY.IDENTITY_GET", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company identity get"},
	{ID: "QBO.COMPANY.IDENTITY_QA_GET", Domain: "company", Risk: "R0", Mode: modeBlocked, Command: "company identity-qa get"},
	{ID: "QBO.COMPANY.INPUT_TO_ACTION_POST", Domain: "company", Risk: "R1", Mode: modeBlocked, Command: "company input-to-action post"},
	{ID: "QBO.COMPANY.ITEMS_IMPORT", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company items import"},
	{ID: "QBO.COMPANY.LIST_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company list get"},
	{ID: "QBO.COMPANY.MARKETING_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company marketing get"},
	{ID: "QBO.COMPANY.MODELEXECUTION_POST", Domain: "company", Risk: "R1", Mode: modeBlocked, Command: "company modelexecution post"},
	{ID: "QBO.COMPANY.PAYMENT_METHOD_CREATE", Domain: "company", Risk: "R2", Mode: modeWired, Command: "company payment-method create"},
	{ID: "QBO.COMPANY.PAYMENT_METHOD_DELETE", Domain: "company", Risk: "R2", Mode: modeWired, Command: "company payment-method delete"},
	{ID: "QBO.COMPANY.PAYMENT_METHOD_EDIT", Domain: "company", Risk: "R2", Mode: modeWired, Command: "company payment-method update edit"},
	{ID: "QBO.COMPANY.PAYMENT_METHOD_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company payment-method get"},
	{ID: "QBO.COMPANY.PREFERENCES_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company preferences get"},
	{ID: "QBO.COMPANY.QBDATAMIGRATION_STATUS_GET", Domain: "company", Risk: "R0", Mode: modeBlocked, Command: "company qbdata-migration-status get"},
	{ID: "QBO.COMPANY.QBLIVE_EXPERIENCE_GET", Domain: "company", Risk: "R0", Mode: modeBlocked, Command: "company qblive-experience get"},
	{ID: "QBO.COMPANY.ROLE_CREATE", Domain: "company", Risk: "R4", Mode: modeBlocked, Command: "company role create"},
	{ID: "QBO.COMPANY.ROLE_DELETE", Domain: "company", Risk: "R4", Mode: modeBlocked, Command: "company role delete"},
	{ID: "QBO.COMPANY.ROLE_EDIT", Domain: "company", Risk: "R4", Mode: modeBlocked, Command: "company role update"},
	{ID: "QBO.COMPANY.ROLE_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company role get"},
	{ID: "QBO.COMPANY.SEARCH", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company search run"},
	{ID: "QBO.COMPANY.SETTINGS_EDIT", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company settings update"},
	{ID: "QBO.COMPANY.SETTINGS_FACADE_GET", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company settings-facade get"},
	{ID: "QBO.COMPANY.SETTINGS_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company settings get"},
	{ID: "QBO.COMPANY.SUPPLIERS_IMPORT", Domain: "company", Risk: "R4", Mode: modeWired, Command: "company suppliers import"},
	{ID: "QBO.COMPANY.SWIFT_NONCAT_POST", Domain: "company", Risk: "R0", Mode: modeBlocked, Command: "company swift-noncat-request post"},
	{ID: "QBO.COMPANY.TAG_CREATE", Domain: "company", Risk: "R2", Mode: modeBlocked, Command: "company tag create"},
	{ID: "QBO.COMPANY.TAG_DELETE", Domain: "company", Risk: "R2", Mode: modeBlocked, Command: "company tag delete"},
	{ID: "QBO.COMPANY.TAG_EDIT", Domain: "company", Risk: "R2", Mode: modeBlocked, Command: "company tag update"},
	{ID: "QBO.COMPANY.TAG_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company tag get"},
	{ID: "QBO.COMPANY.TERM_CREATE", Domain: "company", Risk: "R2", Mode: modeWired, Command: "company term create"},
	{ID: "QBO.COMPANY.TERM_DELETE", Domain: "company", Risk: "R2", Mode: modeWired, Command: "company term delete"},
	{ID: "QBO.COMPANY.TERM_EDIT", Domain: "company", Risk: "R2", Mode: modeWired, Command: "company term update edit"},
	{ID: "QBO.COMPANY.TERM_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company term get"},
	{ID: "QBO.COMPANY.USER_CREATE", Domain: "company", Risk: "R4", Mode: modeBlocked, Command: "company user create"},
	{ID: "QBO.COMPANY.USER_DELETE", Domain: "company", Risk: "R4", Mode: modeBlocked, Command: "company user delete"},
	{ID: "QBO.COMPANY.USER_EDIT", Domain: "company", Risk: "R4", Mode: modeBlocked, Command: "company user update"},
	{ID: "QBO.COMPANY.USER_READ", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company user get"},
	{ID: "QBO.COMPANY.WEBHOOK_VERIFY", Domain: "company", Risk: "R0", Mode: modeRead, Command: "company webhook verify"},
	{ID: "QBO.COSTGROUPS.GROUP_CREATE", Domain: "costgroups", Risk: "R2", Mode: modeWired, Command: "costgroups create"},
	{ID: "QBO.COSTGROUPS.GROUP_DELETE", Domain: "costgroups", Risk: "R2", Mode: modeWired, Command: "costgroups delete"},
	{ID: "QBO.COSTGROUPS.GROUP_LIST", Domain: "costgroups", Risk: "R0", Mode: modeRead, Command: "costgroups list"},
	{ID: "QBO.COSTGROUPS.GROUP_UPDATE", Domain: "costgroups", Risk: "R2", Mode: modeWired, Command: "costgroups update"},
	{ID: "QBO.CUSTOMERS.APPOINTMENT_READ", Domain: "customers", Risk: "R0", Mode: modeBlocked, Command: "customers appointment get"},
	{ID: "QBO.CUSTOMERS.APPOINTMENT_SEARCH", Domain: "customers", Risk: "R0", Mode: modeBlocked, Command: "customers appointment search"},
	{ID: "QBO.CUSTOMERS.CONTRACT_READ", Domain: "customers", Risk: "R0", Mode: modeRead, Command: "customers contract get"},
	{ID: "QBO.CUSTOMERS.CONTRACT_SEARCH", Domain: "customers", Risk: "R0", Mode: modeRead, Command: "customers contract search"},
	{ID: "QBO.CUSTOMERS.CUSTOMER_CREATE", Domain: "customers", Risk: "R4", Mode: modeWired, Command: "customers customer create"},
	{ID: "QBO.CUSTOMERS.CUSTOMER_CREDIT_TRANSFER", Domain: "customers", Risk: "R3", Mode: modeWired, Command: "customers customer run"},
	{ID: "QBO.CUSTOMERS.CUSTOMER_DELETE", Domain: "customers", Risk: "R4", Mode: modeWired, Command: "customers customer delete"},
	{ID: "QBO.CUSTOMERS.CUSTOMER_EDIT", Domain: "customers", Risk: "R4", Mode: modeWired, Command: "customers customer update edit"},
	{ID: "QBO.CUSTOMERS.CUSTOMER_MERGE", Domain: "customers", Risk: "R4", Mode: modeWired, Command: "customers customer update merge"},
	{ID: "QBO.CUSTOMERS.CUSTOMER_READ", Domain: "customers", Risk: "R0", Mode: modeRead, Command: "customers customer get"},
	{ID: "QBO.CUSTOMERS.CUSTOMER_SEARCH", Domain: "customers", Risk: "R0", Mode: modeRead, Command: "customers customer search"},
	{ID: "QBO.CUSTOMERS.CUSTOMER_TYPE_READ", Domain: "customers", Risk: "R0", Mode: modeRead, Command: "customers customer-type get"},
	{ID: "QBO.CUSTOMERS.CUSTOMER_TYPE_SEARCH", Domain: "customers", Risk: "R0", Mode: modeRead, Command: "customers customer-type search"},
	{ID: "QBO.CUSTOMERS.MAILCHIMP_NOTIFICATIONS_POST", Domain: "customers", Risk: "R1", Mode: modeBlocked, Command: "customers mailchimp-notifications post"},
	{ID: "QBO.CUSTOMERS.MCF_BACKEND_GET", Domain: "customers", Risk: "R0", Mode: modeBlocked, Command: "customers mcf-backend get"},
	{ID: "QBO.CUSTOMERS.MC_APPFABRIC_GET", Domain: "customers", Risk: "R0", Mode: modeBlocked, Command: "customers mc-appfabric get"},
	{ID: "QBO.CUSTOMERS.OPPORTUNITY_CREATE", Domain: "customers", Risk: "R2", Mode: modeBlocked, Command: "customers opportunity create"},
	{ID: "QBO.CUSTOMERS.OPPORTUNITY_READ", Domain: "customers", Risk: "R0", Mode: modeBlocked, Command: "customers opportunity get"},
	{ID: "QBO.CUSTOMERS.OPPORTUNITY_SEARCH", Domain: "customers", Risk: "R0", Mode: modeBlocked, Command: "customers opportunity search"},
	{ID: "QBO.CUSTOMERS.OVERVIEW_READ", Domain: "customers", Risk: "R0", Mode: modeRead, Command: "customers overview get"},
	{ID: "QBO.CUSTOMERS.PROMPT_EVALUATOR_POST", Domain: "customers", Risk: "R1", Mode: modeBlocked, Command: "customers prompt-evaluator post"},
	{ID: "QBO.CUSTOMERS.PROPOSAL_CREATE", Domain: "customers", Risk: "R2", Mode: modeWired, Command: "customers proposal create"},
	{ID: "QBO.CUSTOMERS.PROPOSAL_DELETE", Domain: "customers", Risk: "R2", Mode: modeWired, Command: "customers proposal delete"},
	{ID: "QBO.CUSTOMERS.PROPOSAL_EDIT", Domain: "customers", Risk: "R2", Mode: modeWired, Command: "customers proposal update"},
	{ID: "QBO.CUSTOMERS.PROPOSAL_READ", Domain: "customers", Risk: "R0", Mode: modeRead, Command: "customers proposal get"},
	{ID: "QBO.CUSTOMERS.PROPOSAL_SEARCH", Domain: "customers", Risk: "R0", Mode: modeRead, Command: "customers proposal search"},
	{ID: "QBO.CUSTOMERS.PROPOSAL_SVC_CREATE", Domain: "customers", Risk: "R2", Mode: modeWired, Command: "customers proposal-svc create"},
	{ID: "QBO.CUSTOMERS.REALTIME_ER_GET", Domain: "customers", Risk: "R0", Mode: modeBlocked, Command: "customers realtime-entity-resolution get"},
	{ID: "QBO.CUSTOMERS.REVIEWS_GET", Domain: "customers", Risk: "R0", Mode: modeBlocked, Command: "customers reviews get"},
	{ID: "QBO.CUSTOMERS.REVIEW_READ", Domain: "customers", Risk: "R0", Mode: modeRead, Command: "customers review get"},
	{ID: "QBO.CUSTOMERS.REVIEW_SEARCH", Domain: "customers", Risk: "R0", Mode: modeRead, Command: "customers review search"},
	{ID: "QBO.CUSTOMERS.SALES_AGENT_GET", Domain: "customers", Risk: "R0", Mode: modeBlocked, Command: "customers sales-agent get"},
	{ID: "QBO.CUSTOMERS.SALES_AGENT_PROMPT_POST", Domain: "customers", Risk: "R1", Mode: modeBlocked, Command: "customers sales-agent prompt post"},
	{ID: "QBO.CUSTOMOBJECTS.OBJECT_CREATE", Domain: "customobjects", Risk: "R4", Mode: modeBlocked, Command: "customobjects create"},
	{ID: "QBO.CUSTOMOBJECTS.OBJECT_DELETE", Domain: "customobjects", Risk: "R4", Mode: modeBlocked, Command: "customobjects delete"},
	{ID: "QBO.CUSTOMOBJECTS.OBJECT_UPDATE", Domain: "customobjects", Risk: "R4", Mode: modeBlocked, Command: "customobjects update"},
	{ID: "QBO.EXPENSES.B2B_BILLPAY_GET", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses b2b-billpay get"},
	{ID: "QBO.EXPENSES.B2B_NETWORK_GET", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses b2b-network get"},
	{ID: "QBO.EXPENSES.BANK_FEE_CREATE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses bank-fee create"},
	{ID: "QBO.EXPENSES.BILL_CREATE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses bill create"},
	{ID: "QBO.EXPENSES.BILL_DELETE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses bill delete"},
	{ID: "QBO.EXPENSES.BILL_EDIT", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses bill update edit"},
	{ID: "QBO.EXPENSES.BILL_FORM_DELETE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses bill form delete"},
	{ID: "QBO.EXPENSES.BILL_IMPORT", Domain: "expenses", Risk: "R3", Mode: modeWired, Command: "expenses bill import"},
	{ID: "QBO.EXPENSES.BILL_LIST_CREATE", Domain: "expenses", Risk: "R2", Mode: modeBlocked, Command: "expenses bill list create"},
	{ID: "QBO.EXPENSES.BILL_LIST_EDIT", Domain: "expenses", Risk: "R2", Mode: modeBlocked, Command: "expenses bill list edit"},
	{ID: "QBO.EXPENSES.BILL_PAY", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses bill update pay"},
	{ID: "QBO.EXPENSES.BILL_PAYMENT_CREATE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses bill-payment create"},
	{ID: "QBO.EXPENSES.BILL_PAYMENT_DELETE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses bill-payment delete"},
	{ID: "QBO.EXPENSES.BILL_PAYMENT_EDIT", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses bill-payment update"},
	{ID: "QBO.EXPENSES.BILL_PAYMENT_READ", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses bill-payment get"},
	{ID: "QBO.EXPENSES.BILL_PAY_ONBOARDING_GET", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses bill-pay-onboarding get"},
	{ID: "QBO.EXPENSES.BILL_PAY_SERVICE_CREATE", Domain: "expenses", Risk: "R3", Mode: modeBlocked, Command: "expenses bill-pay-service create"},
	{ID: "QBO.EXPENSES.BILL_READ", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses bill get"},
	{ID: "QBO.EXPENSES.BILL_SEARCH", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses bill search"},
	{ID: "QBO.EXPENSES.CHEQUE_BOUNCE", Domain: "expenses", Risk: "R3", Mode: modeWired, Command: "expenses cheque update bounce"},
	{ID: "QBO.EXPENSES.CHEQUE_CREATE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses cheque create"},
	{ID: "QBO.EXPENSES.CHEQUE_DELETE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses cheque delete"},
	{ID: "QBO.EXPENSES.CHEQUE_EDIT", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses cheque update edit"},
	{ID: "QBO.EXPENSES.CHEQUE_READ", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses cheque get"},
	{ID: "QBO.EXPENSES.CHEQUE_REPRINT", Domain: "expenses", Risk: "R1", Mode: modeWired, Command: "expenses cheque export"},
	{ID: "QBO.EXPENSES.CREDIT_CARD_PAYMENT_READ", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses credit-card-payment get"},
	{ID: "QBO.EXPENSES.CREDIT_CARD_PAYMENT_SEARCH", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses credit-card-payment search"},
	{ID: "QBO.EXPENSES.EXPENSE_CREATE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses expense create"},
	{ID: "QBO.EXPENSES.EXPENSE_DELETE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses expense delete"},
	{ID: "QBO.EXPENSES.EXPENSE_EDIT", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses expense update edit"},
	{ID: "QBO.EXPENSES.EXPENSE_FORM_DELETE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses expense form delete"},
	{ID: "QBO.EXPENSES.EXPENSE_LIST_CREATE", Domain: "expenses", Risk: "R2", Mode: modeBlocked, Command: "expenses expense list create"},
	{ID: "QBO.EXPENSES.EXPENSE_LIST_EDIT", Domain: "expenses", Risk: "R2", Mode: modeBlocked, Command: "expenses expense list edit"},
	{ID: "QBO.EXPENSES.EXPENSE_READ", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses expense get"},
	{ID: "QBO.EXPENSES.EXPENSE_RECATEGORISE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses expense update recategorise"},
	{ID: "QBO.EXPENSES.EXPENSE_SEARCH", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses expense search"},
	{ID: "QBO.EXPENSES.EXPENSE_SPLIT", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses expense update split"},
	{ID: "QBO.EXPENSES.GIFT_CERTIFICATE_CREATE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses gift-certificate create"},
	{ID: "QBO.EXPENSES.MILEAGE_CREATE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses mileage create"},
	{ID: "QBO.EXPENSES.MILEAGE_DELETE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses mileage delete"},
	{ID: "QBO.EXPENSES.MILEAGE_EDIT", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses mileage update"},
	{ID: "QBO.EXPENSES.MILEAGE_READ", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses mileage get"},
	{ID: "QBO.EXPENSES.MILEAGE_SEARCH", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses mileage search"},
	{ID: "QBO.EXPENSES.OVERVIEW_READ", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses overview get"},
	{ID: "QBO.EXPENSES.PAYMENTS_FUNDING_GET", Domain: "expenses", Risk: "R3", Mode: modeBlocked, Command: "expenses payments-funding get"},
	{ID: "QBO.EXPENSES.PAYMENT_ACCOUNT_GET", Domain: "expenses", Risk: "R2", Mode: modeRead, Command: "expenses payment-account get"},
	{ID: "QBO.EXPENSES.PURCHASE_SEARCH", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses purchase search"},
	{ID: "QBO.EXPENSES.PYMTS_APPS_ORCH_POST", Domain: "expenses", Risk: "R1", Mode: modeBlocked, Command: "expenses pymts-apps-orch post"},
	{ID: "QBO.EXPENSES.RECEIPT_DELETE", Domain: "expenses", Risk: "R2", Mode: modeBlocked, Command: "expenses receipt delete"},
	{ID: "QBO.EXPENSES.RECEIPT_EXPENSE_CREATE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses receipt-expense create"},
	{ID: "QBO.EXPENSES.RECEIPT_READ", Domain: "expenses", Risk: "R0", Mode: modeBlocked, Command: "expenses receipt get"},
	{ID: "QBO.EXPENSES.RECEIPT_SEARCH", Domain: "expenses", Risk: "R0", Mode: modeBlocked, Command: "expenses receipt search"},
	{ID: "QBO.EXPENSES.RECEIPT_UPLOAD", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses receipt import"},
	{ID: "QBO.EXPENSES.RESOLUTION_CENTER_GET", Domain: "expenses", Risk: "R0", Mode: modeBlocked, Command: "expenses resolution-center get"},
	{ID: "QBO.EXPENSES.SPENDMGMT_GET", Domain: "expenses", Risk: "R0", Mode: modeBlocked, Command: "expenses spendmgmt get"},
	{ID: "QBO.EXPENSES.STAGETXN_GET", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses stagetxn get"},
	{ID: "QBO.EXPENSES.STAGETXN_TEST_GET", Domain: "expenses", Risk: "R0", Mode: modeBlocked, Command: "expenses stagetxn-test get"},
	{ID: "QBO.EXPENSES.SUBSCRIPTION_SBG_GET", Domain: "expenses", Risk: "R0", Mode: modeBlocked, Command: "expenses subscription-sbg get"},
	{ID: "QBO.EXPENSES.SUPPLIER_CREATE", Domain: "expenses", Risk: "R4", Mode: modeWired, Command: "expenses supplier create"},
	{ID: "QBO.EXPENSES.SUPPLIER_DELETE", Domain: "expenses", Risk: "R4", Mode: modeWired, Command: "expenses supplier delete"},
	{ID: "QBO.EXPENSES.SUPPLIER_EDIT", Domain: "expenses", Risk: "R4", Mode: modeWired, Command: "expenses supplier update edit"},
	{ID: "QBO.EXPENSES.SUPPLIER_MERGE", Domain: "expenses", Risk: "R4", Mode: modeWired, Command: "expenses supplier update merge"},
	{ID: "QBO.EXPENSES.SUPPLIER_READ", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses supplier get"},
	{ID: "QBO.EXPENSES.SUPPLIER_SEARCH", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses supplier search"},
	{ID: "QBO.EXPENSES.TIME_ACTIVITY_CREATE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses time-activity create"},
	{ID: "QBO.EXPENSES.TIME_ACTIVITY_DELETE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses time-activity delete"},
	{ID: "QBO.EXPENSES.TIME_ACTIVITY_EDIT", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses time-activity update"},
	{ID: "QBO.EXPENSES.TIME_ACTIVITY_READ", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses time-activity get"},
	{ID: "QBO.EXPENSES.TIME_ACTIVITY_SEARCH", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses time-activity search"},
	{ID: "QBO.EXPENSES.TXN_INGESTION_POST", Domain: "expenses", Risk: "R1", Mode: modeBlocked, Command: "expenses txn-ingestion post"},
	{ID: "QBO.EXPENSES.VENDOR_CREDIT_CREATE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses vendor-credit create"},
	{ID: "QBO.EXPENSES.VENDOR_CREDIT_DELETE", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses vendor-credit delete"},
	{ID: "QBO.EXPENSES.VENDOR_CREDIT_EDIT", Domain: "expenses", Risk: "R2", Mode: modeWired, Command: "expenses vendor-credit update"},
	{ID: "QBO.EXPENSES.VENDOR_CREDIT_READ", Domain: "expenses", Risk: "R0", Mode: modeRead, Command: "expenses vendor-credit get"},
	{ID: "QBO.EXPENSES.WALLET_GET", Domain: "expenses", Risk: "R2", Mode: modeBlocked, Command: "expenses wallet get"},
	{ID: "QBO.FEED.ACCOUNT_LINK", Domain: "feed", Risk: "R3", Mode: modeBlocked, Command: "feed account run"},
	{ID: "QBO.FEED.ACCOUNT_LIST", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed account list"},
	{ID: "QBO.FEED.ACCOUNT_REFRESH", Domain: "feed", Risk: "R2", Mode: modeBlocked, Command: "feed account refresh"},
	{ID: "QBO.FEED.ACH_TRANSFER_CREATE", Domain: "feed", Risk: "R4", Mode: modeBlocked, Command: "feed ach-transfer create"},
	{ID: "QBO.FEED.BANK_ACCOUNT_LOOKUP_GET", Domain: "feed", Risk: "R0", Mode: modeBlocked, Command: "feed bank-account-lookup get"},
	{ID: "QBO.FEED.FINANCIAL_DATA_INTERACTION_POST", Domain: "feed", Risk: "R1", Mode: modeBlocked, Command: "feed financial-data-interaction post"},
	{ID: "QBO.FEED.FINANCIAL_DOCUMENT_GET", Domain: "feed", Risk: "R0", Mode: modeBlocked, Command: "feed financial-document get"},
	{ID: "QBO.FEED.FINANCIAL_PROFILE_GET", Domain: "feed", Risk: "R0", Mode: modeBlocked, Command: "feed financial-profile get"},
	{ID: "QBO.FEED.FINANCIAL_PROFILE_ORCH_POST", Domain: "feed", Risk: "R1", Mode: modeBlocked, Command: "feed financial-profile-orch post"},
	{ID: "QBO.FEED.FINANCIAL_PROVIDER_GET", Domain: "feed", Risk: "R0", Mode: modeBlocked, Command: "feed financial-provider get"},
	{ID: "QBO.FEED.REC_AUTO_ADJUST", Domain: "feed", Risk: "R3", Mode: modeWired, Command: "feed rec run"},
	{ID: "QBO.FEED.REC_REPORT", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed rec get"},
	{ID: "QBO.FEED.RULE_CREATE", Domain: "feed", Risk: "R4", Mode: modeWired, Command: "feed rule create"},
	{ID: "QBO.FEED.RULE_DELETE", Domain: "feed", Risk: "R4", Mode: modeWired, Command: "feed rule delete"},
	{ID: "QBO.FEED.RULE_EDIT", Domain: "feed", Risk: "R4", Mode: modeWired, Command: "feed rule update"},
	{ID: "QBO.FEED.RULE_READ", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed rule get"},
	{ID: "QBO.FEED.RULE_SEARCH", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed rule search"},
	{ID: "QBO.FEED.TXNS_ORCHESTRATION_POST", Domain: "feed", Risk: "R1", Mode: modeBlocked, Command: "feed txns-orchestration post"},
	{ID: "QBO.FEED.TXN_ATTACH", Domain: "feed", Risk: "R2", Mode: modeWired, Command: "feed txn run attach"},
	{ID: "QBO.FEED.TXN_BATCH_ACCEPT", Domain: "feed", Risk: "R3", Mode: modeWired, Command: "feed txn update batch-accept"},
	{ID: "QBO.FEED.TXN_CATEGORISE", Domain: "feed", Risk: "R3", Mode: modeWired, Command: "feed txn update categorise"},
	{ID: "QBO.FEED.TXN_EXCLUDE", Domain: "feed", Risk: "R2", Mode: modeWired, Command: "feed txn update exclude"},
	{ID: "QBO.FEED.TXN_EXCLUDED", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed txn list excluded"},
	{ID: "QBO.FEED.TXN_IMPORT", Domain: "feed", Risk: "R2", Mode: modeWired, Command: "feed txn import"},
	{ID: "QBO.FEED.TXN_LOOKUP", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed txn get"},
	{ID: "QBO.FEED.TXN_MATCH", Domain: "feed", Risk: "R3", Mode: modeWired, Command: "feed txn update match"},
	{ID: "QBO.FEED.TXN_PENDING", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed txn list"},
	{ID: "QBO.FEED.TXN_POPULATION", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed txn population"},
	{ID: "QBO.FEED.TXN_POPULATION_ALL", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed txn population-all"},
	{ID: "QBO.FEED.TXN_POSTED", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed txn list posted"},
	{ID: "QBO.FEED.TXN_SPLIT", Domain: "feed", Risk: "R3", Mode: modeWired, Command: "feed txn update split"},
	{ID: "QBO.FEED.TXN_TAG", Domain: "feed", Risk: "R2", Mode: modeBlocked, Command: "feed txn run tag"},
	{ID: "QBO.FEED.TXN_TRANSFER", Domain: "feed", Risk: "R3", Mode: modeWired, Command: "feed txn run transfer"},
	{ID: "QBO.FEED.TXN_UNDO_EXCLUDED", Domain: "feed", Risk: "R2", Mode: modeWired, Command: "feed txn update undo-excluded"},
	{ID: "QBO.FEED.TXN_UNPOST", Domain: "feed", Risk: "R3", Mode: modeWired, Command: "feed txn update unpost"},
	{ID: "QBO.FEED.TXN_VERIFY", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed txn verify"},
	{ID: "QBO.FEED.VAULT_GET", Domain: "feed", Risk: "R1", Mode: modeBlocked, Command: "feed vault get"},
	{ID: "QBO.GQL.BILLS_WALK", Domain: "gql", Risk: "R0", Mode: modeRead, Command: "gql walk bills"},
	{ID: "QBO.GQL.ITEMS_WALK", Domain: "gql", Risk: "R0", Mode: modeRead, Command: "gql walk items"},
	{ID: "QBO.GQL.MUTATE_ACTIVATE_PRODUCTS", Domain: "gql", Risk: "R2", Mode: modeWired, Command: "gql mutate activate-products"},
	{ID: "QBO.GQL.MUTATE_ASSIGN_DIMENSIONS", Domain: "gql", Risk: "R2", Mode: modeWired, Command: "gql mutate assign-dimensions"},
	{ID: "QBO.GQL.MUTATE_BANK_DISCONNECT", Domain: "gql", Risk: "R3", Mode: modeWired, Command: "gql mutate bank-disconnect"},
	{ID: "QBO.GQL.MUTATE_BATCH_UPDATE_PRODUCTS", Domain: "gql", Risk: "R2", Mode: modeWired, Command: "gql mutate batch-update-products"},
	{ID: "QBO.GQL.MUTATE_CANCEL_RECONCILE", Domain: "gql", Risk: "R3", Mode: modeWired, Command: "gql mutate cancel-reconcile"},
	{ID: "QBO.GQL.MUTATE_CONSUME_INVENTORY", Domain: "gql", Risk: "R2", Mode: modeWired, Command: "gql mutate consume-inventory"},
	{ID: "QBO.GQL.MUTATE_CREATE_ACCOUNT", Domain: "gql", Risk: "R2", Mode: modeWired, Command: "gql mutate create-account"},
	{ID: "QBO.GQL.MUTATE_RECEIVE_INVENTORY", Domain: "gql", Risk: "R2", Mode: modeWired, Command: "gql mutate receive-inventory"},
	{ID: "QBO.GQL.TASKS_WALK", Domain: "gql", Risk: "R0", Mode: modeRead, Command: "gql walk tasks"},
	{ID: "QBO.INTEGRATIONS.APPFLOW_RUN", Domain: "integrations", Risk: "R3", Mode: modeBlocked, Command: "integrations appflow run"},
	{ID: "QBO.INTEGRATIONS.IDX_GET", Domain: "integrations", Risk: "R0", Mode: modeRead, Command: "integrations idx get"},
	{ID: "QBO.INTEGRATIONS.V4_AWS_POST", Domain: "integrations", Risk: "R1", Mode: modeBlocked, Command: "integrations v4-aws post"},
	{ID: "QBO.INTEGRATIONS.WORKFLOW_AUTOMATION_GET", Domain: "integrations", Risk: "R0", Mode: modeRead, Command: "integrations workflowautomation get"},
	{ID: "QBO.INVENTORY.ADJUST_CREATE", Domain: "inventory", Risk: "R3", Mode: modeWired, Command: "inventory adjust create"},
	{ID: "QBO.INVENTORY.CONSIGNMENT_CREATE", Domain: "inventory", Risk: "R2", Mode: modeBlocked, Command: "inventory consignment create"},
	{ID: "QBO.INVENTORY.INVENTORY_AGENTS_POST", Domain: "inventory", Risk: "R1", Mode: modeBlocked, Command: "inventory agents post"},
	{ID: "QBO.INVENTORY.INVENTORY_COST_GET", Domain: "inventory", Risk: "R0", Mode: modeBlocked, Command: "inventory cost-accounting get"},
	{ID: "QBO.INVENTORY.ITEM_CREATE", Domain: "inventory", Risk: "R4", Mode: modeWired, Command: "inventory item create"},
	{ID: "QBO.INVENTORY.ITEM_DELETE", Domain: "inventory", Risk: "R4", Mode: modeWired, Command: "inventory item delete"},
	{ID: "QBO.INVENTORY.ITEM_EDIT", Domain: "inventory", Risk: "R4", Mode: modeWired, Command: "inventory item update"},
	{ID: "QBO.INVENTORY.ITEM_MANAGEMENT_GET", Domain: "inventory", Risk: "R0", Mode: modeBlocked, Command: "inventory item-svc get"},
	{ID: "QBO.INVENTORY.ITEM_READ", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory item get"},
	{ID: "QBO.INVENTORY.ITEM_RECEIPT_CREATE", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory item-receipt create"},
	{ID: "QBO.INVENTORY.ITEM_RECEIPT_EDIT", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory item-receipt update"},
	{ID: "QBO.INVENTORY.ITEM_RECEIPT_READ", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory item-receipt get"},
	{ID: "QBO.INVENTORY.ITEM_RECEIPT_SEARCH", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory item-receipt search"},
	{ID: "QBO.INVENTORY.ITEM_SEARCH", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory item search"},
	{ID: "QBO.INVENTORY.OVERVIEW_READ", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory overview get"},
	{ID: "QBO.INVENTORY.OVERVIEW_SEARCH", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory overview search"},
	{ID: "QBO.INVENTORY.PURCHASE_ORDER_CREATE", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory purchase-order create"},
	{ID: "QBO.INVENTORY.PURCHASE_ORDER_DELETE", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory purchase-order delete"},
	{ID: "QBO.INVENTORY.PURCHASE_ORDER_EDIT", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory purchase-order update edit"},
	{ID: "QBO.INVENTORY.PURCHASE_ORDER_PARTIAL", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory purchase-order update partial"},
	{ID: "QBO.INVENTORY.PURCHASE_ORDER_READ", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory purchase-order get"},
	{ID: "QBO.INVENTORY.PURCHASE_ORDER_SEARCH", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory purchase-order search"},
	{ID: "QBO.INVENTORY.QBBULKACTION_POST", Domain: "inventory", Risk: "R2", Mode: modeBlocked, Command: "inventory bulk-action post"},
	{ID: "QBO.INVENTORY.WAREHOUSE_SERVICE_GET", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory warehouse-service get"},
	{ID: "QBO.PAYROLL.BPAY_CREATE", Domain: "payroll", Risk: "R2", Mode: modeBlocked, Command: "payroll bpay create"},
	{ID: "QBO.PAYROLL.DEDUCTION_CREATE", Domain: "payroll", Risk: "R4", Mode: modeBlocked, Command: "payroll deduction create"},
	{ID: "QBO.PAYROLL.DEDUCTION_EDIT", Domain: "payroll", Risk: "R4", Mode: modeBlocked, Command: "payroll deduction update"},
	{ID: "QBO.PAYROLL.DEDUCTION_READ", Domain: "payroll", Risk: "R0", Mode: modeBlocked, Command: "payroll deduction get"},
	{ID: "QBO.PAYROLL.EMPLOYEE_CREATE", Domain: "payroll", Risk: "R2", Mode: modeWired, Command: "payroll employee create"},
	{ID: "QBO.PAYROLL.EMPLOYEE_EDIT", Domain: "payroll", Risk: "R2", Mode: modeWired, Command: "payroll employee update edit"},
	{ID: "QBO.PAYROLL.EMPLOYEE_READ", Domain: "payroll", Risk: "R0", Mode: modeRead, Command: "payroll employee get"},
	{ID: "QBO.PAYROLL.EMPLOYEE_SEARCH", Domain: "payroll", Risk: "R0", Mode: modeRead, Command: "payroll employee search"},
	{ID: "QBO.PAYROLL.EMPLOYEE_TERMINATE", Domain: "payroll", Risk: "R4", Mode: modeWired, Command: "payroll employee delete"},
	{ID: "QBO.PAYROLL.LEAVE_BALANCE_EDIT", Domain: "payroll", Risk: "R3", Mode: modeBlocked, Command: "payroll leave-balance update"},
	{ID: "QBO.PAYROLL.LEAVE_CATEGORY_CREATE", Domain: "payroll", Risk: "R4", Mode: modeBlocked, Command: "payroll leave-category create"},
	{ID: "QBO.PAYROLL.LEAVE_CATEGORY_EDIT", Domain: "payroll", Risk: "R4", Mode: modeBlocked, Command: "payroll leave-category update"},
	{ID: "QBO.PAYROLL.LEAVE_CATEGORY_READ", Domain: "payroll", Risk: "R0", Mode: modeBlocked, Command: "payroll leave-category get"},
	{ID: "QBO.PAYROLL.LUMP_SUM_CREATE", Domain: "payroll", Risk: "R3", Mode: modeBlocked, Command: "payroll lump-sum create"},
	{ID: "QBO.PAYROLL.OVERVIEW_SEARCH", Domain: "payroll", Risk: "R0", Mode: modeBlocked, Command: "payroll overview search"},
	{ID: "QBO.PAYROLL.PAYG_SUMMARY_CREATE", Domain: "payroll", Risk: "R3", Mode: modeBlocked, Command: "payroll payg-summary create"},
	{ID: "QBO.PAYROLL.PAY_CATEGORY_CREATE", Domain: "payroll", Risk: "R4", Mode: modeBlocked, Command: "payroll pay-category create"},
	{ID: "QBO.PAYROLL.PAY_CATEGORY_EDIT", Domain: "payroll", Risk: "R4", Mode: modeBlocked, Command: "payroll pay-category update"},
	{ID: "QBO.PAYROLL.PAY_CATEGORY_READ", Domain: "payroll", Risk: "R0", Mode: modeBlocked, Command: "payroll pay-category get"},
	{ID: "QBO.PAYROLL.PAY_RUN_CREATE", Domain: "payroll", Risk: "R3", Mode: modeBlocked, Command: "payroll pay-run create"},
	{ID: "QBO.PAYROLL.PAY_RUN_INCLUSION_CREATE", Domain: "payroll", Risk: "R3", Mode: modeBlocked, Command: "payroll pay-run-inclusion create"},
	{ID: "QBO.PAYROLL.PAY_RUN_READ", Domain: "payroll", Risk: "R0", Mode: modeBlocked, Command: "payroll pay-run get"},
	{ID: "QBO.PAYROLL.SETUP_RUN", Domain: "payroll", Risk: "R4", Mode: modeBlocked, Command: "payroll setup run"},
	{ID: "QBO.PAYROLL.STP_CLOSELY_HELD_LODGE", Domain: "payroll", Risk: "R4", Mode: modeBlocked, Command: "payroll stp-closely-held run"},
	{ID: "QBO.PAYROLL.STP_FINALISE", Domain: "payroll", Risk: "R4", Mode: modeBlocked, Command: "payroll stp run finalise"},
	{ID: "QBO.PAYROLL.STP_MICRO_LODGE", Domain: "payroll", Risk: "R4", Mode: modeBlocked, Command: "payroll stp-micro run"},
	{ID: "QBO.PAYROLL.STP_PAY_CREATE", Domain: "payroll", Risk: "R3", Mode: modeBlocked, Command: "payroll stp-pay create"},
	{ID: "QBO.PAYROLL.STP_RESET", Domain: "payroll", Risk: "R4", Mode: modeBlocked, Command: "payroll stp run reset"},
	{ID: "QBO.PAYROLL.STP_UPDATE_CREATE", Domain: "payroll", Risk: "R3", Mode: modeBlocked, Command: "payroll stp-update create"},
	{ID: "QBO.PAYROLL.SUPER_PAY", Domain: "payroll", Risk: "R3", Mode: modeBlocked, Command: "payroll super update"},
	{ID: "QBO.PAYROLL.SUPER_READ", Domain: "payroll", Risk: "R0", Mode: modeBlocked, Command: "payroll super get"},
	{ID: "QBO.PAYROLL.SUPER_RESC_EDIT", Domain: "payroll", Risk: "R3", Mode: modeBlocked, Command: "payroll super-resc update"},
	{ID: "QBO.PAYROLL.TEAM_READ", Domain: "payroll", Risk: "R0", Mode: modeBlocked, Command: "payroll team get"},
	{ID: "QBO.PAYROLL.TEAM_SEARCH", Domain: "payroll", Risk: "R0", Mode: modeBlocked, Command: "payroll team search"},
	{ID: "QBO.PAYROLL.TIMESHEET_CREATE", Domain: "payroll", Risk: "R2", Mode: modeWired, Command: "payroll timesheet create"},
	{ID: "QBO.PAYROLL.TIMESHEET_EDIT", Domain: "payroll", Risk: "R2", Mode: modeWired, Command: "payroll timesheet update"},
	{ID: "QBO.PAYROLL.TIMESHEET_READ", Domain: "payroll", Risk: "R0", Mode: modeRead, Command: "payroll timesheet get"},
	{ID: "QBO.PAYROLL.WORKCOVER_CREATE", Domain: "payroll", Risk: "R3", Mode: modeBlocked, Command: "payroll workcover create"},
	{ID: "QBO.REPORTS.ACCOUNT_LIST_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports account-list get"},
	{ID: "QBO.REPORTS.AGED_PAYABLES_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports aged-payables get"},
	{ID: "QBO.REPORTS.AGED_PAYABLE_DETAIL_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports aged-payable-detail get"},
	{ID: "QBO.REPORTS.AGED_RECEIVABLES_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports aged-receivables get"},
	{ID: "QBO.REPORTS.AGED_RECEIVABLE_DETAIL_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports aged-receivable-detail get"},
	{ID: "QBO.REPORTS.BALANCE_SHEET_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports balance-sheet get"},
	{ID: "QBO.REPORTS.CASH_FLOW_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports cash-flow get"},
	{ID: "QBO.REPORTS.CLASS_SALES_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports class-sales get"},
	{ID: "QBO.REPORTS.CUSTOMER_BALANCE_DETAIL_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports customer-balance-detail get"},
	{ID: "QBO.REPORTS.CUSTOMER_BALANCE_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports customer-balance get"},
	{ID: "QBO.REPORTS.CUSTOMER_INCOME_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports customer-income get"},
	{ID: "QBO.REPORTS.CUSTOMER_SALES_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports customer-sales get"},
	{ID: "QBO.REPORTS.CUSTOM_CREATE", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports custom create"},
	{ID: "QBO.REPORTS.CUSTOM_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports custom get"},
	{ID: "QBO.REPORTS.DEPARTMENT_SALES_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports department-sales get"},
	{ID: "QBO.REPORTS.FORECAST_CREATE", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports forecast create"},
	{ID: "QBO.REPORTS.FORECAST_DELETE", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports forecast delete"},
	{ID: "QBO.REPORTS.FORECAST_EDIT", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports forecast update"},
	{ID: "QBO.REPORTS.FORECAST_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports forecast get"},
	{ID: "QBO.REPORTS.GENERAL_LEDGER_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports general-ledger get"},
	{ID: "QBO.REPORTS.INVENTORY_VALUATION_DETAIL_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports inventory-valuation-detail get"},
	{ID: "QBO.REPORTS.INVENTORY_VALUATION_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports inventory-valuation get"},
	{ID: "QBO.REPORTS.ITEM_SALES_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports item-sales get"},
	{ID: "QBO.REPORTS.JOURNAL_REPORT_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports journal-report get"},
	{ID: "QBO.REPORTS.MANAGEMENT_CREATE", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports management create"},
	{ID: "QBO.REPORTS.MANAGEMENT_EDIT", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports management update"},
	{ID: "QBO.REPORTS.MANAGEMENT_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports management get"},
	{ID: "QBO.REPORTS.PERFORMANCE_CREATE", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports performance create"},
	{ID: "QBO.REPORTS.PERFORMANCE_DELETE", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports performance delete"},
	{ID: "QBO.REPORTS.PERFORMANCE_EDIT", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports performance update"},
	{ID: "QBO.REPORTS.PERFORMANCE_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports performance get"},
	{ID: "QBO.REPORTS.PROFIT_LOSS_DETAIL_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports profit-loss-detail get"},
	{ID: "QBO.REPORTS.PROFIT_LOSS_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports profit-loss get"},
	{ID: "QBO.REPORTS.REPORT_CREATE", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports report create"},
	{ID: "QBO.REPORTS.REPORT_DELETE", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports report delete"},
	{ID: "QBO.REPORTS.REPORT_EDIT", Domain: "reports", Risk: "R2", Mode: modeWired, Command: "reports report update"},
	{ID: "QBO.REPORTS.REPORT_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports report get"},
	{ID: "QBO.REPORTS.TRANSACTION_LIST_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports transaction-list get"},
	{ID: "QBO.REPORTS.TRIAL_BALANCE_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports trial-balance get"},
	{ID: "QBO.REPORTS.VENDOR_BALANCE_DETAIL_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports vendor-balance-detail get"},
	{ID: "QBO.REPORTS.VENDOR_BALANCE_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports vendor-balance get"},
	{ID: "QBO.REPORTS.VENDOR_EXPENSES_READ", Domain: "reports", Risk: "R0", Mode: modeRead, Command: "reports vendor-expenses get"},
	{ID: "QBO.SALES.COMMERCE_CONTROL_GET", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales commerce-control get"},
	{ID: "QBO.SALES.CREDIT_CARD_CREDIT_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales credit-card-credit create"},
	{ID: "QBO.SALES.CREDIT_MEMO_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales credit-memo create"},
	{ID: "QBO.SALES.CREDIT_MEMO_DELETE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales credit-memo delete"},
	{ID: "QBO.SALES.CREDIT_MEMO_EDIT", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales credit-memo update"},
	{ID: "QBO.SALES.CREDIT_MEMO_PDF", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales credit-memo pdf"},
	{ID: "QBO.SALES.CREDIT_MEMO_READ", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales credit-memo get"},
	{ID: "QBO.SALES.CREDIT_MEMO_SEND", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales credit-memo run send"},
	{ID: "QBO.SALES.DELAYED_CHARGE_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales delayed-charge create"},
	{ID: "QBO.SALES.DELAYED_CREDIT_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales delayed-credit create"},
	{ID: "QBO.SALES.ESTIMATE_COPY", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales estimate copy"},
	{ID: "QBO.SALES.ESTIMATE_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales estimate create"},
	{ID: "QBO.SALES.ESTIMATE_DELETE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales estimate delete"},
	{ID: "QBO.SALES.ESTIMATE_EDIT", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales estimate update"},
	{ID: "QBO.SALES.ESTIMATE_PDF", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales estimate pdf"},
	{ID: "QBO.SALES.ESTIMATE_READ", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales estimate get"},
	{ID: "QBO.SALES.ESTIMATE_SEARCH", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales estimate search"},
	{ID: "QBO.SALES.ESTIMATE_SEND", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales estimate run send"},
	{ID: "QBO.SALES.HUB_READ", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales hub get"},
	{ID: "QBO.SALES.HUB_SEARCH", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales hub search"},
	{ID: "QBO.SALES.INVOICE_COPY", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales invoice copy"},
	{ID: "QBO.SALES.INVOICE_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales invoice create"},
	{ID: "QBO.SALES.INVOICE_CUSTOMISE", Domain: "sales", Risk: "R2", Mode: modeBlocked, Command: "sales invoice update customise"},
	{ID: "QBO.SALES.INVOICE_DELETE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales invoice delete"},
	{ID: "QBO.SALES.INVOICE_DISCOUNT", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales invoice update discount"},
	{ID: "QBO.SALES.INVOICE_EDIT", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales invoice update edit"},
	{ID: "QBO.SALES.INVOICE_FORM_DELETE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales invoice form delete"},
	{ID: "QBO.SALES.INVOICE_LIST_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales invoice list create"},
	{ID: "QBO.SALES.INVOICE_LIST_EDIT", Domain: "sales", Risk: "R2", Mode: modeBlocked, Command: "sales invoice list edit"},
	{ID: "QBO.SALES.INVOICE_PDF", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales invoice pdf"},
	{ID: "QBO.SALES.INVOICE_PROGRESS", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales invoice update progress"},
	{ID: "QBO.SALES.INVOICE_READ", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales invoice get"},
	{ID: "QBO.SALES.INVOICE_REMIND", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales invoice run remind"},
	{ID: "QBO.SALES.INVOICE_SEARCH", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales invoice search"},
	{ID: "QBO.SALES.INVOICE_SEND", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales invoice run send"},
	{ID: "QBO.SALES.INVOICE_WRITE_OFF", Domain: "sales", Risk: "R3", Mode: modeWired, Command: "sales invoice update write-off"},
	{ID: "QBO.SALES.KNOWLEDGE_BASE_SEARCH", Domain: "sales", Risk: "R0", Mode: modeBlocked, Command: "sales knowledge-base search"},
	{ID: "QBO.SALES.LATE_FEE_CREATE", Domain: "sales", Risk: "R2", Mode: modeBlocked, Command: "sales late-fee create"},
	{ID: "QBO.SALES.OVERVIEW_READ", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales overview get"},
	{ID: "QBO.SALES.PAYMENT_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales payment create"},
	{ID: "QBO.SALES.PAYMENT_DELETE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales payment delete"},
	{ID: "QBO.SALES.PAYMENT_EDIT", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales payment update"},
	{ID: "QBO.SALES.PAYMENT_READ", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales payment get"},
	{ID: "QBO.SALES.QBONLINE_GRAPHQL_GET", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales qbonline-gateway get"},
	{ID: "QBO.SALES.RECEIPT_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales receipt create"},
	{ID: "QBO.SALES.RECEIPT_DELETE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales receipt delete"},
	{ID: "QBO.SALES.RECEIPT_EDIT", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales receipt update"},
	{ID: "QBO.SALES.RECEIPT_PDF", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales receipt pdf"},
	{ID: "QBO.SALES.RECEIPT_READ", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales receipt get"},
	{ID: "QBO.SALES.RECEIPT_SEND", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales receipt run send"},
	{ID: "QBO.SALES.REFUND_RECEIPT_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales refund-receipt create"},
	{ID: "QBO.SALES.REFUND_RECEIPT_DELETE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales refund-receipt delete"},
	{ID: "QBO.SALES.REFUND_RECEIPT_EDIT", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales refund-receipt update"},
	{ID: "QBO.SALES.REFUND_RECEIPT_READ", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales refund-receipt get"},
	{ID: "QBO.SALES.SALESTXN_RISK_GET", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales salestransactions-risk get"},
	{ID: "QBO.SALES.SALES_AI_CALL_POST", Domain: "sales", Risk: "R1", Mode: modeBlocked, Command: "sales sales-ai-call post"},
	{ID: "QBO.SALES.SALES_CHECKOUT_GET", Domain: "sales", Risk: "R1", Mode: modeBlocked, Command: "sales checkout get"},
	{ID: "QBO.SALES.SALES_ORDER_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales sales-order create"},
	{ID: "QBO.SALES.SALES_ORDER_DELETE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales sales-order delete"},
	{ID: "QBO.SALES.SALES_ORDER_EDIT", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales sales-order update"},
	{ID: "QBO.SALES.SALES_ORDER_READ", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales sales-order get"},
	{ID: "QBO.SALES.SALES_ORDER_SEARCH", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales sales-order search"},
	{ID: "QBO.SALES.SALES_SETTINGS_GET", Domain: "sales", Risk: "R0", Mode: modeRead, Command: "sales salesettings get"},
	{ID: "QBO.SALES.SALES_TRAFFIC_ROUTER_POST", Domain: "sales", Risk: "R0", Mode: modeBlocked, Command: "sales traffic-router post"},
	{ID: "QBO.SALES.STATEMENT_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales statement create"},
	{ID: "QBO.SALES.TIME_INVOICE_CREATE", Domain: "sales", Risk: "R2", Mode: modeWired, Command: "sales time-invoice create"},
	{ID: "QBO.SALES.TXNS_RENDERING_POST", Domain: "sales", Risk: "R1", Mode: modeBlocked, Command: "sales txns-rendering post"},
	{ID: "QBO.TAX.ATO_CONNECT", Domain: "tax", Risk: "R4", Mode: modeBlocked, Command: "tax ato run"},
	{ID: "QBO.TAX.BAS_LODGE", Domain: "tax", Risk: "R4", Mode: modeBlocked, Command: "tax bas run"},
	{ID: "QBO.TAX.BAS_READ", Domain: "tax", Risk: "R0", Mode: modeRead, Command: "tax bas get"},
	{ID: "QBO.TAX.CODE_CREATE", Domain: "tax", Risk: "R4", Mode: modeWired, Command: "tax code create"},
	{ID: "QBO.TAX.CODE_DELETE", Domain: "tax", Risk: "R4", Mode: modeBlocked, Command: "tax code delete"},
	{ID: "QBO.TAX.CODE_EDIT", Domain: "tax", Risk: "R4", Mode: modeBlocked, Command: "tax code update"},
	{ID: "QBO.TAX.CODE_READ", Domain: "tax", Risk: "R0", Mode: modeRead, Command: "tax code get"},
	{ID: "QBO.TAX.CODE_SEARCH", Domain: "tax", Risk: "R0", Mode: modeRead, Command: "tax code search"},
	{ID: "QBO.TAX.EXPERIMENT_ASSIGNMENT_GET", Domain: "tax", Risk: "R0", Mode: modeRead, Command: "tax experiment-assignment get"},
	{ID: "QBO.TAX.GST_AMENDMENTS_READ", Domain: "tax", Risk: "R0", Mode: modeBlocked, Command: "tax gst-amendments get"},
	{ID: "QBO.TAX.GST_DEFERRED_CREATE", Domain: "tax", Risk: "R3", Mode: modeBlocked, Command: "tax gst-deferred create"},
	{ID: "QBO.TAX.GST_PAYMENT_CREATE", Domain: "tax", Risk: "R3", Mode: modeBlocked, Command: "tax gst-payment create"},
	{ID: "QBO.TAX.GST_PAYMENT_DELETE", Domain: "tax", Risk: "R3", Mode: modeBlocked, Command: "tax gst-payment delete"},
	{ID: "QBO.TAX.GST_REFUND_CREATE", Domain: "tax", Risk: "R3", Mode: modeBlocked, Command: "tax gst-refund create"},
	{ID: "QBO.TAX.GST_SETTINGS_EDIT", Domain: "tax", Risk: "R4", Mode: modeBlocked, Command: "tax gst-settings update"},
	{ID: "QBO.TAX.GST_SETUP", Domain: "tax", Risk: "R4", Mode: modeBlocked, Command: "tax gst run"},
	{ID: "QBO.TAX.IAS_READ", Domain: "tax", Risk: "R0", Mode: modeRead, Command: "tax ias get"},
	{ID: "QBO.TAX.INDIRECT_REPORTS_GET", Domain: "tax", Risk: "R0", Mode: modeRead, Command: "tax indirect-reports get"},
	{ID: "QBO.TAX.LODGEIT_EXPORT", Domain: "tax", Risk: "R2", Mode: modeBlocked, Command: "tax lodgeit export"},
	{ID: "QBO.TAX.NEXUS_GET", Domain: "tax", Risk: "R0", Mode: modeRead, Command: "tax nexus get"},
	{ID: "QBO.TAX.TAX_AGENCY_CREATE", Domain: "tax", Risk: "R2", Mode: modeWired, Command: "tax tax-agency create"},
	{ID: "QBO.TAX.TAX_AGENCY_READ", Domain: "tax", Risk: "R0", Mode: modeRead, Command: "tax tax-agency get"},
	{ID: "QBO.TAX.TAX_CALCULATION_POST", Domain: "tax", Risk: "R1", Mode: modeBlocked, Command: "tax taxcalc post"},
	{ID: "QBO.TAX.TAX_FILING_GET", Domain: "tax", Risk: "R2", Mode: modeBlocked, Command: "tax filing-svc get"},
	{ID: "QBO.TAX.TAX_RATE_CREATE", Domain: "tax", Risk: "R2", Mode: modeBlocked, Command: "tax tax-rate create"},
	{ID: "QBO.TAX.TAX_RATE_READ", Domain: "tax", Risk: "R0", Mode: modeRead, Command: "tax tax-rate get"},
	{ID: "QBO.TAX.TPAR_EXPORT", Domain: "tax", Risk: "R2", Mode: modeBlocked, Command: "tax tpar export"},
	{ID: "QBO.TAX.TPAR_READ", Domain: "tax", Risk: "R0", Mode: modeRead, Command: "tax tpar get"},
}

// newActionsCmd builds the `qb actions` command: the machine-readable
// catalog. --mode filters to one mode; unknown modes are usage errors.
func newActionsCmd(flags *rootFlags) *cobra.Command {
	var mode string
	cmd := &cobra.Command{
		Use:   "actions",
		Short: "List catalogued QBO primitives",
		Long: "Agent contract: qb actions --json lists every primitive with usage, params, and notes.\n" +
			"Commands are qb <domain> <entity> <verb>. mode=read/wired honour flags.\n" +
			"mode=blocked documents flags then returns not-wired. mode=excluded has no command.\n" +
			"Wired mutations without readback adapters are also blocked; inspect the JSON verification field.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if mode != "" && mode != "read" && mode != "wired" && mode != "blocked" && mode != "excluded" {
				return &ExitError{Code: ExitInputError, Err: fmt.Errorf("unknown mode %q", mode)}
			}
			rows := catalogPrimitives
			if mode != "" {
				rows = filterModes(rows, mode)
			}
			if flags.asJSON {
				return writeActionsJSON(cmd, rows)
			}
			return writeActionsText(cmd, rows)
		},
	}
	cmd.Flags().StringVar(&mode, "mode", "", "only show rows of this mode: read|wired|blocked|excluded")
	return cmd
}

// filterModes keeps only the rows whose mode equals m.
func filterModes(rows []primitiveEntry, m string) []primitiveEntry {
	var out []primitiveEntry
	for _, e := range rows {
		if string(e.Mode) == m {
			out = append(out, e)
		}
	}
	return out
}

// writeActionsJSON emits the rows as a JSON array of documented entries
// (usage, params, notes filled — the contract agents parse).
func writeActionsJSON(cmd *cobra.Command, rows []primitiveEntry) error {
	out := make([]primitiveEntry, 0, len(rows))
	for _, e := range rows {
		entry := documentedEntry(e)
		if e.Mode == modeWired && e.Risk != "R0" {
			target, _, err := cmd.Root().Find(strings.Fields(e.Command))
			entry.Verification = "blocked_missing_readback"
			if err == nil && target != nil && hasReadbackAdapter(target, e) {
				entry.Verification = "independent_readback"
			}
		}
		out = append(out, entry)
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// writeActionsText renders the human table plus usage lines.
func writeActionsText(cmd *cobra.Command, rows []primitiveEntry) error {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "%-46s %-12s %-4s %-9s %s\n", "ID", "DOMAIN", "RISK", "MODE", "COMMAND")
	for _, e := range rows {
		fmt.Fprintf(out, "%-46s %-12s %-4s %-9s %s\n", e.ID, e.Domain, e.Risk, string(e.Mode), e.Command)
		if u := usageFor(e); u != "" {
			fmt.Fprintf(out, "    %s\n", strings.SplitN(u, "\n", 2)[0])
		}
		if e.Mode == modeWired && e.Risk != "R0" {
			target, _, err := cmd.Root().Find(strings.Fields(e.Command))
			state := "blocked: missing independent readback"
			if err == nil && target != nil && hasReadbackAdapter(target, e) {
				state = "independent readback required"
			}
			fmt.Fprintf(out, "    verification: %s\n", state)
		}
	}
	return nil
}

// catalogModeTotals tallies the whole catalog by mode: total counts equal
// the sum regardless of domain. Single source for doctor coverage and help
// text.
func catalogModeTotals() (wired, read, blocked, excluded, total int) {
	for _, e := range catalogPrimitives {
		total++
		switch e.Mode {
		case modeWired:
			wired++
		case modeRead:
			read++
		case modeBlocked:
			blocked++
		case modeExcluded:
			excluded++
		}
	}
	return wired, read, blocked, excluded, total
}

// coverageFor summarises one domain from the shared action catalog - the same
// source that feeds `qb actions` - as "Coverage: N wired, M read-only, K
// blocked". Domains with no catalog entries return "".
func coverageFor(domain string) string {
	var w, r, b int
	seen := false
	for _, e := range catalogPrimitives {
		if e.Domain != domain {
			continue
		}
		seen = true
		switch e.Mode {
		case modeWired:
			w++
		case modeRead:
			r++
		case modeBlocked:
			b++
		}
	}
	if !seen {
		return ""
	}
	return fmt.Sprintf("Coverage: %d wired, %d read-only, %d blocked", w, r, b)
}

// coverageLine feeds the help template: the root gets the catalog-wide
// line, domain groups get their coverageFor line, leaves stay stock.
func coverageLine(cmd *cobra.Command) string {
	if cmd == nil || !cmd.HasSubCommands() {
		return ""
	}
	if cmd.Parent() == nil {
		w, r, b, e, t := catalogModeTotals()
		return fmt.Sprintf("\n\nCoverage: %d wired, %d read-only, %d blocked, %d excluded of %d actions", w, r, b, e, t)
	}
	if s := coverageFor(cmd.Name()); s != "" {
		return "\n\n" + s
	}
	return ""
}

// appendCoverageCheck extends a doctor result with an informational coverage
// check derived from the catalog. It always reports ok and never flips
// OverallOK, even for a wholly failing session result.
func appendCoverageCheck(res doctor.DoctorResult) doctor.DoctorResult {
	w, r, b, e, t := catalogModeTotals()
	res.Checks = append(res.Checks, doctor.Check{
		Name:   "coverage",
		OK:     true,
		Detail: fmt.Sprintf("%d wired / %d read / %d blocked / %d excluded across %d actions", w, r, b, e, t),
	})
	return res
}
