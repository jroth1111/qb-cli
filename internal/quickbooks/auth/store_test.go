package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// setQBHome returns a temp dir, points QB_HOME at it, and registers cleanup.
// Every test that touches Save/Load/CredPath MUST call this so tests never
// touch the real ~/.config/qb.
func setQBHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("QB_HOME", dir)
	return dir
}

// makeJWT builds a three-segment JWT string with the given exp (Unix seconds).
// The header and signature are dummy; only the payload is decoded.
func makeJWT(exp int64) string {
	header := `{"alg":"none","typ":"JWT"}`
	payload := []byte("{}")
	if exp > 0 {
		payload = []byte(`{"exp":` + itoa(exp) + `}`)
	}
	return b64url(header) + "." + b64url(string(payload)) + ".sig"
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// b64url is base64 raw URL encoding without padding (JWT segment form).
func b64url(s string) string {
	return base64RawURLEncode([]byte(s))
}

func base64RawURLEncode(b []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	var out []byte
	for i := 0; i < len(b); i += 3 {
		var n uint32
		var cnt int
		for j := 0; j < 3; j++ {
			if i+j < len(b) {
				n = (n << 8) | uint32(b[i+j])
				cnt++
			} else {
				n <<= 8
			}
		}
		out = append(out, alphabet[(n>>18)&0x3f])
		out = append(out, alphabet[(n>>12)&0x3f])
		if cnt > 1 {
			out = append(out, alphabet[(n>>6)&0x3f])
		}
		if cnt > 2 {
			out = append(out, alphabet[n&0x3f])
		}
	}
	return string(out)
}

// --- CONTRACT TEST 1: Save/Load round-trip under QB_HOME=temp ---

func TestSaveLoadRoundTrip(t *testing.T) {
	setQBHome(t)
	want := &TokenSet{
		Version:       CurrentVersion,
		CapturedAt:    time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC),
		Source:        "relay-session",
		RealmID:       "1231456789",
		Email:         "tester@example.com",
		Authorization: "Intuit_APIKey intuit_apikey=redacted,intuit_apikey_version=1.0",
		AccessToken:   makeJWT(time.Now().Add(time.Hour).Unix()),
		RefreshToken:  makeJWT(time.Now().Add(30 * 24 * time.Hour).Unix()),

		RequestHeaders: map[string]string{
			"x-csrf-token": "distinct-xcsrf",
			"Accept":       "*/*",
		},
		Cookies: []Cookie{
			{Name: "session", Value: "abc", Domain: ".intuit.com"},
			{Name: "refresh_token", Value: makeJWT(time.Now().Add(30 * 24 * time.Hour).Unix()), Domain: ".qbo.intuit.com"},
		},
	}

	if err := Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// File must exist at CredPath with mode 0600.
	info, err := os.Stat(CredPath())
	if err != nil {
		t.Fatalf("Stat credentials: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("credentials mode = %o, want 0600", perm)
	}

	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.RealmID != want.RealmID || got.Email != want.Email || got.Source != want.Source {
		t.Errorf("round-trip mismatch: got realm=%q email=%q source=%q", got.RealmID, got.Email, got.Source)
	}
	if got.AccessToken != want.AccessToken {
		t.Errorf("access token did not round-trip")
	}
	if got.RequestHeaders["x-csrf-token"] != want.RequestHeaders["x-csrf-token"] {
		t.Errorf("request_headers did not round-trip")
	}
	if len(got.Cookies) != len(want.Cookies) {
		t.Errorf("cookie count: got %d want %d", len(got.Cookies), len(want.Cookies))
	}
}

// --- CONTRACT TEST 2: Save rejects empty TokenSet ---

func TestSaveRejectsEmpty(t *testing.T) {
	setQBHome(t)
	cases := []struct {
		name string
		ts   *TokenSet
	}{
		{"nil", nil},
		{"no cookies no tokens", &TokenSet{Source: "relay-session"}},
		{"empty cookies no tokens", &TokenSet{Cookies: []Cookie{}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Save(tc.ts)
			if err == nil {
				t.Fatalf("Save accepted empty token set; want ErrEmptyTokenSet")
			}
			if err != ErrEmptyTokenSet {
				t.Errorf("Save error = %v, want ErrEmptyTokenSet", err)
			}
			// No file should have been written.
			if _, err := os.Stat(CredPath()); !os.IsNotExist(err) {
				t.Errorf("credentials file exists after rejected Save")
			}
		})
	}
}

// --- CONTRACT TEST 3: LongestExpiry prefers later refresh over short access ---

func TestLongestExpiryPrefersRefresh(t *testing.T) {
	shortAccess := time.Date(2026, 8, 17, 13, 0, 0, 0, time.UTC)
	longRefresh := time.Date(2026, 9, 17, 13, 0, 0, 0, time.UTC)
	ts := &TokenSet{
		AccessExpiry:  shortAccess,
		RefreshExpiry: longRefresh,
		Cookies: []Cookie{
			{Name: "short", Value: "x", Domain: ".intuit.com", Expires: shortAccess},
		},
	}
	got := ts.LongestExpiry()
	if !got.Equal(longRefresh) {
		t.Errorf("LongestExpiry = %v, want %v (refresh should win)", got, longRefresh)
	}
}

func TestLongestExpiryIgnoresZeroTimes(t *testing.T) {
	known := time.Date(2026, 8, 17, 13, 0, 0, 0, time.UTC)
	ts := &TokenSet{
		AccessExpiry: known,
		// RefreshExpiry zero, cookie Expires zero.
		Cookies: []Cookie{{Name: "s", Value: "v", Domain: ".intuit.com"}},
	}
	got := ts.LongestExpiry()
	if !got.Equal(known) {
		t.Errorf("LongestExpiry = %v, want %v (zero times must be ignored)", got, known)
	}
}

func TestLongestExpiryAllZero(t *testing.T) {
	ts := &TokenSet{Cookies: []Cookie{{Name: "s", Value: "v", Domain: ".intuit.com"}}}
	if got := ts.LongestExpiry(); !got.IsZero() {
		t.Errorf("LongestExpiry = %v, want zero when all components are zero", got)
	}
}

// --- CONTRACT TEST 4: RedactedStatus JSON contains no cookie/token values ---

func TestRedactedStatusNoSecrets(t *testing.T) {
	secretCookieValue := "SUPER-SECRET-COOKIE-VALUE-12345"
	secretAccess := "SUPER-SECRET-ACCESS-TOKEN"
	secretRefresh := "SUPER-SECRET-REFRESH-TOKEN"
	secretHeader := "SUPER-SECRET-XCSRF-HEADER"

	ts := &TokenSet{
		Source:        "relay-session",
		RealmID:       "1231456789",
		Email:         "tester@example.com",
		AccessToken:   secretAccess,
		RefreshToken:  secretRefresh,
		AccessExpiry:  time.Now().Add(time.Hour).UTC(),
		RefreshExpiry: time.Now().Add(30 * 24 * time.Hour).UTC(),
		RequestHeaders: map[string]string{
			"x-csrf-token": secretHeader,
		},
		Cookies: []Cookie{
			{Name: "session", Value: secretCookieValue, Domain: ".intuit.com"},
			{Name: "refresh_token", Value: secretRefresh, Domain: ".qbo.intuit.com"},
		},
	}

	status := ts.RedactedStatus()
	b, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("Marshal status: %v", err)
	}
	jsonStr := string(b)

	forbidden := []string{secretCookieValue, secretAccess, secretRefresh, secretHeader}
	for _, secret := range forbidden {
		if strings.Contains(jsonStr, secret) {
			t.Errorf("RedactedStatus JSON leaked secret %q:\n%s", secret, jsonStr)
		}
	}

	// Positive checks: the redacted view SHOULD carry presence flags + count.
	if !status.HasAccessToken {
		t.Error("HasAccessToken should be true")
	}
	if !status.HasRefreshToken {
		t.Error("HasRefreshToken should be true")
	}
	if !status.HasRequestHeaders {
		t.Error("HasRequestHeaders should be true")
	}
	if status.RequestHeaderCount != 1 {
		t.Errorf("RequestHeaderCount = %d, want 1", status.RequestHeaderCount)
	}
	if status.CookieCount != 2 {
		t.Errorf("CookieCount = %d, want 2", status.CookieCount)
	}

}

