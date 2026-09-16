package main

import (
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql/routes"
)

func TestEndpointForVerifiedRoutes(t *testing.T) {
	// Freeze every historical override plus the live-proven task route so a
	// shared-table change cannot silently reroute an existing operation.
	for operation, want := range map[string]string{
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
		"TaskManagementTasks":                 EndpointSpendLists,
	} {
		t.Run(operation, func(t *testing.T) {
			shared, found := routes.Lookup(operation)
			if !found || shared != want {
				t.Fatalf("shared Lookup = (%q, %v); want (%q, true)", shared, found, want)
			}
			if got := endpointFor(operation); got != shared {
				t.Errorf("generator endpoint = %q; shared Lookup = %q", got, shared)
			}
		})
	}
}

func TestEndpointForUnmappedRoutes(t *testing.T) {
	for _, operation := range []string{"", "UnmappedRoutingFixture", "GetBills", "Items", "taskmanagementtasks"} {
		t.Run(operation, func(t *testing.T) {
			if endpoint, found := routes.Lookup(operation); found || endpoint != "" {
				t.Fatalf("shared Lookup = (%q, %v); want (empty, false)", endpoint, found)
			}
			if got := endpointFor(operation); got != EndpointDefault {
				t.Errorf("generator endpoint = %q; want default %q", got, EndpointDefault)
			}
		})
	}
}
