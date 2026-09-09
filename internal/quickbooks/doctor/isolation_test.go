package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRealCredentialStoreUnwrittenBySuite runs LAST in the package (Go runs
// tests in source order within a file, and this file is placed after all
// others via a separate final check) — instead of relying on ordering, it
// verifies that if the real store exists, its realm is not any realm used by
// doctor tests (9001/1111/2222). A regression in test isolation writes realm
// 9001 here and this test fails loudly.
func TestRealCredentialStoreNotPollutedByTests(t *testing.T) {
	dir := realQBHomeWithoutTestIsolation()
	cred := filepath.Join(dir, "credentials.json")
	data, err := os.ReadFile(cred)
	if err != nil {
		t.Skip("no real credential store on this machine")
	}
	for _, marker := range []string{"\"9001\"", "\"1111\"", "\"2222\""} {
		if stringContains(string(data), marker) {
			t.Fatalf("real credential store at %s contains test realm %s — "+
				"a doctor/client test wrote outside QB_HOME isolation", cred, marker)
		}
	}
}

func realQBHomeWithoutTestIsolation() string {
	if v := os.Getenv("QB_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "qb")
	}
	return filepath.Join(home, ".config", "qb")
}

func stringContains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
