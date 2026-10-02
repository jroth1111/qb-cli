//go:build linux

package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"strings"

	"github.com/godbus/dbus/v5"
)

// Secret values use stdin/stdout, never command arguments or subprocess logs.
var runSecretTool = func(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "secret-tool", args...)
	cmd.Stdin = bytes.NewReader(input)
	return cmd.Output()
}

func nativeRecoveryKey(ctx context.Context, id string, create []byte) ([]byte, error) {
	if create != nil {
		args := []string{"store", "--label=qb login recovery", "application", "qb-login-recovery", "enrollment", id}
		_, err := runSecretTool(ctx, []byte(base64.StdEncoding.EncodeToString(create)), args...)
		if err != nil {
			return nil, ErrRecoveryKeyUnavailable
		}
		return create, nil
	}
	out, err := lookupUnlockedSecret(ctx, id)
	if err != nil {
		return nil, ErrRecoveryKeyUnavailable
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(out)))
	if err != nil || len(key) != 32 {
		return nil, ErrRecoveryKeyUnavailable
	}
	return key, nil
}

// SearchItems/GetSecrets never call Unlock or Prompt. Locked keyrings fail
// closed instead of asking for approval in an unattended recovery process.
var lookupUnlockedSecret = func(ctx context.Context, id string) ([]byte, error) {
	address := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if !strings.HasPrefix(address, "unix:") || strings.Contains(address, ";") {
		return nil, ErrRecoveryKeyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, ErrRecoveryKeyUnavailable
	}
	defer func() { _ = conn.Close() }()
	service := conn.Object("org.freedesktop.secrets", "/org/freedesktop/secrets")
	var unlocked, locked []dbus.ObjectPath
	err = service.CallWithContext(ctx, "org.freedesktop.Secret.Service.SearchItems", 0, map[string]string{"application": "qb-login-recovery", "enrollment": id}).Store(&unlocked, &locked)
	if err != nil || len(unlocked) != 1 || len(locked) != 0 {
		return nil, ErrRecoveryKeyUnavailable
	}
	var output dbus.Variant
	var session dbus.ObjectPath
	err = service.CallWithContext(ctx, "org.freedesktop.Secret.Service.OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&output, &session)
	if err != nil {
		return nil, ErrRecoveryKeyUnavailable
	}
	defer conn.Object("org.freedesktop.secrets", session).CallWithContext(ctx, "org.freedesktop.Secret.Session.Close", 0)
	type secret struct {
		Session     dbus.ObjectPath
		Parameters  []byte
		Value       []byte
		ContentType string
	}
	var values map[dbus.ObjectPath]secret
	err = service.CallWithContext(ctx, "org.freedesktop.Secret.Service.GetSecrets", 0, unlocked, session).Store(&values)
	if err != nil || len(values) != 1 {
		return nil, ErrRecoveryKeyUnavailable
	}
	return values[unlocked[0]].Value, nil
}

func deleteNativeRecoveryKey(ctx context.Context, id string) error {
	_, err := runSecretTool(ctx, nil, "clear", "application", "qb-login-recovery", "enrollment", id)
	if err != nil {
		return ErrRecoveryKeyUnavailable
	}
	return nil
}