// --- CONTRACT TEST 5: AuthenticatedURL true only for qbo.intuit.com/app/* ---

func TestAuthenticatedURL(t *testing.T) {
	good := []string{
		"https://qbo.intuit.com/app/homepage",
		"https://qbo.intuit.com/app/customer",
		"https://qbo.intuit.com/app/invoice?txn=123",
	}
	for _, u := range good {
		if !AuthenticatedURL(u) {
			t.Errorf("AuthenticatedURL(%q) = false, want true", u)
		}
	}
	bad := []string{
		DefaultLoginURL,
		"https://accounts.intuit.com/app/sign-in",
		"https://qbo.intuit.com/app/sign-in",
		"https://qbo.intuit.com/UNAUTHENTICATED",
		"https://accounts.intuit.com/",
		"https://example.com/app/homepage",
		"",
	}
	for _, u := range bad {
		if AuthenticatedURL(u) {
			t.Errorf("AuthenticatedURL(%q) = true, want false", u)
		}
	}
}

// --- ClassifyCookies behavior ---

func TestClassifyCookiesPromotesRefreshAndAccess(t *testing.T) {
	now := time.Now().UTC()
	accessExp := now.Add(time.Hour)
	refreshExp := now.Add(30 * 24 * time.Hour)

	ts := &TokenSet{
		Cookies: []Cookie{
			{Name: "access_token", Value: makeJWT(accessExp.Unix()), Domain: ".intuit.com"},
			{Name: "refresh_token", Value: makeJWT(refreshExp.Unix()), Domain: ".qbo.intuit.com"},
			{Name: "tracking_id", Value: "opaque", Domain: ".intuit.com"},
		},
	}
	ts.ClassifyCookies()

	if ts.AccessToken == "" {
		t.Error("ClassifyCookies did not promote access_token")
	}
	if ts.RefreshToken == "" {
		t.Error("ClassifyCookies did not promote refresh_token")
	}
	if ts.AccessExpiry.Unix() != accessExp.Unix() {
		t.Errorf("AccessExpiry = %v, want %v", ts.AccessExpiry, accessExp)
	}
	if ts.RefreshExpiry.Unix() != refreshExp.Unix() {
		t.Errorf("RefreshExpiry = %v, want %v", ts.RefreshExpiry, refreshExp)
	}
	// Non-credential cookie stays in the jar.
	if len(ts.Cookies) != 3 {
		t.Errorf("ClassifyCookies should not remove cookies from jar; got %d", len(ts.Cookies))
	}
}

