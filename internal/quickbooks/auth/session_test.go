package auth

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func longevitySession(t *testing.T) *TokenSet {
	t.Helper()
	setQBHome(t)
	tok := &TokenSet{RealmID: "1", Email: "person@example.test", Authorization: "Intuit_APIKey intuit_apikey=old", CapturedAt: time.Now().UTC(), RequestHeaders: map[string]string{"Authorization": "Intuit_APIKey intuit_apikey=old", "csrftoken": "old-csrf", "x-csrf-token": "distinct"}, Cookies: []Cookie{{Name: "sid", Value: "old", Domain: ".intuit.com", Path: "/"}, {Name: "qbo.csrftoken", Value: "old-csrf", Domain: "qbo.intuit.com", Path: "/"}}}
	if err := Save(tok); err != nil {
		t.Fatal(err)
	}
	tok, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return tok
}
func sessionTestRequest(t *testing.T, tok *TokenSet) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "https://qbo.intuit.com/api/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range tok.RequestHeaders {
		req.Header.Set(k, v)
	}
	BindRequest(req, tok, "")
	req, err = PrepareSessionRequest(req)
	if err != nil {
		t.Fatal(err)
	}
	return req
}
func cookieResponse(values ...string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Set-Cookie": values}}
}

func TestSessionRotationPersistsAndRefreshesNextRequest(t *testing.T) {
	tok := longevitySession(t)
	req := sessionTestRequest(t, tok)
	if err := ObserveSessionResponse(req, cookieResponse("sid=new; Domain=intuit.com; Path=/; Secure; HttpOnly", "qbo.csrftoken=new-csrf; Path=/; Secure")); err != nil {
		t.Fatal(err)
	}
	next := sessionTestRequest(t, tok)
	if !strings.Contains(next.Header.Get("Cookie"), "sid=new") || next.Header.Get("csrftoken") != "new-csrf" || next.Header.Get("x-csrf-token") != "distinct" {
		t.Fatal("rotation or distinct CSRF header lost")
	}
	current, _ := Load()
	if current.SessionID != tok.SessionID || current.LastUsedAt.IsZero() {
		t.Fatal("rotation changed login or missed activity")
	}
	info, err := os.Stat(CredPath())
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("credentials not private")
	}
}

func TestSessionRotationCannotResurrectLogoutOrReplaceLogin(t *testing.T) {
	for _, replace := range []bool{false, true} {
		t.Run(map[bool]string{false: "logout", true: "new login"}[replace], func(t *testing.T) {
			tok := longevitySession(t)
			req := sessionTestRequest(t, tok)
			if replace {
				if err := Save(&TokenSet{RealmID: "2", Authorization: tok.Authorization}); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := Delete(); err != nil {
					t.Fatal(err)
				}
			}
			if !errors.Is(ObserveSessionResponse(req, cookieResponse("sid=stale; Domain=intuit.com; Path=/")), ErrSessionChanged) {
				t.Fatal("stale writer accepted")
			}
			if _, err := PrepareSessionRequest(req); !errors.Is(err, ErrSessionChanged) {
				t.Fatal("request may cross login boundary")
			}
			current, err := Load()
			if replace {
				if err != nil || current.RealmID != "2" {
					t.Fatal("new login lost")
				}
			} else if !errors.Is(err, ErrNoCredentials) {
				t.Fatal("logout resurrected")
			}
		})
	}
}

func TestSessionRotationCASAndDeletion(t *testing.T) {
	tok := longevitySession(t)
	a, b := sessionTestRequest(t, tok), sessionTestRequest(t, tok)
	if err := ObserveSessionResponse(b, cookieResponse("sid=new; Domain=intuit.com; Path=/")); err != nil {
		t.Fatal(err)
	}
	if err := ObserveSessionResponse(a, cookieResponse("sid=old-again; Domain=intuit.com; Path=/")); err != nil {
		t.Fatal(err)
	}
	current, _ := Load()
	if !strings.Contains(sessionTestRequest(t, current).Header.Get("Cookie"), "sid=new") {
		t.Fatal("late response rolled back cookie")
	}
	if err := ObserveSessionResponse(sessionTestRequest(t, current), cookieResponse("sid=; Domain=intuit.com; Path=/; Max-Age=0")); err != nil {
		t.Fatal(err)
	}
	current, _ = Load()
	if strings.Contains(sessionTestRequest(t, current).Header.Get("Cookie"), "sid=") {
		t.Fatal("deleted cookie retained")
	}
}

