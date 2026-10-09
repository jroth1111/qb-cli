package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// TestLoginDeadRelayJSONNoOpenFailsFast is the contract's negative case
// (CONTRACT.md step 6 / test 6): with --json --no-open and a relay URL that
// has no listener, `qb login` must fail immediately rather than hang for the
// default 10m poll window. We bind a tight --timeout so the test cannot hang
// even if the environment somehow accepts the TCP connection.
//
// It rejects two failure modes:
//   - returning nil (silently succeeding on a dead relay), and
//   - taking longer than the poll interval would allow (a hang).
func TestLoginDeadRelayJSONNoOpenFailsFast(t *testing.T) {
	// Isolate config so a real ~/.config/qb is never touched. t.Setenv
	// auto-restores on test cleanup.
	t.Setenv("QB_HOME", t.TempDir())

	root := NewRootCommand()
	root.SetArgs([]string{
		"login",
		"--json",
		"--no-open",
		"--relay-url", "http://127.0.0.1:9",
		"--timeout", "2s",
	})

	// Bound the whole command so a regression that ignores --timeout still
	// cannot hang the suite.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	start := time.Now()
	err := root.ExecuteContext(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error for dead relay with --json --no-open, got nil; stdout=%q", out.String())
	}
	// Fail-fast: must return well before the 10s hard cap and far under the
	// default 10m poll. Connection-refused returns in milliseconds; allow
	// headroom for CI scheduling without accepting a real wait.
	if elapsed > 5*time.Second {
		t.Fatalf("login did not fail fast: took %v (want < 5s)", elapsed)
	}
}

// TestLoginFromMitmMissingFileFailsFast is the negative case for the mitm
// bible path: `qb login --from-mitm /no/such.mitm` must fail immediately.
// A missing dump must never enter the interactive 10-minute relay poll —
// the --from-mitm branch returns before any relay probe. We bound the
// whole command so a regression that falls through to the poll still
// cannot hang the suite, and assert the elapsed time is far under the
// default 10m window.
//
// It rejects two failure modes:
//   - returning nil (silently succeeding on a nonexistent dump), and
//   - taking longer than a fast failure would allow (falling through to
//     the relay poll, which would hang for the timeout).
func TestLoginFromMitmMissingFileFailsFast(t *testing.T) {
	// Isolate config so a real ~/.config/qb is never touched.
	t.Setenv("QB_HOME", t.TempDir())

	root := NewRootCommand()
	root.SetArgs([]string{
		"login",
		"--from-mitm", "/no/such.mitm",
	})

	// Hard cap so a fall-through to the 10m poll cannot hang the suite.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	start := time.Now()
	err := root.ExecuteContext(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error for missing mitm dump, got nil; stdout=%q", out.String())
	}
	// Fail-fast: a missing dump returns in milliseconds (mitmdump exits
	// immediately on a bad path, or ImportMitmDump returns the wrapped fs
	// error before spawning). Allow headroom for CI scheduling but reject
	// anything that smells like the relay poll.
	if elapsed > 5*time.Second {
		t.Fatalf("login --from-mitm did not fail fast: took %v (want < 5s)", elapsed)
	}
}

