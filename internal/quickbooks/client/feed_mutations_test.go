package client

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- mutation contract server ----------------------------------------------

// capturedPost records one mutation POST the server received.
type capturedPost struct {
	accept string
	path   string
	query  string
	body   []byte
}

// mutationServer serves the reads the mutation builders hydrate from
// (getTransactions + register/transactions) and captures every mutation
// POST path/query/body for assertion.
type mutationServer struct {
	*httptest.Server

	feedItems         []map[string]any
	acceptedItems     []map[string]any
	excludedItems     []map[string]any
	statesRead        []string
	suppressStateMove bool
	regRows           []map[string]any

	mu      sync.Mutex
	posts   []capturedPost
	txCalls int
}

func newMutationServer(t *testing.T, feedItems, regRows []map[string]any) *mutationServer {
	t.Helper()
	ms := &mutationServer{feedItems: feedItems, regRows: regRows}
	ms.Server = httptest.NewServer(http.HandlerFunc(ms.handle))
	t.Cleanup(ms.Close)
	return ms
}

func (ms *mutationServer) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.HasSuffix(r.URL.Path, "/getTransactions"):
		ms.mu.Lock()
		ms.txCalls++
		ms.statesRead = append(ms.statesRead, r.URL.Query().Get("reviewState"))
		ms.mu.Unlock()
		items := ms.feedItems
		if r.URL.Query().Get("reviewState") == "ACCEPTED" {
			items = ms.acceptedItems
		}
		if r.URL.Query().Get("reviewState") == "EXCLUDED" {
			items = ms.excludedItems
		}
		if items == nil {
			items = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	case strings.HasSuffix(r.URL.Path, "/advancedMatchDetails"):
		var candidates []map[string]any
		for _, reg := range ms.regRows {
			c := map[string]any{"qboTxnId": jsonNumberString(reg["txnId"]), "txnTypeId": reg["txnTypeId"], "qboTxnSeqId": reg["sequence"]}
			if _, ok := reg["editSequence"]; ok {
				c["txnSyncToken"] = "2"
			} // deliberately differs from register version 0
			if amount, ok := registerPaymentAmount(reg); ok {
				c["amount"] = amount
			}
			candidates = append(candidates, c)
		}
		var row map[string]any
		for _, x := range ms.feedItems {
			if mapStr(x, "id") == r.URL.Query().Get("id") {
				row = x
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"homeCurrencyMatchingTxns": candidates, "olbTxn": row})
	case strings.HasSuffix(r.URL.Path, "/register/transactions/"):
		_ = json.NewEncoder(w).Encode(ms.regRows)
	case r.Method == http.MethodPost:
		body, _ := io.ReadAll(r.Body)
		ms.mu.Lock()
		ms.posts = append(ms.posts, capturedPost{path: r.URL.Path, query: r.URL.RawQuery, body: body, accept: r.Header.Get("Accept")})
		ms.mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/acceptTransactions") {
			var request struct {
				Rows []map[string]any `json:"olbTxns"`
			}
			if err := json.Unmarshal(body, &request); err != nil {
				w.WriteHeader(400)
				return
			}
			var success []map[string]any
			for _, entry := range request.Rows {
				selected := entry["selectedMatches"].(map[string]any)
				for i, row := range ms.feedItems {
					if row["id"] != entry["id"] {
						continue
					}
					posted := shallowCopyMap(row)
					posted["matchedQboTxns"] = selected["matchedTxns"]
					ms.acceptedItems = append(ms.acceptedItems, posted)
					ms.feedItems = append(ms.feedItems[:i], ms.feedItems[i+1:]...)
					success = append(success, map[string]any{"id": entry["id"], "qboAccount": map[string]any{"accountId": entry["qboAccountId"]}, "matchedQboTxns": selected["matchedTxns"]})
					break
				}
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": success})
		} else if strings.HasSuffix(r.URL.Path, "/undoTransactions") || strings.HasSuffix(r.URL.Path, "/excludeTransactions") {
			var req struct {
				Next struct {
					State string `json:"reviewState"`
				} `json:"nextTxnInfo"`
				IDs struct {
					IDs []string `json:"olbTxnIds"`
				} `json:"txnIdList"`
			}
			_ = json.Unmarshal(body, &req)
			source := &ms.feedItems
			if req.Next.State == "ACCEPTED" {
				source = &ms.acceptedItems
			}
			if req.Next.State == "EXCLUDED" {
				source = &ms.excludedItems
			}
			target := &ms.feedItems
			if strings.HasSuffix(r.URL.Path, "/excludeTransactions") {
				target = &ms.excludedItems
			}
			var success []map[string]any
			for _, id := range req.IDs.IDs {
				for i, row := range *source {
					if feedRowMatchesID(row, id) {
						if !ms.suppressStateMove {
							*target = append(*target, row)
							*source = append((*source)[:i], (*source)[i+1:]...)
						}
						success = append(success, map[string]any{"olbTxnId": row["olbTxnId"]})
						break
					}
				}
			}
			if strings.HasSuffix(r.URL.Path, "/excludeTransactions") {
				_ = json.NewEncoder(w).Encode(map[string]any{})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"olbtxns": map[string]any{"success": success}})
			}
		} else if strings.HasSuffix(r.URL.Path, "/batchAcceptTransactions") {
			var req struct {
				Next struct {
					Account string `json:"accountId"`
				} `json:"nextTxnInfo"`
				List struct {
					Rows []map[string]any `json:"olbTxns"`
				} `json:"txnList"`
			}
			_ = json.Unmarshal(body, &req)
			accepted := []map[string]any{}
			for _, row := range req.List.Rows {
				accepted = append(accepted, map[string]any{"olbTxnId": row["olbTxnId"], "qboAccount": map[string]any{"accountId": req.Next.Account}, "addedQboTxns": []any{map[string]any{"qboTxnId": "900003"}}})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"acceptedTxns": accepted})
		} else {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
		}
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (ms *mutationServer) lastPost(t *testing.T) capturedPost {
	t.Helper()
	ms.mu.Lock()
	defer ms.mu.Unlock()
	if len(ms.posts) == 0 {
		t.Fatal("no mutation POST captured")
	}
	return ms.posts[len(ms.posts)-1]
}

