package client

import (
	"context"
	"errors"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"net/http"
	"testing"
)

func TestMaintainSessionReadOnlyAndTemporaryFailures(t *testing.T) {
	for _, status := range []int{200, 401, 500} {
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
	if _, err = ac.postJSON(context.Background(), "https://qbo.intuit.com/api/v3/company/1/purchase", []byte(`{}`)); !errors.Is(err, ErrWriteNotReplayed) {
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
	if _, err := ac.get(context.Background(), "https://qbo.intuit.com/api/neo/v1/company/1/olb/ng/getInitialData", ""); !errors.Is(err, auth.ErrSessionChanged) {
		t.Fatalf("err=%v", err)
	}
	count, _, _, _ := srv.snap()
	if count != 1 {
		t.Fatal("request replayed across companies")
	}
}
