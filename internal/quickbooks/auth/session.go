package auth

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

var ErrSessionChanged = errors.New("QBO session changed or logged out; start a new command for the intended company")

var ErrSessionIdentityUnverified = errors.New("cannot prove the renewing QBO principal; run qb login to bind a fresh session")

// SameSession allows credential rotation within a login, never a later login.
func SameSession(a, b *TokenSet) bool {
	if a == nil || b == nil || a.RealmID != b.RealmID || !strings.EqualFold(a.Email, b.Email) {
		return false
	}
	if a.SessionID != "" || b.SessionID != "" {
		return a.SessionID != "" && a.SessionID == b.SessionID
	}
	return a.CapturedAt.Equal(b.CapturedAt) && a.Authorization == b.Authorization
}

type sessionRequestKey struct{}

type retainBrowserKey struct{}

func WithRetainedBrowser(ctx context.Context, retain bool) context.Context {
	return context.WithValue(ctx, retainBrowserKey{}, retain)
}
func retainBrowser(ctx context.Context) bool {
	if value, ok := ctx.Value(retainBrowserKey{}).(bool); ok {
		return value
	}
	return true
}

type sessionRequest struct {
	home, host string
	token      *TokenSet
}

// BindRequest records the profile and capture that authorized this request.
func BindRequest(req *http.Request, tok *TokenSet, host string) {
	if req.URL == nil || !intuitHost(strings.ToLower(req.URL.Hostname())) {
		return
	}
	*req = *req.WithContext(context.WithValue(req.Context(), sessionRequestKey{}, sessionRequest{HomeDir(), host, tok}))
}

func sessionHeaders(t *TokenSet, host string) map[string]string {
	if host != "" {
		return t.URIHostHeaders[host]
	}
	out := map[string]string{}
	maps.Copy(out, t.RequestHeaders)
	csrf := ""
	for _, c := range t.Cookies {
		if c.Name == "qbo.csrftoken" && (c.Expires.IsZero() || c.Expires.After(time.Now())) {
			csrf = c.Value
			break
		}
	}
	for k, v := range map[string]string{"Authorization": t.Authorization, "apikey": t.APIKey, "authtype": t.TokenType, "intuit_appid": t.IntuitAppID, "csrftoken": csrf, "x-csrf-token": csrf} {
		if headerGet(out, k) == "" && v != "" {
			out[k] = v
		}
	}
	return out
}

// PrepareSessionRequest reloads rotated credentials without changing operation-
// specific overrides. Missing/logout/replaced profiles fail before any dial.
func PrepareSessionRequest(req *http.Request) (*http.Request, error) {
	s, ok := req.Context().Value(sessionRequestKey{}).(sessionRequest)
	if !ok {
		return req, nil
	}
	if req.URL.Scheme != "https" {
		return nil, fmt.Errorf("QBO credentials require HTTPS")
	}
	current, err := loadAt(s.home)
	if err != nil {
		return nil, ErrSessionChanged
	}
	if !SameSession(s.token, current) {
		return nil, ErrSessionChanged
	}
	for _, key := range []string{"intuit-company-id", "intuit-realm-id"} {
		if id := req.Header.Get(key); id != "" && id != current.RealmID {
			return nil, ErrSessionChanged
		}
	}
	next := req.Clone(req.Context())
	oldHeaders, newHeaders := sessionHeaders(s.token, s.host), sessionHeaders(current, s.host)
	for _, key := range []string{"Authorization", "apikey", "authtype", "intuit_appid", "csrftoken", "x-csrf-token"} {
		old, new := headerGet(oldHeaders, key), headerGet(newHeaders, key)
		if old != "" && new != "" && next.Header.Get(key) == old {
			next.Header.Set(key, new)
		}
	}
	if s.token.AuditAuthorization != "" && next.Header.Get("Authorization") == s.token.AuditAuthorization && current.AuditAuthorization != "" {
		next.Header.Set("Authorization", current.AuditAuthorization)
	}
	next.Header.Del("Cookie")
	if cookies := CookieHeaderForURL(current.Cookies, next.URL, time.Now()); cookies != "" {
		next.Header.Set("Cookie", cookies)
	}
	s.token = current
	return next.WithContext(context.WithValue(next.Context(), sessionRequestKey{}, s)), nil
}

func cookieDomain(c Cookie) string {
	d := strings.TrimPrefix(strings.ToLower(c.Domain), ".")
	if d == "" {
		d = "qbo.intuit.com"
	}
	return d
}
func cookiePath(c Cookie) string {
	if c.Path == "" {
		return "/"
	}
	return c.Path
}
func cookieKey(c Cookie) string { return c.Name + "\x00" + cookieDomain(c) + "\x00" + cookiePath(c) }

func sameCookie(a, b Cookie) bool {
	return cookieKey(a) == cookieKey(b) && a.Value == b.Value && a.Expires.Equal(b.Expires) && a.Secure == b.Secure && a.HTTPOnly == b.HTTPOnly && a.HostOnly == b.HostOnly
}
func intuitHost(h string) bool { return h == "intuit.com" || strings.HasSuffix(h, ".intuit.com") }

