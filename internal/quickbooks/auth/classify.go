package auth

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// jwtSegmentCount is the number of dot-separated segments in a JWT
// (header.payload.signature). Cookie values that match this shape are
// decoded for an `exp` claim rather than treated as opaque session ids.
const jwtSegmentCount = 3

// refreshCookieName matches cookie names that carry a refresh token.
// Case-insensitive: Intuit has used both `refresh_token` and `refreshToken`.
var refreshCookieName = regexp.MustCompile(`(?i)refresh`)

// accessTokenCookieName matches cookie names that carry an access or id
// token. Case-insensitive.
var accessTokenCookieName = regexp.MustCompile(`(?i)(access_token|id_token)`)

// HasUsableCredential reports whether t can authenticate ATS. Only an
// Intuit_APIKey Authorization (typed field, AccessToken, or
// request_headers) counts. Ticket cookies alone 401; they are not usable.
func (t *TokenSet) HasUsableCredential() bool {
	return t.HasATSAuthorization()
}

// LongestExpiry returns the latest expiry among all cookie Expires, the
// access-token expiry, and the refresh-token expiry. Zero times are
// ignored (a session cookie with no expiry, or a token with no known exp,
// does not pull the result back to the zero time). Returns the zero time
// only when every component is zero/unknown.
func (t *TokenSet) LongestExpiry() time.Time {
	var best time.Time
	consider := func(x time.Time) {
		if x.IsZero() {
			return
		}
		if best.IsZero() || x.After(best) {
			best = x
		}
	}
	if t != nil {
		consider(t.AccessExpiry)
		consider(t.RefreshExpiry)
		for i := range t.Cookies {
			consider(t.Cookies[i].Expires)
		}
	}
	return best
}

// RedactedStatus builds the secret-free Status view of t for CLI output.
// It copies only non-secret fields: cookie count (not values), token
// presence flags (not token strings), and metadata. No cookie Value and
// no AccessToken/RefreshToken string ever reaches the returned Status, so
// it is safe to JSON-marshal to stdout.
func (t *TokenSet) RedactedStatus() Status {
	s := Status{
		CapturedAt: time.Time{},
	}
	if t == nil {
		return s
	}
	s.OK = t.HasUsableCredential()
	s.Source = t.Source
	s.RealmID = t.RealmID
	s.CompanyName = t.CompanyName
	s.Email = t.Email
	s.CookieCount = len(t.Cookies)
	s.HasAccessToken = t.AccessToken != ""
	s.HasRefreshToken = t.RefreshToken != ""
	s.HasATSAuthorization = t.HasATSAuthorization()
	s.HasRequestHeaders = len(t.RequestHeaders) > 0
	s.RequestHeaderCount = len(t.RequestHeaders)

	s.AccessExpiry = t.AccessExpiry
	s.RefreshExpiry = t.RefreshExpiry
	s.LongestExpiry = t.LongestExpiry()
	s.CapturedAt = t.CapturedAt
	s.FinalURL = t.FinalURL
	return s
}

// ClassifyCookies inspects t.Cookies and promotes credential-bearing
// cookies into the AccessToken / RefreshToken / *Expiry fields. It is
// idempotent: cookies are classified by name and JWT shape, and the
// longest-lived refresh token wins so a stale short-lived one never
// overwrites a better capture.
//
// Rules (from the qb contract):
//   - A cookie whose value is a JWT (three dot-separated segments) has its
//     `exp` claim decoded and contributes to the relevant expiry.
//   - A name matching (?i)refresh        -> RefreshToken (keep the longest-lived).
//   - A name matching (?i)(access_token|id_token) -> AccessToken.
//
// Non-matching cookies are left in t.Cookies (the full jar is persisted so
// frontend APIs can be replayed).
func (t *TokenSet) ClassifyCookies() {
	if t == nil {
		return
	}

	var bestRefresh string
	var bestRefreshExp time.Time
	var bestAccess string
	var bestAccessExp time.Time

	for _, c := range t.Cookies {
		exp := jwtExpiry(c.Value) // zero if not a JWT or no exp claim

		switch {
		case refreshCookieName.MatchString(c.Name):
			// Keep the longest-lived refresh token. A zero exp means "no
			// known expiry" — treat that as the longest possible so a
			// refresh token without exp is preferred over one that expires.
			if bestRefresh == "" || refreshLonger(exp, bestRefreshExp) {
				bestRefresh = c.Value
				bestRefreshExp = exp
			}
		case accessTokenCookieName.MatchString(c.Name):
			if bestAccess == "" || accessLonger(exp, bestAccessExp) {
				bestAccess = c.Value
				bestAccessExp = exp
			}
		}
	}

	if bestRefresh != "" {
		t.RefreshToken = bestRefresh
		if !bestRefreshExp.IsZero() {
			t.RefreshExpiry = bestRefreshExp
		}
	}
	if bestAccess != "" {
		t.AccessToken = bestAccess
		if !bestAccessExp.IsZero() {
			t.AccessExpiry = bestAccessExp
		}
	}
}

