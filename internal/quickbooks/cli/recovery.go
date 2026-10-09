package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var enrollRecovery = auth.EnrollLoginRecovery
var recoveryInput = promptRecoveryInput

func promptRecoveryInput(ctx context.Context, cmd *cobra.Command, prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) || !term.IsTerminal(int(os.Stderr.Fd())) {
		return "", errors.New("use --credentials-stdin for non-interactive enrollment")
	}
	state, err := term.GetState(fd)
	if err != nil {
		return "", errors.New("could not prepare hidden terminal input")
	}
	defer func() { _ = term.Restore(fd, state) }()
	fmt.Fprint(cmd.ErrOrStderr(), prompt)
	type result struct {
		value []byte
		err   error
	}
	done := make(chan result, 1)
	go func() { b, err := term.ReadPassword(fd); done <- result{b, err} }()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case r := <-done:
		fmt.Fprintln(cmd.ErrOrStderr())
		if r.err != nil {
			clear(r.value)
			return "", errors.New("hidden terminal input failed")
		}
		value := string(r.value)
		clear(r.value)
		return value, nil
	}
}

func readRecoveryCredentials(ctx context.Context, input io.Reader) (auth.RecoverySecrets, error) {
	type result struct {
		secrets auth.RecoverySecrets
		err     error
	}
	done := make(chan result, 1)
	go func() {
		limited := &io.LimitedReader{R: input, N: 16385}
		decoder := json.NewDecoder(limited)
		decoder.DisallowUnknownFields()
		var secrets auth.RecoverySecrets
		err := decoder.Decode(&secrets)
		if err == nil {
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				err = errors.New("credentials stdin must contain one JSON object")
			}
		}
		if err != nil || limited.N == 0 || secrets.Username == "" || secrets.Password == "" {
			err = errors.New("credentials stdin must contain a JSON object with username, password and optional totp")
		}
		done <- result{secrets, err}
	}()
	select {
	case <-ctx.Done():
		return auth.RecoverySecrets{}, ctx.Err()
	case r := <-done:
		return r.secrets, r.err
	}
}

func recoveryResult(cmd *cobra.Command, flags *rootFlags, value any, err error) error {
	cmd.SilenceErrors = flags.asJSON
	if flags.asJSON {
		if err != nil {
			value = map[string]any{"ok": false, "error": err.Error()}
			var failure *auth.LoginFailure
			if errors.As(err, &failure) {
				value.(map[string]any)["diagnostics"] = failure.Evidence
			}
			var selection *auth.CompanySelectionRequired
			if errors.As(err, &selection) {
				value.(map[string]any)["companies"] = selection.Companies
				value.(map[string]any)["requires_company_selection"] = true
			}
		}
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(value)
	} else if err == nil {
		_ = json.NewEncoder(cmd.OutOrStdout()).Encode(value)
	}
	if err != nil {
		return &ExitError{Code: ExitAuthError, Err: err, Silent: flags.asJSON}
	}
	return nil
}

