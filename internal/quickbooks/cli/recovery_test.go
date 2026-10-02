package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/spf13/cobra"
)

func runRecoveryCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestRecoveryCredentialsStdinSupportsAutomationAndRedactsErrors(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	oldInput, oldEnroll := recoveryInput, enrollRecovery
	defer func() { recoveryInput = oldInput; enrollRecovery = oldEnroll }()
	recoveryInput = func(context.Context, *cobra.Command, string) (string, error) {
		t.Fatal("stdin mode prompted")
		return "", nil
	}
	called := false
	enrollRecovery = func(_ context.Context, secrets auth.RecoverySecrets, opts auth.RecoveryOptions) error {
		called = true
		if secrets.Username != "fixture-user" || secrets.Password != "fixture-private" || secrets.TOTP != "fixture-seed" || !opts.Bootstrap || !opts.TestProfile {
			t.Fatal("stdin fields/options lost")
		}
		return errors.New("fixture enrollment outcome")
	}
	root := NewRootCommand()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(`{"username":"fixture-user","password":"fixture-private","totp":"fixture-seed"}`))
	root.SetArgs([]string{"auth", "enroll", "--launch", "--enable-recovery", "--credentials-stdin", "--bootstrap", "--test-profile", "--json", "--no-input"})
	if err := root.Execute(); err == nil || !called {
		t.Fatal("automated enrollment did not execute")
	}
	var result map[string]any
	if json.Unmarshal(out.Bytes(), &result) != nil {
		t.Fatal("machine-mode output was not JSON")
	}
	for _, secret := range []string{"fixture-user", "fixture-private", "fixture-seed"} {
		if strings.Contains(out.String(), secret) {
			t.Fatal("stdin secret leaked")
		}
	}
	if _, err := readRecoveryCredentials(context.Background(), strings.NewReader(`{"username":"fixture","password":"fixture-private","unexpected-secret":"fixture-private"}`)); err == nil || strings.Contains(err.Error(), "fixture-private") {
		t.Fatal("decoder error leaked or accepted unknown fields")
	}
}

func TestRecoveryStdinRequiresOneBoundedObject(t *testing.T) {
	valid := `{"username":"fixture","password":"fixture-private"}`
	for _, input := range []string{valid + ` {"password":"fixture-private"}`, valid + " trailing-private", valid + strings.Repeat(" ", 17000)} {
		if _, err := readRecoveryCredentials(context.Background(), strings.NewReader(input)); err == nil || strings.Contains(err.Error(), "fixture-private") {
			t.Fatal("accepted trailing/oversized stdin or disclosed its content")
		}
	}
	if _, err := readRecoveryCredentials(context.Background(), strings.NewReader(valid+"\n")); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryEnrollmentDefaultAndMachineModesDoNotPrompt(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	old := recoveryInput
	defer func() { recoveryInput = old }()
	recoveryInput = func(context.Context, *cobra.Command, string) (string, error) {
		t.Fatal("machine/default path prompted for secrets")
		return "", nil
	}
	for _, args := range [][]string{
		{"auth", "enroll", "--json"},
		{"auth", "enroll", "--launch", "--enable-recovery", "--json"},
		{"auth", "enroll", "--launch", "--enable-recovery", "--no-input", "--json"},
		{"auth", "enroll", "--launch", "--enable-recovery", "--dry-run", "--json"},
	} {
		out, _ := runRecoveryCLI(t, args...)
		var result map[string]any
		if json.Unmarshal([]byte(out), &result) != nil {
			t.Fatal("machine output was not structured JSON")
		}
	}
}

func TestRecoveryEnrollmentHiddenInputAndNoSecretsInOutput(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	oldInput, oldEnroll := recoveryInput, enrollRecovery
	defer func() { recoveryInput = oldInput; enrollRecovery = oldEnroll }()
	values := []string{"synthetic-user", "synthetic-password", "synthetic-seed", "ENROLL"}
	i := 0
	recoveryInput = func(context.Context, *cobra.Command, string) (string, error) { v := values[i]; i++; return v, nil }
	called := false
	enrollRecovery = func(_ context.Context, secrets auth.RecoverySecrets, opts auth.RecoveryOptions) error {
		called = true
		if secrets.Username != values[0] || secrets.Password != values[1] || secrets.TOTP != values[2] || opts.KeyFile != "/private/key" {
			t.Fatal("hidden input or provider not passed to enrollment")
		}
		return nil
	}
	out, err := runRecoveryCLI(t, "auth", "enroll", "--launch", "--enable-recovery", "--key-file", "/private/key")
	if err != nil || !called || i != 4 {
		t.Fatal("enrollment not executed")
	}
	for _, secret := range values[:3] {
		if strings.Contains(out, secret) {
			t.Fatal("secret leaked to output")
		}
	}
}

func TestRecoveryEnrollmentHarnessBlocksBeforePrompt(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	t.Setenv("PRINTING_PRESS_DOGFOOD", "1")
	old := recoveryInput
	defer func() { recoveryInput = old }()
	recoveryInput = func(context.Context, *cobra.Command, string) (string, error) {
		t.Fatal("harness prompted")
		return "", nil
	}
	out, err := runRecoveryCLI(t, "auth", "enroll", "--launch", "--enable-recovery", "--json")
	if err == nil || !strings.Contains(out, "refused") {
		t.Fatal("harness allowed enrollment")
	}
}