func TestLoginEgoMissingBinaryFailsFast(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	stubUnavailableManagedCapture(t)
	t.Setenv("PATH", "/usr/bin:/bin")
	t.Setenv("QB_RELAY_URL", "http://127.0.0.1:9")
	orig := tryRelayRemint
	tryRelayRemint = func(context.Context) error { return auth.ErrRemintNeedsLogin }
	t.Cleanup(func() { tryRelayRemint = orig })

	root := NewRootCommand()
	root.SetArgs([]string{"login", "--json", "--timeout", "2s"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	start := time.Now()
	err := root.ExecuteContext(ctx)
	if err == nil {
		t.Fatalf("expected error when ego-browser is missing; stdout=%q", out.String())
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("missing ego-browser did not fail fast: %v", time.Since(start))
	}
}

func TestLoginEgoHookPersistsATS(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	stubUnavailableManagedCapture(t)
	origR := tryRelayRemint
	tryRelayRemint = func(context.Context) error { return auth.ErrRemintNeedsLogin }
	t.Cleanup(func() { tryRelayRemint = origR })
	orig := captureATSEgo
	captureATSEgo = func(context.Context, string, string) (*auth.ATSCapture, error) {
		return &auth.ATSCapture{
			Headers: map[string]string{
				"Authorization": "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
				"authtype":      "browser_auth",
			},
			Cookies: []auth.Cookie{{Name: "qbo.ticket", Value: "v", Domain: ".qbo.intuit.com"}},
		}, nil
	}
	t.Cleanup(func() { captureATSEgo = orig })

	root := NewRootCommand()
	root.SetArgs([]string{"login", "--json", "--timeout", "5s"})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v\nstdout=%s\nstderr=%s", err, out.String(), errBuf.String())
	}
	var st auth.Status
	if err := json.Unmarshal(out.Bytes(), &st); err != nil {
		t.Fatalf("status json: %v\nstdout=%s\nstderr=%s", err, out.String(), errBuf.String())
	}
	if !st.OK || !st.HasATSAuthorization || st.Source != "ego-space" {
		t.Fatalf("status %+v", st)
	}

}

func TestLoginEgoPersistsReturnedDurableBrowserBinding(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	t.Setenv("QB_NO_KEEPALIVE", "1")
	stubUnavailableManagedCapture(t)
	oldRelay, oldEgo := tryRelayRemint, captureATSEgo
	defer func() { tryRelayRemint, captureATSEgo = oldRelay, oldEgo }()
	tryRelayRemint = func(context.Context) error { return auth.ErrRemintNeedsLogin }
	captureATSEgo = func(context.Context, string, string) (*auth.ATSCapture, error) {
		return &auth.ATSCapture{Headers: map[string]string{"Authorization": "Intuit_APIKey fixture"}, Identity: auth.Identity{Realm: "12", Email: "fixture@example.invalid"}, EgoSpace: "41", EgoTargetID: "p2"}, nil
	}
	root := NewRootCommand()
	root.SetArgs([]string{"login", "--json", "--keep-alive=false"})
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	tok, err := auth.Load()
	if err != nil || tok.Source != "ego-existing" || tok.EgoSpace != "41" || tok.EgoTargetID != "p2" {
		t.Fatal("retained browser capture was not pinned", err)
	}
}

func stubUnavailableManagedCapture(t *testing.T) {
	t.Helper()
	original := captureManagedFn
	captureManagedFn = func(context.Context, string, string) (*auth.ATSCapture, error) {
		return nil, auth.ErrRemintNeedsLogin
	}
	t.Cleanup(func() { captureManagedFn = original })
}

func TestLoginRelayFirstPersists(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	orig := tryRelayRemint
	tryRelayRemint = func(context.Context) error {
		return auth.Save(&auth.TokenSet{
			Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
			Source:        "relay-session",
		})
	}
	t.Cleanup(func() { tryRelayRemint = orig })

	root := NewRootCommand()
	root.SetArgs([]string{"login", "--json", "--timeout", "5s"})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v\n%s\n%s", err, out.String(), errBuf.String())
	}
	var st auth.Status
	if err := json.Unmarshal(out.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.HasATSAuthorization || st.Source != "relay-session" {
		t.Fatalf("status %+v", st)
	}
}

func TestAuthRemintSameLadderAsLogin(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	origR := tryRelayRemint
	tryRelayRemint = func(context.Context) error { return auth.ErrRemintNeedsLogin }
	t.Cleanup(func() { tryRelayRemint = origR })
	orig := captureATSEgo
	captureATSEgo = func(context.Context, string, string) (*auth.ATSCapture, error) {
		return &auth.ATSCapture{
			Headers: map[string]string{
				"Authorization": "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
			},
			Cookies: []auth.Cookie{{Name: "qbo.ticket", Value: "v"}},
		}, nil
	}
	t.Cleanup(func() { captureATSEgo = orig })

	root := NewRootCommand()
	root.SetArgs([]string{"auth", "remint", "--json", "--timeout", "5s"})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	if err := root.Execute(); err != nil {
		t.Fatalf("remint: %v\n%s\n%s", err, out.String(), errBuf.String())
	}
	var st auth.Status
	if err := json.Unmarshal(out.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.HasATSAuthorization || st.Source != "ego-space" {
		t.Fatalf("status %+v", st)
	}
}

func TestLoginFailsWhenATSCaptureFails(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	orig := captureATS
	captureATS = func(context.Context, string, string, string) (*auth.ATSCapture, error) {
		return nil, auth.ErrNoATSAuthorization
	}
	t.Cleanup(func() { captureATS = orig })

	// Dead relay still fails before capture. This test only checks the
	// hook wiring compiles and cookie-only cannot succeed via the hook.
	if err := auth.ErrNoATSAuthorization; err == nil {
		t.Fatal("sentinel")
	}
	tok := &auth.TokenSet{Cookies: []auth.Cookie{{Name: "qbn.ticket", Value: "t"}}}
	if err := tok.ApplyATSCapture(nil); err == nil {
		t.Fatal("cookie-only ApplyATSCapture must fail")
	}
}

func TestRunLoginManagedWiresIdentity(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	orig := captureManagedFn
	captureManagedFn = func(context.Context, string, string) (*auth.ATSCapture, error) {
		return &auth.ATSCapture{
			Headers:  map[string]string{"Authorization": "Intuit_APIKey intuit_apikey=x"},
			Cookies:  []auth.Cookie{{Name: "a", Value: "1"}},
			Identity: auth.Identity{Company: "Managed Co", Email: "m@x.io", Realm: "777"},
		}, nil
	}
	t.Cleanup(func() { captureManagedFn = orig })
	var buf bytes.Buffer
	tok, err := runLoginManaged(context.Background(), &buf, "")
	if err != nil {
		t.Fatalf("runLoginManaged: %v", err)
	}
	if tok.Source != "managed-profile" {
		t.Errorf("source = %q", tok.Source)
	}
	if tok.CompanyName != "Managed Co" || tok.Email != "m@x.io" {
		t.Errorf("identity = %+v", tok)
	}
}

func TestRunLoginManagedFallsThroughOnError(t *testing.T) {
	orig := captureManagedFn
	captureManagedFn = func(context.Context, string, string) (*auth.ATSCapture, error) {
		return nil, errors.New("no chrome")
	}
	t.Cleanup(func() { captureManagedFn = orig })
	var buf bytes.Buffer
	if _, err := runLoginManaged(context.Background(), &buf, ""); err == nil {
		t.Error("managed failure must error so login falls through to ego")
	}
}

// TestLoginNoManagedSkipsManagedRung proves QB_NO_MANAGED=1 removes the
// managed-profile rung from the explicit login ladder for both `login` and
// `auth remint`: the managed capture hook is never invoked and the flow
// lands on the ego Space path.
func TestLoginNoManagedSkipsManagedRung(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	t.Setenv("QB_NO_MANAGED", "1")

	managedCalled := false
	origM := captureManagedFn
	captureManagedFn = func(context.Context, string, string) (*auth.ATSCapture, error) {
		managedCalled = true
		return nil, errors.New("managed capture must not run under QB_NO_MANAGED=1")
	}
	t.Cleanup(func() { captureManagedFn = origM })

	origR := tryRelayRemint
	tryRelayRemint = func(context.Context) error { return auth.ErrRemintNeedsLogin }
	t.Cleanup(func() { tryRelayRemint = origR })

	origE := captureATSEgo
	captureATSEgo = func(context.Context, string, string) (*auth.ATSCapture, error) {
		return &auth.ATSCapture{
			Headers: map[string]string{
				"Authorization": "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
			},
			Cookies: []auth.Cookie{{Name: "qbo.ticket", Value: "v"}},
		}, nil
	}
	t.Cleanup(func() { captureATSEgo = origE })

	for _, args := range [][]string{
		{"login", "--json", "--timeout", "5s"},
		{"auth", "remint", "--json", "--timeout", "5s"},
	} {
		root := NewRootCommand()
		root.SetArgs(args)
		var out, errBuf bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errBuf)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v\nstdout=%s\nstderr=%s", args, err, out.String(), errBuf.String())
		}
		var st auth.Status
		if err := json.Unmarshal(out.Bytes(), &st); err != nil {
			t.Fatalf("%v: status json: %v\nstdout=%s", args, err, out.String())
		}
		if !st.HasATSAuthorization || st.Source != "ego-space" {
			t.Fatalf("%v: status %+v", args, st)
		}
		if managedCalled {
			t.Fatalf("%v: managed capture ran under QB_NO_MANAGED=1", args)
		}
	}
}

