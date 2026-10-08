package client

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestV3MutationRequiresIndependentIntendedState(t *testing.T) {
	for _, mode := range []string{"applied", "unchanged", "wrong identity", "changed funding", "read failure"} {
		t.Run(mode, func(t *testing.T) {
			saveUsable(t)
			posts, gets := 0, 0
			state := map[string]any{"Id": "51", "SyncToken": "1", "PrivateNote": "old", "PaymentType": "Cash", "AccountRef": map[string]any{"value": "44"}, "Line": []any{map[string]any{"Id": "1", "Amount": 10.0, "AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "7"}}}}}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					posts++
					var submitted map[string]any
					_ = json.NewDecoder(r.Body).Decode(&submitted)
					if mode != "unchanged" {
						maps.Copy(state, submitted)
						state["SyncToken"] = "2"
					}
					if mode == "changed funding" {
						state["AccountRef"] = map[string]any{"value": "209"}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"Purchase": map[string]any{"Id": "51", "PrivateNote": "new"}})
					return
				}
				gets++
				if posts > 0 && mode == "read failure" {
					w.WriteHeader(503)
					return
				}
				if posts > 0 && mode == "wrong identity" {
					state["Id"] = "99"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"Purchase": state})
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			res, err := ReplayMutate(context.Background(), "Purchase", "update", "51", map[string]string{"memo": "new"})
			if mode == "applied" {
				if err != nil || !res.Verified {
					t.Fatalf("verified mutation failed: %v", err)
				}
			} else if !errors.Is(err, ErrMutationUnverified) {
				t.Fatalf("false verified success: %v", err)
			}
			wantGets := 2
			if mode == "read failure" {
				wantGets = 4 // one before-state plus three bounded readback attempts
			}
			if posts != 1 || gets != wantGets {
				t.Fatalf("writes=%d reads=%d; must not replay mutation", posts, gets)
			}
		})
	}
}

func TestReadbackComparisonRejectsDroppedLinesAndClass(t *testing.T) {
	want := normalizedJSON(map[string]any{"Line": []any{map[string]any{"Id": "1", "Amount": 10, "AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "7"}}}}})
	for _, got := range []any{map[string]any{"Line": []any{}}, map[string]any{"Line": []any{map[string]any{"Id": "1", "Amount": 10, "AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "7"}, "ClassRef": map[string]any{"value": "wrong"}}}}}} {
		if expectedFields(want, normalizedJSON(got), "Purchase") == nil {
			t.Fatal("accepted unexpected accounting lines/Class")
		}
	}
}

func TestReadbackPreservesUnrequestedNestedFields(t *testing.T) {
	before := map[string]any{"BillAddr": map[string]any{"Line1": "old", "City": "Melbourne"}}
	request := map[string]any{"BillAddr": map[string]any{"Line1": "new"}}
	bad := map[string]any{"BillAddr": map[string]any{"Line1": "new"}}
	if preservedMapFields(before, request, bad, "Customer") == nil {
		t.Fatal("silently lost unrequested address field")
	}
	good := map[string]any{"BillAddr": map[string]any{"Line1": "new", "City": "Melbourne"}}
	if err := preservedMapFields(before, request, good, "Customer"); err != nil {
		t.Fatal(err)
	}
}

func TestReadbackAllowsOnlyOmittedEmptyCustomExtensions(t *testing.T) {
	if err := expectedFields(map[string]any{"CustomExtensions": []any{}}, map[string]any{}, "Purchase.Line[0]"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []map[string]any{
		{"CustomExtensions": []any{map[string]any{"value": "retain"}}},
		{"Line": []any{}},
		{"ClassRef": map[string]any{"value": "parent"}},
	} {
		if expectedFields(want, map[string]any{}, "Purchase.Line[0]") == nil {
			t.Fatal("accepted missing nonempty extensions or accounting fields")
		}
	}
}

func TestAttachableReadbackAllowsSignedURLRotationButPreservesDocument(t *testing.T) {
	for _, changed := range []string{"url", "documentId", "FileName", "Size", "ContentType", "Note"} {
		t.Run(changed, func(t *testing.T) {
			saveUsable(t)
			before := map[string]any{"Id": "7", "SyncToken": "1", "TempDownloadUri": "https://download.example/old-signature", "documentId": "doc1", "FileName": "evidence.pdf", "Size": 10.0, "ContentType": "application/pdf", "Note": "retain", "AttachableRef": []any{map[string]any{"EntityRef": map[string]any{"type": "Deposit", "value": "8"}}}}
			want := map[string]any{"Id": "7", "SyncToken": "1", "sparse": true, "AttachableRef": []any{map[string]any{"EntityRef": map[string]any{"type": "Purchase", "value": "9"}}}}
			after := maps.Clone(before)
			after["TempDownloadUri"] = "https://download.example/new-signature"
			after["AttachableRef"] = want["AttachableRef"]
			if changed != "url" {
				if changed == "Size" {
					after[changed] = 11.0
				} else {
					after[changed] = "changed"
				}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]any{"Attachable": after})
			}))
			defer server.Close()
			interceptHTTP(t, server.URL)
			ac, err := newAPIClient()
			if err != nil {
				t.Fatal(err)
			}
			err = verifyV3Readback(context.Background(), ac, "Attachable", "update", "7", before, want, t.TempDir())
			if changed == "url" && err != nil {
				t.Fatal(err)
			}
			if changed != "url" && !errors.Is(err, ErrMutationUnverified) {
				t.Fatal("lost document metadata accepted")
			}
		})
	}
}

func TestAttachableReadbackMatchesLinksByEntityIdentity(t *testing.T) {
	old := map[string]any{"EntityRef": map[string]any{"type": "Deposit", "value": "8"}, "IncludeOnSend": false}
	new := map[string]any{"EntityRef": map[string]any{"type": "Purchase", "value": "9"}, "IncludeOnSend": false}
	want := map[string]any{"AttachableRef": []any{old, new}}
	if err := expectedFields(want, map[string]any{"AttachableRef": []any{new, old}}, "Attachable"); err != nil {
		t.Fatal(err)
	}
	for _, rows := range [][]any{
		{new, new}, {old}, {old, new, new},
		{old, map[string]any{"EntityRef": map[string]any{"type": "Purchase", "value": "10"}, "IncludeOnSend": false}},
		{old, map[string]any{"EntityRef": map[string]any{"type": "Purchase", "value": "9"}, "IncludeOnSend": true}},
	} {
		if expectedFields(want, map[string]any{"AttachableRef": rows}, "Attachable") == nil {
			t.Fatal("accepted missing/duplicate/wrong attachment link or changed send flag")
		}
	}
}
