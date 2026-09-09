package auth

import (
	"strings"
	"time"
)

// TokenSet is the persisted QBO session captured by `qb login`.
// Secrets live only in this file (mode 0600). Never print field values
// that hold credentials.
type TokenSet struct {
	Version      int       `json:"version"`
	CapturedAt   time.Time `json:"captured_at"`
	Source       string    `json:"source"` // relay-session | chrome-cookies | press-auth
	LoginURL     string    `json:"login_url"`
	FinalURL     string    `json:"final_url"`
	RealmID      string    `json:"realm_id,omitempty"`
	CompanyName  string    `json:"company_name,omitempty"`
	Email        string    `json:"email,omitempty"`
	AccessToken  string    `json:"access_token,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	TokenType    string    `json:"token_type,omitempty"`
	APIKey       string    `json:"api_key,omitempty"`
	IntuitAppID  string    `json:"intuit_appid,omitempty"`
	// Authorization is the full Authorization header from a live QBO
	// request (e.g. "Intuit_APIKey intuit_apikey=...,intuit_apikey_version=1.0").
	// When set, the HTTP client sends it verbatim.
	Authorization string `json:"authorization,omitempty"`
	// AuditAuthorization is the Intuit_APIKey the Audit Log UI sends to
	// audit.api.intuit.com (shorter than the ATS banking key).
	AuditAuthorization string `json:"audit_authorization,omitempty"`
	// RequestHeaders is the last successful ATS request header map
	// (Authorization, apikey, distinct x-csrf-token, User-Agent, etc.).
	// Cookie is stored in Cookies instead. Values are secret.
	RequestHeaders map[string]string `json:"request_headers,omitempty"`
	// URIHostHeaders is leftover-host request header maps keyed by host.
	// Authorization is that host's own Intuit_APIKey, not ATS.
	// Remint ApplyATSCapture does not overwrite this map.
	URIHostHeaders map[string]map[string]string `json:"uri_host_headers,omitempty"`

	AccessExpiry  time.Time `json:"access_expiry,omitempty"`
	RefreshExpiry time.Time `json:"refresh_expiry,omitempty"`
	Cookies       []Cookie  `json:"cookies"`
	RelayURL      string    `json:"relay_url,omitempty"`
}

// Cookie is one captured browser cookie. Value is secret.
type Cookie struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Expires  time.Time `json:"expires,omitempty"`
	Secure   bool      `json:"secure"`
	HTTPOnly bool      `json:"http_only"`
}

// Status is the secret-free view of a TokenSet for CLI output.
type Status struct {
	OK                  bool   `json:"ok"`
	Source              string `json:"source"`
	RealmID             string `json:"realm_id,omitempty"`
	CompanyName         string `json:"company_name,omitempty"`
	Email               string `json:"email,omitempty"`
	CookieCount         int    `json:"cookie_count"`
	HasAccessToken      bool   `json:"has_access_token"`
	HasRefreshToken     bool   `json:"has_refresh_token"`
	HasATSAuthorization bool   `json:"has_ats_authorization"`
	HasRequestHeaders   bool   `json:"has_request_headers"`
	RequestHeaderCount  int    `json:"request_header_count,omitempty"`

	AccessExpiry  time.Time `json:"access_expiry,omitempty"`
	RefreshExpiry time.Time `json:"refresh_expiry,omitempty"`
	LongestExpiry time.Time `json:"longest_expiry,omitempty"`
	CapturedAt    time.Time `json:"captured_at"`
	FinalURL      string    `json:"final_url,omitempty"`
}

const (
	CurrentVersion = 1
	// DefaultLoginURL is the QBO web sign-in that yields the SPA session
	// the frontend APIs actually use.
	DefaultLoginURL = "https://accounts.intuit.com/app/sign-in?app_group=QBO&asset_alias=Intuit.accounting.core.qbowebapp&app_environment=prod"
	DefaultRelayURL = "http://127.0.0.1:9224"
)

// AuthenticatedURL reports whether u is a post-login QBO app URL.
func AuthenticatedURL(u string) bool {
	if u == "" {
		return false
	}
	return strings.Contains(u, "qbo.intuit.com/app/") &&
		!strings.Contains(u, "sign-in") &&
		!strings.Contains(u, "UNAUTHENTICATED")
}
