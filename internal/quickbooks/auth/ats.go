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
type ATSCapture struct {
	Headers            map[string]string
	Cookies            []Cookie
	AuditAuthorization string
	// Identity is the signed-in user/company mined from the hydrated tab
	// DOM during capture. Best-effort: empty when the tab was not
	// hydrated or evaluation was unavailable.
	Identity Identity
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

// MergeCookies unions extra into t.Cookies by name (extra wins).
func (t *TokenSet) MergeCookies(extra []Cookie) {
	if t == nil || len(extra) == 0 {
		return
	}
	byName := make(map[string]int, len(t.Cookies))
	for i, c := range t.Cookies {
		byName[c.Name] = i
	}
	for _, c := range extra {
		if c.Name == "" {
			continue
		}
		if i, ok := byName[c.Name]; ok {
			t.Cookies[i] = c
			continue
		}
		byName[c.Name] = len(t.Cookies)
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
