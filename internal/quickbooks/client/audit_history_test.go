package client

import "testing"

func TestAuditHistoricalWindow(t *testing.T) {
	for _, tc := range []struct {
		name   string
		filter AuditFilter
		bad    bool
	}{
		{"local timezone", AuditFilter{FromDate: "2026-07-31T00:00:00+10:00", ToDate: "2026-07-31T23:59:59.999+10:00", Offset: 50}, false},
		{"missing end", AuditFilter{FromDate: "2026-07-31T00:00:00Z"}, true},
		{"missing timezone", AuditFilter{FromDate: "2026-07-31", ToDate: "2026-08-01"}, true},
		{"reversed", AuditFilter{FromDate: "2026-08-01T00:00:00Z", ToDate: "2026-07-31T00:00:00Z"}, true},
		{"negative offset", AuditFilter{Offset: -1}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b, err := auditWindow(tc.filter)
			if (err != nil) != tc.bad {
				t.Fatalf("error=%v", err)
			}
			if !tc.bad && (a != "2026-07-30T14:00:00Z" || b != "2026-07-31T13:59:59.999Z") {
				t.Fatalf("wrong UTC window %s %s", a, b)
			}
		})
	}
}

func TestAuditEntityIDIsSeparateFromAuditID(t *testing.T) {
	raw, ok := extractAuditEvents([]byte(`{"logs":[{"what":{"qboAuditId":"audit-9","eventType":"DELETE","entity":{"ELEMENTID":"transaction-5","TX_DATE":"2025-08-18","TX_AMOUNT":"121.5000000"}},"when":{"createdDate":"2026-07-31T05:00:00Z"}}],"pageInfo":{"totalLogs":96,"startIndex":50,"size":50}}`))
	if !ok {
		t.Fatal("response not recognized")
	}
	x := projectAudit(raw).Events[0]
	if x.ID != "audit-9" || x.EntityID != "transaction-5" || x.TransactionDate != "2025-08-18" || x.Amount != "121.5000000" {
		t.Fatalf("lost lineage: %+v", x)
	}
}
