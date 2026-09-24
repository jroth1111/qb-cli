package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/spf13/cobra"
)

// newAuthCmd builds the `qb auth` parent command with login/status/logout
// children. `qb login` is a top-level alias of `qb auth login`.
func newAuthCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage QuickBooks credentials",
	}
	cmd.AddCommand(newAuthLoginCmd(flags))
	cmd.AddCommand(newAuthRemintCmd(flags))
	cmd.AddCommand(newAuthStatusCmd(flags))
	cmd.AddCommand(newAuthWhoamiCmd(flags))
	cmd.AddCommand(newAuthLogoutCmd(flags))
	cmd.AddCommand(newAuthKeepaliveCmd(flags))
	return cmd
}

// fetchTabIdentity evaluates the live tab DOM via relay CDP. Tests replace
// it so whoami is exercised without Chrome.
var fetchTabIdentity = auth.FetchTabIdentity

// newAuthWhoamiCmd implements `qb auth whoami`: report the company actually
// signed in right now. It reads identity from the live relay tab (the
// session, not the file), then compares against stored credentials so a
// company switch is impossible to miss. No secrets are printed.
func newAuthWhoamiCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the company signed in live (and how stored creds compare)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runWhoami(cmd, flags)
		},
	}
	return cmd
}

// runWhoami resolves the live tab identity first and stored credentials
// second. Exit codes: 0 with a live answer; non-zero when no live tab
// exists and nothing usable is stored.
func runWhoami(cmd *cobra.Command, flags *rootFlags) error {
	stdout := cmd.OutOrStdout()
	ctx := cmd.Context()
	stored, _ := auth.Load()
	relayURL := resolveRelayURL(flags.relayURL)
	live, liveErr := liveTabIdentity(ctx, relayURL)
	if liveErr == nil {
		printWhoami(stdout, flags, live, stored)
		return nil
	}
	if stored != nil && stored.CompanyName != "" {
		if flags.asJSON {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(map[string]any{
				"ok": false, "live": false,
				"company": stored.CompanyName, "email": stored.Email, "realm": stored.RealmID,
			})
			return nil
		}
		fmt.Fprintf(stdout, "No live QBO tab (%v).\n", liveErr)
		fmt.Fprintf(stdout, "Stored session: %s (%s, realm %s) — run `qb login` to refresh.\n",
			stored.CompanyName, stored.Email, stored.RealmID)
		return &ExitError{Code: ExitAuthError, Err: errors.New("no live QBO tab"), Silent: flags.asJSON}
	}
	return &ExitError{Code: ExitAuthError, Err: fmt.Errorf("no live QBO tab: %w", liveErr), Silent: flags.asJSON}
}

// liveTabIdentity attaches to the first authenticated relay tab and mines
// the signed-in identity from its DOM.
func liveTabIdentity(ctx context.Context, relayURL string) (auth.Identity, error) {
	tabs, err := listTabs(ctx, relayURL)
	if err != nil {
		return auth.Identity{}, err
	}
	tab := findAuthTab(tabs)
	if tab == nil {
		return auth.Identity{}, errors.New("no authenticated qbo.intuit.com tab on relay")
	}
	return fetchTabIdentity(ctx, relayURL, tab.ID)
}

