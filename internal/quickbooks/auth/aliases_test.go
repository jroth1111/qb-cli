package auth

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRestoresCapturedAliasesWithoutWriting(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, credentialsFile)
	raw := []byte(`{"realm_id":"123","request_headers":{"Authorization":"Intuit_APIKey test","ApiKey":"key","AuthType":"browser_auth","Intuit_AppID":"app","Intuit-Company-Id":"123"}}`)
	if err := os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	tok, err := loadAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Authorization != "Intuit_APIKey test" || tok.AccessToken != tok.Authorization || tok.APIKey != "key" || tok.TokenType != "browser_auth" || tok.IntuitAppID != "app" {
		t.Fatal("missing typed aliases")
	}
	after, err := os.ReadFile(file)
	if err != nil || !bytes.Equal(raw, after) {
		t.Fatal("load changed credential file")
	}
}

func TestCapturedAliasesPreserveTypedValuesAndIdentity(t *testing.T) {
	tok := &TokenSet{RealmID: "123", APIKey: "existing", RequestHeaders: map[string]string{"Authorization": "Intuit_APIKey test", "apikey": "other"}}
	if err := tok.restoreCapturedAliases(); err != nil {
		t.Fatal(err)
	}
	if tok.APIKey != "existing" || tok.RealmID != "123" || tok.Email != "" {
		t.Fatal("overwrote typed values or invented identity")
	}
	tok.RequestHeaders["intuit-company-id"] = "456"
	if tok.restoreCapturedAliases() == nil {
		t.Fatal("accepted mismatched realm")
	}
}

func TestCapturedAliasesDoNotPromoteOtherAuthSchemes(t *testing.T) {
	tok := &TokenSet{RequestHeaders: map[string]string{"authorization": "Bearer other"}}
	if err := tok.restoreCapturedAliases(); err != nil {
		t.Fatal(err)
	}
	if tok.Authorization != "" || tok.AccessToken != "" {
		t.Fatal("promoted unrelated authorization")
	}
}
