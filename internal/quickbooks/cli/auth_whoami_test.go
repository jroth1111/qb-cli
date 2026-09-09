package cli

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func runWhoamiCmd(t *testing.T, tabList string, args ...string) (string, error) {
	t.Helper()
	var relay string
	if tabList != "" {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(tabList))
		}))
		t.Cleanup(srv.Close)
		relay = srv.URL
	} else {
		relay = "http://127.0.0.1:1"
	}
	t.Setenv("QB_RELAY_URL", relay)
	root := newRootCmd(&rootFlags{})
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(append([]string{"auth", "whoami"}, args...))
	err := root.Execute()
	return buf.String(), err
}

const whoamiTabs = `[{"id":"T1","type":"page","url":"https://qbo.intuit.com/app/banking"}]`

func stubFetchTabIdentity(t *testing.T, id auth.Identity, err error) {
	t.Helper()
	orig := fetchTabIdentity
	fetchTabIdentity = func(context.Context, string, string) (auth.Identity, error) {
		return id, err
	}
	t.Cleanup(func() { fetchTabIdentity = orig })
}

func TestWhoamiLiveMatch(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	mustSave := &auth.TokenSet{
		Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		RealmID:       "12345",
		CompanyName:   "Test Company 2",
	}
	if err := auth.Save(mustSave); err != nil {
		t.Fatal(err)
	}
	stubFetchTabIdentity(t, auth.Identity{Company: "Test Company 2", Email: "a@b.c", Realm: "12345"}, nil)
	out, err := runWhoamiCmd(t, whoamiTabs)
	if err != nil {
		t.Fatalf("whoami: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Signed in live as Test Company 2") {
		t.Errorf("missing live announcement:\n%s", out)
	}
	if !strings.Contains(out, "Stored session matches.") {
		t.Errorf("missing match confirmation:\n%s", out)
	}
}

func TestWhoamiLiveMismatchWarns(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	if err := auth.Save(&auth.TokenSet{
		Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		RealmID:       "999",
		CompanyName:   "Other Co",
	}); err != nil {
		t.Fatal(err)
	}
	stubFetchTabIdentity(t, auth.Identity{Company: "Test Company 2", Email: "a@b.c", Realm: "12345"}, nil)
	out, err := runWhoamiCmd(t, whoamiTabs)
	if err != nil {
		t.Fatalf("whoami: %v\n%s", err, out)
	}
	if !strings.Contains(out, "WARNING: stored session is Other Co") {
		t.Errorf("missing switch warning:\n%s", out)
	}
}

func TestWhoamiNoTabFallsBackToStored(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	if err := auth.Save(&auth.TokenSet{
		Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
		RealmID:       "12345",
		CompanyName:   "Test Company 2",
		Email:         "a@b.c",
	}); err != nil {
		t.Fatal(err)
	}
	out, err := runWhoamiCmd(t, `[]`)
	if err == nil {
		t.Fatalf("expected non-zero exit with no live tab, got:\n%s", out)
	}
	if !strings.Contains(out, "Stored session: Test Company 2") {
		t.Errorf("missing stored fallback:\n%s", out)
	}
}

func TestAnnounceCompany(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	announceCompany(&buf, &auth.TokenSet{CompanyName: "Test Company 2", Email: "a@b.c", RealmID: "1"})
	if got := buf.String(); !strings.Contains(got, "Logged in as Test Company 2 (a@b.c, realm 1).") {
		t.Errorf("announce = %q", got)
	}
	buf.Reset()
	announceCompany(&buf, &auth.TokenSet{RealmID: "1"})
	if got := buf.String(); !strings.Contains(got, "Logged in as (realm 1) (realm 1).") {
		t.Errorf("realm fallback announce = %q", got)
	}
}