func (ms *mutationServer) postCount() int {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return len(ms.posts)
}

func (ms *mutationServer) txCallCount() int {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	return ms.txCalls
}

// feedRowFixture returns a getTransactions item shaped like the live SPA
// response: known id/date/account fields plus fields the projection never
// typed (suggestionConfidence, matchTransactionsMap, lowContext). The
// unknown keys are the point — a lossy projection drops them on the wire.
func feedRowFixture(olbTxnID string) map[string]any {
	return map[string]any{
		"id":                                 olbTxnID + ":ofx",
		"olbTxnId":                           olbTxnID,
		"origDescription":                    "TEST FEED ROW " + olbTxnID,
		"description":                        "Test Feed Row " + olbTxnID,
		"olbTxnDate":                         "2026-08-23T00:00:00.000Z",
		"qboAccountId":                       "204",
		"olbAccountId":                       "olb-204",
		"amount":                             float64(-50),
		"openBalance":                        float64(-50),
		"acceptType":                         "NONE",
		"suggestionConfidence":               "HIGH",
		"matchTransactionsMap":               map[string]any{},
		"originalCategoryId":                 "9",
		"categorySource":                     "AUTO",
		"lowContext":                         false,
		"isHighConfidenceAdd":                true,
		"olbTxnCreateDate":                   "2026-08-23T00:00:00.000Z",
		"complexTransactionScore":            float64(0),
		"userDeleted":                        false,
		"creditCardPayment":                  false,
		"transfer":                           false,
		"categoryTaxGuidance":                "NONE",
		"categoryAlternativeIds":             []any{},
		"linkedTxns":                         []any{},
		"mapOfAccounts":                      map[string]any{"9": "Expense"},
		"categoryExplanation":                "test",
		"categoryConfidenceScore":            float64(1),
		"addMatchType":                       "ADD",
		"scheduleCId":                        "sc-1",
		"complexTransaction":                 false,
		"catMerchantId":                      "m-1",
		"olbSessionId":                       "sess-1",
		"categoryAcceptanceConfidence":       "HIGH",
		"categoryAcceptanceConfidenceReason": "rule",
		"addAsQboTxn": map[string]any{
			"nameTypeId":    "1",
			"createName":    false,
			"createAccount": false,
			"txnTypeId":     "54",
			"txnFdmName":    "Expense",
			"currencyType":  "AUD",
			"details": []any{map[string]any{
				"categoryId": "9",
				"billable":   false,
			}},
		},
	}
}

