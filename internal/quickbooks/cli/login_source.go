package cli

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/spf13/cobra"
)

func addSourceFlags(cmd *cobra.Command, lf *loginFlags) {
	cmd.Flags().StringVar(&lf.source, "source", "auto", "session source: auto, relay, ego, chrome, managed (explicit sources never fall back)")
	cmd.Flags().StringVar(&lf.cdpURL, "cdp-url", "", "Chrome HTTP CDP discovery endpoint, e.g. http://127.0.0.1:9222")
	cmd.Flags().StringVar(&lf.egoSpace, "ego-space", "qb-gql", "existing Ego space for --source ego; never automatically claimed")
	cmd.Flags().StringVar(&lf.targetID, "target-id", "", "existing banking tab target ID (or Ego label); tab will be reloaded")
}

func runSourceLogin(cmd *cobra.Command, flags *rootFlags, lf *loginFlags) error {
	if lf.fromMitm != "" || lf.noOpen {
		return fmt.Errorf("--source cannot be combined with --from-mitm or --no-open")
	}
	timeout := flags.timeout
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
	defer cancel()
	keep, _ := cmd.Flags().GetBool("keep-alive")
	ctx = auth.WithRetainedBrowser(ctx, keep)
	var cap *auth.ATSCapture
	var err error
	endpoint := ""
	switch lf.source {
	case "ego":
		ctx = auth.WithExistingEgo(ctx, lf.egoSpace, lf.targetID)
		cap, err = captureATSEgo(ctx, auth.BankingCaptureURL, auth.BankingCaptureURL)
	case "managed":
		var tok *auth.TokenSet
		tok, err = runLoginManaged(ctx, cmd.ErrOrStderr(), lf.loginURL)
		if err == nil {
			return persistLogin(cmd, flags, tok)
		}
	case "relay", "chrome":
		endpoint = resolveRelayURL(flags.relayURL)
		if lf.source == "chrome" {
			endpoint = lf.cdpURL
		}
		endpoint = strings.TrimRight(endpoint, "/")
		if err = validateCaptureEndpoint(endpoint); err != nil {
			return err
		}
		var tabs []relayTab
		tabs, err = listTabs(ctx, endpoint)
		if err != nil {
			break
		}
		var selected []relayTab
		for _, tab := range tabs {
			u, parseErr := url.Parse(tab.URL)
			if parseErr == nil && auth.AuthenticatedURL(tab.URL) && u.Path == "/app/banking" &&
				(lf.targetID == "" || tab.ID == lf.targetID) {
				selected = append(selected, tab)
			}
		}
		if len(selected) != 1 {
			return fmt.Errorf("select exactly one existing QBO banking tab using --target-id; found %d", len(selected))
		}
		before, identityErr := fetchTabIdentity(ctx, endpoint, selected[0].ID)
		if identityErr != nil || before.Realm == "" || before.Email == "" {
			return fmt.Errorf("existing tab lacks a verifiable company and principal")
		}
		cap, err = captureATS(ctx, endpoint, selected[0].ID, auth.BankingCaptureURL)
		if err == nil && (cap == nil || before.Realm != cap.Identity.Realm || !strings.EqualFold(strings.TrimSpace(before.Email), strings.TrimSpace(cap.Identity.Email))) {
			err = fmt.Errorf("company or principal changed during capture; credentials unchanged")
		}
	default:
		return fmt.Errorf("unknown session source %q", lf.source)
	}
	if err != nil {
		return &ExitError{Code: ExitAuthError, Err: err}
	}
	tok := &auth.TokenSet{Version: auth.CurrentVersion, CapturedAt: time.Now().UTC(), Source: lf.source + "-existing", RelayURL: endpoint, FinalURL: auth.BankingCaptureURL}
	if err := tok.ApplyATSCapture(cap); err != nil {
		return err
	}
	tok.ClassifyCookies()
	return persistLogin(cmd, flags, tok)
}

func validateCaptureEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid CDP discovery endpoint")
	}
	ip := net.ParseIP(u.Hostname())
	localHTTP := u.Scheme == "http" && (u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback()))
	if u.Scheme != "https" && !localHTTP {
		return fmt.Errorf("CDP discovery requires HTTPS or loopback HTTP")
	}
	return nil
}
