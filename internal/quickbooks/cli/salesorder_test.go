package cli

import (
	"strings"
	"testing"
)

func TestSalesOrderMutationPlanUsesNativeServiceAndOnlyLocalFlags(t *testing.T) {
	out, err := runCoverageRoot(t, "--json", "--yes", "--dry-run", "sales", "sales-order", "create", "--customer", "10", "--order-date", "10/09/2026", "--currency", "AUD", "--line-items", `[{"Amount":1,"SalesItemLineDetail":{"ItemRef":{"value":"7"},"Qty":1,"UnitPrice":1}}]`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "commercecontrol.api.intuit.com/graphql") || strings.Contains(out, "/invoice") {
		t.Fatal(out)
	}
	if strings.Contains(out, `"yes":`) || strings.Contains(out, `"json":`) {
		t.Fatal("global flags leaked into native payload")
	}
	for _, field := range []string{`"customer": "10"`, `"currency": "AUD"`, `"order-date": "10/09/2026"`} {
		if !strings.Contains(out, field) {
			t.Fatalf("explicit flag lost: %s in %s", field, out)
		}
	}
}

func TestSalesOrderReadRejectsIgnoredActiveFlag(t *testing.T) {
	_, err := runCoverageRoot(t, "--dry-run", "sales", "sales-order", "get", "--active", "true")
	if err == nil || !strings.Contains(err.Error(), "do not support --active") {
		t.Fatalf("err=%v", err)
	}
}
