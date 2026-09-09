package auth

import (
	"strings"
	"testing"
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

func TestHeaderMapStringifies(t *testing.T) {
	t.Parallel()
	got := headerMap(map[string]any{"Authorization": "Intuit_APIKey x", "N": 1})
	if got["Authorization"] != "Intuit_APIKey x" || got["N"] != "1" {
		t.Errorf("headerMap = %v", got)
	}
}
