package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// managedHeadlessEnv flips the managed login window off (CI). Production
// logins run headed: the user signs in normally in a visible window.
const managedHeadlessEnv = "QB_MANAGED_HEADLESS"

// ManagedProfileDir is the persistent Chromium profile for QBO login,
// scoped under the qb home so QB_HOME isolation (tests, multi-tenant)
// applies. The directory survives logins: a still-valid session signs
// straight in with no clicks, and all browser state (cookies,
// localStorage, IndexedDB) persists — not just the exported jar.
func ManagedProfileDir() string {
	return filepath.Join(HomeDir(), "profiles", "qbo")
}

// CaptureManaged launches a managed Chromium window on the persistent
// profile, waits for the user to complete Intuit sign-in, intercepts one
// ATS Intuit_APIKey request, mines identity from the hydrated DOM, and
// returns headers+cookies+identity. Secrets never touch logs; the profile
// dir is created 0700 and never deleted.
//
// A cookie jar alone cannot authenticate QBO API calls (cookie-only
// sessions 401): the ATS Authorization header is mandatory, which is why
// the profile must still yield one intercepted request rather than being
// treated as the credential itself.
func CaptureManaged(ctx context.Context, loginURL, bankingURL string) (*ATSCapture, error) {
	return captureManaged(ctx, loginURL, bankingURL, os.Getenv(managedHeadlessEnv) == "1")
}

// captureManagedHeadless runs the refresh variant: headless, straight to
// the banking page (a warm profile loads authenticated; a cold one fails
// at the auth wait instead of blocking for a sign-in that never comes).
func captureManagedHeadless(ctx context.Context, bankingURL string) (*ATSCapture, error) {
	return captureManaged(ctx, bankingURL, bankingURL, true)
}

func captureManaged(ctx context.Context, loginURL, bankingURL string, headless bool) (*ATSCapture, error) {
	if loginURL == "" {
		loginURL = DefaultLoginURL
	}
	if bankingURL == "" {
		bankingURL = BankingCaptureURL
	}
	profile := ManagedProfileDir()
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return nil, fmt.Errorf("creating managed profile: %w", err)
	}

	allocOpts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	allocOpts = append(allocOpts,
		chromedp.UserDataDir(profile),
		chromedp.Flag("headless", headless),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	if err := chromedp.Run(browserCtx, chromedp.Navigate(loginURL)); err != nil {
		return nil, fmt.Errorf("managed navigate: %w", err)
	}
	if err := waitManagedAuth(browserCtx); err != nil {
		return nil, err
	}

	headers, err := interceptManagedATS(browserCtx, bankingURL)
	if err != nil {
		return nil, err
	}
	var identity Identity
	if err := chromedp.Run(browserCtx, chromedp.Evaluate(IdentityJS, &identity)); err != nil {
		identity = Identity{}
	}
	cookies, err := snapshotManagedCookies(browserCtx)
	if err != nil {
		return nil, fmt.Errorf("managed cookie snapshot: %w", err)
	}
	return &ATSCapture{Headers: headers, Cookies: cookies, Identity: identity}, nil
}

// waitManagedAuth polls location.href until the tab leaves the sign-in wall
// for a post-login app URL. Transient evaluation errors during navigation
// read as not-yet; the ctx deadline bounds the whole wait.
func waitManagedAuth(ctx context.Context) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("managed login did not complete: %w", ctx.Err())
		case <-ticker.C:
			var href string
			if err := chromedp.Run(ctx, chromedp.Evaluate(`location.href`, &href)); err != nil {
				continue
			}
			if AuthenticatedURL(href) {
				return nil
			}
		}
	}
}

// interceptManagedATS enables Fetch on ATS URLs, navigates to banking, and
// returns the first Intuit_APIKey Authorization header map observed. Every
// paused request is continued so the page never hangs.
func interceptManagedATS(ctx context.Context, bankingURL string) (map[string]string, error) {
	pattern := "*://qbo.intuit.com/ats/*"
	stage := fetch.RequestStageRequest
	found := make(chan map[string]string, 1)
	chromedp.ListenTarget(ctx, func(ev any) {
		paused, ok := ev.(*fetch.EventRequestPaused)
		if !ok || paused.Request == nil {
			return
		}
		go func() {
			_ = fetch.ContinueRequest(paused.RequestID).Do(ctx)
		}()
		if !strings.Contains(paused.Request.URL, "/ats/v1/") {
			return
		}
		for name, value := range paused.Request.Headers {
			if strings.EqualFold(name, "authorization") && isIntuitAPIKey(fmt.Sprint(value)) {
				select {
				case found <- headerMap(paused.Request.Headers):
				default:
				}
				return
			}
		}
	})
	enable := chromedp.ActionFunc(func(ctx context.Context) error {
		return fetch.Enable().WithPatterns([]*fetch.RequestPattern{{
			URLPattern:   pattern,
			RequestStage: stage,
		}}).Do(ctx)
	})
	if err := chromedp.Run(ctx, enable, chromedp.Navigate(bankingURL)); err != nil {
		return nil, fmt.Errorf("managed banking navigate: %w", err)
	}
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("%w: managed ATS intercept timed out", ErrNoATSAuthorization)
	case headers := <-found:
		return headers, nil
	}
}

// headerMap flattens CDP header storage into the plain map the capture
// pipeline consumes. Values stringify; Cookie is dropped by the caller.
func headerMap(in map[string]any) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = fmt.Sprint(v)
	}
	return out
}

// snapshotManagedCookies dumps the whole managed browser jar and keeps the
// Intuit/QuickBooks entries (login spans subdomains).
func snapshotManagedCookies(ctx context.Context) ([]Cookie, error) {
	var raw []*network.Cookie
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		got, err := storage.GetCookies().Do(ctx)
		if err != nil {
			return err
		}
		raw = got
		return nil
	}))
	if err != nil {
		return nil, err
	}
	var out []Cookie
	for _, c := range raw {
		d := strings.ToLower(c.Domain)
		if !strings.Contains(d, "intuit") && !strings.Contains(d, "quickbooks") {
			continue
		}
		path := c.Path
		if path == "" {
			path = "/"
		}
		out = append(out, Cookie{
			Name: c.Name, Value: c.Value, Domain: c.Domain,
			Path:     path,
			Expires:  time.Unix(int64(c.Expires), 0).UTC(),
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
		})
	}
	return out, nil
}
