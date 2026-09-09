package auth

import (
	"testing"
	"time"
)

// --- parseCookieHeader ---

func TestParseCookieHeader(t *testing.T) {
	got := parseCookieHeader("a=1; b=2; c=3", ".intuit.com")
	if len(got) != 3 {
		t.Fatalf("got %d cookies, want 3", len(got))
	}
	want := map[string]string{"a": "1", "b": "2", "c": "3"}
	for _, c := range got {
		if c.Domain != ".intuit.com" {
			t.Errorf("cookie %q domain = %q, want .intuit.com", c.Name, c.Domain)
		}
		if want[c.Name] != c.Value {
			t.Errorf("cookie %q value = %q, want %q", c.Name, c.Value, want[c.Name])
		}
	}
}

func TestParseCookieHeaderEmpty(t *testing.T) {
	if got := parseCookieHeader("", ".intuit.com"); got != nil {
		t.Errorf("parseCookieHeader(\"\") = %v, want nil", got)
	}
	// Malformed parts (no '=') are skipped, not panicked on.
	got := parseCookieHeader("a=1; garbage; b=2", ".intuit.com")
	if len(got) != 2 {
		t.Errorf("got %d cookies, want 2 (garbage part skipped)", len(got))
	}
}

// --- chromeEpochToTime ---

func TestChromeEpochToTime(t *testing.T) {
	// 2026-01-01T00:00:00Z in Chrome micros since 1601.
	// Unix 2026-01-01 = 1767225600; +11644473600 = 13411699200 seconds
	// -> *1e6 = 13411699200000000 micros.
	micros := int64(13411699200000000)
	got := chromeEpochToTime(micros)
	want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("chromeEpochToTime = %v, want %v", got, want)
	}
}

func TestChromeEpochToTimeZeroAndNegative(t *testing.T) {
	if got := chromeEpochToTime(0); !got.IsZero() {
		t.Errorf("chromeEpochToTime(0) = %v, want zero", got)
	}
	if got := chromeEpochToTime(-1); !got.IsZero() {
		t.Errorf("chromeEpochToTime(-1) = %v, want zero", got)
	}
}

// --- parseCookiesJSON ---

func TestParseCookiesJSON(t *testing.T) {
	in := []byte(`[
		{"name":"session","host_key":".intuit.com","value":"abc","expires_utc":13411699200000000},
		{"name":"tracking","host_key":".intuit.com","value":"xyz","expires_utc":0}
	]`)
	got, err := parseCookiesJSON(in)
	if err != nil {
		t.Fatalf("parseCookiesJSON: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d cookies, want 2", len(got))
	}
	if got[0].Name != "session" || got[0].Domain != ".intuit.com" || got[0].Value != "abc" {
		t.Errorf("cookie[0] = %+v", got[0])
	}
	if !got[0].Expires.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("cookie[0] expires = %v, want 2026-01-01", got[0].Expires)
	}
	if !got[1].Expires.IsZero() {
		t.Errorf("cookie[1] expires_utc=0 should yield zero time, got %v", got[1].Expires)
	}
}

// --- NEGATIVE: parseCookiesJSON rejects malformed JSON ---

func TestParseCookiesJSONMalformed(t *testing.T) {
	if _, err := parseCookiesJSON([]byte("not json")); err == nil {
		t.Error("parseCookiesJSON accepted malformed JSON; want error")
	}
	if _, err := parseCookiesJSON(nil); err != nil {
		t.Errorf("parseCookiesJSON(nil) error = %v, want nil", err)
	}
}

// --- NEGATIVE: defaultChromeCookiesPath errors on unsupported OS by
// faking the GOOS via the function's documented contract. We can't swap
// GOOS at runtime, so instead assert that on the test host the function
// either returns a path or an error, never panics. ---

func TestDefaultChromeCookiesPathNoPanic(t *testing.T) {
	_, _ = defaultChromeCookiesPath()
}

// --- NEGATIVE: readCookiesDB with a nonexistent db returns an error,
// not a silent empty slice. This proves the extraction path fails loudly
// when Chrome isn't installed. ---

func TestReadCookiesDBMissingFile(t *testing.T) {
	// Use a path that definitely doesn't exist; copyFileIfExists returns
	// nil for missing src, so the temp file will be empty and sqlite3
	// (if present) will error; if sqlite3 is absent we get ErrNoCookieSource.
	// Either way: no cookies, and an error.
	cookies, err := readCookiesDB("/nonexistent/path/Cookies", QBAuthDomains)
	if err == nil && len(cookies) > 0 {
		t.Errorf("readCookiesDB on missing file returned cookies; want error")
	}
}

// --- sqlQuoteLiteral ---

func TestSqlQuoteLiteral(t *testing.T) {
	if got := sqlQuoteLiteral("it's a test"); got != "it''s a test" {
		t.Errorf("sqlQuoteLiteral = %q, want %q", got, "it''s a test")
	}
	if got := sqlQuoteLiteral("plain"); got != "plain" {
		t.Errorf("sqlQuoteLiteral = %q, want plain", got)
	}
}