// printWhoami reports the live identity first, then how stored credentials
// compare. A company switch between stored and live is flagged loudly: it
// is exactly the state where running a command would hit an unexpected
// company.
func printWhoami(stdout io.Writer, flags *rootFlags, live auth.Identity, stored *auth.TokenSet) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"ok": true, "live": true,
			"company": live.Company, "email": live.Email, "realm": live.Realm,
		})
		return
	}
	fmt.Fprintf(stdout, "Signed in live as %s (%s, realm %s).\n", live.Company, live.Email, live.Realm)
	if stored == nil || stored.RealmID == "" {
		fmt.Fprintln(stdout, "No stored session to compare.")
		return
	}
	if stored.RealmID != live.Realm {
		fmt.Fprintf(stdout, "WARNING: stored session is %s (realm %s) — run `qb login` before mutating.\n",
			stored.CompanyName, stored.RealmID)
		return
	}
	fmt.Fprintln(stdout, "Stored session matches.")
}
func newAuthRemintCmd(flags *rootFlags) *cobra.Command {
	lf := &loginFlags{}
	cmd := &cobra.Command{
		Use:   "remint",
		Short: "Recapture ATS Intuit_APIKey (same ladder as login)",
		Long: `Recapture the QBO ATS Authorization header.

Same ladder as ` + "`qb login`" + `: open QBO tab on the OMP relay, else managed
persistent-profile Chromium, else ego Space.
` + "`--from-mitm PATH`" + ` imports a dump. ` + "`--no-open`" + ` is relay-only.

401s auto-remint through the same ladder (headless where possible).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLogin(cmd, flags, lf)
		},
	}
	cmd.Flags().StringVar(&lf.loginURL, "login-url", auth.DefaultLoginURL,
		"QBO sign-in URL")
	cmd.Flags().BoolVar(&lf.noOpen, "no-open", false,
		"relay only: wait for an already-open QBO tab (no ego Space)")
	cmd.Flags().StringVar(&lf.fromMitm, "from-mitm", "",
		"optional mitmproxy dump instead of a live intercept")
	cmd.Flags().Bool("keep-alive", true, "automatically maintain the captured session; --keep-alive=false disables startup")
	addSourceFlags(cmd, lf)
	return cmd
}

// newAuthStatusCmd implements `qb auth status`: load the persisted TokenSet
// and print its RedactedStatus. Exits non-zero when credentials are missing
// or unusable so scripts can branch on the exit code.
func newAuthStatusCmd(flags *rootFlags) *cobra.Command {
	var live bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show saved QuickBooks credential status (no secrets)",
		RunE: func(cmd *cobra.Command, args []string) error {
			tok, err := auth.Load()
			if err != nil {
				if errors.Is(err, auth.ErrNoCredentials) {
					printStatusMissing(cmd, flags)
					return &ExitError{Code: ExitAuthError, Err: err, Silent: flags.asJSON}
				}
				return &ExitError{Code: ExitAuthError, Err: fmt.Errorf("loading credentials: %w", err)}
			}
			status := tok.RedactedStatus()
			status.OK = tok.HasUsableCredential()
			if !status.OK {
				printStatus(cmd.OutOrStdout(), cmd.ErrOrStderr(), flags, status)
				return &ExitError{
					Code:   ExitAuthError,
					Err:    errors.New("saved credentials are present but not usable"),
					Silent: flags.asJSON,
				}
			}
			if live {
				return runStatusLive(cmd, flags, tok, status)
			}
			printStatus(cmd.OutOrStdout(), cmd.ErrOrStderr(), flags, status)
			return nil
		},
	}
	cmd.Flags().BoolVar(&live, "live", false,
		"verify the session against QBO instead of trusting file timestamps")
	return cmd
}

// runStatusLive proves the saved session against the v3 query channel the
// CLI actually uses (ATS headers — not the homepage shell, which redirects
// to sign-in even for API-healthy jars). A stale jar reads STALE with a
// non-zero exit so scripts can branch to `qb login`; transport failures
// are reported distinctly from proof-of-death.
func runStatusLive(cmd *cobra.Command, flags *rootFlags, tok *auth.TokenSet, status auth.Status) error {
	stdout := cmd.OutOrStdout()
	if err := auth.PingV3(cmd.Context(), tok); err != nil {
		printStatus(stdout, cmd.ErrOrStderr(), flags, status)
		if errors.Is(err, auth.ErrSessionStale) {
			fmt.Fprintln(stdout, "  liveness: STALE — run `qb login` to sign in again")
			return &ExitError{
				Code:   ExitAuthError,
				Err:    err,
				Silent: flags.asJSON,
			}
		}
		fmt.Fprintln(stdout, "  liveness: unknown (could not reach QBO)")
		return &ExitError{
			Code:   ExitAuthError,
			Err:    fmt.Errorf("liveness check failed: %w", err),
			Silent: flags.asJSON,
		}
	}
	printStatus(stdout, cmd.ErrOrStderr(), flags, status)
	fmt.Fprintln(stdout, "  liveness: live (verified against QBO)")
	return nil
}

// newAuthLogoutCmd implements `qb auth logout`: delete the persisted
// credentials file. Idempotent — a missing file is success.
func newAuthLogoutCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Delete saved QuickBooks credentials",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := auth.Delete(); err != nil {
				return &ExitError{Code: ExitAuthError, Err: fmt.Errorf("deleting credentials: %w", err)}
			}
			_ = stopKeeper()
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(map[string]any{"ok": true, "action": "logout"})
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "Credentials removed.")
			}
			return nil
		},
	}
	return cmd
}

// printStatus writes the secret-free Status to the appropriate stream. In
// --json mode the Status object is the only thing on stdout (instructions
// belong on stderr). In text mode a human-readable summary goes to stdout.
func printStatus(stdout, stderr io.Writer, flags *rootFlags, s auth.Status) {
	if flags.asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(s)
		return
	}
	if !s.OK {
		fmt.Fprintln(stdout, "No usable QuickBooks credentials.")
		return
	}
	fmt.Fprintf(stdout, "QuickBooks session: %s\n", sourceLabel(s.Source))
	if s.CompanyName != "" {
		fmt.Fprintf(stdout, "  company: %s\n", s.CompanyName)
	}
	if s.Email != "" {
		fmt.Fprintf(stdout, "  email:   %s\n", s.Email)
	}
	if s.RealmID != "" {
		fmt.Fprintf(stdout, "  realm:   %s\n", s.RealmID)
	}
	fmt.Fprintf(stdout, "  cookies: %d\n", s.CookieCount)
	if s.HasRefreshToken {
		fmt.Fprintln(stdout, "  refresh: yes")
	} else if s.HasAccessToken {
		fmt.Fprintln(stdout, "  access:  yes (no refresh token)")
	}
	if !s.LongestExpiry.IsZero() {
		fmt.Fprintf(stdout, "  expires: %s\n", s.LongestExpiry.Format("2006-01-02 15:04 MST"))
	}
}

// printStatusMissing reports the no-credentials state without leaking that
// the file vs. parse path failed.
func printStatusMissing(cmd *cobra.Command, flags *rootFlags) {
	if flags.asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		_ = enc.Encode(auth.Status{OK: false})
		return
	}
	fmt.Fprintln(cmd.OutOrStdout(), "No saved QuickBooks credentials. Run `qb login` to sign in.")
}

func sourceLabel(s string) string {
	switch s {
	case "relay-session":
		return "captured from Chrome relay session"
	case "press-auth":
		return "captured via press-auth"
	case "chrome-cookies":
		return "captured from Chrome cookie store"
	default:
		if s == "" {
			return "captured"
		}
		return s
	}
}