func newAuthEnrollCmd(flags *rootFlags) *cobra.Command {
	var launch, enabled, credentialsStdin bool
	var opts auth.RecoveryOptions
	cmd := &cobra.Command{
		Use: "enroll", Short: "Enroll hidden username/password/TOTP for autonomous login recovery",
		Long:        "Enroll username/password/TOTP for automatic sign-in.\n\nDefault: print instructions only. --launch --enable-recovery permits browser verification\nand future unattended sign-ins. Hidden terminal prompts are the default.\n--credentials-stdin accepts a JSON object with username, password and optional totp,\nincluding from automation; --json and --no-input are supported with that explicit option.\nSecrets never appear in command arguments or output. --bootstrap performs the initial\ncredential login in a fresh QB_HOME, so headless setup does not require a manual login.\n\nDefault key storage: macOS Keychain / Linux Secret Service. --gpg-recipient works on both\nplatforms with an accessible GPG key. --key-file supports unattended services after restart.\nNo plaintext fallback. Enrollment does not change account security settings.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{"mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if auth.IsHarness() {
				return recoveryResult(cmd, flags, nil, errors.New("enrollment is refused under verification harnesses"))
			}
			if !launch || !enabled || flags.dryRun {
				return recoveryResult(cmd, flags, map[string]any{"ok": true, "enrolled": false, "next_command": "qb auth enroll --launch --enable-recovery", "requires": "managed login or --bootstrap; terminal prompts or --credentials-stdin; available encryption key"}, nil)
			}
			if (flags.noInput || flags.asJSON) && !credentialsStdin {
				return recoveryResult(cmd, flags, nil, errors.New("use --credentials-stdin for automated or machine-mode enrollment"))
			}
			if opts.KeyFile != "" && opts.GPGRecipient != "" {
				return recoveryResult(cmd, flags, nil, errors.New("choose --key-file or --gpg-recipient, not both"))
			}
			timeout := flags.timeout
			if timeout <= 0 {
				timeout = 2 * time.Minute
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()
			var secrets auth.RecoverySecrets
			if credentialsStdin {
				var err error
				secrets, err = readRecoveryCredentials(ctx, cmd.InOrStdin())
				if err != nil {
					return recoveryResult(cmd, flags, nil, err)
				}
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), "This machine will be able to sign in using your password and authenticator seed without further approval. Unexpected verification challenges stop recovery. Financial writes are never automatically replayed.")
				username, err := recoveryInput(ctx, cmd, "Intuit username/email (hidden): ")
				if err != nil {
					return recoveryResult(cmd, flags, nil, err)
				}
				password, err := recoveryInput(ctx, cmd, "Intuit password (hidden): ")
				if err != nil {
					return recoveryResult(cmd, flags, nil, err)
				}
				seed, err := recoveryInput(ctx, cmd, "Authenticator BASE32 seed or otpauth URI (hidden; Enter if none): ")
				if err != nil {
					return recoveryResult(cmd, flags, nil, err)
				}
				confirm, err := recoveryInput(ctx, cmd, "Type ENROLL to authorize autonomous recovery: ")
				if err != nil {
					return recoveryResult(cmd, flags, nil, err)
				}
				if confirm != "ENROLL" {
					return recoveryResult(cmd, flags, nil, errors.New("enrollment cancelled"))
				}
				secrets = auth.RecoverySecrets{Username: username, Password: password, TOTP: seed}
			}
			opts.ChooseCompany = func(ctx context.Context, companies []string) (string, error) {
				return chooseRecoveryCompany(ctx, cmd, flags, companies)
			}
			err := enrollRecovery(ctx, secrets, opts)
			secrets.Username, secrets.Password, secrets.TOTP = "", "", ""
			if err != nil {
				return recoveryResult(cmd, flags, nil, err)
			}
			status, err := auth.LoginRecoveryStatus()
			if err == nil {
				status.KeepAlive = autoStartKeeper(cmd, flags)
			}
			return recoveryResult(cmd, flags, status, err)
		},
	}
	cmd.Flags().BoolVar(&launch, "launch", false, "launch the owned browser to independently verify enrollment")
	cmd.Flags().BoolVar(&enabled, "enable-recovery", false, "authorize future autonomous password/TOTP sign-ins")
	cmd.Flags().BoolVar(&credentialsStdin, "credentials-stdin", false, "read username/password/totp JSON from stdin without interactive prompts")
	cmd.Flags().BoolVar(&opts.Bootstrap, "bootstrap", false, "perform the first login with supplied credentials in a fresh QB_HOME")
	cmd.Flags().BoolVar(&opts.TestProfile, "test-profile", false, "mark a bootstrapped isolated profile for controlled local-auth-loss verification")
	cmd.Flags().BoolVar(&opts.Headed, "headed", false, "use visible managed Chromium for enrollment and subsequent recovery")
	cmd.Flags().StringVar(&opts.Company, "company", "", "exact company name for first login; a single available company is selected automatically")
	cmd.Flags().Bool("keep-alive", true, "maintain the enrolled session; --keep-alive=false disables startup")
	cmd.Flags().StringVar(&opts.KeyFile, "key-file", "", "external private file containing a 32-byte encryption key (path only)")
	cmd.Flags().StringVar(&opts.GPGRecipient, "gpg-recipient", "", "full trusted GPG encryption-key fingerprint (macOS/Linux)")
	return cmd
}

