package client

import "testing"

func TestAuditEventIdentityFallback(t *testing.T) {
	raw, ok := extractAuditEvents([]byte(`{"logs":[{"what":{"eventType":"IMPORT_DATA","idempotenceKey":"event-key-1"},"when":{"createdDate":"2026-08-03T00:00:00Z"}}]}`))
	if !ok {
		t.Fatal("unrecognized response")
	}
	x := projectAudit(raw).Events[0]
	if x.ID != "event-key-1" {
		t.Fatalf("lost audit event identity: %+v", x)
	}
}
