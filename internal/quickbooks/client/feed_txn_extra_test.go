package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// --- transfer ---------------------------------------------------------------

// TestReplayTransferContract asserts transfer rides
// batchAcceptTransactions?acceptOnly=true with each full feed row shaped to
// the captured Transfer accept: acceptType TRANSFER, transfer:true,
// addAsQboTxn.txnTypeId 26, txnDate from the row, and one detail whose
// categoryId is the destination account (live contract TC2 2026-09-19:
// source 44 → destination 45).
func TestReplayTransferContract(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, []map[string]any{feedRowFixture("9")}, nil)
	interceptHTTP(t, ms.URL)
	_, err := ReplayTransfer(context.Background(), "44", []string{"9"}, "45")
	if !errors.Is(err, ErrReadbackUnavailable) || ms.postCount() != 0 {
		t.Fatalf("unverified transfer must be blocked before POST: %v", err)
	}
}

// TestReplayTransferValidationFailsFast asserts input guards run before any
// dial: empty ids, empty destination account.
func TestReplayTransferValidationFailsFast(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, []map[string]any{feedRowFixture("9")}, nil)
	interceptHTTP(t, ms.URL)

	if _, err := ReplayTransfer(context.Background(), "44", nil, "45"); !errors.Is(err, ErrEmptyTransferIDs) {
		t.Fatalf("empty ids: got %v, want ErrEmptyTransferIDs", err)
	}
	if _, err := ReplayTransfer(context.Background(), "44", []string{"9"}, "  "); !errors.Is(err, ErrEmptyTransferAccount) {
		t.Fatalf("empty account: got %v, want ErrEmptyTransferAccount", err)
	}
	if ms.txCallCount() != 0 || ms.postCount() != 0 {
		t.Fatalf("dialled on invalid input (tx=%d posts=%d)", ms.txCallCount(), ms.postCount())
	}
}

// TestReplayTransferUnknownRowFails asserts an id absent from the pending
// feed fails with ErrFeedRowsNotFound before the mutation POST.
func TestReplayTransferUnknownRowFails(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, []map[string]any{feedRowFixture("9")}, nil)
	interceptHTTP(t, ms.URL)

	_, err := ReplayTransfer(context.Background(), "44", []string{"77"}, "45")
	if !errors.Is(err, ErrFeedRowsNotFound) {
		t.Fatalf("got %v, want ErrFeedRowsNotFound", err)
	}
	if ms.postCount() != 0 {
		t.Fatalf("mutation POST sent for missing row (%d posts)", ms.postCount())
	}
}

// --- unpost -----------------------------------------------------------------

// TestReplayUnpostContract asserts unpost POSTs undoTransactions with
// nextTxnInfo.reviewState ACCEPTED and the olbTxnIds list — the Posted
// tab's Undo contract (verified live TC2 2026-09-19).
func TestReplayUnpostContract(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, nil, nil)
	row := feedRowFixture("9")
	row["qboAccountId"] = "44"
	row["matchedQboTxns"] = []any{map[string]any{"qboTxnId": "7", "txnFdmName": "Purchase", "clearState": "CLEARED"}}
	ms.regRows = []map[string]any{{"txnId": "7", "lineAccountId": "44", "clearState": 1}}
	ms.bookRecords = map[string]map[string]any{"7": {"Id": "7", "TotalAmt": 25.0}}
	ms.acceptedItems = []map[string]any{row}
	interceptHTTP(t, ms.URL)

	if err := ReplayUnpost(context.Background(), "44", []string{"9"}); err != nil {
		t.Fatalf("ReplayUnpost: %v", err)
	}
	post := ms.lastPost(t)
	if !strings.HasSuffix(post.path, "/olb/ng/undoTransactions") {
		t.Fatalf("POST path = %q, want .../olb/ng/undoTransactions", post.path)
	}
	body := decodePostBody(t, post)
	info := body["nextTxnInfo"].(map[string]any)
	if info["accountId"] != "44" || info["reviewState"] != "ACCEPTED" ||
		info["nextTransactionIndex"] != float64(-1) || info["sort"] != "-txnDate" {
		t.Fatalf("nextTxnInfo = %+v, want accountId 44 reviewState ACCEPTED", info)
	}
	ids := body["txnIdList"].(map[string]any)["olbTxnIds"].([]any)
	if len(ids) != 1 || ids[0] != "9" {
		t.Fatalf("olbTxnIds = %v, want [9]", ids)
	}
	if ms.txCallCount() < 3 || ms.statesRead[0] != "ACCEPTED" {
		t.Fatalf("unpost must read accepted before-state and verify transition: %v", ms.statesRead)
	}
}