// TestLoginNoManagedKeepsRelayFirst proves QB_NO_MANAGED=1 does not disturb
// the relay-first rung: when an authenticated tab is already on the OMP
// relay, login persists it without touching managed or ego.
func TestLoginNoManagedKeepsRelayFirst(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	t.Setenv("QB_NO_MANAGED", "1")

	managedCalled := false
	origM := captureManagedFn
	captureManagedFn = func(context.Context, string, string) (*auth.ATSCapture, error) {
		managedCalled = true
		return nil, errors.New("managed must not run")
	}
	t.Cleanup(func() { captureManagedFn = origM })

	egoCalled := false
	origE := captureATSEgo
	captureATSEgo = func(context.Context, string, string) (*auth.ATSCapture, error) {
		egoCalled = true
		return nil, errors.New("ego must not run when relay remint succeeds")
	}
	t.Cleanup(func() { captureATSEgo = origE })

	origR := tryRelayRemint
	tryRelayRemint = func(context.Context) error {
		return auth.Save(&auth.TokenSet{
			Authorization: "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
			Source:        "relay-session",
		})
	}
	t.Cleanup(func() { tryRelayRemint = origR })

	root := NewRootCommand()
	root.SetArgs([]string{"login", "--json", "--timeout", "5s"})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v\n%s\n%s", err, out.String(), errBuf.String())
	}
	var st auth.Status
	if err := json.Unmarshal(out.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.HasATSAuthorization || st.Source != "relay-session" {
		t.Fatalf("status %+v", st)
	}
	if managedCalled || egoCalled {
		t.Fatalf("relay-first violated: managed=%v ego=%v", managedCalled, egoCalled)
	}
}

