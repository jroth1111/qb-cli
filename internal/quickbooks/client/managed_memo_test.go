package client

import (
	"strings"
	"testing"
)

func TestManagedMemoPreservation(t *testing.T) {
	block := "[[QBO-METADATA:test:v1]]\n[Card: Example]\n[[/QBO-METADATA:test:v1]]"
	replacement := "[[QBO-METADATA:test:v1]]\n[Card: Corrected]\n[[/QBO-METADATA:test:v1]]"
	for _, tc := range []struct {
		name, old, proposed, want string
		fail                      bool
	}{
		{"ordinary replacement", "old invoice", "new invoice", "new invoice", false},
		{"preserve managed", "old\n" + block, "new invoice", "new invoice\n\n" + block, false},
		{"replace managed", "old\n" + block, "new\n" + replacement, "new\n" + replacement, false},
		{"legacy retained", "old [Card: Example] [Parent: unresolved]", "new [Card: Example] [Parent: unresolved]", "new [Card: Example] [Parent: unresolved]", false},
		{"legacy loss blocked", "old [Card: Example] [Parent: unresolved]", "new invoice", "", true},
		{"legacy qualification loss blocked", "[Card: Example] [Inferred parent QBO: Purchase/1] [Provisional]", "[Card: Example]", "", true},
		{"legacy migration", "old [Card: Example]", "new\n" + block, "new\n" + block, false},
		{"broken existing", "[[QBO-METADATA:x]]oops", "new", "", true},
		{"duplicate new", "old", block + block, "", true},
		{"orphan close", "old", "[[/QBO-METADATA:x]]", "", true},
		{"mismatched embedded close", "old", "[[QBO-METADATA:x]]text[[/QBO-METADATA:y]][[/QBO-METADATA:x]]", "", true},
		{"no truncation", block, strings.Repeat("界", 4000), "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := preserveManagedMemo(tc.old, tc.proposed)
			if (err != nil) != tc.fail || !tc.fail && got != tc.want {
				t.Fatalf("result mismatch: error=%v", err)
			}
		})
	}
}

func TestPurchaseMemoUpdateKeepsManagedSection(t *testing.T) {
	block := "[[QBO-METADATA:test:v1]]\n[Card: Example]\n[[/QBO-METADATA:test:v1]]"
	existing := map[string]any{"Id": "123", "SyncToken": "1", "PrivateNote": "old\n" + block, "PaymentType": "CreditCard", "AccountRef": map[string]any{"value": "2"}}
	body, err := buildUpdateBody("Purchase", map[string]string{"memo": "new invoice"}, existing)
	if err != nil || body["PrivateNote"] != "new invoice\n\n"+block {
		t.Fatalf("managed metadata was not retained: %v", err)
	}
	if existing["PrivateNote"] != "old\n"+block {
		t.Fatal("input record changed")
	}
}
