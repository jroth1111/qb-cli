package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/spf13/cobra"
)

// loginFlags holds flags local to the login command. Persistent flags
// (--json, --relay-url, --timeout, --home) live on rootFlags.
type loginFlags struct {
	loginURL string
	noOpen   bool
	fromMitm string
}

// captureATS intercepts one ATS Intuit_APIKey request via the OMP relay.
// Tests replace this so login can be exercised without Chrome.
var captureATS = auth.CaptureATSFromRelay

// newLoginCmd builds the top-level `qb login` command — an alias of
// `qb auth login`. Default path is an isolated ego-browser Space + ATS
// Fetch/Network intercept. --no-open adopts a live OMP-relay tab.
// --from-mitm imports a dump.
func newLoginCmd(flags *rootFlags) *cobra.Command {
	return buildLoginCmd("login", flags)
}

// newAuthLoginCmd builds the `qb auth login` command. It is functionally
// identical to `qb login`; both share runLogin so behavior never diverges.
func newAuthLoginCmd(flags *rootFlags) *cobra.Command {
	return buildLoginCmd("login", flags)
}

func buildLoginCmd(use string, flags *rootFlags) *cobra.Command {
	lf := &loginFlags{}
	cmd := &cobra.Command{
		Use:   use,
		Short: "Capture a QBO session (relay tab, managed profile, else ego Space)",
		Long: `Capture the ATS Authorization header (Intuit_APIKey) and cookie jar.

Order:
  1. If a qbo.intuit.com/app/* tab is already on the OMP relay, intercept it.
  2. Else open a managed Chromium window on a persistent profile
     (QB_HOME/profiles/qbo) and sign in there normally. A still-valid
     profile signs straight in; ATS key + identity are mined in-context.
  3. Else open an isolated ego-browser Space (does not touch your daily tabs).
     Complete Intuit sign-in there if prompted.

You sign in; qb mines the rest: ATS headers, cookies, and the signed-in
company/email/realm from the hydrated tab, all into the 0600 credentials
file. Every success leads with the bound company. ` + "`qb auth status --live`" + ` re-verifies later.

--no-open: only the relay path (wait for a tab; never open a browser).
--from-mitm PATH: import a mitmproxy dump (no browser).
Cookie-only sessions are rejected. --json never prints secrets.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd, flags, lf)
		},
	}
	cmd.Flags().StringVar(&lf.loginURL, "login-url", auth.DefaultLoginURL,
		"QBO sign-in URL")
	cmd.Flags().BoolVar(&lf.noOpen, "no-open", false,
		"relay only: wait for an already-open QBO tab (no ego Space)")
	cmd.Flags().StringVar(&lf.fromMitm, "from-mitm", "",
		"import credentials from a mitmproxy dump file (no Chrome/relay)")
	return cmd
}

// tryRelayRemint is the fast path used by login/remint. Tests replace it.
var tryRelayRemint = auth.TryRelayRemint

func runLogin(cmd *cobra.Command, flags *rootFlags, lf *loginFlags) error {
	if lf.fromMitm != "" {
		return runLoginFromMitm(cmd, flags, lf)
	}
	if lf.noOpen {
		return runLoginRelay(cmd, flags, lf)
	}
	timeout := flags.timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	if err := tryRelayRemint(ctx); err == nil {
		tok, lerr := auth.Load()
		if lerr != nil {
			return &ExitError{Code: ExitAuthError, Err: lerr, Silent: flags.asJSON}
		}
		fmt.Fprintln(cmd.ErrOrStderr(), "Captured ATS headers from an open QBO tab.")
		return persistLogin(cmd, flags, tok)
	}
	// Managed persistent profile first: a visible Chromium on a stable
	// profile dir the user signs into normally. Falls through to the
	// isolated ego Space only when managed Chrome cannot start.
	if tok, merr := runLoginManaged(ctx, cmd.ErrOrStderr(), lf.loginURL); merr == nil {
		return persistLogin(cmd, flags, tok)
	} else {
		fmt.Fprintf(cmd.ErrOrStderr(), "qb: managed browser unavailable (%v); trying ego Space...\n", merr)
	}
	return runLoginEgo(cmd, flags, lf)
}

// captureManagedFn runs the managed-profile capture. Tests replace it so
// login is exercised without launching Chrome.
var captureManagedFn = auth.CaptureManaged

// runLoginManaged captures via the persistent managed Chromium profile and
// returns the token set (unp persisted; the caller persists). Any error
// means managed Chrome is unusable here — the caller falls through.
func runLoginManaged(ctx context.Context, stderr io.Writer, loginURL string) (*auth.TokenSet, error) {
	if loginURL == "" {
		loginURL = auth.DefaultLoginURL
	}
	fmt.Fprintln(stderr, "Opening managed Chromium (persistent profile) for QBO login.")
	fmt.Fprintln(stderr, "Complete Intuit sign-in in that window if prompted.")
	cap, err := captureManagedFn(ctx, loginURL, auth.BankingCaptureURL)
	if err != nil {
		return nil, err
	}
	tok := &auth.TokenSet{
		Version:    auth.CurrentVersion,
		CapturedAt: time.Now().UTC(),
		Source:     "managed-profile",
		LoginURL:   loginURL,
		FinalURL:   auth.BankingCaptureURL,
	}
	if err := tok.ApplyATSCapture(cap); err != nil {
		return nil, err
	}
	tok.ClassifyCookies()
	return tok, nil
}

// runLoginEgo is the default isolated path: ego-browser Space + ATS intercept.
func runLoginEgo(cmd *cobra.Command, flags *rootFlags, lf *loginFlags) error {
	stderr := cmd.ErrOrStderr()
	loginURL := lf.loginURL
	if loginURL == "" {
		loginURL = auth.DefaultLoginURL
	}
	timeout := flags.timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	fmt.Fprintln(stderr, "Opening isolated ego-browser Space for QBO login.")
	fmt.Fprintln(stderr, "Complete Intuit sign-in in that Space if prompted.")
	fmt.Fprintf(stderr, "Waiting up to %s for an ATS Intuit_APIKey request...\n", timeout)

	cap, err := captureATSEgo(ctx, loginURL, auth.BankingCaptureURL)
	if err != nil {
		if errors.Is(err, auth.ErrEgoMissing) {
			fmt.Fprintln(stderr, "qb: ego-browser is not installed.")
			fmt.Fprintln(stderr, "Install ego lite, or use --from-mitm PATH / --no-open (OMP relay).")
		} else {
			fmt.Fprintf(stderr, "qb: ego ATS capture failed: %v\n", err)
		}
		return &ExitError{Code: ExitAuthError, Err: fmt.Errorf("ego capture: %w", err), Silent: flags.asJSON}
	}
	tok := &auth.TokenSet{
		Version:    auth.CurrentVersion,
		CapturedAt: time.Now().UTC(),
		Source:     "ego-space",
		LoginURL:   loginURL,
		FinalURL:   auth.BankingCaptureURL,
	}
	if err := tok.ApplyATSCapture(cap); err != nil {
		return &ExitError{Code: ExitAuthError, Err: err, Silent: flags.asJSON}
	}
	tok.ClassifyCookies()
	return persistLogin(cmd, flags, tok)
}

// captureATSEgo is the ego capture hook. Tests replace it.
var captureATSEgo = auth.CaptureATSFromEgo

// persistLogin saves tok and prints RedactedStatus. No secrets on stdout.
// Identity (company/email) arrives inside tok via capture-time DOM mining
// (ego script / relay Runtime.evaluate); persist adds no fetching of its own.
func persistLogin(cmd *cobra.Command, flags *rootFlags, tok *auth.TokenSet) error {
	stderr := cmd.ErrOrStderr()
	stdout := cmd.OutOrStdout()
	if err := auth.Save(tok); err != nil {
		fmt.Fprintf(stderr, "qb: saving credentials failed: %v\n", err)
		return &ExitError{Code: ExitAuthError, Err: fmt.Errorf("saving credentials: %w", err), Silent: flags.asJSON}
	}
	status := tok.RedactedStatus()
	status.OK = tok.HasATSAuthorization()
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(status)
	} else {
		announceCompany(stdout, tok)
		printStatus(stdout, stderr, flags, status)
		fmt.Fprintf(stderr, "Saved %d cookie(s) to %s\n", status.CookieCount, auth.CredPath())
	}
	return nil
}

// announceCompany leads every login success with the signed-in company so
// it is impossible to finish login unsure which company is bound. Falls
// back to the realm when mining left the name empty.
func announceCompany(stdout io.Writer, tok *auth.TokenSet) {
	company := tok.CompanyName
	if company == "" {
		company = "(realm " + tok.RealmID + ")"
	}
	if tok.Email != "" {
		fmt.Fprintf(stdout, "Logged in as %s (%s, realm %s).\n", company, tok.Email, tok.RealmID)
		return
	}
	fmt.Fprintf(stdout, "Logged in as %s (realm %s).\n", company, tok.RealmID)
}

// lastLoginHint returns the previously-mined login email for the sign-in
// prompt, or "" when no prior login exists. Best-effort: any load failure
// means a generic prompt.
func lastLoginHint() string {
	tok, err := auth.Load()
	if err != nil || tok == nil {
		return ""
	}
	return tok.Email
}

// runLoginRelay adopts an already-open tab via the OMP DevTools relay.
func runLoginRelay(cmd *cobra.Command, flags *rootFlags, lf *loginFlags) error {
	stderr := cmd.ErrOrStderr()
	relayURL := resolveRelayURL(flags.relayURL)
	loginURL := lf.loginURL
	if loginURL == "" {
		loginURL = auth.DefaultLoginURL
	}
	timeout := flags.timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()

	if err := probeRelay(ctx, relayURL); err != nil {
		fmt.Fprintf(stderr, "qb: Chrome relay not reachable at %s: %v\n", relayURL, err)
		fmt.Fprintln(stderr, "Start Chrome with the OMP relay (remote debugging port 9224) and retry.")
		return &ExitError{
			Code:   ExitRelayError,
			Err:    fmt.Errorf("relay unreachable at %s: %w", relayURL, err),
			Silent: flags.asJSON,
		}
	}

	// Step 8/11: look for an already-authenticated tab before waiting.
	tabs, err := listTabs(ctx, relayURL)
	if err != nil {
		fmt.Fprintf(stderr, "qb: relay /json/list failed: %v\n", err)
		return &ExitError{Code: ExitRelayError, Err: fmt.Errorf("listing tabs: %w", err), Silent: flags.asJSON}
	}

	authTab := findAuthTab(tabs)

	if authTab == nil {
		onLogin := findTabOnURL(tabs, loginURL)
		if onLogin == nil && !lf.noOpen {
			if err := openLoginURL(stderr, loginURL); err != nil {
				fmt.Fprintf(stderr, "qb: could not open login URL: %v\n", err)
			}
		}
		fmt.Fprintln(stderr, "Complete the Intuit sign-in in Chrome.")
		if hint := lastLoginHint(); hint != "" {
			fmt.Fprintf(stderr, "  account: %s (last used)\n", hint)
		}
		fmt.Fprintf(stderr, "  waiting up to %s for an authenticated qbo.intuit.com tab...\n", timeout)

		// Step 10: poll /json/list every 1s until authenticated or timeout.
		authTab, err = waitForAuthTab(ctx, relayURL)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				fmt.Fprintln(stderr, "qb: timed out waiting for sign-in to complete.")
			} else {
				fmt.Fprintf(stderr, "qb: polling relay failed: %v\n", err)
			}
			return &ExitError{Code: ExitAuthError, Err: fmt.Errorf("login did not complete: %w", err), Silent: flags.asJSON}
		}
	}

	finalURL := authTab.URL

	// Step 12: fresh jar straight from the live tab over CDP (the on-disk
	// Chrome store lags it). Falls back to press-auth/store capture.
	cookies, cookieSource, err := relayTabCookies(ctx, relayURL, authTab.ID)
	if err != nil || len(cookies) == 0 {
		cookies, cookieSource, err = captureCookies()
	}
	if err != nil {
		fmt.Fprintf(stderr, "qb: cookie capture failed: %v\n", err)
		return &ExitError{Code: ExitAuthError, Err: fmt.Errorf("capturing cookies: %w", err), Silent: flags.asJSON}
	}
	if len(cookies) == 0 {
		fmt.Fprintln(stderr, "qb: no Intuit cookies captured; sign in fully and retry.")
		return &ExitError{Code: ExitAuthError, Err: errors.New("no Intuit cookies captured"), Silent: flags.asJSON}
	}

	// Step 13: live ATS key. Best-effort on a short leash: when the relay
	// does not deliver Fetch events the key never arrives, and the cookie
	// refresh below is the working vector — never burn the full timeout
	// here when the tab is already authenticated.
	fmt.Fprintln(stderr, "Intercepting one ATS banking request (Intuit_APIKey)...")
	atsCtx, atsCancel := context.WithTimeout(ctx, 90*time.Second)
	cap, capErr := captureATS(atsCtx, relayURL, authTab.ID, auth.BankingCaptureURL)
	atsCancel()

	// Step 14: identity from the hydrated tab DOM (independent of ATS).
	identity, _ := fetchTabIdentity(ctx, relayURL, authTab.ID)

	if capErr == nil {
		tok := &auth.TokenSet{
			Version:    auth.CurrentVersion,
			CapturedAt: time.Now().UTC(),
			LoginURL:   loginURL,
			FinalURL:   finalURL,
			RealmID:    auth.ExtractRealmID(finalURL),
			Cookies:    cookies,
			RelayURL:   relayURL,
		}
		tok.ClassifyCookies()
		if err := tok.ApplyATSCapture(cap); err != nil {
			fmt.Fprintf(stderr, "qb: %v\n", err)
			return &ExitError{Code: ExitAuthError, Err: err, Silent: flags.asJSON}
		}
		tok.ApplyIdentity(identity)
		tok.Source = "relay-session"
		return persistLogin(cmd, flags, tok)
	}
	// ATS intercept failed: refresh instead of failing. Fresh cookies plus
	// the stored ATS key resurrect the session (proven live 2026-09-08).
	fmt.Fprintf(stderr, "qb: ATS intercept unavailable (%v); refreshing stored session with live cookies...\n", capErr)
	return refreshStoredSession(cmd, flags, cookies, identity, cookieSource)
}

// relayTabCookies prefers the live tab CDP jar and reports its source.
func relayTabCookies(ctx context.Context, relayURL, tabID string) ([]auth.Cookie, string, error) {
	cookies, err := auth.FetchTabCookies(ctx, relayURL, tabID)
	if err != nil {
		return nil, "", err
	}
	return cookies, "relay-tab", nil
}

// refreshStoredSession merges a fresh cookie jar (+mined identity) into the
// stored session and persists when the result is usable. This is the relay
// fallback when ATS interception is unavailable: the stored ATS key plus
// live cookies is a working session. Fails only when nothing usable exists.
func refreshStoredSession(cmd *cobra.Command, flags *rootFlags, cookies []auth.Cookie, identity auth.Identity, source string) error {
	stderr := cmd.ErrOrStderr()
	tok, err := auth.Load()
	if err != nil {
		fmt.Fprintf(stderr, "qb: no stored session to refresh (run `qb login` in ego: %v)\n", err)
		return &ExitError{Code: ExitAuthError, Err: fmt.Errorf("no stored session: %w", err), Silent: flags.asJSON}
	}
	tok.MergeCookies(cookies)
	tok.ApplyIdentity(identity)
	tok.CapturedAt = time.Now().UTC()
	if tok.Source == "" {
		tok.Source = source
	}
	if !tok.HasUsableCredential() {
		fmt.Fprintln(stderr, "qb: refreshed jar still has no ATS key; full login required.")
		return &ExitError{Code: ExitAuthError, Err: auth.ErrNoATSAuthorization, Silent: flags.asJSON}
	}
	fmt.Fprintln(stderr, "Session refreshed from live tab cookies; kept stored ATS key.")
	return persistLogin(cmd, flags, tok)
}

// runLoginFromMitm handles `qb login --from-mitm PATH` (and the
// `qb auth remint --from-mitm PATH` alias). It replays the mitmproxy dump
// via auth.ImportMitmDump — which writes credentials.json directly — then
// loads the persisted TokenSet and prints its RedactedStatus. No Chrome is
// launched and no relay is probed, so a missing dump fails fast instead of
// entering the 10-minute interactive poll.
func runLoginFromMitm(cmd *cobra.Command, flags *rootFlags, lf *loginFlags) error {
	stderr := cmd.ErrOrStderr()
	stdout := cmd.OutOrStdout()

	if err := auth.ImportMitmDump(lf.fromMitm); err != nil {
		if errors.Is(err, auth.ErrMitmProxyMissing) {
			fmt.Fprintln(stderr, "qb: mitmproxy not found; install it to import a dump:")
			fmt.Fprintln(stderr, "  brew install mitmproxy")
		} else {
			fmt.Fprintf(stderr, "qb: importing mitm dump %q failed: %v\n", lf.fromMitm, err)
		}
		return &ExitError{Code: ExitAuthError, Err: fmt.Errorf("mitm import: %w", err), Silent: flags.asJSON}
	}

	tok, err := auth.Load()
	if err != nil {
		fmt.Fprintf(stderr, "qb: credentials written but could not be reloaded: %v\n", err)
		return &ExitError{Code: ExitAuthError, Err: fmt.Errorf("reloading imported credentials: %w", err), Silent: flags.asJSON}
	}

	status := tok.RedactedStatus()
	status.OK = tok.HasUsableCredential()
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(status)
	} else {
		printStatus(stdout, stderr, flags, status)
		fmt.Fprintf(stderr, "Imported %d cookie(s) to %s\n", status.CookieCount, auth.CredPath())
	}
	return nil
}

// resolveRelayURL applies the --relay-url > $QB_RELAY_URL > default cascade.
func resolveRelayURL(flag string) string {
	if flag != "" {
		return strings.TrimRight(flag, "/")
	}
	if v := os.Getenv("QB_RELAY_URL"); v != "" {
		return strings.TrimRight(v, "/")
	}
	return auth.DefaultRelayURL
}

// --- Relay probe / polling ---

// relayVersion is the subset of /json/version we need (presence only).
type relayVersion struct {
	Browser string `json:"Browser"`
}

// relayTab is the subset of a /json/list entry we read. The relay rejects
// Target.* CDP, so we only ever read tab metadata over HTTP — no WebSocket,
// no Target domain.
type relayTab struct {
	ID    string `json:"id"`
	URL   string `json:"url"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// probeRelay does GET {relay}/json/version. A reachable relay returns 200;
// anything else (connection refused, timeout, non-200) is an error.
func probeRelay(ctx context.Context, relayURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, relayURL+"/json/version", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("relay /json/version returned %s", resp.Status)
	}
	var v relayVersion
	_ = json.NewDecoder(resp.Body).Decode(&v)
	return nil
}

// listTabs fetches /json/list and returns page-type tabs.
func listTabs(ctx context.Context, relayURL string) ([]relayTab, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, relayURL+"/json/list", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("relay /json/list returned %s", resp.Status)
	}
	var tabs []relayTab
	if err := json.NewDecoder(resp.Body).Decode(&tabs); err != nil {
		return nil, fmt.Errorf("decoding /json/list: %w", err)
	}
	return tabs, nil
}

