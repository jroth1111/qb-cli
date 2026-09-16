// Package routes shares verified GraphQL endpoint routing between the
// catalog generator and runtime without depending on generated output.
package routes

// GraphQL hosts used by the captured operations.
const (
	EndpointDefault         = "https://qbo.intuit.com/api/v4/graphql"
	EndpointWarehouse       = "https://warehouse-management-svc.api.intuit.com/graphql"
	EndpointCommerceControl = "https://commercecontrol.api.intuit.com/graphql"
	EndpointSpendLists      = "https://smallbusiness.api.intuit.com/graphql"
	EndpointCoaCore         = "https://coa-core.api.intuit.com/graphql"
)

// opEndpoints retains the generator's hand-verified routing evidence from
// the /tmp/qbo-cap corpus: warehouse inventory operations came from
// order-management-ui chunk 3275 and inventory-addon-ui chunk 1156;
// smallbusiness operations came from tasks-ui / app-revx-ui bundles.
// Add overrides only when capture evidence establishes the endpoint.
var opEndpoints = map[string]string{
	"CommerceGetAvailableInventory":       EndpointWarehouse,
	"CommerceInventoryLocations":          EndpointWarehouse,
	"CommerceInventoryQuantities":         EndpointWarehouse,
	"CommerceInventoryMovement":           EndpointWarehouse,
	"CommerceGenerateSerialLotNumbers":    EndpointWarehouse,
	"CommerceConsumeInventory":            EndpointWarehouse,
	"CommerceReceiveInventory":            EndpointWarehouse,
	"CreateAttachment":                    EndpointSpendLists,
	"CreateAttachment_qbo":                EndpointSpendLists,
	"CustomFieldDefinitionCreateMutation": EndpointSpendLists,
	"CustomFieldsQuery":                   EndpointSpendLists,
	"CustomFieldsQueryCES":                EndpointSpendLists,
	"GetContacts":                         EndpointSpendLists,
	"QbAppFoundationDeleteAttachment":     EndpointSpendLists,
	"QbAppFoundationReadAttachments":      EndpointSpendLists,
	"UpdateAttachment":                    EndpointSpendLists,
	"UpdateFieldDefinitionCreateMutation": EndpointSpendLists,
	// Live frontend request captured 2026-09-12; sanitized evidence:
	// .sol/evidence/live-sweep-20260912/captures/task-query-sanitized.json
	"TaskManagementTasks": EndpointSpendLists,
	// Live SPA mutation captured 2026-09-14: the chart-of-accounts UI posts
	// CreateAccount to coa-core, not the v4 gateway (which answers PLT-2000).
	// Evidence: .sol/evidence/fusion-live-residuals-17/coa-ui-mutation.json
	"CreateAccount": EndpointCoaCore,
	// Live products UI capture 2026-09-15: activation uses the commerce
	// control supergraph's updateItemStatus operation.
	"UpdateItemStatus": EndpointCommerceControl,
	// Live products UI capture 2026-09-15: the quick-edit grid sends
	// BatchUpdateProducts to the same commerce-control supergraph.
	"BatchUpdateProducts": EndpointCommerceControl,
	// Same capture family: product reads resolve on commercecontrol; the
	// v4 gateway answers FieldUndefined (getProduct/dataAccessProducts).
	"GetProduct":  EndpointCommerceControl,
	"GetProducts": EndpointCommerceControl,
	// Live probe 2026-09-16: assignDimensionsForProducts resolves on
	// commercecontrol (products-and-services-core module); the v4 gateway
	// answers 403 ApplicationAuthorizationFailed.
	"AssignDimensionsForProducts": EndpointCommerceControl,
	// Live probe 2026-09-16: appFoundationsCustomDimensionDefinitions resolves
	// on the smallbusiness host; v4 answers 403 and commercecontrol reports
	// the field undefined.
	"GetCustomDimensionDefinitions": EndpointSpendLists,
}

// Lookup returns a verified endpoint override. An unknown operation returns
// ("", false), leaving the caller responsible for its fallback endpoint.
func Lookup(operation string) (endpoint string, found bool) {
	endpoint, found = opEndpoints[operation]
	return endpoint, found
}
