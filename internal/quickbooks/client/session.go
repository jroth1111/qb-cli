// session.go provides session-level guards for the qb CLI.
//
// RequireSession verifies that a usable QBO session is saved without
// performing any network activity — it constructs a new API client (which
// loads and validates credentials via auth.Load) and discards it. This is
// the gate that mutation command stubs call before returning
// ErrMutationNotWired, so a missing-credentials failure is deterministic
// and never reaches the network.
package client

import (
	"errors"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

var ErrWriteNotReplayed = errors.New("QBO rejected the write with 401; session renewed but write was not replayed; inspect current state before retrying")

func (c *apiClient) reloadedSession() (*apiClient, error) {
	next, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if !auth.SameBrowserSession(c.tok, next.tok) {
		return nil, auth.ErrSessionChanged
	}
	return next, nil
}

// ErrMutationNotWired is returned by mutation command stubs. The mutation
// commands exist in the CLI tree so the command catalog is complete, but no
// mutation API has been captured or wired: they fail fast with this typed
// error rather than silently no-op'ing or touching QBO. Callers can branch
// on it with errors.Is.
var ErrMutationNotWired = errors.New("mutation not wired: no captured API")

// RequireSession verifies that a usable QBO session is saved. It calls
// newAPIClient (which loads credentials via auth.Load and checks
// HasUsableCredential) and discards the client — no network activity
// occurs. Returns ErrNoCredentials (wrapping auth.ErrNoCredentials) when
// no credentials are saved or the session lacks an ATS Intuit_APIKey.
func RequireSession() error {
	ac, err := newAPIClient()
	if err != nil {
		return err
	}
	_ = ac
	return nil
}
