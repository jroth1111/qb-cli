package auth

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEgoCaptureNeverRetakesUserControlledSpace(t *testing.T) {
	dir, err := scriptDir()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, egoCaptureScript))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "claimTaskSpace(") {
		t.Fatal("capture may silently retake a user-controlled space")
	}
	if !strings.Contains(string(body), "QB_KEEP_CAPTURE_SPACE") {
		t.Fatal("login retention flag not consumed")
	}
}

func TestCaptureATSFromEgoMissingBinary(t *testing.T) {
	t.Setenv("PATH", "/usr/bin:/bin")
	_, err := CaptureATSFromEgo(context.Background(), "", "")
	if !errors.Is(err, ErrEgoMissing) {
		t.Fatalf("got %v, want ErrEgoMissing", err)
	}
}

func TestCaptureATSFromEgoTimeoutReportsDeadlineNotSignal(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ego-browser")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 10\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := CaptureATSFromEgo(ctx, "", "")
	if !errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "signal: killed") {
		t.Fatalf("timeout presented as process failure: %v", err)
	}
}

func TestCaptureATSFromEgoFakeBinary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ego-browser")
	script := `#!/bin/sh
# Fake ego-browser: ignore stdin, write a capture file, print redacted JSON.
python3 - <<'PY'
import json, os
out = os.environ["QB_CAPTURE_OUT"]
cap = {
  "headers": {
    "Authorization": "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
    "authtype": "browser_auth",
    "csrftoken": "short",
    "x-csrf-token": "long-distinct",
  },
  "audit_authorization": "Intuit_APIKey intuit_apikey=audit,intuit_apikey_version=1.0",
  "cookies": [{"name": "qbo.ticket", "value": "v", "domain": ".qbo.intuit.com", "path": "/", "secure": True, "http_only": True}],
}
open(out, "w").write(json.dumps(cap))
print(json.dumps({"ok": True, "header_count": 4, "cookie_count": 1}))
PY
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cap, err := CaptureATSFromEgo(ctx, DefaultLoginURL, BankingCaptureURL)
	if err != nil {
		t.Fatal(err)
	}
	if !isIntuitAPIKey(headerGet(cap.Headers, "authorization")) {
		t.Fatal("missing Intuit_APIKey")
	}
	if headerGet(cap.Headers, "x-csrf-token") == headerGet(cap.Headers, "csrftoken") {
		t.Fatal("csrf pair must stay distinct")
	}
	if cap.AuditAuthorization != "Intuit_APIKey intuit_apikey=audit,intuit_apikey_version=1.0" {
		t.Fatalf("audit authorization was not retained: length=%d", len(cap.AuditAuthorization))
	}
	if len(cap.Cookies) != 1 || cap.Cookies[0].Name != "qbo.ticket" {
		t.Fatalf("cookies %+v", cap.Cookies)
	}
	// Fake stdout must not be required to contain secrets; the capture file is the source.
	_ = json.RawMessage(nil)
}

func TestCaptureATSFromEgoFakeBinaryNoAuthRejected(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "ego-browser")
	script := `#!/bin/sh
python3 - <<'PY'
import json, os
open(os.environ["QB_CAPTURE_OUT"], "w").write(json.dumps({"headers": {"Accept": "*/*"}, "cookies": []}))
print('{"ok":true}')
PY
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := CaptureATSFromEgo(ctx, "", "")
	if !errors.Is(err, ErrNoATSAuthorization) {
		t.Fatalf("got %v, want ErrNoATSAuthorization", err)
	}
}