// regRowFixture returns one register transaction row in the shape the live
// register/transactions read serves (bare array of objects; ids numeric).
func regRowFixture(txnID, txnTypeID string, payment float64) map[string]any {
	id, _ := strconv.Atoi(txnID)
	typeID, _ := strconv.Atoi(txnTypeID)
	return map[string]any{
		"txnId":         float64(id),
		"txnTypeId":     float64(typeID),
		"txnTypeString": "Expense",
		"sequence":      float64(0),
		"editSequence":  float64(0),
		"date":          "2026-08-23T10:00:00+10:00",
		"payment":       payment,
		"amount":        -payment,
		"accountId":     float64(7),
		"lineAccountId": float64(204),
	}
}

// decodePostBody unmarshals a captured POST body.
func decodePostBody(t *testing.T, p capturedPost) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(p.body, &body); err != nil {
		t.Fatalf("POST body not JSON: %v\n%s", err, p.body)
	}
	return body
}

// olbTxnAt returns txnList.olbTxns[i] from a decoded batchAccept body.
func olbTxnAt(t *testing.T, body map[string]any, i int) map[string]any {
	t.Helper()
	txnList, ok := body["txnList"].(map[string]any)
	if !ok {
		t.Fatalf("missing txnList: %v", body)
	}
	txns, ok := txnList["olbTxns"].([]any)
	if !ok || len(txns) <= i {
		t.Fatalf("txnList.olbTxns = %v, want index %d", txnList["olbTxns"], i)
	}
	row, ok := txns[i].(map[string]any)
	if !ok {
		t.Fatalf("olbTxns[%d] not an object", i)
	}
	return row
}

// --- batch accept -----------------------------------------------------------

// TestReplayBatchAcceptPostsFullRow is the end-to-end contract test for the
// Post button: one feed row is fetched, mutated to acceptType ADD and POSTed
// to batchAcceptTransactions?acceptOnly=true inside txnList.olbTxns — with
// every unknown feed-row field still present.
func TestReplayBatchAcceptPostsFullRow(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, []map[string]any{feedRowFixture("3")}, nil)
	interceptHTTP(t, ms.URL)

	status, err := ReplayBatchAccept(context.Background(), "204", []string{"3"})
	if err != nil {
		t.Fatalf("ReplayBatchAccept: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	post := ms.lastPost(t)
	if !strings.HasSuffix(post.path, "/olb/ng/batchAcceptTransactions") {
		t.Fatalf("POST path = %q, want .../olb/ng/batchAcceptTransactions", post.path)
	}
	if post.query != "acceptOnly=true" {
		t.Fatalf("POST query = %q, want acceptOnly=true", post.query)
	}
	body := decodePostBody(t, post)
	info, ok := body["nextTxnInfo"].(map[string]any)
	if !ok {
		t.Fatalf("missing nextTxnInfo: %v", body)
	}
	if info["accountId"] != "204" || info["nextTransactionIndex"] != float64(-1) ||
		info["reviewState"] != "PENDING" || info["sort"] != "-txnDate" {
		t.Fatalf("nextTxnInfo = %+v", info)
	}
	row := olbTxnAt(t, body, 0)
	if row["acceptType"] != "ADD" {
		t.Fatalf("acceptType = %v, want ADD", row["acceptType"])
	}
	if row["olbTxnId"] != "3" {
		t.Fatalf("olbTxnId = %v, want 3", row["olbTxnId"])
	}
	// Unknown fields must survive the round trip — a lossy projection
	// would drop them.
	for _, field := range []string{
		"suggestionConfidence", "matchTransactionsMap", "originalCategoryId",
		"categorySource", "lowContext", "isHighConfidenceAdd", "mapOfAccounts",
		"categoryAcceptanceConfidence", "olbSessionId", "catMerchantId",
	} {
		if _, ok := row[field]; !ok {
			t.Fatalf("feed-row field %q dropped from POST body", field)
		}
	}
	add, ok := row["addAsQboTxn"].(map[string]any)
	if !ok {
		t.Fatal("addAsQboTxn missing")
	}
	if add["txnTypeId"] != "54" {
		t.Fatalf("addAsQboTxn.txnTypeId = %v, want 54", add["txnTypeId"])
	}
}

// TestReplayBatchAcceptDisplayIDResolves asserts the :ofx display id form
// resolves the same live row.
func TestReplayBatchAcceptDisplayIDResolves(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, []map[string]any{feedRowFixture("3")}, nil)
	interceptHTTP(t, ms.URL)

	if _, err := ReplayBatchAccept(context.Background(), "204", []string{"3:ofx"}); err != nil {
		t.Fatalf("ReplayBatchAccept with display id: %v", err)
	}
	row := olbTxnAt(t, decodePostBody(t, ms.lastPost(t)), 0)
	if row["olbTxnId"] != "3" {
		t.Fatalf("olbTxnId = %v, want 3", row["olbTxnId"])
	}
}

