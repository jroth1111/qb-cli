//go:build linux

package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestLinuxNativeKeyDoesNotUseUnlockingLookupCLI(t *testing.T) {
	oldLookup, oldRun := lookupUnlockedSecret, runSecretTool
	defer func() { lookupUnlockedSecret = oldLookup; runSecretTool = oldRun }()
	want := bytes.Repeat([]byte{3}, 32)
	lookups, stores := 0, 0
	lookupUnlockedSecret = func(context.Context, string) ([]byte, error) {
		lookups++
		return []byte(base64.StdEncoding.EncodeToString(want)), nil
	}
	runSecretTool = func(_ context.Context, input []byte, args ...string) ([]byte, error) {
		stores++
		if args[0] != "store" || !bytes.Equal(input, []byte(base64.StdEncoding.EncodeToString(want))) || strings.Contains(strings.Join(args, " "), string(input)) {
			t.Fatal("secret put in argv or unexpected provider operation")
		}
		return nil, nil
	}
	if _, err := nativeRecoveryKey(context.Background(), "fixture", want); err != nil {
		t.Fatal(err)
	}
	got, err := nativeRecoveryKey(context.Background(), "fixture", nil)
	if err != nil || !bytes.Equal(got, want) || lookups != 1 || stores != 1 {
		t.Fatal("key read did not use the no-prompt Secret Service path")
	}
	lookupUnlockedSecret = func(context.Context, string) ([]byte, error) { return nil, errors.New("locked") }
	if _, err = nativeRecoveryKey(context.Background(), "fixture", nil); err != ErrRecoveryKeyUnavailable {
		t.Fatal("locked keyring failure not sanitized")
	}
}
