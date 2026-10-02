package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestManagedProfileDirScopedToHome(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	dir := ManagedProfileDir()
	if !strings.HasPrefix(dir, t.TempDir()) && !strings.Contains(dir, "profiles") {
		t.Errorf("profile dir = %q, want under temp QB_HOME profiles", dir)
	}
	if !strings.HasSuffix(dir, "qbo") {
		t.Errorf("profile dir = %q, want qbo leaf", dir)
	}
}

func TestManagedSessionCookiesRemainSessionCookies(t *testing.T) {
	for _, expiry := range []float64{-1, 0} {
		if !managedCookieExpiry(expiry).IsZero() {
			t.Fatal("CDP session cookie became an expired persistent cookie")
		}
	}
	if got := managedCookieExpiry(1700000000); !got.Equal(time.Unix(1700000000, 0)) {
		t.Fatal("persistent cookie expiry changed")
	}
}

func TestManagedProfileCannotFollowUserProfileSymlinks(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	other := t.TempDir()
	if err := os.Symlink(other, filepath.Join(HomeDir(), "profiles")); err != nil {
		t.Fatal(err)
	}
	if err := prepareManagedProfile(); err == nil {
		t.Fatal("managed browser followed an unowned profile symlink")
	}
	if _, err := os.Stat(filepath.Join(other, "qbo")); !os.IsNotExist(err) {
		t.Fatal("created files outside the owned profile")
	}
}

func TestHeaderMapStringifies(t *testing.T) {
	t.Parallel()
	got := headerMap(map[string]any{"Authorization": "Intuit_APIKey x", "N": 1})
	if got["Authorization"] != "Intuit_APIKey x" || got["N"] != "1" {
		t.Errorf("headerMap = %v", got)
	}
}