// TestReplayBatchAcceptUnknownRowFails asserts an id absent from the pending
// feed fails with ErrFeedRowsNotFound and never reaches the mutation POST.
func TestReplayBatchAcceptUnknownRowFails(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, []map[string]any{feedRowFixture("3")}, nil)
	interceptHTTP(t, ms.URL)

	_, err := ReplayBatchAccept(context.Background(), "204", []string{"99"})
	if !errors.Is(err, ErrFeedRowsNotFound) {
		t.Fatalf("got %v, want ErrFeedRowsNotFound", err)
	}
	if ms.postCount() != 0 {
		t.Fatalf("mutation POST sent for missing row (%d posts)", ms.postCount())
	}
}

// TestReplayBatchAcceptMissingCategoryFailsBeforePost asserts the live
// service's category-less ADD behavior is surfaced as an input error rather
// than reported as a false-success HTTP 200.
func TestReplayBatchAcceptMissingCategoryFailsBeforePost(t *testing.T) {
	saveUsable(t)
	row := feedRowFixture("5")
	row["addAsQboTxn"].(map[string]any)["details"] = []any{}
	ms := newMutationServer(t, []map[string]any{row}, nil)
	interceptHTTP(t, ms.URL)

	_, err := ReplayBatchAccept(context.Background(), "204", []string{"5"})
	if !errors.Is(err, ErrMissingAcceptCategory) {
		t.Fatalf("got %v, want ErrMissingAcceptCategory", err)
	}
	if ms.postCount() != 0 {
		t.Fatalf("mutation POST sent for category-less row (%d posts)", ms.postCount())
	}
}

// --- categorise --------------------------------------------------------------

