package client

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestKeeperRecoversConfirmedBrowserSignInBeforeBankingDispatch(t *testing.T) {
	for _, failure := range []error{auth.ErrRemintNeedsLogin, auth.ErrEgoUserControl, auth.ErrSessionChanged, auth.ErrRecoveryAttention, context.DeadlineExceeded} {
		t.Run(failure.Error(), func(t *testing.T) {
			saveUsable(t)
			expected, _ := auth.Load()
			expected.Source, expected.EgoSpace, expected.EgoTargetID = "ego-existing", "52", "p1"
			if err := auth.Save(expected); err != nil {
				t.Fatal(err)
			}
			oldProbe, oldRefresh, oldNative := maintenanceBankingProbe, refreshQuiet, extendNativeTicket
			defer func() { maintenanceBankingProbe, refreshQuiet, extendNativeTicket = oldProbe, oldRefresh, oldNative }()
			probes, renewals, native := 0, 0, 0
			maintenanceBankingProbe = func(context.Context, *apiClient, string) (bool, int, string, error) {
				probes++
				if probes == 1 {
					return false, 0, "", failure
				}
				return true, 200, "session accepted", nil
			}
			refreshQuiet = func(context.Context, *auth.TokenSet, bool) error { renewals++; return nil }
			extendNativeTicket = func(context.Context, *auth.TokenSet, bool) error { native++; return nil }
			ok, status, state := MaintainSession(context.Background(), expected, true)
			if errors.Is(failure, auth.ErrRemintNeedsLogin) {
				if !ok || status != 200 || state != "live" || renewals != 1 || native != 1 || probes != 2 {
					t.Fatal("pre-dispatch sign-in wall was not healed", ok, status, state, renewals, native, probes)
				}
			} else if ok || renewals != 0 || native != 0 || probes != 1 || state != bankingMaintenanceFailure(failure) {
				t.Fatal("non-authentication failure invoked recovery", ok, state, renewals, native, probes)
			}
		})
	}
}

func TestMaintainSessionReadOnlyAndTemporaryFailures(t *testing.T) {
	for _, status := range []int{200, 401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			saveUsable(t)
			expected, _ := auth.Load()
			srv := newDomServer(t, status, `{"accounts":[]}`)
			interceptHTTP(t, srv.URL)
			prev := refreshQuiet
			calls := 0
			refreshQuiet = func(context.Context, *auth.TokenSet, bool) error { calls++; return context.DeadlineExceeded }
			t.Cleanup(func() { refreshQuiet = prev })
			ok, _, state := MaintainSession(context.Background(), expected, false)
			if status == 200 && !ok {
				t.Fatal("healthy session rejected")
			}
			if status != 200 && state != "temporarily_unavailable" {
				t.Fatalf("transient outcome=%s", state)
			}
			_, method, _, _ := srv.snap()
			if method != http.MethodGet {
				t.Fatalf("keeper used %s", method)
			}
			if status != 401 && calls != 0 {
				t.Fatal("renewed without authentication rejection")
			}
		})
	}
}

func TestMaintainSessionStopsForEgoUserControl(t *testing.T) {
	saveUsable(t)
	expected, _ := auth.Load()
	srv := newDomServer(t, 401, `{"error":"expired"}`)
	interceptHTTP(t, srv.URL)
	old := refreshQuiet
	refreshQuiet = func(context.Context, *auth.TokenSet, bool) error { return auth.ErrEgoUserControl }
	t.Cleanup(func() { refreshQuiet = old })
	if ok, status, state := MaintainSession(context.Background(), expected, false); ok || status != 401 || state != "user_control" {
		t.Fatalf("ok=%v status=%d state=%s", ok, status, state)
	}
}

func TestMaintainSessionCannotSwitchBrowserWithinSameLogin(t *testing.T) {
	for _, field := range []string{"source", "space", "target"} {
		t.Run(field, func(t *testing.T) {
			saveUsable(t)
			expected, _ := auth.Load()
			expected.Source, expected.EgoSpace, expected.EgoTargetID = "ego-existing", "52", "p1"
			if err := auth.Save(expected); err != nil {
				t.Fatal(err)
			}
			expected, _ = auth.Load()
			changed := *expected
			switch field {
			case "source":
				changed.Source = "managed-profile"
			case "space":
				changed.EgoSpace = "53"
			case "target":
				changed.EgoTargetID = "p2"
			}
			if err := auth.Save(&changed); err != nil {
				t.Fatal(err)
			}
			if !auth.SameSession(expected, &changed) {
				t.Fatal("fixture must preserve the login identity")
			}
			srv := newDomServer(t, 200, `{"accounts":[]}`)
			interceptHTTP(t, srv.URL)
			ok, status, state := MaintainSession(context.Background(), expected, true)
			if ok || status != 0 || state != "session_changed" {
				t.Fatalf("browser switch accepted: ok=%v status=%d state=%s", ok, status, state)
			}
			if calls, _, _, _ := srv.snap(); calls != 0 {
				t.Fatal("different browser was contacted")
			}
		})
	}
}