// TestReplayUndoStillExcludes asserts the shared undo builder keeps the
// EXCLUDED review state for undo-excluded.
func TestReplayUndoStillExcludes(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, nil, nil)
	row := feedRowFixture("9")
	row["qboAccountId"] = "44"
	ms.excludedItems = []map[string]any{row}
	interceptHTTP(t, ms.URL)

	if err := ReplayUndo(context.Background(), "44", []string{"9"}); err != nil {
		t.Fatalf("ReplayUndo: %v", err)
	}
	body := decodePostBody(t, ms.lastPost(t))
	info := body["nextTxnInfo"].(map[string]any)
	if info["reviewState"] != "EXCLUDED" {
		t.Fatalf("reviewState = %v, want EXCLUDED", info["reviewState"])
	}
}

// TestPlanTransferUnpost asserts the dry-run builders reject empty input and
// carry the right endpoint + review state without dialing.
func TestPlanTransferUnpost(t *testing.T) {
	if _, err := PlanTransfer("44", nil, "45"); !errors.Is(err, ErrEmptyTransferIDs) {
		t.Fatalf("PlanTransfer empty ids: got %v", err)
	}
	if _, err := PlanTransfer("44", []string{"9"}, ""); !errors.Is(err, ErrEmptyTransferAccount) {
		t.Fatalf("PlanTransfer empty account: got %v", err)
	}
	tp, err := PlanTransfer("44", []string{"9"}, "45")
	if err != nil {
		t.Fatalf("PlanTransfer: %v", err)
	}
	if !strings.Contains(tp.URL, "batchAcceptTransactions") || tp.Method != http.MethodPost || tp.Dial {
		t.Fatalf("plan = %+v", tp)
	}
	if _, err := PlanUnpost("44", nil); !errors.Is(err, ErrEmptyUndoIDs) {
		t.Fatalf("PlanUnpost empty ids: got %v", err)
	}
	up, err := PlanUnpost("44", []string{"9"})
	if err != nil {
		t.Fatalf("PlanUnpost: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(up.Body, &body); err != nil {
		t.Fatalf("plan body not JSON: %v", err)
	}
	info := body["nextTxnInfo"].(map[string]any)
	if info["reviewState"] != "ACCEPTED" {
		t.Fatalf("plan reviewState = %v, want ACCEPTED", info["reviewState"])
	}
}

// --- attach -----------------------------------------------------------------

// attachServer serves the v3 upload + attachable endpoints and captures
// each POST body for assertion.
type attachServer struct {
	*httptest.Server
	mu    sync.Mutex
	posts []capturedPost
}

func newAttachServer(t *testing.T) *attachServer {
	t.Helper()
	as := &attachServer{}
	var uploaded []byte
	var saved map[string]any
	as.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet {
			switch {
			case strings.Contains(r.URL.Path, "/download/"):
				_, _ = w.Write([]byte("https://qbo.intuit.com/test-upload-content"))
			case r.URL.Path == "/test-upload-content":
				_, _ = w.Write(uploaded)
			default:
				if saved == nil {
					saved = map[string]any{"Id": "1000000202", "FileName": "probe.txt", "SyncToken": "0"}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"Attachable": saved})
			}
			return
		}
		body, _ := io.ReadAll(r.Body)
		as.mu.Lock()
		as.posts = append(as.posts, capturedPost{path: r.URL.Path, query: r.URL.RawQuery, body: body})
		as.mu.Unlock()
		switch {
		case strings.HasSuffix(r.URL.Path, "/upload"):
			r.Body = io.NopCloser(bytes.NewReader(body))
			f, _, err := r.FormFile("file_content_0")
			if err != nil {
				t.Error(err)
			} else {
				uploaded, _ = io.ReadAll(f)
				_ = f.Close()
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"AttachableResponse": []any{map[string]any{
					"Attachable": map[string]any{"Id": "1000000202", "FileName": "probe.txt"},
				}},
			})
		case strings.HasSuffix(r.URL.Path, "/attachable"):
			var submitted map[string]any
			_ = json.Unmarshal(body, &submitted)
			if saved == nil {
				saved = map[string]any{}
			}
			maps.Copy(saved, submitted)
			id := jsonNumberString(submitted["Id"])
			if id == "" {
				id = "1000000203"
			}
			saved["Id"] = id
			saved["SyncToken"] = "1"
			_ = json.NewEncoder(w).Encode(map[string]any{
				"Attachable": map[string]any{"Id": id, "SyncToken": "1"},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(as.Close)
	return as
}

func (as *attachServer) postsWith(suffix string) []capturedPost {
	as.mu.Lock()
	defer as.mu.Unlock()
	var out []capturedPost
	for _, p := range as.posts {
		if strings.HasSuffix(p.path, suffix) {
			out = append(out, p)
		}
	}
	return out
}

// TestReplayTxnAttachNoteContract asserts --note alone POSTs a note-only
// Attachable carrying AttachableRef EntityRef{type,value} (verified live
// TC2 2026-09-19: created id 1000000181 on Invoice 7).
func TestReplayTxnAttachNoteContract(t *testing.T) {
	saveUsable(t)
	as := newAttachServer(t)
	interceptHTTP(t, as.URL)

	res, err := ReplayTxnAttach(context.Background(), "Invoice", "7", "probe note", "", nil)
	if err != nil {
		t.Fatalf("ReplayTxnAttach: %v", err)
	}
	if res.AttachableID != "1000000203" || res.TxnType != "Invoice" || res.TxnID != "7" {
		t.Fatalf("result = %+v", res)
	}
	posts := as.postsWith("/attachable")
	if len(posts) != 1 {
		t.Fatalf("attachable posts = %d, want 1", len(posts))
	}
	body := decodePostBody(t, posts[0])
	if body["Note"] != "probe note" {
		t.Fatalf("Note = %v", body["Note"])
	}
	refs := body["AttachableRef"].([]any)
	if len(refs) != 1 {
		t.Fatalf("AttachableRef = %v, want 1", refs)
	}
	er := refs[0].(map[string]any)["EntityRef"].(map[string]any)
	if er["type"] != "Invoice" || er["value"] != "7" {
		t.Fatalf("EntityRef = %+v, want Invoice/7", er)
	}
	if len(as.postsWith("/upload")) != 0 {
		t.Fatal("note-only attach must not hit /upload")
	}
}

// TestReplayTxnAttachFileContract asserts --file uploads via /upload then
// sparse-links the new Attachable id with AttachableRef — and --note rides
// the same link update when both are given.
func TestReplayTxnAttachFileContract(t *testing.T) {
	saveUsable(t)
	as := newAttachServer(t)
	interceptHTTP(t, as.URL)

	res, err := ReplayTxnAttach(context.Background(), "Invoice", "7", "receipt", "probe.txt", []byte("data"))
	if err != nil {
		t.Fatalf("ReplayTxnAttach: %v", err)
	}
	if res.AttachableID != "1000000202" || res.FileName != "probe.txt" {
		t.Fatalf("result = %+v", res)
	}
	if len(as.postsWith("/upload")) != 1 {
		t.Fatalf("upload posts = %d, want 1", len(as.postsWith("/upload")))
	}
	links := as.postsWith("/attachable")
	if len(links) != 1 {
		t.Fatalf("attachable link posts = %d, want 1", len(links))
	}
	body := decodePostBody(t, links[0])
	if body["Id"] != "1000000202" || body["SyncToken"] != "0" || body["sparse"] != true {
		t.Fatalf("link body = %+v, want sparse link on uploaded id", body)
	}
	if body["Note"] != "receipt" {
		t.Fatalf("Note = %v, want receipt", body["Note"])
	}
	er := body["AttachableRef"].([]any)[0].(map[string]any)["EntityRef"].(map[string]any)
	if er["type"] != "Invoice" || er["value"] != "7" {
		t.Fatalf("EntityRef = %+v, want Invoice/7", er)
	}
}

// TestReplayTxnAttachValidation asserts input guards run before any dial.
func TestReplayTxnAttachValidation(t *testing.T) {
	saveUsable(t)
	as := newAttachServer(t)
	interceptHTTP(t, as.URL)

	if _, err := ReplayTxnAttach(context.Background(), "Invoice", "", "n", "", nil); !errors.Is(err, errAttachMissingID) {
		t.Fatalf("missing id: got %v", err)
	}
	if _, err := ReplayTxnAttach(context.Background(), "", "7", "n", "", nil); !errors.Is(err, errAttachMissingType) {
		t.Fatalf("missing type: got %v", err)
	}
	if _, err := ReplayTxnAttach(context.Background(), "Invoice", "7", "", "", nil); !errors.Is(err, errAttachMissingBody) {
		t.Fatalf("missing body: got %v", err)
	}
	if len(as.posts) != 0 {
		t.Fatalf("dialled on invalid input (%d posts)", len(as.posts))
	}
}

// TestPlanTxnAttach asserts the dry-run builder validates and describes the
// two-step contract without dialing.
func TestPlanTxnAttach(t *testing.T) {
	if _, err := PlanTxnAttach("Invoice", "", "n", ""); err == nil {
		t.Fatal("empty id must fail")
	}
	if _, err := PlanTxnAttach("Invoice", "7", "", ""); err == nil {
		t.Fatal("empty note+file must fail")
	}
	p, err := PlanTxnAttach("Invoice", "7", "n", "f.pdf")
	if err != nil {
		t.Fatalf("PlanTxnAttach: %v", err)
	}
	if p.Dial || p.Method != http.MethodPost || !strings.Contains(p.URL, "/attachable") {
		t.Fatalf("plan = %+v", p)
	}
	if !strings.Contains(p.Note, "upload") {
		t.Fatalf("file plan note = %q, want upload step", p.Note)
	}
}