func newAuthRecoveryCmd(flags *rootFlags) *cobra.Command {
	parent := &cobra.Command{Use: "recovery", Short: "Inspect or revoke autonomous login recovery"}
	parent.AddCommand(&cobra.Command{Use: "status", Short: "Show secret-free enrollment and recovery status", Args: cobra.NoArgs, Annotations: map[string]string{"mcp:read-only": "true"}, RunE: func(cmd *cobra.Command, _ []string) error {
		status, err := auth.LoginRecoveryStatus()
		return recoveryResult(cmd, flags, status, err)
	}})
	var rebindLaunch bool
	rebind := &cobra.Command{Use: "rebind", Short: "Rebind encrypted enrollment to a verified same-company/principal session", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !rebindLaunch || flags.dryRun {
			return recoveryResult(cmd, flags, map[string]any{"rebound": false, "requires": "--launch; current source must be signed in as the enrolled company and principal"}, nil)
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), flags.timeout)
		defer cancel()
		err := auth.RebindLoginRecovery(ctx)
		return recoveryResult(cmd, flags, map[string]any{"rebound": err == nil}, err)
	}}
	rebind.Flags().BoolVar(&rebindLaunch, "launch", false, "permit owned-source verification and encrypted re-binding; never changes company or principal")
	parent.AddCommand(rebind)
	var retryLaunch bool
	retry := &cobra.Command{Use: "retry", Short: "Request one bound recovery attempt after operator attention", Args: cobra.NoArgs, Annotations: map[string]string{"mcp:hidden": "true"}, RunE: func(cmd *cobra.Command, _ []string) error {
		if !retryLaunch || flags.dryRun {
			return recoveryResult(cmd, flags, map[string]any{"retried": false, "requires": "--launch; recovery must remain enabled and bound; cooldown and provider challenges remain enforced"}, nil)
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), flags.timeout)
		defer cancel()
		if err := auth.RetryLoginRecovery(ctx); err != nil {
			return recoveryResult(cmd, flags, nil, err)
		}
		status, err := auth.LoginRecoveryStatus()
		return recoveryResult(cmd, flags, status, err)
	}}
	retry.Flags().BoolVar(&retryLaunch, "launch", false, "permit one owned-source recovery attempt without changing disabled policy or identity")
	parent.AddCommand(retry)
	for _, forget := range []bool{false, true} {
		name := "disable"
		description := "Disable autonomous sign-in without deleting stored login secrets"
		if forget {
			name = "forget"
			description = "Disable recovery and remove its encrypted secrets and owned keyring entry"
		}
		parent.AddCommand(&cobra.Command{Use: name, Short: description, Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			if flags.dryRun {
				return recoveryResult(cmd, flags, map[string]any{"ok": true, "dry_run": true}, nil)
			}
			err := auth.DisableLoginRecovery(cmd.Context(), forget)
			return recoveryResult(cmd, flags, map[string]any{"ok": err == nil, "enabled": false, "forgotten": forget}, err)
		}})
	}
	var path string
	var write bool
	key := &cobra.Command{Use: "key-create", Short: "Provision a private recovery key file, never overwriting an existing file", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if auth.IsHarness() {
			return recoveryResult(cmd, flags, nil, auth.ErrRecoveryDisabled)
		}
		if !write || flags.dryRun {
			return recoveryResult(cmd, flags, map[string]any{"ok": true, "written": false, "requires": "absolute --path in an existing private directory outside QB_HOME; --write"}, nil)
		}
		err := auth.CreateRecoveryKeyFile(cmd.Context(), path)
		return recoveryResult(cmd, flags, map[string]any{"ok": err == nil, "written": err == nil}, err)
	}}
	key.Flags().StringVar(&path, "path", "", "absolute private key-file path outside QB_HOME")
	key.Flags().BoolVar(&write, "write", false, "explicitly create the key file")
	parent.AddCommand(key)
	var verifyLaunch, localLoss bool
	verify := &cobra.Command{Use: "verify", Short: "Empirically verify warm reuse or credential recovery on an isolated test profile", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !verifyLaunch || flags.dryRun {
			return recoveryResult(cmd, flags, map[string]any{"ok": true, "verified": false, "requires": "--launch; --local-auth-loss requires a bootstrapped --test-profile"}, nil)
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), flags.timeout)
		defer cancel()
		result, err := auth.VerifyLoginRecovery(ctx, localLoss)
		return recoveryResult(cmd, flags, result, err)
	}}
	verify.Flags().BoolVar(&verifyLaunch, "launch", false, "permit managed browser verification")
	verify.Flags().BoolVar(&localLoss, "local-auth-loss", false, "clear only the isolated test profile's local auth state, then verify autonomous recovery")
	parent.AddCommand(verify)
	return parent
}