func TestRejectedWriteIsNotReplayedAfterRenewal(t *testing.T) {
	saveUsable(t)
	srv := newDomServer(t, 401, `{"error":"expired"}`)
	interceptHTTP(t, srv.URL)
	old := remint
	remint = func(context.Context) error { return nil }
	t.Cleanup(func() { remint = old })
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ac.postJSON(context.Background(), "https://qbo.intuit.com/api/v3/company/"+ac.realm+"/purchase", []byte(`{}`)); !errors.Is(err, ErrWriteNotReplayed) {
		t.Fatalf("err=%v", err)
	}
	count, _, _, _ := srv.snap()
	if count != 1 {
		t.Fatalf("write replayed %d times", count)
	}
}

func TestReadRenewalCannotSwitchCompany(t *testing.T) {
	saveUsable(t)
	srv := newDomServer(t, 401, `{}`)
	interceptHTTP(t, srv.URL)
	old := remint
	remint = func(context.Context) error {
		return auth.Save(&auth.TokenSet{RealmID: "different", Authorization: "Intuit_APIKey intuit_apikey=other"})
	}
	t.Cleanup(func() { remint = old })
	ac, _ := newAPIClient()
	if _, err := ac.get(context.Background(), "https://qbo.intuit.com/api/neo/v1/company/"+ac.realm+"/olb/ng/getInitialData", ""); !errors.Is(err, auth.ErrSessionChanged) {
		t.Fatalf("err=%v", err)
	}
	count, _, _, _ := srv.snap()
	if count != 1 {
		t.Fatal("request replayed across companies")
	}
}

func TestKeeperNativeExtensionRunsOnlyAfterHealthyManagedProbe(t *testing.T) {
	for _, code := range []int{200, 401, 403, 500} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			saveUsable(t)
			tok, _ := auth.Load()
			tok.Source = "managed-profile"
			_ = auth.Save(tok)
			srv := newDomServer(t, code, `{"accounts":[]}`)
			interceptHTTP(t, srv.URL)
			oldExtend, oldRefresh := extendNativeTicket, refreshQuiet
			t.Cleanup(func() { extendNativeTicket = oldExtend; refreshQuiet = oldRefresh })
			calls := 0
			extendNativeTicket = func(context.Context, *auth.TokenSet, bool) error { calls++; return context.DeadlineExceeded }
			refreshQuiet = func(context.Context, *auth.TokenSet, bool) error { return context.DeadlineExceeded }
			ok, _, _ := MaintainSession(context.Background(), tok, true)
			if code == 200 && (ok || calls != 1) {
				t.Fatal("native failure must not report successful maintenance")
			}
			if code != 200 && calls != 0 {
				t.Fatal("native maintenance ran on a rejected/transient banking response")
			}
		})
	}
}

func TestHealthyEgoBankProbeCannotHideNativeStop(t *testing.T) {
	for _, tc := range []struct {
		err   error
		state string
	}{
		{auth.ErrEgoUserControl, "user_control"},
		{auth.ErrRecoveryAttention, "needs_attention"},
		{auth.ErrNativeTicketPending, "native_extension_pending"},
		{auth.ErrSessionChanged, "session_changed"},
	} {
		t.Run(tc.state, func(t *testing.T) {
			saveUsable(t)
			tok, _ := auth.Load()
			tok.Source, tok.EgoSpace, tok.EgoTargetID = "ego-existing", "52", "p1"
			if err := auth.Save(tok); err != nil {
				t.Fatal(err)
			}
			tok, _ = auth.Load()
			srv := newDomServer(t, 200, `{"accounts":[]}`)
			interceptHTTP(t, srv.URL)
			old := extendNativeTicket
			t.Cleanup(func() { extendNativeTicket = old })
			extendNativeTicket = func(context.Context, *auth.TokenSet, bool) error { return tc.err }
			ok, status, state := MaintainSession(context.Background(), tok, true)
			if ok || status != 200 || state != tc.state {
				t.Fatalf("ok=%v status=%d state=%s", ok, status, state)
			}
		})
	}
}