// TestReplayCategoriseFoldsCategoryID asserts the categorise CLI rides the
// same batchAcceptTransactions?acceptOnly=true call with the chosen category
// written to addAsQboTxn.details[0].categoryId and the rest of the row
// untouched.
func TestReplayCategoriseFoldsCategoryID(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, []map[string]any{feedRowFixture("3")}, nil)
	interceptHTTP(t, ms.URL)

	status, err := ReplayCategorise(context.Background(), "204", []string{"3"}, TransactionDetail{
		CategoryRef: &RefValue{Value: "7"},
	})
	if err != nil {
		t.Fatalf("ReplayCategorise: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	post := ms.lastPost(t)
	if !strings.HasSuffix(post.path, "/olb/ng/batchAcceptTransactions") || post.query != "acceptOnly=true" {
		t.Fatalf("POST = %s?%s, want .../batchAcceptTransactions?acceptOnly=true", post.path, post.query)
	}
	row := olbTxnAt(t, decodePostBody(t, post), 0)
	add := row["addAsQboTxn"].(map[string]any)
	details := add["details"].([]any)
	d0 := details[0].(map[string]any)
	if d0["categoryId"] != "7" {
		t.Fatalf("details[0].categoryId = %v, want 7", d0["categoryId"])
	}
	if row["suggestionConfidence"] != "HIGH" {
		t.Fatalf("unknown field suggestionConfidence lost: %v", row["suggestionConfidence"])
	}
}

func TestReplayCategoriseClassAndMemo(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, []map[string]any{feedRowFixture("3")}, nil)
	interceptHTTP(t, ms.URL)
	memo := "Fee [Card: Maggie]"
	_, err := ReplayCategorise(context.Background(), "204", []string{"3"}, TransactionDetail{CategoryRef: &RefValue{Value: "7"}, ClassRef: &RefValue{Value: "800398"}, Memo: &memo})
	if err != nil {
		t.Fatal(err)
	}
	post := ms.lastPost(t)
	row := olbTxnAt(t, decodePostBody(t, post), 0)
	add := row["addAsQboTxn"].(map[string]any)
	line := add["details"].([]any)[0].(map[string]any)
	if line["klassId"] != "800398" || add["txnMemo"] != memo {
		t.Fatalf("annotation not sent: %#v", add)
	}
}

// --- split ------------------------------------------------------------------

