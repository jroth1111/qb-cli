package auth

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"
)

func TestExistingEgoRenewalUsesPinnedSource(t *testing.T) {
	setQBHome(t)
	tok := &TokenSet{Authorization: "Intuit_APIKey old", Source: "ego-existing", RealmID: "1", Email: "a@example.test", EgoSpace: "named", EgoTargetID: "target"}
	if err := Save(tok); err != nil {
		t.Fatal(err)
	}
	old := captureExistingEgo
	t.Cleanup(func() { captureExistingEgo = old })
	called := false
	captureExistingEgo = func(ctx context.Context, login, banking string) (*ATSCapture, error) {
		called = true
		opt, ok := ctx.Value(existingEgoKey{}).(existingEgoOptions)
		if !ok || opt.space != "named" || opt.target != "target" || login != BankingCaptureURL {
			t.Fatal("lost existing source")
		}
		deadline, _ := ctx.Deadline()
		if time.Until(deadline) > 30*time.Second {
			t.Fatal("unbounded renewal")
		}
		return nil, ErrEgoUserControl
	}
	err := RemintATS(context.Background())
	if !called || !errors.Is(err, ErrEgoUserControl) {
		t.Fatalf("unexpected renewal: %v", err)
	}
	got, err := Load()
	if err != nil || got.SessionID != tok.SessionID || got.Authorization != tok.Authorization {
		t.Fatal("failed renewal changed credentials")
	}
}

func TestCookieHeaderPreservesBrowserQuotesWithoutInjection(t *testing.T) {
	u, _ := url.Parse("https://qbo.intuit.com/api/v3/company/1/query")
	jar := []Cookie{
		{Name: "prefs", Value: `{"option":"value"}`, Domain: "qbo.intuit.com", Path: "/"},
		{Name: "bad", Value: "value; injected=1", Domain: "qbo.intuit.com", Path: "/"},
		{Name: "bad\r\n", Value: "value", Domain: "qbo.intuit.com", Path: "/"},
		{Name: "line", Value: "value\nInjected: x", Domain: "qbo.intuit.com", Path: "/"},
	}
	if got := CookieHeaderForURL(jar, u, time.Now()); got != `prefs={"option":"value"}` {
		t.Fatalf("changed browser bytes or accepted injection: %q", got)
	}
}