func TestHealthyAPIRecoversSignedOutOwnedBrowserOnce(t *testing.T) {
	for _, source := range []string{"managed-profile", "ego-existing"} {
		t.Run(source, func(t *testing.T) {
			saveUsable(t)
			tok, _ := auth.Load()
			tok.Source = source
			if source == "ego-existing" {
				tok.EgoSpace, tok.EgoTargetID = "52", "p1"
			}
			if err := auth.Save(tok); err != nil {
				t.Fatal(err)
			}
			srv := newDomServer(t, 200, `{"accounts":[]}`)
			interceptHTTP(t, srv.URL)
			oldExtend, oldRefresh := extendNativeTicket, refreshQuiet
			t.Cleanup(func() { extendNativeTicket = oldExtend; refreshQuiet = oldRefresh })
			extensions, recoveries := 0, 0
			extendNativeTicket = func(_ context.Context, current *auth.TokenSet, _ bool) error {
				extensions++
				if extensions == 1 {
					return auth.ErrRemintNeedsLogin
				}
				if current.CredentialGeneration != tok.CredentialGeneration+1 || current.Source != source {
					t.Fatal("did not load the recovered pinned session")
				}
				return nil
			}
			refreshQuiet = func(_ context.Context, current *auth.TokenSet, allowed bool) error {
				recoveries++
				if !allowed || !auth.SameSession(tok, current) || current.Source != source || current.EgoSpace != tok.EgoSpace || current.EgoTargetID != tok.EgoTargetID {
					t.Fatal("recovery lost its authorization or source binding")
				}
				next := *current
				next.CredentialGeneration++
				return auth.Save(&next)
			}
			var events []MaintenanceEvent
			ctx := WithMaintenanceObserver(context.Background(), func(event MaintenanceEvent) { events = append(events, event) })
			ok, code, state := MaintainSession(ctx, tok, true)
			if !ok || code != 200 || state != "live" || extensions != 2 || recoveries != 1 {
				t.Fatalf("ok=%v code=%d state=%s extensions=%d recoveries=%d", ok, code, state, extensions, recoveries)
			}
			requests, method, _, _ := srv.snap()
			if requests != 2 || method != http.MethodGet {
				t.Fatal("recovery lacked GET-only independent readback")
			}
			var phases []MaintenancePhase
			for _, event := range events {
				phases = append(phases, event.Phase)
				if event.HTTPStatus != 200 {
					t.Fatal("mixed-state phase lost the successful API result")
				}
			}
			want := []MaintenancePhase{MaintenanceBankingLive, MaintenanceBrowserSignedOut, MaintenanceRecovering, MaintenanceBankingReverified, MaintenanceNativeVerified}
			if !reflect.DeepEqual(phases, want) {
				t.Fatal("missing ordered daemon diagnostics", phases)
			}
			encoded, _ := json.Marshal(events)
			for _, secretField := range []string{"password", "totp", "cookie", "authorization", "email"} {
				if strings.Contains(string(encoded), secretField) {
					t.Fatal("maintenance diagnostics exposed secret-shaped fields")
				}
			}
		})
	}
}

func TestSignedOutNativeBrowserDoesNotLoopOrIgnoreRecoveryFailures(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		failure error
		state   string
	}{
		{"challenge", auth.ErrRecoveryAttention, "needs_attention"},
		{"key-unavailable", auth.ErrRecoveryKeyUnavailable, "needs_attention"},
		{"disabled", auth.ErrRecoveryDisabled, "temporarily_unavailable"},
		{"cooldown", auth.ErrRecoveryCooldown, "temporarily_unavailable"},
		{"wrong-identity", auth.ErrSessionChanged, "session_changed"},
		{"user-control", auth.ErrEgoUserControl, "user_control"},
		{"unknown", context.DeadlineExceeded, "temporarily_unavailable"},
		{"still-signed-out", nil, "needs_login"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			saveUsable(t)
			tok, _ := auth.Load()
			tok.Source = "managed-profile"
			if err := auth.Save(tok); err != nil {
				t.Fatal(err)
			}
			srv := newDomServer(t, 200, `{"accounts":[]}`)
			interceptHTTP(t, srv.URL)
			oldExtend, oldRefresh := extendNativeTicket, refreshQuiet
			t.Cleanup(func() { extendNativeTicket = oldExtend; refreshQuiet = oldRefresh })
			extensions, recoveries := 0, 0
			extendNativeTicket = func(context.Context, *auth.TokenSet, bool) error { extensions++; return auth.ErrRemintNeedsLogin }
			refreshQuiet = func(context.Context, *auth.TokenSet, bool) error { recoveries++; return fixture.failure }
			ok, _, state := MaintainSession(context.Background(), tok, true)
			if ok || state != fixture.state || recoveries != 1 || extensions > 2 {
				t.Fatal("recovery looped or hid its failure", state, recoveries, extensions)
			}
		})
	}
}

func TestHealthyAPIDoesNotRecoverForUncertainNativeFailures(t *testing.T) {
	for _, failure := range []error{context.DeadlineExceeded, auth.ErrRecoveryAttention, auth.ErrSessionChanged, auth.ErrEgoUserControl, auth.ErrNativeTicketPending} {
		t.Run(failure.Error(), func(t *testing.T) {
			saveUsable(t)
			tok, _ := auth.Load()
			tok.Source = "managed-profile"
			if err := auth.Save(tok); err != nil {
				t.Fatal(err)
			}
			srv := newDomServer(t, 200, `{"accounts":[]}`)
			interceptHTTP(t, srv.URL)
			oldExtend, oldRefresh := extendNativeTicket, refreshQuiet
			t.Cleanup(func() { extendNativeTicket = oldExtend; refreshQuiet = oldRefresh })
			extendNativeTicket = func(context.Context, *auth.TokenSet, bool) error { return failure }
			refreshQuiet = func(context.Context, *auth.TokenSet, bool) error {
				t.Fatal("uncertain native failure resolved login credentials")
				return nil
			}
			if ok, _, _ := MaintainSession(context.Background(), tok, true); ok {
				t.Fatal("native failure was reported healthy")
			}
		})
	}
}