func TestClassifyCookiesKeepsLongestRefresh(t *testing.T) {
	short := time.Now().Add(time.Hour).UTC()
	long := time.Now().Add(30 * 24 * time.Hour).UTC()
	shortJWT := makeJWT(short.Unix())
	longJWT := makeJWT(long.Unix())

	// Order deliberately puts the short-lived one first to ensure we don't
	// just keep the last seen.
	ts := &TokenSet{
		Cookies: []Cookie{
			{Name: "refresh_token", Value: shortJWT, Domain: ".intuit.com"},
			{Name: "RefreshToken", Value: longJWT, Domain: ".qbo.intuit.com"},
		},
	}
	ts.ClassifyCookies()
	if ts.RefreshToken != longJWT {
		t.Error("ClassifyCookies should keep the longest-lived refresh token")
	}
	if ts.RefreshExpiry.Unix() != long.Unix() {
		t.Errorf("RefreshExpiry = %v, want %v (longest)", ts.RefreshExpiry, long)
	}
}

func TestClassifyCookiesIgnoresNonJWTValues(t *testing.T) {
	ts := &TokenSet{
		Cookies: []Cookie{
			{Name: "access_token", Value: "not-a-jwt", Domain: ".intuit.com"},
			{Name: "refresh_token", Value: "also-not-jwt", Domain: ".intuit.com"},
		},
	}
	ts.ClassifyCookies()
	// Values are still promoted (name match is enough), but expiry stays zero.
	if ts.AccessToken != "not-a-jwt" {
		t.Error("ClassifyCookies should promote by name even without JWT exp")
	}
	if !ts.AccessExpiry.IsZero() {
		t.Errorf("AccessExpiry should be zero for non-JWT, got %v", ts.AccessExpiry)
	}
}

// --- ExtractRealmID ---

