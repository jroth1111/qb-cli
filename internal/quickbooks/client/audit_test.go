package client

import (
	"strings"
	"testing"
)

func TestAuditEntitiesMatchesCapturedUI(t *testing.T) {
	if len(auditEntities) != 64 {
		t.Fatalf("len=%d want 64 (current /app/auditlog request)", len(auditEntities))
	}
	want := []string{
		"audit_info",
		"com.intuit.finance.accounting.financialtransactions.FinancialTransaction",
		"com.intuit.commerce.inventorymanagement.InventoryAdjustment",
		"com.intuit.commerce.productinformationmanagement.ItemV2",
		"com.intuit.commerce.indirecttax.withholdingtax.TaxReturn",
		"com.intuit.ies.doneness.referenceapp.Transaction",
		"com.intuit.ies.doneness.referenceapp.Customer",
		"com.intuit.foundation.ecosystemintegration.taskmanagement.tasklifecyclemanagement.TaskV2",
	}
	have := map[string]bool{}
	for _, e := range auditEntities {
		have[e] = true
	}
	for _, k := range want {
		if !have[k] {
			t.Errorf("missing %s", k)
		}
	}
}

func TestAuditThisMonthWindowShape(t *testing.T) {
	from, to := auditThisMonthWindow()
	const layout = "2006-01-02T15:04:05.000Z"
	if len(from) != len(layout) || !strings.HasSuffix(from, "Z") {
		t.Fatalf("from=%q", from)
	}
	if to <= from {
		t.Fatalf("to=%q from=%q", to, from)
	}
}

func TestExtractAuditEventsNestedLogs(t *testing.T) {
	body := []byte(`{"pageInfo":{"total":1},"logs":[{"who":{"userId":"u1"},"what":{"qboAuditId":"a1","eventType":"UPDATED","defaultEventDescription":"Invoice updated","entityType":"Invoice"},"when":{"createdDate":"2026-08-01T00:00:00.000Z"}}]}`)
	got, ok := extractAuditEvents(body)
	if !ok {
		t.Fatal("expected logs")
	}
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	out := projectAudit(got)
	if out.Counts["events"] != 1 || len(out.Events) != 1 {
		t.Fatalf("proj=%+v", out)
	}
	e := out.Events[0]
	if e.ID != "a1" || e.User != "u1" || e.EventType != "UPDATED" || e.Name != "Invoice updated" || e.Date == "" {
		t.Fatalf("event=%+v", e)
	}
}

func TestExtractAuditEventsIgnoresObjectEntity(t *testing.T) {
	body := []byte(`{"logs":[{"who":{"userId":"u"},"what":{"qboAuditId":"id1","eventType":"CREATED","entity":{"id":"9"},"entityType":"Invoice"},"when":{"createdDate":"2026-08-01T00:00:00.000Z"}}]}`)
	got, ok := extractAuditEvents(body)
	if !ok || len(got) != 1 || got[0].What.QBOAuditID != "id1" {
		t.Fatalf("ok=%v got=%+v", ok, got)
	}
}

func TestExtractAuditEventsUnknownShape(t *testing.T) {
	if _, ok := extractAuditEvents([]byte(`{"nope":true}`)); ok {
		t.Fatal("unknown shape must fail")
	}
}

func TestFilterAuditEventsRejectsWrongType(t *testing.T) {
	in := []AuditEvent{{EventType: "LOGIN", Name: "audit_info"}, {EventType: "VOID", Name: "Invoice"}}
	got := filterAuditEvents(in, AuditFilter{EventType: "VOID"})
	if len(got) != 1 || got[0].EventType != "VOID" {
		t.Fatalf("got=%+v", got)
	}
}

func TestProjectAuditPrefersTXName(t *testing.T) {
	body := []byte(`{"logs":[{"who":{"userId":"u"},"what":{"qboAuditId":"id1","eventType":"ADD","entityType":"audit_info","defaultEventDescription":"audit_info","entity":{"TX_NAME":"QB-CLI-TEST Supplier","DOC_NUM":"1005"}},"when":{"createdDate":"2026-08-01T00:00:00.000Z"}}]}`)
	got, ok := extractAuditEvents(body)
	if !ok {
		t.Fatal("expected logs")
	}
	out := projectAudit(got)
	if len(out.Events) != 1 || out.Events[0].Name != "QB-CLI-TEST Supplier #1005" {
		t.Fatalf("event=%+v", out.Events)
	}
}
