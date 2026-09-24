package client

import "testing"

func TestAuditUpdateUsesUpdateTimestamp(t *testing.T) {
	raw, ok := extractAuditEvents([]byte(`{"logs":[{"what":{"eventType":"UPDATE","idempotenceKey":"event-1"},"when":{"createdDate":"2019-05-18T10:19:14Z","updatedDate":"2026-08-11T12:00:00Z"}}]}`))
	if !ok {
		t.Fatal("unrecognized audit response")
	}
	x := projectAudit(raw).Events[0]
	if x.Date != "2026-08-11T12:00:00Z" {
		t.Fatalf("wrong event timestamp: %s", x.Date)
	}
}