func TestExtractRealmID(t *testing.T) {
	cases := []struct {
		url  string
		want string
	}{
		{"https://qbo.intuit.com/app/homepage?realmId=1231456789", "1231456789"},
		{"https://qbo.intuit.com/app/homepage?realmid=1231456789", "1231456789"},
		{"https://qbo.intuit.com/app/homepage?realm=1231456789", "1231456789"},
		{"https://qbo.intuit.com/app/homepage?companyLid=1231456789", "1231456789"},
		{"https://qbo.intuit.com/app/homepage", ""},
		{"https://example.com/", ""},
		{"", ""},
	}
	for _, tc := range cases {
		got := ExtractRealmID(tc.url)
		if got != tc.want {
			t.Errorf("ExtractRealmID(%q) = %q, want %q", tc.url, got, tc.want)
		}
	}
}

// --- HomeDir / CredPath ---

func TestHomeDirQBHomeOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QB_HOME", dir)
	if got := HomeDir(); got != dir {
		t.Errorf("HomeDir = %q, want %q", got, dir)
	}
	want := filepath.Join(dir, "credentials.json")
	if got := CredPath(); got != want {
		t.Errorf("CredPath = %q, want %q", got, want)
	}
}

// --- Load missing file -> typed error ---

func TestLoadMissingFile(t *testing.T) {
	setQBHome(t)
	_, err := Load()
	if err == nil {
		t.Fatal("Load on missing file: want ErrNoCredentials, got nil")
	}
	if err != ErrNoCredentials {
		t.Errorf("Load error = %v, want ErrNoCredentials", err)
	}
}

// --- Delete is idempotent ---

func TestDeleteIdempotent(t *testing.T) {
	setQBHome(t)
	// No file exists yet.
	if err := Delete(); err != nil {
		t.Errorf("Delete on missing file: %v, want nil", err)
	}
	// Create then delete.
	if err := Save(&TokenSet{Authorization: "Intuit_APIKey intuit_apikey=redacted,intuit_apikey_version=1.0", Cookies: []Cookie{{Name: "x", Value: "y", Domain: ".intuit.com"}}}); err != nil {

		t.Fatalf("Save: %v", err)
	}
	if err := Delete(); err != nil {
		t.Errorf("Delete on existing file: %v", err)
	}
	if _, err := os.Stat(CredPath()); !os.IsNotExist(err) {
		t.Errorf("credentials file still exists after Delete")
	}
}

// --- Negative: HasUsableCredential on nil and empty ---