func TestCookieScopeAndExpiration(t *testing.T) {
	u, _ := url.Parse("https://sub.qbo.intuit.com/api/v3")
	cookies := []Cookie{{Name: "host", Value: "no", Domain: "qbo.intuit.com", Path: "/"}, {Name: "domain", Value: "yes", Domain: ".qbo.intuit.com", Path: "/api"}, {Name: "path", Value: "no", Domain: ".qbo.intuit.com", Path: "/apis"}, {Name: "expired", Value: "no", Domain: ".qbo.intuit.com", Path: "/", Expires: time.Now().Add(-time.Hour)}}
	if got := CookieHeaderForURL(cookies, u, time.Now()); got != "domain=yes" {
		t.Fatalf("unexpected scoped cookies: %s", got)
	}
	u, _ = url.Parse("https://evil.invalid/")
	if CookieHeaderForURL(cookies, u, time.Now()) != "" {
		t.Fatal("cookies escaped Intuit")
	}
	tok := longevitySession(t)
	req := sessionTestRequest(t, tok)
	before := len(tok.Cookies)
	_ = ObserveSessionResponse(req, cookieResponse("bad=value; Domain=evil.invalid; Path=/"))
	cur, _ := Load()
	if len(cur.Cookies) != before {
		t.Fatal("foreign cookie admitted")
	}
}

func TestRenewalPreservesAllCredentialFamiliesAndIdentity(t *testing.T) {
	tok := longevitySession(t)
	cap := &ATSCapture{Headers: map[string]string{"Authorization": "Intuit_APIKey intuit_apikey=new", "intuit-company-id": "1"}, Identity: Identity{Realm: "1", Email: tok.Email}, AuditAuthorization: "audit-new", HostHeaders: map[string]map[string]string{"roles.api.intuit.com": {"Authorization": "service-new"}}}
	if err := SaveRenewedCapture(tok, cap, "managed-profile"); err != nil {
		t.Fatal(err)
	}
	cur, _ := Load()
	if cur.SessionID != tok.SessionID || cur.AuditAuthorization != "audit-new" || cur.URIHostHeaders["roles.api.intuit.com"]["Authorization"] != "service-new" {
		t.Fatal("refresh dropped captured credentials")
	}
	cap.Identity.Email = "other@example.test"
	if !errors.Is(SaveRenewedCapture(cur, cap, "managed-profile"), ErrSessionChanged) {
		t.Fatal("principal switched")
	}
	cap.Identity.Email = ""
	if !errors.Is(SaveRenewedCapture(cur, cap, "managed-profile"), ErrSessionIdentityUnverified) {
		t.Fatal("unknown principal admitted")
	}
}

func TestProfileLockIsExclusiveAndCancellable(t *testing.T) {
	setQBHome(t)
	unlock, err := LockProfile(context.Background(), HomeDir(), "renewal")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := LockProfile(ctx, HomeDir(), "renewal"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("lock not exclusive/cancellable")
	}
}

func TestRenewalDoesNotRollBackConcurrentCookieRotation(t *testing.T) {
	tok := longevitySession(t)
	req := sessionTestRequest(t, tok)
	if err := ObserveSessionResponse(req, cookieResponse("qbo.csrftoken=new; Path=/; Secure")); err != nil {
		t.Fatal(err)
	}
	cap := &ATSCapture{Headers: tok.RequestHeaders, Cookies: tok.Cookies, Identity: Identity{Realm: "1", Email: tok.Email}, HostHeaders: map[string]map[string]string{"other.api.intuit.com": {"intuit-company-id": "2", "Authorization": "wrong-company"}}}
	if err := SaveRenewedCapture(tok, cap, "managed-profile"); err != nil {
		t.Fatal(err)
	}
	current, _ := Load()
	if current.RequestHeaders["csrftoken"] != "new" {
		t.Fatal("capture rolled back rotated CSRF")
	}
	if current.URIHostHeaders["other.api.intuit.com"] != nil {
		t.Fatal("captured another company's service headers")
	}
}

func TestAuthenticatedURLIsAnOriginBoundary(t *testing.T) {
	for _, u := range []string{"https://evil.test/qbo.intuit.com/app/banking", "https://qbo.intuit.com.evil.test/app/banking", "http://qbo.intuit.com/app/banking", "https://qbo.intuit.com@evil.test/app/banking"} {
		if AuthenticatedURL(u) {
			t.Fatalf("untrusted URL admitted: %s", u)
		}
	}
	if !AuthenticatedURL("https://qbo.intuit.com/app/banking") {
		t.Fatal("QBO origin rejected")
	}
}

func TestRetainedBrowserContext(t *testing.T) {
	if !retainBrowser(context.Background()) || retainBrowser(WithRetainedBrowser(context.Background(), false)) {
		t.Fatal("capture lifetime policy lost")
	}
}

func TestHarnessCannotCaptureOrPersistRotatedCredentials(t *testing.T) {
	tok := longevitySession(t)
	req := sessionTestRequest(t, tok)
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	if _, err := CaptureATSFromEgo(context.Background(), "", ""); !errors.Is(err, ErrRemintNeedsLogin) {
		t.Fatal("harness entered browser capture")
	}
	if err := ObserveSessionResponse(req, cookieResponse("sid=fake; Domain=intuit.com; Path=/")); err != nil {
		t.Fatal(err)
	}
	cur, _ := Load()
	if cur.Cookies[0].Value != "old" {
		t.Fatal("harness overwrote credentials")
	}
}
