package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// TestCrmNeedsRelayTab verifies that CRM operations fail with
// ErrCrmNeedsRelayTab when no relay is reachable: the transport routes
// through the authenticated qbo.intuit.com relay tab (crm_cdp.go) and the
// retired blanket external-block error no longer exists.
func TestCrmNeedsRelayTab(t *testing.T) {
	crmSaveSession(t)
	t.Setenv("QB_RELAY_URL", httptestDeadRelay())

	ctx := context.Background()
	if _, err := ReplayLeadGet(ctx, "L1"); !errors.Is(err, ErrCrmNeedsRelayTab) {
		t.Errorf("lead get: got %v, want ErrCrmNeedsRelayTab", err)
	}
	if _, err := ReplayLeadCreate(ctx, CrmLeadInput{DisplayName: "test"}); !errors.Is(err, ErrCrmNeedsRelayTab) {
		t.Errorf("lead create: got %v, want ErrCrmNeedsRelayTab", err)
	}
}

// httptestDeadRelay returns a closed server URL: deterministic connection
// refused without touching the real default relay port.
func httptestDeadRelay() string {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()
	return srv.URL
}

// TestCrmPlansStillValidateIDs ensures ID validation happens before any
// transport work.
func TestCrmPlansStillValidateIDs(t *testing.T) {
	_, err := PlanLeadGet("")
	if !errors.Is(err, ErrEmptyLeadID) {
		t.Fatalf("err = %v, want ErrEmptyLeadID", err)
	}
}

var _ = auth.TokenSet{}