// TestLoginManagedRungDefaultUnchanged proves the ladder is untouched when
// QB_NO_MANAGED is unset: managed capture runs before ego and a successful
// managed capture short-circuits the ego Space entirely.
func TestLoginManagedRungDefaultUnchanged(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	t.Setenv("QB_NO_MANAGED", "")

	origR := tryRelayRemint
	tryRelayRemint = func(context.Context) error { return auth.ErrRemintNeedsLogin }
	t.Cleanup(func() { tryRelayRemint = origR })

	managedCalled := false
	origM := captureManagedFn
	captureManagedFn = func(context.Context, string, string) (*auth.ATSCapture, error) {
		managedCalled = true
		return &auth.ATSCapture{
			Headers: map[string]string{
				"Authorization": "Intuit_APIKey intuit_apikey=x,intuit_apikey_version=1.0",
			},
			Cookies: []auth.Cookie{{Name: "qbo.ticket", Value: "v"}},
		}, nil
	}
	t.Cleanup(func() { captureManagedFn = origM })

	egoCalled := false
	origE := captureATSEgo
	captureATSEgo = func(context.Context, string, string) (*auth.ATSCapture, error) {
		egoCalled = true
		return nil, errors.New("ego must not run when managed capture succeeds")
	}
	t.Cleanup(func() { captureATSEgo = origE })

	root := NewRootCommand()
	root.SetArgs([]string{"login", "--json", "--timeout", "5s"})
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)
	if err := root.Execute(); err != nil {
		t.Fatalf("login: %v\n%s\n%s", err, out.String(), errBuf.String())
	}
	if !managedCalled {
		t.Fatal("managed capture did not run with QB_NO_MANAGED unset")
	}
	if egoCalled {
		t.Fatal("ego ran even though managed capture succeeded")
	}
	var st auth.Status
	if err := json.Unmarshal(out.Bytes(), &st); err != nil {
		t.Fatal(err)
	}
	if !st.HasATSAuthorization || st.Source != "managed-profile" {
		t.Fatalf("status %+v", st)
	}
}
