package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsUpdateRejectsMissingReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), "qbAppFoundationQbSettings") && !strings.Contains(string(body), "UpdateSettings") {
			_, _ = w.Write([]byte(`{"data":{"qbAppFoundationQbSettings":{"finance":{"accounting":{"accountingCore":{"accountingCoreSettings":{"entityVersion":"1"}}}}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"updateQbAppFoundationQbSettings":null}}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	_, err := ReplaySettingsUpdate(context.Background(), map[string]any{"accountNumbersEnabled": true})
	if err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}