// CookieHeaderForURL honors expiration, host/domain and path scope. VS ticket
// filtering and the existing 200-cookie ceiling retain QBO query compatibility.
func CookieHeaderForURL(cookies []Cookie, u *url.URL, now time.Time) string {
	if u == nil || u.Scheme != "https" || !intuitHost(strings.ToLower(u.Hostname())) {
		return ""
	}
	var keep []Cookie
	path := u.Path
	if path == "" {
		path = "/"
	}
	for _, c := range cookies {
		domain, cp := cookieDomain(c), cookiePath(c)
		host := strings.ToLower(u.Hostname())
		if c.Name == "" || c.Value == "" || strings.HasPrefix(c.Name, "VS") || (!c.Expires.IsZero() && !c.Expires.After(now)) || c.Secure && u.Scheme != "https" {
			continue
		}
		if host != domain && (c.HostOnly || !strings.HasPrefix(c.Domain, ".") || !strings.HasSuffix(host, "."+domain)) {
			continue
		}
		if path != cp && (!strings.HasPrefix(path, cp) || !strings.HasSuffix(cp, "/") && path[len(cp)] != '/') {
			continue
		}
		keep = append(keep, c)
	}
	sort.SliceStable(keep, func(i, j int) bool { return len(cookiePath(keep[i])) > len(cookiePath(keep[j])) })
	if len(keep) > 200 {
		keep = keep[:200]
	}
	out := make([]string, 0, len(keep))
	for _, c := range keep {
		out = append(out, (&http.Cookie{Name: c.Name, Value: c.Value}).String())
	}
	return strings.Join(out, "; ")
}

