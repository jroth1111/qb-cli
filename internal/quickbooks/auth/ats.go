package auth

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNoATSAuthorization is returned when interactive login captured cookies
// but never observed an ATS request bearing Authorization: Intuit_APIKey.
// Cookie-only sessions 401 against ATS; they must not be saved as success.
var ErrNoATSAuthorization = errors.New("no ATS Intuit_APIKey Authorization captured; open qbo.intuit.com/app/banking and retry, or use --from-mitm")

// BankingCaptureURL is the SPA page that issues getInitialData / getTransactions.
const BankingCaptureURL = "https://qbo.intuit.com/app/banking"

// AuditCaptureURL is the Audit Log UI page that POSTs /v1/audit/logs to
// audit.api.intuit.com. The Intuit_APIKey it sends is shorter than the ATS
// banking key and is stored separately in TokenSet.AuditAuthorization.
const AuditCaptureURL = "https://qbo.intuit.com/app/auditlog"

// ATSCapture is one intercepted ATS request plus optional CDP cookies.
// AuditAuthorization holds the audit-ui Intuit_APIKey when captured in the
// same remint pass; it may be empty when only the banking key was observed.
// SecondaryHeaders optionally holds an apikey-bearing qbo.intuit.com request
// (/olb/, /api/v4/graphql) captured in the same pass: the new banking SPA
// splits the credential pair across request families, so the apikey/authtype/
// intuit_appid fields come from it when the primary request lacks them.
type ATSCapture struct {
	Headers          map[string]string
	SecondaryHeaders map[string]string
	Cookies          []Cookie
	// HostHeaders carries per-service-host request header maps harvested
	// during the same pass (any *.api.intuit.com request that carries its
	// own Intuit_APIKey). It refreshes TokenSet.URIHostHeaders — the
	// leftover-host credential surface older captures used to populate.
	HostHeaders        map[string]map[string]string
	AuditAuthorization string
	// Identity is the signed-in user/company mined from the hydrated tab
	// DOM during capture. Best-effort: empty when the tab was not
	// hydrated or evaluation was unavailable.
	Identity Identity
	Evidence *LoginEvidence
}

// HasATSAuthorization reports whether t holds the SPA-minted ATS
// Authorization header (typed field or request_headers). Ticket cookies
// alone are not enough.
func (t *TokenSet) HasATSAuthorization() bool {
	if t == nil {
		return false
	}
	if isIntuitAPIKey(t.Authorization) || isIntuitAPIKey(t.AccessToken) {
		return true
	}
	return isIntuitAPIKey(headerGet(t.RequestHeaders, "authorization"))
}

// HasAuditAuthorization reports whether t holds the separate Audit Log UI
// credential captured from audit.api.intuit.com. The banking ATS key is not
// interchangeable with this service-host key.
func (t *TokenSet) HasAuditAuthorization() bool {
	if t == nil {
		return false
	}
	authz := strings.TrimSpace(t.AuditAuthorization)
	if authz == "" {
		authz = strings.TrimSpace(headerGet(t.URIHostHeaders["audit.api.intuit.com"], "authorization"))
	}
	return isIntuitAPIKey(authz)
}

// ApplyATSHeaders copies a captured ATS request header map onto t.
// Cookie is ignored (the jar lives in t.Cookies). Values are secret.
func (t *TokenSet) ApplyATSHeaders(headers map[string]string) {
	if t == nil || len(headers) == 0 {
		return
	}
	stored := make(map[string]string, len(headers))
	for k, v := range headers {
		if strings.EqualFold(k, "cookie") || v == "" {
			continue
		}
		stored[k] = v
	}
	t.RequestHeaders = stored
	if v := headerGet(stored, "authorization"); v != "" {
		t.Authorization = v
		t.AccessToken = v
	}
	if v := headerGet(stored, "authtype"); v != "" {
		t.TokenType = v
	}
	if v := headerGet(stored, "apikey"); v != "" {
		t.APIKey = v
	}
	if v := headerGet(stored, "intuit_appid"); v != "" {
		t.IntuitAppID = v
	}
	// RealmID always follows the captured intuit-company-id so the
	// stored realm and the request headers cannot diverge.
	if v := headerGet(stored, "intuit-company-id"); v != "" {
		t.RealmID = v
	}
}

// MergeCookies unions extra by cookie identity (name/domain/path).
func (t *TokenSet) MergeCookies(extra []Cookie) {
	if t == nil || len(extra) == 0 {
		return
	}
	byName := make(map[string]int, len(t.Cookies))
	for i, c := range t.Cookies {
		byName[cookieKey(c)] = i
	}
	for _, c := range extra {
		if c.Name == "" {
			continue
		}
		if i, ok := byName[cookieKey(c)]; ok {
			t.Cookies[i] = c
			continue
		}
		byName[cookieKey(c)] = len(t.Cookies)
		t.Cookies = append(t.Cookies, c)
	}
}

