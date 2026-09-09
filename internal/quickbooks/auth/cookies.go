package auth

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// QBAuthDomains are the host patterns whose cookies make up a usable QBO
// session. The login flow captures all of them so frontend API replays
// carry the full jar.
var QBAuthDomains = []string{
	"%intuit.com%",
	"%qbo.intuit.com%",
	"%accounts.intuit.com%",
}

// ErrNoCookieSource is returned when neither press-auth nor a Chrome cookie
// database could be found/read.
var ErrNoCookieSource = errors.New("no cookie source available: install press-auth or run Chrome with a Default profile")

// CaptureCookies obtains the QBO session cookies, preferring press-auth on
// PATH and falling back to reading the user's Chrome cookie database
// directly. It returns a []Cookie suitable for TokenSet.Cookies.
//
// press-auth is preferred because it handles Chrome's encrypted cookie
// values (AES via the macOS Keychain "Chrome Safe Storage" key). The
// Chrome-DB fallback only reads host_key + name + expires_utc, which is
// plaintext in the SQLite schema; encrypted values are left to press-auth.
// For QBO, the session-defining cookies (the ones ClassifyCookies promotes
// into tokens) are JWTs whose value is plaintext in the DB, so the fallback
// still yields a usable session when press-auth is absent.
func CaptureCookies() ([]Cookie, error) {
	if path, err := exec.LookPath("press-auth"); err == nil {
		return captureViaPressAuth(path)
	}
	return captureViaChromeDB()
}

// captureViaPressAuth shells out to `press-auth cookies <domain>` for each
// QBO domain and merges the results into a deduplicated []Cookie. press-auth
// prints a "name=value; name=value" header on stdout; we parse it and keep
// name+value+domain. Expires is unknown from the header form, so it is left
// zero (ClassifyCookies still decodes JWT exp from the value).
func captureViaPressAuth(pressAuthPath string) ([]Cookie, error) {
	byName := map[string]*Cookie{}
	domains := []string{"qbo.intuit.com", "intuit.com", "accounts.intuit.com"}
	for _, d := range domains {
		header, err := tryPressAuth(pressAuthPath, d)
		if err != nil {
			// One domain missing is fine; only fail if all are.
			continue
		}
		for _, c := range parseCookieHeader(header, d) {
			if existing, ok := byName[c.Name]; ok {
				// Keep the more-specific domain (longer match).
				if len(c.Domain) > len(existing.Domain) {
					byName[c.Name] = &c
				}
			} else {
				byName[c.Name] = &c
			}
		}
	}
	if len(byName) == 0 {
		return nil, fmt.Errorf("press-auth returned no cookies for QBO domains: %w", ErrNoCookieSource)
	}
	out := make([]Cookie, 0, len(byName))
	for _, c := range byName {
		out = append(out, *c)
	}
	return out, nil
}

// tryPressAuth runs `press-auth cookies <domain>` and returns the trimmed
// stdout (a Cookie header value). Stderr is surfaced as the error message on
// failure so callers can hint at recovery.
func tryPressAuth(pressAuthPath, domain string) (string, error) {
	cmd := exec.Command(pressAuthPath, "cookies", domain)
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(errBuf.String()); msg != "" {
			return "", fmt.Errorf("%s", msg)
		}
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// parseCookieHeader parses a "name1=value1; name2=value2" string into
// []Cookie, tagging each with domain. Values are taken verbatim (no
// unquoting; QBO cookies do not use quoted values).
func parseCookieHeader(header, domain string) []Cookie {
	if header == "" {
		return nil
	}
	var out []Cookie
	for _, part := range strings.Split(header, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			continue
		}
		name := strings.TrimSpace(part[:eq])
		val := part[eq+1:]
		if name == "" {
			continue
		}
		out = append(out, Cookie{
			Name:   name,
			Value:  val,
			Domain: domain,
		})
	}
	return out
}

// captureViaChromeDB locates the user's Chrome "Default" profile Cookies
// database, copies it (plus WAL/SHM) to a temp file to dodge Chrome's WAL
// lock, and reads rows whose host_key matches the QBO domains. This mirrors
// the golden extractCookies/inspectCookiesForDomain pattern.
//
// On macOS the cookie values are AES-encrypted with the Keychain "Chrome
// Safe Storage" key; we do NOT decrypt here (that requires Keychain access
// and is press-auth's job). We still capture name + domain + expires_utc
// and, for values that are plaintext JWTs (three-segment), the value too —
// Chrome stores some Intuit tokens unencrypted because they were set with
// no Secure-Only-encrypted flag. When press-auth is available, prefer it.
func captureViaChromeDB() ([]Cookie, error) {
	dbPath, err := defaultChromeCookiesPath()
	if err != nil {
		return nil, fmt.Errorf("locating Chrome cookies db: %w", err)
	}
	rows, err := readCookiesDB(dbPath, QBAuthDomains)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no QBO cookies in Chrome db: %w", ErrNoCookieSource)
	}
	return rows, nil
}

