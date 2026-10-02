package auth

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecoveryGPGRealIsolatedRoundTrip(t *testing.T) {
	gpg, err := exec.LookPath("gpg")
	if err != nil {
		t.Skip("gpg unavailable")
	}
	dir, err := os.MkdirTemp("", "qb-gpg-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	if err = os.Chmod(dir, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GNUPGHOME", dir)
	t.Cleanup(func() {
		if conf, e := exec.LookPath("gpgconf"); e == nil {
			_ = exec.Command(conf, "--homedir", dir, "--kill", "gpg-agent").Run()
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, gpg, "--batch", "--pinentry-mode", "loopback", "--passphrase", "", "--quick-generate-key", "qb synthetic test <fixture@example.invalid>", "rsa2048", "encr", "1d")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err = cmd.Run(); err != nil {
		t.Fatalf("isolated synthetic GPG key generation failed: %v (%s)", err, stderr.String())
	}
	out, err := exec.CommandContext(ctx, gpg, "--batch", "--with-colons", "--list-keys").Output()
	if err != nil {
		t.Fatal("isolated GPG key enumeration failed")
	}
	fingerprint := ""
	for line := range strings.SplitSeq(string(out), "\n") {
		parts := strings.Split(line, ":")
		if len(parts) > 9 && parts[0] == "fpr" {
			fingerprint = parts[9]
			break
		}
	}
	if fingerprint == "" {
		t.Fatal("isolated GPG key fingerprint missing")
	}
	r := &recoveryRecord{Provider: "gpg", GPGRecipient: fingerprint}
	want := bytes.Repeat([]byte{7}, 32)
	got, err := r.key(ctx, want)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("GPG master-key round-trip failed")
	}
	if bytes.Contains(r.EncryptedKey, want) {
		t.Fatal("GPG ciphertext contains plaintext master key")
	}
	got, err = r.key(ctx, nil)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("autonomous GPG decrypt failed")
	}
	if _, err = os.Stat(filepath.Join(dir, "private-keys-v1.d")); err != nil {
		t.Fatal("fixture did not use isolated private keys")
	}
}

func TestRecoveryGPGCannotPromptOrLeakProviderErrors(t *testing.T) {
	previous := runRecoveryGPG
	defer func() { runRecoveryGPG = previous }()
	runRecoveryGPG = func(_ context.Context, _ []byte, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		for _, required := range []string{"--batch", "--no-tty", "--no-auto-key-retrieve", "--no-encrypt-to", "--pinentry-mode error"} {
			if !strings.Contains(joined, required) {
				t.Fatal("GPG recovery could prompt, fetch keys, or encrypt to additional recipients")
			}
		}
		return nil, errors.New("fixture provider error containing secret material")
	}
	r := &recoveryRecord{Provider: "gpg", EncryptedKey: []byte("encrypted fixture")}
	if _, err := r.key(context.Background(), nil); err != ErrRecoveryKeyUnavailable {
		t.Fatal("provider output leaked into error")
	}
}
