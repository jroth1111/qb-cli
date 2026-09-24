package auth

import "fmt"

// restoreCapturedAliases supports older header-map-only captures in memory.
// It never rewrites the credential file, replaces typed values, derives an
// identity, or borrows credentials from a different profile/host.
func (t *TokenSet) restoreCapturedAliases() error {
	value := headerGet(t.RequestHeaders, "authorization")
	if !isIntuitAPIKey(value) {
		return nil
	}
	if realm := headerGet(t.RequestHeaders, "intuit-company-id"); realm != "" && t.RealmID != "" && realm != t.RealmID {
		return fmt.Errorf("captured headers disagree with saved company; capture a fresh login")
	}
	// Conflicting typed authorization is not evidence for filling other fields.
	if t.Authorization != "" && t.Authorization != value {
		return nil
	}
	if t.Authorization == "" {
		t.Authorization = value
	}
	if t.AccessToken == "" {
		t.AccessToken = value
	}
	if t.APIKey == "" {
		t.APIKey = headerGet(t.RequestHeaders, "apikey")
	}
	if t.TokenType == "" {
		t.TokenType = headerGet(t.RequestHeaders, "authtype")
	}
	if t.IntuitAppID == "" {
		t.IntuitAppID = headerGet(t.RequestHeaders, "intuit_appid")
	}
	return nil
}
