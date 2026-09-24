package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBatchQueryReadbackKeepsFullEntityAndAttachment(t *testing.T) {
	saveUsable(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/v3/company/12345/batch" {
			t.Errorf("wrong batch route: %s %s", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"BatchItemResponse":[{"bId":"notes","QueryResponse":{"Attachable":[{"Id":"9","Note":"metadata","AttachableRef":[{"EntityRef":{"type":"Deposit","value":"7"}}]}]}}]}`)
	}))
	defer srv.Close()
	interceptHTTP(t, srv.URL)
	items, err := ReplayBatch(context.Background(), `[{"bId":"notes","Query":"SELECT * FROM Attachable WHERE AttachableRef.EntityRef.value IN ('7') MAXRESULTS 1000"}]`)
	if err != nil {
		t.Fatal(err)
	}
	row := items[0].(map[string]any)["QueryResponse"].(map[string]any)["Attachable"].([]any)[0].(map[string]any)
	if row["Note"] != "metadata" || row["AttachableRef"] == nil {
		t.Fatal("lost full readback")
	}
}

func TestBatchRejectsUncorrelatedOrFailedReceipts(t *testing.T) {
	for _, body := range []string{`{}`, `{"BatchItemResponse":[]}`, `{"BatchItemResponse":[{"bId":"wrong","QueryResponse":{}}]}`, `{"BatchItemResponse":[{"bId":"one","Fault":{"type":"ValidationFault"}}]}`, `{"BatchItemResponse":[{"bId":"one"}]}`} {
		t.Run(body, func(t *testing.T) {
			saveUsable(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			if _, err := ReplayBatch(context.Background(), `[{"bId":"one","Query":"SELECT * FROM Purchase"}]`); err == nil {
				t.Fatal("false batch success")
			}
		})
	}
}

func TestBatchRejectsAmbiguousRequestBeforeCredentials(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	for _, body := range []string{`[{}]`, `[{"bId":"same"},{"bId":"same"}]`} {
		if _, err := ReplayBatch(context.Background(), body); err == nil {
			t.Fatal("accepted ambiguous request")
		}
	}
}
