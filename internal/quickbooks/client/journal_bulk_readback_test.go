package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/proof"
)

func TestJournalBulkCreateVerifiesEveryIntendedEntry(t *testing.T) {
	for _, mode := range []string{"applied", "reordered receipts", "wrong amount", "wrong account", "wrong class", "wrong date", "missing description", "missing note", "partial fault", "missing receipt", "duplicate receipt", "shared entity", "read failure"} {
		t.Run(mode, func(t *testing.T) {
			saveUsable(t)
			posts, gets := 0, 0
			state := map[string]map[string]any{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost {
					posts++
					var request struct {
						Items []map[string]any `json:"BatchItemRequest"`
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Fatal(err)
					}
					var receipts []any
					for i, item := range request.Items {
						id := strconv.Itoa(80 + i)
						entry := item["JournalEntry"].(map[string]any)
						entry["Id"] = id
						state[id] = entry
						if i == 1 && mode == "partial fault" {
							receipts = append(receipts, map[string]any{"bId": item["bId"], "Fault": map[string]any{"type": "ValidationFault"}})
							continue
						}
						if i == 1 && mode == "shared entity" {
							id = "80"
						}
						receipts = append(receipts, map[string]any{"bId": item["bId"], "JournalEntry": map[string]any{"Id": id}})
					}
					switch mode {
					case "reordered receipts":
						receipts[0], receipts[1] = receipts[1], receipts[0]
					case "missing receipt":
						receipts = receipts[:1]
					case "duplicate receipt":
						receipts[1] = receipts[0]
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"BatchItemResponse": receipts})
					return
				}
				gets++
				if mode == "read failure" {
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
				entry := state[id]
				if entry == nil {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if id == "80" {
					line := entry["Line"].([]any)[0].(map[string]any)
					detail := line["JournalEntryLineDetail"].(map[string]any)
					switch mode {
					case "wrong amount":
						line["Amount"] = 91.0
					case "wrong account":
						detail["AccountRef"] = map[string]any{"value": "wrong"}
					case "wrong class":
						detail["ClassRef"] = map[string]any{"value": "wrong"}
					case "wrong date":
						entry["TxnDate"] = "2026-05-01"
					case "missing description":
						delete(line, "Description")
					case "missing note":
						delete(entry, "PrivateNote")
					}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"JournalEntry": entry})
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			ctx := proof.WithScope(context.Background())
			items := []JournalBulkItem{
				{Date: "30/04/2026", Amount: 90, FromAccount: "71", ToAccount: "151", Class: "unit-14", Memo: "source evidence", Description: "AC deep clean"},
				{Date: "2026-05-12", Amount: 250, FromAccount: "71", ToAccount: "151", Class: "unit-14", Description: "Blinds and bathroom deep clean"},
			}
			res, err := ReplayJournalBulkCreate(ctx, items)
			ok := mode == "applied" || mode == "reordered receipts"
			if ok {
				if err != nil || !proof.Confirmed(ctx) || gets != 2 {
					t.Fatalf("unverified intended create: res=%+v err=%v gets=%d", res, err, gets)
				}
			} else if !errors.Is(err, ErrMutationUnverified) || proof.Confirmed(ctx) {
				t.Fatalf("false completion: res=%+v err=%v proof=%v", res, err, proof.Confirmed(ctx))
			}
			if posts != 1 || res == nil || len(res.Items) == 0 {
				t.Fatalf("write replay or missing recovery receipt: posts=%d res=%+v", posts, res)
			}
		})
	}
}

func TestJournalBulkCreateRejectsInvalidIntentsBeforeSubmission(t *testing.T) {
	for _, raw := range []string{
		`[{"amount":-1,"from-account":"71","to-account":"151"}]`,
		`[{"amount":1,"from-account":" ","to-account":"151"}]`,
		`[{"amount":1,"from-account":"71","to-account":" 71 "}]`,
		`[{"date":"2026-02-30","amount":1,"from-account":"71","to-account":"151"}]`,
	} {
		if _, err := ParseJournalBulkItems(raw); err == nil {
			t.Fatalf("accepted invalid intent %s", raw)
		}
	}
}
