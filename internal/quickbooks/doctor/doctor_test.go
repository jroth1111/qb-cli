package doctor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// saveToken writes a TokenSet with controllable realm/headers/expiry.
func saveToken(t *testing.T, realm, hdrCompany string, expired bool) {
	t.Helper()
	t.Setenv("QB_HOME", t.TempDir()) // isolate: never write the real credential store
	tok := &auth.TokenSet{
		Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		RealmID:       realm,
		Cookies: []auth.Cookie{
			{Name: "qbo.csrftoken", Value: "csrf-value"},
		},
	}
	if hdrCompany != "" {
		tok.RequestHeaders = map[string]string{"intuit-company-id": hdrCompany}
	}
	if !expired {
		tok.AccessExpiry = farFuture()
	}
	if err := auth.Save(tok); err != nil {
		t.Fatal(err)
	}
}

// swapProbeTarget routes client.ProbeSession's qbo.intuit.com request to srv.
func swapProbeTarget(t *testing.T, srv *httptest.Server) {
	t.Helper()
	interceptHTTP(t, srv.URL)
}

// healthyServer returns 200 for any path.
func healthyServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"accounts":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// rejectingServer returns 401 for any path (stale-credential simulation).
func rejectingServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func checkByName(res DoctorResult, name string) Check {
	for _, c := range res.Checks {
		if c.Name == name {
			return c
		}
	}
	return Check{Name: name + " (missing)"}
}

// TestRunDoctorAllGreen verifies the fully-healthy session: every check OK.
func TestRunDoctorAllGreen(t *testing.T) {
	saveToken(t, "9001", "9001", false)
	srv := healthyServer(t)
	swapProbeTarget(t, srv)

	res := RunDoctor(context.Background())
	if !res.OverallOK {
		t.Fatalf("OverallOK = false; checks: %+v", res.Checks)
	}
	for _, name := range []string{"credentials-file", "has-ats-auth", "csrf-present", "realm-consistency", "live-probe", "expiry"} {
		c := checkByName(res, name)
		if !c.OK {
			t.Errorf("check %q failed: %s", name, c.Detail)
		}
	}
}

// TestRunDoctorMixedRealm detects a stored realm that diverges from the
// captured intuit-company-id header — the shipped bug this command guards.
func TestRunDoctorMixedRealm(t *testing.T) {
	saveToken(t, "1111", "2222", false)
	srv := healthyServer(t)
	swapProbeTarget(t, srv)

	res := RunDoctor(context.Background())
	c := checkByName(res, "realm-consistency")
	if c.OK {
		t.Fatal("realm-consistency should fail on mismatched realm/header")
	}
	if !res.OverallOK {
		// overall must reflect the failure
	} else {
		t.Error("OverallOK = true despite mixed realm")
	}
	// live probe still runs and reports independently
	if pc := checkByName(res, "live-probe"); !pc.OK && pc.Detail == "" {
		t.Error("live-probe should still execute with its own detail")
	}
}

// TestRunDoctorStaleCredentials simulates the server rejecting the session:
// live-probe fails while static checks stay green.
func TestRunDoctorStaleCredentials(t *testing.T) {
	saveToken(t, "9001", "9001", false)
	srv := rejectingServer(t)
	swapProbeTarget(t, srv)

	res := RunDoctor(context.Background())
	pc := checkByName(res, "live-probe")
	if pc.OK {
		t.Fatal("live-probe should fail against a 401 server")
	}
	if res.OverallOK {
		t.Error("OverallOK = true despite rejected credentials")
	}
}

// TestRunDoctorMissingCredentials is the fast-fail negative: no creds file
// means only one failing check and no live network attempt beyond it.
func TestRunDoctorMissingCredentials(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())

	res := RunDoctor(context.Background())
	if res.OverallOK {
		t.Fatal("OverallOK = true without credentials")
	}
	c := checkByName(res, "credentials-file")
	if c.OK {
		t.Error("credentials-file should fail when absent")
	}
	if len(res.Checks) != 1 {
		t.Errorf("checks = %d, want 1 (no point probing without a file)", len(res.Checks))
	}
}

// TestRunDoctorExpiredToken flags an expired access token even though the
// other static checks pass.
func TestRunDoctorExpiredToken(t *testing.T) {
	saveToken(t, "9001", "9001", true)
	srv := healthyServer(t)
	swapProbeTarget(t, srv)

	res := RunDoctor(context.Background())
	c := checkByName(res, "expiry")
	if c.OK {
		t.Error("expiry should fail on a past access_expiry")
	}
}

// farFuture returns a time far in the future for a valid token.
func farFuture() time.Time { return time.Now().Add(365 * 24 * time.Hour) }

// interceptHTTP routes production qbo.intuit.com requests to srv by
// swapping http.DefaultTransport (restored on cleanup).
func interceptHTTP(t *testing.T, rawurl string) {
	t.Helper()
	orig := http.DefaultTransport
	u, _ := url.Parse(rawurl)
	rt := &http.Transport{}
	http.DefaultTransport = &rewriteTransport{scheme: u.Scheme, host: u.Host, rt: rt}
	t.Cleanup(func() {
		rt.CloseIdleConnections()
		http.DefaultTransport = orig
	})
}

type rewriteTransport struct {
	scheme, host string
	rt           http.RoundTripper
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.URL.Scheme = r.scheme
	clone.URL.Host = r.host
	clone.Host = ""
	return r.rt.RoundTrip(clone)
}