// findAuthTab returns the first page tab on an authenticated QBO URL.
func findAuthTab(tabs []relayTab) *relayTab {
	for i := range tabs {
		t := &tabs[i]
		if t.Type != "" && t.Type != "page" {
			continue
		}
		if auth.AuthenticatedURL(t.URL) {
			return t
		}
	}
	return nil
}

// findTabOnURL returns the first page tab whose URL contains needle (used to
// detect a tab already showing the sign-in page).
func findTabOnURL(tabs []relayTab, needle string) *relayTab {
	if needle == "" {
		return nil
	}
	for i := range tabs {
		t := &tabs[i]
		if t.Type != "" && t.Type != "page" {
			continue
		}
		if strings.Contains(t.URL, needle) {
			return t
		}
	}
	return nil
}

// waitForAuthTab polls /json/list every second until an authenticated tab
// appears or the context deadline fires.
func waitForAuthTab(ctx context.Context, relayURL string) (*relayTab, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		tabs, err := listTabs(ctx, relayURL)
		if err != nil {
			// Transient relay blips are tolerated until the deadline.
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
		} else if t := findAuthTab(tabs); t != nil {
			return t, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// openLoginURL launches the user's Chrome at the Intuit sign-in URL via the
// macOS `open` helper. It does not spawn a second user-data-dir.
func openLoginURL(stderr io.Writer, loginURL string) error {
	if runtime.GOOS != "darwin" {
		fmt.Fprintf(stderr, "qb: --no-open not set and `open` is macOS-only; open this URL manually:\n  %s\n", loginURL)
		return nil
	}
	if _, err := exec.LookPath("open"); err != nil {
		fmt.Fprintf(stderr, "qb: `open` not on PATH; open this URL manually:\n  %s\n", loginURL)
		return nil
	}
	return exec.Command("open", loginURL).Start()
}

// --- Cookie capture ---

// captureCookies delegates to auth.CaptureCookies, which prefers
// `press-auth cookies <domain>` on PATH and falls back to reading Chrome's
// on-disk cookie store for the Intuit domains. The returned source labels
// which mechanism supplied the cookies; when the relay already confirmed an
// authenticated tab the caller overrides this with "relay-session".
func captureCookies() ([]auth.Cookie, string, error) {
	cookies, err := auth.CaptureCookies()
	if err != nil {
		return nil, "", err
	}
	source := "chrome-cookies"
	if _, err := exec.LookPath("press-auth"); err == nil {
		source = "press-auth"
	}
	return cookies, source, nil
}
