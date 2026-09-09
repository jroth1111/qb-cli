package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRemintATSShortDeadlineDoesNotSpawnEgo(t *testing.T) {
	setQBHome(t)
	t.Setenv("QB_RELAY_URL", "http://127.0.0.1:9")
	t.Setenv("PATH", "/usr/bin:/bin")
	ctx, cancel := context.WithTimeout(context.Background(), RemintTimeout)
	defer cancel()
	start := time.Now()
	err := RemintATS(ctx)
	if err == nil {
		t.Fatal("expected remint failure")
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("short remint hung: %v", time.Since(start))
	}
}

func TestRemintFromEgoFakeBinary(t *testing.T) {
	setQBHome(t)
	if err := Save(&TokenSet{
		Authorization: "Intuit_APIKey intuit_apikey=old,intuit_apikey_version=1.0",
		Source:        "mitm-login",
	}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "ego-browser")
	script := `#!/bin/sh
python3 - <<'PY'
import json, os
open(os.environ["QB_CAPTURE_OUT"], "w").write(json.dumps({
  "headers": {"Authorization": "Intuit_APIKey intuit_apikey=new,intuit_apikey_version=1.0", "authtype": "browser_auth"},
  "cookies": [{"name": "qbo.ticket", "value": "v", "domain": ".qbo.intuit.com", "path": "/"}],
}))
print('{"ok":true}')
PY
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := remintFromEgo(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "ego-space" {
		t.Fatalf("source %q", got.Source)
	}
	if got.Authorization != "Intuit_APIKey intuit_apikey=new,intuit_apikey_version=1.0" {
		t.Fatal("authorization not replaced")
	}
}

func TestTryRelayRemintNoAuthTab(t *testing.T) {
	t.Setenv("QB_RELAY_URL", "http://127.0.0.1:9")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := TryRelayRemint(ctx)
	if err == nil {
		t.Fatal("dead relay must fail")
	}
	if errors.Is(err, ErrEgoMissing) {
		t.Fatal("relay remint must not require ego")
	}
}