// refreshLonger reports whether candidate exp is a longer-lived refresh
// expiry than the current best. A zero candidate means "no known expiry",
// which is treated as longer than any finite expiry (refresh tokens
// frequently omit exp).
func refreshLonger(candidate, current time.Time) bool {
	if candidate.IsZero() {
		return true
	}
	if current.IsZero() {
		return false
	}
	return candidate.After(current)
}

// accessLonger reports whether candidate exp is a longer-lived access
// expiry than the current best. Unlike refresh, a zero candidate is NOT
// preferred: access tokens always carry exp, and a missing exp more
// likely means "not a JWT" than "never expires".
func accessLonger(candidate, current time.Time) bool {
	if candidate.IsZero() {
		return false
	}
	if current.IsZero() {
		return true
	}
	return candidate.After(current)
}

// jwtExpiry returns the `exp` claim of a JWT value, or the zero time if
// the value is not a three-segment JWT or has no numeric exp claim. It
// mirrors pressauth.DecodeJWT/Exp but is self-contained so package auth
// has no dependency on pressauth (which pulls chromedp).
func jwtExpiry(value string) time.Time {
	parts := strings.Split(value, ".")
	if len(parts) != jwtSegmentCount {
		return time.Time{}
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims map[string]any
	if err := json.Unmarshal(body, &claims); err != nil {
		return time.Time{}
	}
	raw, ok := claims["exp"]
	if !ok {
		return time.Time{}
	}
	switch v := raw.(type) {
	case float64:
		return time.Unix(int64(v), 0).UTC()
	case int64:
		return time.Unix(v, 0).UTC()
	case int:
		return time.Unix(int64(v), 0).UTC()
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return time.Time{}
		}
		return time.Unix(n, 0).UTC()
	default:
		return time.Time{}
	}
}

// ExtractRealmID pulls the QBO realm (company) id out of a post-login URL.
// QBO surfaces it in a few places depending on entry point:
//   - query parameter: realmId, realm, companyLid (case-insensitive)
//   - an all-digit path segment (e.g. /v3/company/<realmId>/...)
//
// Returns "" when no realm id can be found.
func ExtractRealmID(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		// Fall back to a crude scan so a malformed URL still yields a realm
		// id if one is present as a query-style fragment.
		return scanRealmID(rawURL)
	}

	// Query keys are matched case-insensitively: Intuit and various QBO
	// entry points emit realmId, realmid, realm, companyLid, companylid
	// (and other casings). url.Values is case-sensitive, so fold manually.
	realmKeys := []string{"realmid", "realm", "companylid"}
	for key, vs := range u.Query() {
		if !hasAnyFold(realmKeys, key) {
			continue
		}
		if len(vs) > 0 && vs[0] != "" && isRealmID(vs[0]) {
			return vs[0]
		}
	}

	// Path segment: walk segments and return the first all-digit one that
	// looks like a realm id (QBO realm ids are 9-12 digit numbers, but we
	// accept any all-digit segment of reasonable length to stay forward-
	// compatible).
	for seg := range strings.SplitSeq(u.Path, "/") {
		if isRealmID(seg) {
			return seg
		}
	}

	// Some QBO redirects stash the realm in the fragment query. Keys are
	// matched case-insensitively, same as the main query above.
	if u.Fragment != "" {
		if frag, err := url.ParseQuery(u.Fragment); err == nil {
			for key, vs := range frag {
				if !hasAnyFold(realmKeys, key) {
					continue
				}
				if len(vs) > 0 && isRealmID(vs[0]) {
					return vs[0]
				}
			}
		}
	}
	return ""
}

// scanRealmID is the fallback used when url.Parse fails: it looks for an
// all-digit run of 9+ characters, which matches QBO realm ids without
// false-positiving on short numeric query values.
func scanRealmID(s string) string {
	for _, key := range []string{"realmId=", "realm=", "companyLid=", "companylid="} {
		if i := strings.Index(strings.ToLower(s), strings.ToLower(key)); i >= 0 {
			rest := s[i+len(key):]
			end := strings.IndexAny(rest, "&\"'#")
			val := rest
			if end >= 0 {
				val = rest[:end]
			}
			if isRealmID(val) {
				return val
			}
		}
	}
	return ""
}

// isRealmID reports whether s is a plausible QBO realm id: all digits,
// 9-16 chars. The lower bound filters out small numeric path segments
// (version numbers, status codes); the upper bound is generous.
func isRealmID(s string) bool {
	if len(s) < 9 || len(s) > 16 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// hasAnyFold reports whether s matches any of the candidates under
// Unicode case-folding. Used for case-insensitive query-parameter keys
// (url.Values lookups are case-sensitive).
func hasAnyFold(candidates []string, s string) bool {
	for _, c := range candidates {
		if strings.EqualFold(c, s) {
			return true
		}
	}
	return false
}
