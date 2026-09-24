package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBatchPartialPersistenceIsUnverifiedAndNeverReplayed(t *testing.T) {
	saveUsable(t)
	writes, reads := 0, 0
	state := map[string]map[string]any{"1": {"Id": "1", "SyncToken": "1", "Name": "old1"}, "2": {"Id": "2", "SyncToken": "1", "Name": "old2"}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			writes++
			state["1"]["Name"] = "new1"
			_ = json.NewEncoder(w).Encode(map[string]any{"BatchItemResponse": []any{map[string]any{"bId": "a", "Class": map[string]any{"Id": "1"}}, map[string]any{"bId": "b", "Class": map[string]any{"Id": "2"}}}})
			return
		}
		reads++
		id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		_ = json.NewEncoder(w).Encode(map[string]any{"Class": state[id]})
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	items, err := ReplayBatch(context.Background(), `[{"bId":"a","operation":"update","Class":{"Id":"1","SyncToken":"1","Name":"new1","sparse":true}},{"bId":"b","operation":"update","Class":{"Id":"2","SyncToken":"1","Name":"new2","sparse":true}}]`)
	if !errors.Is(err, ErrMutationUnverified) || len(items) != 2 || writes != 1 || reads != 4 {
		t.Fatalf("partial=%d writes=%d reads=%d err=%v", len(items), writes, reads, err)
	}
}

func TestFeedPostingReceiptDoesNotProveStateChange(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, []map[string]any{feedRowFixture("3")}, nil)
	ms.suppressStateMove = true
	interceptHTTP(t, ms.URL)
	_, err := ReplayBatchAccept(context.Background(), "204", []string{"3"})
	if !errors.Is(err, ErrMutationUnverified) || ms.postCount() != 1 {
		t.Fatalf("false posting success/replayed write: %v", err)
	}
}

func TestCategoriseEmptyMemoClearMustBeReadBack(t *testing.T) {
	saveUsable(t)
	ms := newMutationServer(t, []map[string]any{feedRowFixture("3")}, nil)
	stale, clear := "stale note", ""
	ms.memoOverride = &stale
	interceptHTTP(t, ms.URL)
	_, err := ReplayCategorise(context.Background(), "204", []string{"3"}, TransactionDetail{CategoryRef: &RefValue{Value: "7"}, Memo: &clear})
	if !errors.Is(err, ErrMutationUnverified) || ms.postCount() != 1 {
		t.Fatalf("ignored memo clear or replayed write: %v", err)
	}
}

func TestAttachNoteReceiptDoesNotProveSavedNote(t *testing.T) {
	saveUsable(t)
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			_, _ = w.Write([]byte(`{"Attachable":{"Id":"9"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"Attachable":{"Id":"9","Note":"old","AttachableRef":[{"EntityRef":{"type":"Purchase","value":"51"}}]}}`))
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	_, err := ReplayTxnAttach(context.Background(), "Purchase", "51", "wanted note", "", nil)
	if !errors.Is(err, ErrMutationUnverified) || posts != 1 {
		t.Fatalf("false note success/replay: %v", err)
	}
}

func TestRuleSaveReceiptDoesNotProveChangedRule(t *testing.T) {
	saveUsableURIHost(t)
	posts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			posts++
			_, _ = w.Write([]byte(`{"olbRule":{"id":4,"ruleName":"wanted"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"rules":[{"id":4,"ruleName":"old","editSequence":1,"ruleOrder":1}]}`))
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	_, err := ReplayRuleSave(context.Background(), map[string]string{"id": "4", "name": "wanted"})
	if !errors.Is(err, ErrMutationUnverified) || posts != 1 {
		t.Fatalf("false rule success/replay: %v", err)
	}
}
