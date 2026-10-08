package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMappedMatchPinsSettlementAndLedgerDatesSeparately(t *testing.T) {
	feed := map[string]any{"amount": 10.0, "olbTxnDate": "2020-02-01T10:00:00+10:00"}
	book := map[string]any{"amount": 10.0, "lineAccountId": "93", "txnTypeId": "27", "sequence": "0", "clearState": "2", "date": "2020-01-01T10:00:00+10:00"}
	m := MatchMapping{OlbTxnID: "1", MatchID: "2", ExpectedDate: "2020-02-01"}
	if checkMappedMatch(m, feed, book, "93") == nil {
		t.Fatal("legacy date precondition must still pin both dates")
	}
	m.ExpectedLedgerDate = "2020-01-01"
	if err := validateMatchMappings([]MatchMapping{m}); err != nil {
		t.Fatal(err)
	}
	if err := checkMappedMatch(m, feed, book, "93"); err != nil {
		t.Fatal(err)
	}
	book["date"] = "2020-01-02T10:00:00+10:00"
	if checkMappedMatch(m, feed, book, "93") == nil {
		t.Fatal("changed ledger date accepted")
	}
	book["date"] = "2020-01-01T10:00:00+10:00"
	feed["olbTxnDate"] = "2020-02-02T10:00:00+10:00"
	if checkMappedMatch(m, feed, book, "93") == nil {
		t.Fatal("changed feed date accepted")
	}
	m.ExpectedDate = ""
	if validateMatchMappings([]MatchMapping{m}) == nil {
		t.Fatal("ledger-only date precondition accepted")
	}
	m.ExpectedDate = "2020-02-01"
	m.ExpectedLedgerDate = "not-a-date"
	if validateMatchMappings([]MatchMapping{m}) == nil {
		t.Fatal("invalid ledger date accepted")
	}
}