func TestHasUsableCredential(t *testing.T) {
	var nilTS *TokenSet
	if nilTS.HasUsableCredential() {
		t.Error("nil TokenSet should not be usable")
	}
	if (&TokenSet{}).HasUsableCredential() {
		t.Error("empty TokenSet should not be usable")
	}
	if (&TokenSet{AccessToken: "x"}).HasUsableCredential() {
		t.Error("opaque access token without Intuit_APIKey must not be usable")
	}
	if (&TokenSet{Cookies: []Cookie{{Name: "x", Value: "y", Domain: ".intuit.com"}}}).HasUsableCredential() {
		t.Error("cookies without Intuit_APIKey must not be usable")
	}
	if !(&TokenSet{Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0"}).HasUsableCredential() {
		t.Error("Intuit_APIKey Authorization must be usable")
	}

}

// --- Negative: RedactedStatus on nil does not panic and leaks nothing ---

func TestRedactedStatusNilSafe(t *testing.T) {
	var nilTS *TokenSet
	s := nilTS.RedactedStatus()
	if s.OK {
		t.Error("nil TokenSet RedactedStatus should have OK=false")
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "value") {
		t.Errorf("nil RedactedStatus JSON leaked a value field: %s", b)
	}
}

// --- Negative: Save with cookies-only (no tokens) succeeds, proving the
// empty-rejection is about credentials, not about token fields ---

func TestSaveRejectsCookiesOnly(t *testing.T) {
	setQBHome(t)
	ts := &TokenSet{
		Source:  "chrome-cookies",
		Cookies: []Cookie{{Name: "session", Value: "v", Domain: ".intuit.com"}},
	}
	if err := Save(ts); err == nil {
		t.Fatal("Save cookies-only must fail")
	}
}

// --- helper: sorted cookie names for stable assertions ---

func sortedCookieNames(ts *TokenSet) []string {
	names := make([]string, 0, len(ts.Cookies))
	for _, c := range ts.Cookies {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	return names
}

var _ = sortedCookieNames // reserved for future assertions

// --- HasUsableCredential: Authorization-only is usable (canonical path) ---

func TestHasUsableCredentialAuthorizationOnly(t *testing.T) {
	ts := &TokenSet{
		Source:        "mitm-login",
		Authorization: "Intuit_APIKey intuit_apikey=redacted,intuit_apikey_version=1.0",
	}
	if !ts.HasUsableCredential() {
		t.Error("TokenSet with only Authorization should be usable")
	}
}

// --- HasUsableCredential: Intuit_APIKey access token is usable ---

func TestHasUsableCredentialIntuitAPIKeyAccessToken(t *testing.T) {
	ts := &TokenSet{AccessToken: "Intuit_APIKey intuit_apikey=redacted,intuit_apikey_version=1.0"}
	if !ts.HasUsableCredential() {
		t.Error("TokenSet with Intuit_APIKey access token should be usable")
	}
	// A bare opaque access token (not the Intuit_APIKey scheme) is still
	// usable via the generic cookie/token fallback below — but here we
	// assert the scheme-specific path specifically.
}

// --- HasUsableCredential: qbn.ticket cookie only is usable ---

func TestHasUsableCredentialTicketCookieOnly(t *testing.T) {
	ts := &TokenSet{
		Cookies: []Cookie{{Name: "qbn.ticket", Value: "redacted", Domain: ".intuit.com"}},
	}
	if ts.HasUsableCredential() {
		t.Error("qbn.ticket alone must not be usable")
	}
}

func TestHasUsableCredentialQBOTicketCookieOnly(t *testing.T) {
	ts := &TokenSet{
		Cookies: []Cookie{{Name: "qbo.ticket", Value: "redacted", Domain: ".intuit.com"}},
	}
	if ts.HasUsableCredential() {
		t.Error("qbo.ticket alone must not be usable")
	}
}

func TestHasUsableCredentialAnyCookieRejected(t *testing.T) {
	ts := &TokenSet{
		Cookies: []Cookie{{Name: "session", Value: "redacted", Domain: ".intuit.com"}},
	}
	if ts.HasUsableCredential() {
		t.Error("any-cookie jar must not be usable")
	}
}

// --- ImportMitmDump: missing dump file fails (no network, no mitmdump
// spawn on a bad path) ---

func TestImportMitmDumpMissingFile(t *testing.T) {
	setQBHome(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist.mitm")
	err := ImportMitmDump(missing)
	if err == nil {
		t.Fatal("ImportMitmDump of missing file should fail")
	}
	// If mitmdump is absent the error is ErrMitmProxyMissing (covered
	// separately); otherwise the missing dump must surface as a wrapped
	// fs error naming the path, never as ErrNoUsableCredential (the dump
	// never ran, so there is nothing to classify).
	if errors.Is(err, ErrNoUsableCredential) {
		t.Errorf("missing-file failure should not be ErrNoUsableCredential: %v", err)
	}
	if !errors.Is(err, ErrMitmProxyMissing) {
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("missing-file error should name the path %q: %v", missing, err)
		}
	}
}

// --- ImportMitmDump: missing mitmdump binary yields ErrMitmProxyMissing ---
// This test shoves a bogus PATH so exec.LookPath cannot find mitmdump. It
// does not hit the network and does not require a real dump file because
// the binary check runs before the dump is opened.

func TestImportMitmDumpMissingMitmdump(t *testing.T) {
	setQBHome(t)
	// Preserve a minimal PATH with no mitmdump. /usr/bin is kept so core
	// tools still resolve inside exec.LookPath's own machinery.
	t.Setenv("PATH", "/usr/bin:/bin")
	err := ImportMitmDump("/tmp/anything.mitm")
	if !errors.Is(err, ErrMitmProxyMissing) {
		t.Errorf("ImportMitmDump without mitmdump: want ErrMitmProxyMissing, got %v", err)
	}
}

func TestHasUsableCredentialRequestHeadersAuthorization(t *testing.T) {
	ts := &TokenSet{
		RequestHeaders: map[string]string{
			"Authorization": "Intuit_APIKey intuit_apikey=redacted,intuit_apikey_version=1.0",
		},
	}
	if !ts.HasUsableCredential() {
		t.Error("TokenSet with only request_headers.Authorization should be usable")
	}
}

func TestHasUsableCredentialRequestHeadersWithoutAuthRejected(t *testing.T) {
	ts := &TokenSet{
		RequestHeaders: map[string]string{
			"Accept": "*/*",
		},
	}
	if ts.HasUsableCredential() {
		t.Error("headers without Authorization or cookies must not be usable")
	}
}