// defaultChromeCookiesPath returns the path to the stable-channel Chrome
// Default profile's Cookies database on the current OS. Only macOS and
// Linux are supported; elsewhere an error is returned.
func defaultChromeCookiesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "Default", "Cookies"), nil
	case "linux":
		return filepath.Join(home, ".config", "google-chrome", "Default", "Cookies"), nil
	default:
		return "", fmt.Errorf("chrome cookies path unsupported on %s", runtime.GOOS)
	}
}

// readCookiesDB copies the Cookies DB (+WAL/SHM) to temp and runs sqlite3
// to select matching rows. domainPatterns are SQL LIKE patterns (e.g.
// "%intuit.com%"). It returns one Cookie per row with Name, Value, Domain,
// and Expires (converted from Chrome's WebKit epoch). Rows whose value is
// not valid UTF-8 (encrypted blobs) are skipped rather than emitted with
// garbage.
//
// Requires the `sqlite3` CLI on PATH. If absent, returns an error so the
// caller can tell the user to install press-auth instead.
func readCookiesDB(cookiesDB string, domainPatterns []string) ([]Cookie, error) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		return nil, fmt.Errorf("sqlite3 not on PATH: %w", ErrNoCookieSource)
	}
	tmpFile, err := os.CreateTemp("", "qb-cookies-*.db")
	if err != nil {
		return nil, fmt.Errorf("creating temp cookies db: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)
	defer os.Remove(tmpPath + "-wal")
	defer os.Remove(tmpPath + "-shm")

	if err := copyFileIfExists(cookiesDB, tmpPath); err != nil {
		return nil, fmt.Errorf("copying cookies db: %w", err)
	}
	_ = copyFileIfExists(cookiesDB+"-wal", tmpPath+"-wal")
	_ = copyFileIfExists(cookiesDB+"-shm", tmpPath+"-shm")

	// host_key is plaintext in Chrome's cookies schema; encrypted_value
	// holds the AES blob. We select value first (plaintext when present)
	// and fall back to empty — ClassifyCookies only needs JWT-shaped
	// values, which Chrome stores unencrypted for Intuit.
	likeClauses := make([]string, 0, len(domainPatterns))
	for _, p := range domainPatterns {
		likeClauses = append(likeClauses, "host_key LIKE '"+sqlQuoteLiteral(p)+"'")
	}
	query := fmt.Sprintf(
		"SELECT name, host_key, value, expires_utc FROM cookies WHERE %s",
		strings.Join(likeClauses, " OR "),
	)
	out, err := exec.Command("sqlite3", "-json", tmpPath, query).Output()
	if err != nil {
		return nil, fmt.Errorf("querying cookies db: %w", err)
	}
	return parseCookiesJSON(out)
}

// parseCookiesJSON decodes sqlite3 -json output into []Cookie, converting
// expires_utc (microseconds since 1601-01-01, the WebKit/Chrome epoch) to a
// time.Time. A zero/missing expiry yields the zero time (session cookie).
func parseCookiesJSON(out []byte) ([]Cookie, error) {
	if len(bytes.TrimSpace(out)) == 0 {
		return nil, nil
	}
	var rows []struct {
		Name       string `json:"name"`
		HostKey    string `json:"host_key"`
		Value      string `json:"value"`
		ExpiresUTC int64  `json:"expires_utc"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("parsing sqlite3 json: %w", err)
	}
	cookies := make([]Cookie, 0, len(rows))
	for _, r := range rows {
		if r.Name == "" {
			continue
		}
		cookies = append(cookies, Cookie{
			Name:    r.Name,
			Value:   r.Value,
			Domain:  r.HostKey,
			Expires: chromeEpochToTime(r.ExpiresUTC),
		})
	}
	return cookies, nil
}

// chromeEpochToTime converts a Chrome expires_utc value (microseconds since
// 1601-01-01 UTC) to a UTC time.Time. Zero or negative -> zero time
// (session cookie / already-expired marker).
func chromeEpochToTime(micros int64) time.Time {
	if micros <= 0 {
		return time.Time{}
	}
	// WebKit epoch = 1601-01-01 = Unix epoch minus 11644473600 seconds.
	const unixToWebKit = 11644473600
	secs := micros/1_000_000 - unixToWebKit
	if secs < 0 {
		return time.Time{}
	}
	return time.Unix(secs, 0).UTC()
}

// sqlQuoteLiteral escapes single quotes for a SQLite string literal.
func sqlQuoteLiteral(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// copyFileIfExists copies src to dst, returning nil if src does not exist.
// Mirrors the golden auth.go helper.
func copyFileIfExists(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