func TestMatchMappingValidationAndExactIDs(t *testing.T) {
	for _, input := range []string{`[]`, `[{"olb_txn_id":"1:ofx","match_id":"2"}]`, `[{"olb_txn_id":1,"match_id":"2"}]`, `[{"olb_txn_id":"1","match_id":"2","typo":true}]`, `[{"olb_txn_id":"1","match_id":"2"},{"olb_txn_id":"3","match_id":"2"}]`, `[{"olb_txn_id":"1","match_id":"2"}] {}`} {
		p := filepath.Join(t.TempDir(), "input.json")
		if err := os.WriteFile(p, []byte(input), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadMatchMappings(p); err == nil {
			t.Fatalf("accepted %s", input)
		}
	}
	p := filepath.Join(t.TempDir(), "input.json")
	_ = os.WriteFile(p, []byte(`[{"olb_txn_id":"9223372036854775807","match_id":"3700000000000882703"}]`), 0600)
	got, err := LoadMatchMappings(p)
	if err != nil || got[0].MatchID != "3700000000000882703" {
		t.Fatalf("IDs lost precision: %v %v", got, err)
	}
	plan, err := PlanMappedMatch("44", got)
	if err != nil || plan.Dial || !strings.Contains(string(plan.Body), got[0].MatchID) {
		t.Fatalf("invalid dry plan: %v", err)
	}
}

func TestMappedMatchBatchedSubmissionAndReadOnlyRecovery(t *testing.T) {
	for _, scenario := range []string{"ok", "pinned-late-date", "pinned-ledger-drift", "crosswired", "partial-response", "partial-readback", "class-drift", "amount-conflict", "already-allocated", "ambiguous-register"} {
		t.Run(scenario, func(t *testing.T) {
			t.Setenv("QB_HTTP_TRANSPORT", "")
			saveUsable(t)
			posts, registers := 0, 0
			applied := false
			feed := func(id string) map[string]any {
				amount := -10.0
				if id == "2" {
					amount = -20
				}
				return map[string]any{"id": id + ":ofx", "olbTxnId": id, "amount": amount, "olbTxnDate": "2020-01-01T10:00:00+10:00", "origDescription": "fixture " + id, "qboAccountId": "204"}
			}
			book := func(id string) map[string]any {
				amount := -10.0
				if id == "20" {
					amount = -20
				}
				if scenario == "amount-conflict" && id == "20" {
					amount = -30
				}
				klass, clear := "5", "2"
				if applied {
					clear = "1"
				}
				if applied && scenario == "class-drift" {
					klass = "6"
				}
				date := "2020-01-01T10:00:00+10:00"
				switch scenario {
				case "pinned-late-date":
					date = "2020-04-01T10:00:00+10:00"
				case "pinned-ledger-drift":
					date = "2020-04-02T10:00:00+10:00"
				}
				return map[string]any{"txnId": id, "amount": amount, "lineAccountId": "204", "accountId": "6", "klassId": klass, "txnTypeId": "54", "sequence": "0", "date": date, "memo": "fixture", "clearState": clear}
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/register/transactions/"):
					registers++
					rows := []map[string]any{}
					if r.Header.Get("X-Range") == "items=0-299" {
						rows = []map[string]any{book("10"), book("20")}
					}
					if scenario == "ambiguous-register" && len(rows) > 0 {
						rows = append(rows, book("10"))
					}
					_ = json.NewEncoder(w).Encode(rows)
				case strings.HasSuffix(r.URL.Path, "/getTransactions"):
					rows := []map[string]any{}
					if r.URL.Query().Get("startIndex") == "0" {
						if r.URL.Query().Get("reviewState") == "PENDING" && !applied {
							rows = []map[string]any{feed("1"), feed("2")}
						}
						if r.URL.Query().Get("reviewState") == "ACCEPTED" && (applied || scenario == "already-allocated") {
							for _, id := range []string{"1", "2"} {
								if scenario == "partial-readback" && id == "2" {
									continue
								}
								row := feed(id)
								target := id + "0"
								if scenario == "crosswired" {
									if id == "1" {
										target = "20"
									} else {
										target = "10"
									}
								}
								row["matchedQboTxns"] = []any{map[string]any{"qboTxnId": target, "txnTypeId": "54", "qboTxnSeqId": "0", "amount": row["amount"]}}
								rows = append(rows, row)
							}
						}
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"items": rows})
				case strings.HasSuffix(r.URL.Path, "/advancedMatchDetails"):
					id := strings.TrimSuffix(r.URL.Query().Get("id"), ":ofx")
					row := feed(id)
					candidates := []any{map[string]any{"qboTxnId": id + "0", "txnTypeId": "54", "qboTxnSeqId": "0", "txnSyncToken": "1", "amount": row["amount"]}}
					if scenario == "pinned-late-date" && r.URL.Query().Get("highDate") != "2020-04-01" {
						candidates = nil
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"olbTxn": row, "homeCurrencyMatchingTxns": candidates})
				case r.Method == http.MethodPost:
					posts++
					applied = true
					var body struct {
						Rows []map[string]any `json:"olbTxns"`
					}
					_ = json.NewDecoder(r.Body).Decode(&body)
					if len(body.Rows) != 2 {
						t.Errorf("not batched")
					}
					success := []any{}
					for i, row := range body.Rows {
						if scenario == "partial-response" && i == 1 {
							continue
						}
						links := row["selectedMatches"].(map[string]any)["matchedTxns"]
						if links.([]any)[0].(map[string]any)["qboTxnId"] != fmt.Sprint(i+1)+"0" {
							t.Errorf("crosswired request")
						}
						success = append(success, map[string]any{"id": row["id"], "qboAccount": map[string]any{"accountId": "204"}, "matchedQboTxns": links})
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"success": success})
				default:
					t.Errorf("unexpected request %s", r.URL)
				}
			}))
			defer server.Close()
			interceptHTTP(t, server.URL)
			mappings := []MatchMapping{{OlbTxnID: "1", MatchID: "10"}, {OlbTxnID: "2", MatchID: "20"}}
			if scenario == "pinned-late-date" || scenario == "pinned-ledger-drift" {
				for i := range mappings {
					mappings[i].ExpectedDate = "2020-01-01"
					mappings[i].ExpectedLedgerDate = "2020-04-01"
				}
			}
			result, err := ReplayMappedMatch(context.Background(), "204", mappings)
			if scenario == "ok" || scenario == "pinned-late-date" {
				if err != nil || !result.Verified || posts != 1 || registers != 4 {
					t.Fatalf("result=%+v err=%v posts=%d register reads=%d", result, err, posts, registers)
				}
				if _, err := VerifyMappedMatchEvidence(context.Background(), result.Evidence); err != nil {
					t.Fatal(err)
				}
				if posts != 1 {
					t.Fatal("recovery replayed mutation")
				}
			} else {
				if err == nil {
					t.Fatal("unsafe scenario passed")
				}
				beforePost := scenario == "amount-conflict" || scenario == "already-allocated" || scenario == "ambiguous-register" || scenario == "pinned-ledger-drift"
				if beforePost && posts != 0 || !beforePost && posts != 1 {
					t.Fatalf("posts=%d", posts)
				}
			}
		})
	}
}
