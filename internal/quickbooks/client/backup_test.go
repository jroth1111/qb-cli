package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Backup create posts {"data":{"backupType","companyRealmId"}} to
// backuprestore.api.intuit.com/v1/backup. The realm comes from the stored
// credentials, not a flag — the test seeds one through the usual client env.

func backupServer(t *testing.T) *domServer {
	t.Helper()
	saveUsableURIHost(t)
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		s.mu.Lock()
		s.calls++
		s.lastMeth = r.Method
		s.lastPath = r.URL.Path
		s.lastBody = b
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/v1/backup") && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"157118525","backupType":"incremental","status":"QUEUED"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"error":[{"message":"unmatched"}]}`))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func TestBackupCreatePostsRealmAndType(t *testing.T) {
	srv := backupServer(t)
	res, err := ReplayBackupCreate(context.Background(), "incremental")
	if err != nil {
		t.Fatalf("backup create: %v", err)
	}
	if res.Status != http.StatusCreated || res.Op != "backup create" {
		t.Fatalf("result = %+v", res)
	}
	vars := srv.sentMap()
	data, ok := vars["data"].(map[string]any)
	if !ok {
		t.Fatalf("body = %v", vars)
	}
	if data["backupType"] != "incremental" {
		t.Fatalf("backupType = %v", data["backupType"])
	}
	if data["companyRealmId"] == "" {
		t.Fatal("companyRealmId empty — realm not sourced from credentials")
	}
}

func TestBackupCreateDefaultsIncremental(t *testing.T) {
	srv := backupServer(t)
	if _, err := ReplayBackupCreate(context.Background(), " "); err != nil {
		t.Fatalf("backup create: %v", err)
	}
	vars := srv.sentMap()
	if vars["data"].(map[string]any)["backupType"] != "incremental" {
		t.Fatalf("vars = %v", vars)
	}
}

func TestBackupCreateAcceptsFullAndComplete(t *testing.T) {
	backupServer(t)
	for _, ty := range []string{"full", "complete", "FULL", " Incremental "} {
		if _, err := ReplayBackupCreate(context.Background(), ty); err != nil {
			t.Fatalf("type %q: %v", ty, err)
		}
	}
}

func TestBackupCreateRejectsBadType(t *testing.T) {
	for _, ty := range []string{"partial", "manual", "all", "incremental,full"} {
		if _, err := ReplayBackupCreate(context.Background(), ty); err == nil ||
			!strings.Contains(err.Error(), "incremental | full | complete") {
			t.Fatalf("type %q: err = %v", ty, err)
		}
	}
}
