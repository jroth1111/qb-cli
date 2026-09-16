package gql

import (
	"reflect"
	"sort"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql/routes"
)

func TestCatalogVerifiedRoutes(t *testing.T) {
	original := append([]Op(nil), catalogEntries...)
	built := buildCatalog()
	listed := Ops("")
	if len(listed) != len(original) || len(built) != len(original) {
		t.Fatalf("catalog size: Ops=%d buildCatalog=%d entries=%d", len(listed), len(built), len(original))
	}
	listedByName := make(map[string]*Op, len(listed))
	for _, op := range listed {
		listedByName[op.Name] = op
	}
	var changed []string
	for i, entry := range original {
		want := entry
		if endpoint, found := routes.Lookup(entry.Name); found {
			want.Endpoint = endpoint
		}
		if doc, found := docOverrides[entry.Name]; found {
			want.Document = doc
		}
		op, err := Lookup(entry.Name)
		if err != nil {
			t.Fatal(err)
		}
		for surface, got := range map[string]*Op{
			"Lookup": op, "Ops": listedByName[entry.Name], "buildCatalog": built[entry.Name],
		} {
			if !reflect.DeepEqual(got, &want) {
				t.Errorf("%s(%s) differs from the original operation with its shared route applied", surface, entry.Name)
			}
		}
		if built[entry.Name] == &catalogEntries[i] {
			t.Errorf("buildCatalog(%s) points into the embedded catalog", entry.Name)
		}
		if op.Endpoint != entry.Endpoint {
			changed = append(changed, entry.Name)
		}
	}
	if !reflect.DeepEqual(catalogEntries, original) {
		t.Error("building the runtime catalog mutated embedded entries")
	}
	sort.Strings(changed)
	// TaskManagementTasks (tasks-ui bundle), CreateAccount (coa-core), the
	// products UI commerce-control family (UpdateItemStatus,
	// BatchUpdateProducts, GetProduct, GetProducts), the product-dimensions
	// pair (AssignDimensionsForProducts also on commerce-control) and the
	// customization-ui dimension query (GetCustomDimensionDefinitions on
	// smallbusiness) are the ops whose verified route differs from catalog
	// host.
	wantChanged := []string{"AssignDimensionsForProducts", "BatchUpdateProducts", "CreateAccount", "GetCustomDimensionDefinitions", "GetProduct", "GetProducts", "TaskManagementTasks", "UpdateItemStatus"}
	if !reflect.DeepEqual(changed, wantChanged) {
		t.Errorf("route changes = %v; want %v", changed, wantChanged)
	}
	t.Logf("runtime route changes: %v (%d of %d operations)", changed, len(changed), len(original))
	task, err := Lookup("TaskManagementTasks")
	if err != nil {
		t.Fatal(err)
	}
	if task.Endpoint != EndpointSpendLists {
		t.Errorf("TaskManagementTasks endpoint = %q; want %q", task.Endpoint, EndpointSpendLists)
	}
	createAccount, err := Lookup("CreateAccount")
	if err != nil {
		t.Fatal(err)
	}
	if createAccount.Endpoint != "https://coa-core.api.intuit.com/graphql" {
		t.Errorf("CreateAccount endpoint = %q; want https://coa-core.api.intuit.com/graphql", createAccount.Endpoint)
	}
	status, err := Lookup("UpdateItemStatus")
	if err != nil {
		t.Fatal(err)
	}
	if status.Endpoint != EndpointCommerceControl {
		t.Errorf("UpdateItemStatus endpoint = %q; want %q", status.Endpoint, EndpointCommerceControl)
	}
	batch, err := Lookup("BatchUpdateProducts")
	if err != nil {
		t.Fatal(err)
	}
	if batch.Endpoint != EndpointCommerceControl {
		t.Errorf("BatchUpdateProducts endpoint = %q; want %q", batch.Endpoint, EndpointCommerceControl)
	}
	for _, name := range []string{"GetProduct", "GetProducts"} {
		get, err := Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		if get.Endpoint != EndpointCommerceControl {
			t.Errorf("%s endpoint = %q; want %q", name, get.Endpoint, EndpointCommerceControl)
		}
	}
}

func TestBuildCatalogPreservesUnmappedEndpoint(t *testing.T) {
	original := catalogEntries
	t.Cleanup(func() { catalogEntries = original })
	catalogEntries = []Op{{
		Name: "UnmappedRoutingFixture", Endpoint: "https://unmapped.invalid/graphql",
	}}
	built := buildCatalog()
	if got := built["UnmappedRoutingFixture"]; !reflect.DeepEqual(got, &catalogEntries[0]) {
		t.Error("buildCatalog replaced an unmapped operation's existing endpoint or metadata")
	}
}

func TestPublicEndpointCompatibility(t *testing.T) {
	for name, endpoints := range map[string][2]string{
		"default":   {EndpointDefault, "https://qbo.intuit.com/api/v4/graphql"},
		"warehouse": {EndpointWarehouse, "https://warehouse-management-svc.api.intuit.com/graphql"},
		"commerce":  {EndpointCommerceControl, "https://commercecontrol.api.intuit.com/graphql"},
		"spend":     {EndpointSpendLists, "https://smallbusiness.api.intuit.com/graphql"},
	} {
		if endpoints[0] != endpoints[1] {
			t.Errorf("%s endpoint = %q; want %q", name, endpoints[0], endpoints[1])
		}
	}
}