// TestReplaySplitDetailsContract asserts split rides batchAcceptTransactions
// ?acceptOnly=true with addAsQboTxn.details holding the observed per-line
// shape: categoryId, billable, amount (string), description, taxApplicableOn.
func TestReplaySplitDetailsContract(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, []map[string]any{feedRowFixture("3")}, nil)
	interceptHTTP(t, ms.URL)

	lines := []SplitLine{
		{CategoryRef: RefValue{Value: "7"}, Amount: -30},
		{CategoryRef: RefValue{Value: "8"}, Amount: -20},
	}
	status, err := ReplaySplit(context.Background(), "204", []string{"3"}, "Expense", lines)
	if err != nil {
		t.Fatalf("ReplaySplit: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	post := ms.lastPost(t)
	if !strings.HasSuffix(post.path, "/olb/ng/batchAcceptTransactions") || post.query != "acceptOnly=true" {
		t.Fatalf("POST = %s?%s, want .../batchAcceptTransactions?acceptOnly=true", post.path, post.query)
	}
	row := olbTxnAt(t, decodePostBody(t, post), 0)
	if row["acceptType"] != "ADD" {
		t.Fatalf("acceptType = %v, want ADD", row["acceptType"])
	}
	add := row["addAsQboTxn"].(map[string]any)
	details, ok := add["details"].([]any)
	if !ok || len(details) != 2 {
		t.Fatalf("addAsQboTxn.details = %v, want 2 entries", add["details"])
	}
	want := []struct{ cat, amt string }{{"7", "30"}, {"8", "20"}}
	for i, w := range want {
		d := details[i].(map[string]any)
		if d["categoryId"] != w.cat {
			t.Fatalf("details[%d].categoryId = %v, want %s", i, d["categoryId"], w.cat)
		}
		if d["amount"] != w.amt {
			t.Fatalf("details[%d].amount = %v, want %s (string)", i, d["amount"], w.amt)
		}
		if d["billable"] != false || d["description"] != "" || d["taxApplicableOn"] != "SALES" {
			t.Fatalf("details[%d] = %+v, want captured line shape", i, d)
		}
	}
}

// --- match --------------------------------------------------------------------

// TestReplayMatchSelectedMatchesContract asserts match POSTs
// /olb/ng/acceptTransactions with the reduced entry shape and resolves every
// selectedMatches.matchedTxns field from the account register read.
func TestReplayMatchSelectedMatchesContract(t *testing.T) {
	saveUsable(t)
	tok, err := auth.Load()
	if err != nil {
		t.Fatal(err)
	}
	tok.RequestHeaders = map[string]string{"Authorization": tok.Authorization, "Accept": "*/*"}
	if err := auth.Save(tok); err != nil {
		t.Fatal(err)
	}
	ms := newMutationServer(t,
		[]map[string]any{feedRowFixture("2")},
		[]map[string]any{
			regRowFixture("4", "54", 10),
			regRowFixture("5", "54", 15),
		})
	interceptHTTP(t, ms.URL)

	status, err := ReplayMatch(context.Background(), "204", []string{"2"},
		[]MatchTxn{{TxnID: "4"}, {TxnID: "5"}})
	if err != nil {
		t.Fatalf("ReplayMatch: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	post := ms.lastPost(t)
	if post.accept != "*/*" {
		t.Fatalf("banking Accept = %q, want captured */*", post.accept)
	}
	if !strings.HasSuffix(post.path, "/olb/ng/acceptTransactions") {
		t.Fatalf("POST path = %q, want .../olb/ng/acceptTransactions", post.path)
	}
	if post.query != "" {
		t.Fatalf("POST query = %q, want none", post.query)
	}
	body := decodePostBody(t, post)
	if _, hasTxnList := body["txnList"]; hasTxnList {
		t.Fatalf("acceptTransactions body must not carry txnList: %v", body)
	}
	txns, ok := body["olbTxns"].([]any)
	if !ok || len(txns) != 1 {
		t.Fatalf("olbTxns = %v, want 1 entry", body["olbTxns"])
	}
	entry := txns[0].(map[string]any)
	if entry["acceptType"] != "MATCH" {
		t.Fatalf("acceptType = %v, want MATCH", entry["acceptType"])
	}
	if entry["id"] != "2:ofx" {
		t.Fatalf("id = %v, want 2:ofx (feed display id)", entry["id"])
	}
	if entry["olbTxnDate"] != "2026-08-23T00:00:00.000Z" {
		t.Fatalf("olbTxnDate = %v", entry["olbTxnDate"])
	}
	if entry["qboAccountId"] != "204" {
		t.Fatalf("qboAccountId = %v, want 204", entry["qboAccountId"])
	}
	sel, ok := entry["selectedMatches"].(map[string]any)
	if !ok {
		t.Fatalf("selectedMatches missing: %v", entry)
	}
	if sel["addAdjQboTxn"] != nil || sel["addAsQboTxn"] != nil {
		t.Fatalf("selectedMatches extras = %v, want null", sel)
	}
	matched, ok := sel["matchedTxns"].([]any)
	if !ok || len(matched) != 2 {
		t.Fatalf("matchedTxns = %v, want 2", sel["matchedTxns"])
	}
	want := []struct{ id, amt string }{{"4", "10.00"}, {"5", "15.00"}}
	for i, w := range want {
		m := matched[i].(map[string]any)
		if m["qboTxnId"] != w.id || m["txnTypeId"] != "54" ||
			m["qboTxnSeqId"] != "0" || m["txnSyncToken"] != "2" || m["paymentAmount"] != w.amt {
			t.Fatalf("matchedTxns[%d] = %+v, want qboTxnId=%s txnTypeId=54 seq=0 sync=2 amount=%s", i, m, w.id, w.amt)
		}
	}
}

// TestReplayMatchUnknownTargetFails asserts a --match-id absent from the
// register fails with ErrMatchTargetNotFound before the mutation POST.
func TestReplayMatchUnknownTargetFails(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t,
		[]map[string]any{feedRowFixture("2")},
		[]map[string]any{regRowFixture("4", "54", 10)})
	interceptHTTP(t, ms.URL)

	_, err := ReplayMatch(context.Background(), "204", []string{"2"}, []MatchTxn{{TxnID: "99"}})
	if !errors.Is(err, ErrMatchTargetNotFound) {
		t.Fatalf("got %v, want ErrMatchTargetNotFound", err)
	}
	if ms.postCount() != 0 {
		t.Fatalf("mutation POST sent for unmatched target (%d posts)", ms.postCount())
	}
}

// TestReplayMatchMissingRegisterFieldFails asserts a register row lacking a
// contract field (txnTypeId) fails with ErrMatchFieldMissing rather than
// fabricating a value.
func TestReplayMatchMissingRegisterFieldFails(t *testing.T) {
	saveUsable(t)
	bad := regRowFixture("4", "54", 10)
	delete(bad, "txnTypeId")
	ms := newMutationServer(t,
		[]map[string]any{feedRowFixture("2")},
		[]map[string]any{bad})
	interceptHTTP(t, ms.URL)

	_, err := ReplayMatch(context.Background(), "204", []string{"2"}, []MatchTxn{{TxnID: "4"}})
	if !errors.Is(err, ErrMatchFieldMissing) {
		t.Fatalf("got %v, want ErrMatchFieldMissing", err)
	}
	if ms.postCount() != 0 {
		t.Fatalf("mutation POST sent on missing field (%d posts)", ms.postCount())
	}
}

// --- credential guards --------------------------------------------------------

// TestFeedMutationReplaysNoCreds asserts every repaired replay entry point
// fails with ErrNoCredentials before any network activity when no session is
// saved.
func TestFeedMutationReplaysNoCreds(t *testing.T) {
	noCredsDir(t)
	for name, call := range map[string]func() error{
		"batch-accept": func() error {
			_, err := ReplayBatchAccept(context.Background(), "204", []string{"3"})
			return err
		},
		"categorise": func() error {
			_, err := ReplayCategorise(context.Background(), "204", []string{"3"}, TransactionDetail{CategoryRef: &RefValue{Value: "7"}})
			return err
		},
		"split": func() error {
			_, err := ReplaySplit(context.Background(), "204", []string{"3"}, "Expense", []SplitLine{
				{CategoryRef: RefValue{Value: "7"}, Amount: -6},
				{CategoryRef: RefValue{Value: "8"}, Amount: -4},
			})
			return err
		},
		"match": func() error {
			_, err := ReplayMatch(context.Background(), "204", []string{"3"}, []MatchTxn{{TxnID: "4"}})
			return err
		},
	} {
		start := time.Now()
		err := call()
		assertFast(t, time.Since(start))
		if !errors.Is(err, ErrNoCredentials) {
			t.Fatalf("%s: got %v, want ErrNoCredentials", name, err)
		}
	}
}

// TestFeedMutationReplaysEmptyIDsFailFast asserts the input-class guards run
// before the credentials check — a malformed call fails as an input error
// even with no session.
func TestFeedMutationReplaysEmptyIDsFailFast(t *testing.T) {
	noCredsDir(t)
	if _, err := ReplayBatchAccept(context.Background(), "204", nil); !errors.Is(err, ErrEmptyBatchAcceptIDs) {
		t.Fatalf("batch-accept: got %v, want ErrEmptyBatchAcceptIDs", err)
	}
	if _, err := ReplayCategorise(context.Background(), "204", nil, TransactionDetail{CategoryRef: &RefValue{Value: "7"}}); !errors.Is(err, ErrEmptyCategoriseIDs) {
		t.Fatalf("categorise: got %v, want ErrEmptyCategoriseIDs", err)
	}
	if _, err := ReplaySplit(context.Background(), "204", nil, "Expense", []SplitLine{
		{CategoryRef: RefValue{Value: "7"}, Amount: -6},
		{CategoryRef: RefValue{Value: "8"}, Amount: -4},
	}); !errors.Is(err, ErrEmptySplitIDs) {
		t.Fatalf("split: got %v, want ErrEmptySplitIDs", err)
	}
	if _, err := ReplayMatch(context.Background(), "204", nil, []MatchTxn{{TxnID: "4"}}); !errors.Is(err, ErrEmptyMatchIDs) {
		t.Fatalf("match: got %v, want ErrEmptyMatchIDs", err)
	}
}

// --- unit: feedRowMatchesID ---------------------------------------------------

func TestFeedRowMatchesID(t *testing.T) {
	row := feedRowFixture("3")
	for _, id := range []string{"3", "3:ofx"} {
		if !feedRowMatchesID(row, id) {
			t.Fatalf("id %q did not match row", id)
		}
	}
	for _, id := range []string{"", "4", "ofx", "3:OFX"} {
		if feedRowMatchesID(row, id) {
			t.Fatalf("id %q wrongly matched row", id)
		}
	}
}