// ObserveSessionResponse persists rotation as a conditional merge. It never
// recreates a logged-out profile, overwrites a new login, or rolls back a cookie
// another concurrent request has already changed. Callers retain the HTTP
// outcome even if local persistence fails: a write may already be applied.
func ObserveSessionResponse(req *http.Request, resp *http.Response) error {
	if IsHarness() {
		return nil
	}
	s, ok := req.Context().Value(sessionRequestKey{}).(sessionRequest)
	if !ok || resp == nil {
		return nil
	}
	origin := req.URL
	if resp.Request != nil && resp.Request.URL != nil && resp.Request.URL.Host != origin.Host {
		return nil
	}
	if origin.Scheme != "https" || !intuitHost(strings.ToLower(origin.Hostname())) {
		return nil
	}
	updates := resp.Cookies()
	now := time.Now().UTC()
	if len(updates) == 0 && now.Sub(s.token.LastUsedAt) < time.Minute {
		return nil
	}
	unlock, err := credentialLock(s.home)
	if err != nil {
		return err
	}
	defer unlock()
	current, err := loadAt(s.home)
	if err != nil {
		return ErrSessionChanged
	}
	if !SameSession(s.token, current) {
		return ErrSessionChanged
	}
	before := map[string]Cookie{}
	for _, c := range s.token.Cookies {
		before[cookieKey(c)] = c
	}
	changed := false
	for _, wire := range updates {
		domain := strings.TrimPrefix(strings.ToLower(wire.Domain), ".")
		hostOnly := domain == ""
		if hostOnly {
			domain = strings.ToLower(origin.Hostname())
		}
		host := strings.ToLower(origin.Hostname())
		if !intuitHost(domain) || (host != domain && !strings.HasSuffix(host, "."+domain)) {
			continue
		}
		p := wire.Path
		if p == "" || p[0] != '/' {
			p = "/"
			if i := strings.LastIndex(origin.Path, "/"); i > 0 {
				p = origin.Path[:i]
			}
		}
		expiry := wire.Expires
		if wire.MaxAge > 0 {
			expiry = now.Add(time.Duration(wire.MaxAge) * time.Second)
		}
		c := Cookie{Name: wire.Name, Value: wire.Value, Domain: domain, Path: p, Expires: expiry, Secure: wire.Secure, HTTPOnly: wire.HttpOnly, HostOnly: hostOnly}
		if !hostOnly {
			c.Domain = "." + domain
		}
		key := cookieKey(c)
		idx := -1
		for i, existing := range current.Cookies {
			if cookieKey(existing) == key {
				idx = i
				break
			}
		}
		old, existed := before[key]
		if idx >= 0 && (!existed || !sameCookie(current.Cookies[idx], old)) {
			continue
		}
		if idx < 0 && existed {
			continue
		}
		remove := wire.MaxAge < 0 || !expiry.IsZero() && !expiry.After(now)
		if remove {
			if idx >= 0 {
				current.Cookies = append(current.Cookies[:idx], current.Cookies[idx+1:]...)
				changed = true
			}
			continue
		}
		if idx >= 0 {
			current.Cookies[idx] = c
		} else {
			current.Cookies = append(current.Cookies, c)
		}
		if c.Name == "qbo.csrftoken" && existed {
			headers := []map[string]string{current.RequestHeaders}
			for _, h := range current.URIHostHeaders {
				headers = append(headers, h)
			}
			for _, h := range headers {
				for k, v := range h {
					if (strings.EqualFold(k, "csrftoken") || strings.EqualFold(k, "x-csrf-token")) && v == old.Value {
						h[k] = c.Value
					}
				}
			}
		}
		changed = true
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && now.Sub(current.LastUsedAt) >= time.Minute {
		current.LastUsedAt = now
		changed = true
	}
	if !changed {
		return nil
	}
	return saveAt(s.home, current)
}

// SaveRenewedCapture uses every captured credential family, while pinning the
// refresh to the original login and its observed company/principal.
func SaveRenewedCapture(expected *TokenSet, cap *ATSCapture, source string) error {
	if expected == nil || expected.RealmID == "" || cap == nil {
		return ErrSessionChanged
	}
	realm := headerGet(cap.Headers, "intuit-company-id")
	if realm == "" {
		realm = cap.Identity.Realm
	}
	if realm != expected.RealmID || cap.Identity.Realm != "" && cap.Identity.Realm != realm {
		return ErrSessionChanged
	}
	oldPrincipal, newPrincipal := headerGet(expected.RequestHeaders, "intuit-user-id"), headerGet(cap.Headers, "intuit-user-id")
	if oldPrincipal != "" && newPrincipal != "" && oldPrincipal != newPrincipal {
		return ErrSessionChanged
	}
	if expected.Email != "" && cap.Identity.Email != "" {
		if !strings.EqualFold(strings.TrimSpace(expected.Email), strings.TrimSpace(cap.Identity.Email)) {
			return ErrSessionChanged
		}
	} else {
		oldID, newID := headerGet(expected.RequestHeaders, "intuit-user-id"), headerGet(cap.Headers, "intuit-user-id")
		if oldID == "" || newID == "" || oldID != newID {
			return ErrSessionIdentityUnverified
		}
	}
	unlock, err := credentialLock(HomeDir())
	if err != nil {
		return err
	}
	defer unlock()
	current, err := Load()
	if err != nil || !SameSession(expected, current) {
		return ErrSessionChanged
	}
	if current.CapturedAt.After(expected.CapturedAt) {
		return nil
	}
	// A normal request may rotate cookies while browser capture is in flight.
	// Keep those newer per-cookie values rather than replaying the old browser jar.
	previous, latest := map[string]Cookie{}, map[string]Cookie{}
	for _, c := range expected.Cookies {
		previous[cookieKey(c)] = c
	}
	for _, c := range current.Cookies {
		latest[cookieKey(c)] = c
	}
	copyCapture := *cap
	copyCapture.Cookies = nil
	csrfFrom, csrfTo := "", ""
	for _, c := range cap.Cookies {
		old, hadOld := previous[cookieKey(c)]
		now, hasNow := latest[cookieKey(c)]
		if hadOld != hasNow || hadOld && !sameCookie(old, now) {
			if c.Name == "qbo.csrftoken" {
				csrfFrom, csrfTo = c.Value, now.Value
			}
			continue
		}
		copyCapture.Cookies = append(copyCapture.Cookies, c)
	}
	copyCapture.HostHeaders = map[string]map[string]string{}
	for host, h := range cap.HostHeaders {
		if id := headerGet(h, "intuit-company-id"); id != "" && id != realm {
			continue
		}
		if id := headerGet(h, "intuit-realm-id"); id != "" && id != realm {
			continue
		}
		copyCapture.HostHeaders[host] = maps.Clone(h)
	}
	if err := current.ApplyATSCapture(&copyCapture); err != nil {
		return err
	}
	if csrfFrom != "" {
		headers := []map[string]string{current.RequestHeaders}
		for _, h := range current.URIHostHeaders {
			headers = append(headers, h)
		}
		for _, h := range headers {
			for k, v := range h {
				if (strings.EqualFold(k, "csrftoken") || strings.EqualFold(k, "x-csrf-token")) && v == csrfFrom {
					if csrfTo == "" {
						delete(h, k)
					} else {
						h[k] = csrfTo
					}
				}
			}
		}
	}
	current.Email = expected.Email
	current.Source = source
	current.CapturedAt = time.Now().UTC()
	if err := saveAt(HomeDir(), current); err != nil {
		return fmt.Errorf("saving renewed QBO session: %w", err)
	}
	return nil
}

// EnsureSession gives legacy credential files an epoch without a new login.
func EnsureSession() (*TokenSet, error) {
	unlock, err := credentialLock(HomeDir())
	if err != nil {
		return nil, err
	}
	defer unlock()
	tok, err := Load()
	if err != nil {
		return nil, err
	}
	if tok.SessionID == "" {
		if err := saveAt(HomeDir(), tok); err != nil {
			return nil, err
		}
	}
	return tok, nil
}