// ApplyATSCapture writes intercepted ATS headers (and optional CDP cookies)
// onto t. Returns ErrNoATSAuthorization when the capture has no Intuit_APIKey.
func (t *TokenSet) ApplyATSCapture(cap *ATSCapture) error {
	if t == nil {
		return ErrNoATSAuthorization
	}
	if cap == nil || !isIntuitAPIKey(headerGet(cap.Headers, "authorization")) {
		return ErrNoATSAuthorization
	}
	t.ApplyATSHeaders(cap.Headers)
	// Fill apikey/authtype/intuit_appid gaps from a secondary apikey-bearing
	// request. Never overwrites: a primary /ats/-era request that carries
	// the trio itself stays authoritative.
	if sec := cap.SecondaryHeaders; len(sec) > 0 {
		if t.APIKey == "" {
			t.APIKey = headerGet(sec, "apikey")
		}
		if t.TokenType == "" {
			t.TokenType = headerGet(sec, "authtype")
		}
		if t.IntuitAppID == "" {
			t.IntuitAppID = headerGet(sec, "intuit_appid")
		}
	}
	// Fresh per-host headers replace stale entries; hosts not seen this pass
	// keep their existing maps.
	if len(cap.HostHeaders) > 0 {
		if t.URIHostHeaders == nil {
			t.URIHostHeaders = map[string]map[string]string{}
		}
		for host, headers := range cap.HostHeaders {
			if len(headers) > 0 {
				t.URIHostHeaders[host] = headers
			}
		}
	}
	t.MergeCookies(cap.Cookies)
	t.ApplyIdentity(cap.Identity)
	if cap.AuditAuthorization != "" {
		// Merge rule: a non-empty audit key overwrites the previous one;
		// an empty audit key preserves any existing AuditAuthorization so
		// a banking-only remint never wipes a known-good audit key.
		t.AuditAuthorization = cap.AuditAuthorization
	}
	if !t.HasATSAuthorization() {
		return ErrNoATSAuthorization
	}
	return nil
}

// ApplyIdentity fills empty company/email/realm fields from a capture-time
// DOM mining. Captured headers (intuit-company-id) win for realm: only an
// empty RealmID takes the mined value. Never overwrites.
func (t *TokenSet) ApplyIdentity(id Identity) {
	if t == nil {
		return
	}
	if t.CompanyName == "" {
		t.CompanyName = id.Company
	}
	if t.Email == "" {
		t.Email = id.Email
	}
	if t.RealmID == "" {
		t.RealmID = id.Realm
	}
}

func isIntuitAPIKey(v string) bool {
	return strings.HasPrefix(strings.TrimSpace(v), "Intuit_APIKey")
}

// isATSCredentialRequest reports whether an intercepted request carries the
// first-party Intuit_APIKey credential the CLI replays. The SPA historically
// sent it on /ats/v1/ calls; the new banking experience emits the same key on
// first-party qbo.intuit.com requests such as /api/neo/.../ipd/signedAuthData.
// Requests that also carry an `apikey` header belong to the olb/v4-graphql
// surface and use a different key, so they do not qualify.
func isATSCredentialRequest(reqURL string, headers map[string]string) bool {
	if !isIntuitAPIKey(headerGet(headers, "authorization")) {
		return false
	}
	if strings.Contains(reqURL, "/ats/") {
		return true
	}
	if !strings.Contains(reqURL, "://qbo.intuit.com/") {
		return false
	}
	return headerGet(headers, "apikey") == ""
}

// isAPIKeyHeaderRequest reports whether an intercepted qbo.intuit.com request
// carries the secondary `apikey` credential (the olb/v4-graphql surface).
// Its headers fill the apikey/authtype/intuit_appid fields the primary
// first-party request no longer carries.
func isAPIKeyHeaderRequest(reqURL string, headers map[string]string) bool {
	return strings.Contains(reqURL, "://qbo.intuit.com/") &&
		headerGet(headers, "apikey") != ""
}

// ServiceHost extracts a separately keyed service host. Neo requests on
// qbo.intuit.com with an apikey header use a different Intuit_APIKey from
// the primary ATS capture and must refresh URIHostHeaders too.
func ServiceHost(reqURL string, headers map[string]string) string {
	if !isIntuitAPIKey(headerGet(headers, "authorization")) {
		return ""
	}
	if strings.HasPrefix(reqURL, "https://qbo.intuit.com/api/neo/") && headerGet(headers, "apikey") != "" {
		return "qbo.intuit.com"
	}
	if !strings.Contains(reqURL, ".api.intuit.com/") {
		return ""
	}
	if _, rest, ok := strings.Cut(reqURL, "://"); ok {
		if j := strings.IndexByte(rest, '/'); j > 0 {
			return rest[:j]
		}
	}
	return ""
}

// redactAuthKey returns a secret-free summary of an Authorization header for
// logs: the Intuit_APIKey prefix and its length, or "none" when empty. It
// never includes the credential value.
func redactAuthKey(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "none"
	}
	if isIntuitAPIKey(v) {
		return fmt.Sprintf("Intuit_APIKey(len=%d)", len(v))
	}
	return fmt.Sprintf("auth(len=%d)", len(v))
}

func headerGet(h map[string]string, name string) string {
	if len(h) == 0 {
		return ""
	}
	if v, ok := h[name]; ok {
		return v
	}
	for k, v := range h {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}
