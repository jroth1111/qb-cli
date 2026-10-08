package auth

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var ErrRecoveryKeyUnavailable = errors.New("recovery key unavailable: unlock the Keychain, Secret Service or GPG agent, or provision a private --key-file; plaintext fallback is disabled")
var ErrRecoveryDisabled = errors.New("automatic login recovery is not enrolled or is disabled")
var ErrRecoveryAttention = errors.New("login stopped at an unsupported or rejected Intuit step; inspect login diagnostics")
var ErrRecoveryCooldown = errors.New("login recovery is in cooldown; inspect qb auth recovery status")
var errRecoveryInvalid = errors.New("invalid or insecure recovery enrollment; saved credentials unchanged")

const recoveryFile = "login-recovery.json"
const recoveryCooldown = time.Minute

type recoveryProvider string

const (
	recoveryNative  recoveryProvider = "native"
	recoveryKeyFile recoveryProvider = "key-file"
	recoveryGPG     recoveryProvider = "gpg"
)

type recoveryState string

const (
	recoveryNotEnrolled      recoveryState = "not_enrolled"
	recoveryEnrolled         recoveryState = "enrolled"
	recoveryRecovering       recoveryState = "recovering"
	recoveryRecovered        recoveryState = "recovered"
	recoveryKeyUnavailable   recoveryState = "key_unavailable"
	recoveryNeedsAttention   recoveryState = "needs_attention"
	recoveryIdentityMismatch recoveryState = "identity_mismatch"
	recoveryDisabled         recoveryState = "disabled"
)

type RecoverySecrets struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTP     string `json:"totp,omitempty"`
}

type RecoveryOptions struct {
	KeyFile      string
	GPGRecipient string
	Bootstrap    bool
	TestProfile  bool
	Headed       bool
}

type recoveryRecord struct {
	Version      int              `json:"version"`
	ID           string           `json:"id"`
	Home         string           `json:"home"`
	SessionID    string           `json:"session_id"`
	Realm        string           `json:"realm"`
	Email        string           `json:"email"`
	Provider     recoveryProvider `json:"provider"`
	KeyFile      string           `json:"key_file,omitempty"`
	GPGRecipient string           `json:"gpg_recipient,omitempty"`
	EncryptedKey []byte           `json:"encrypted_key,omitempty"`
	Nonce        []byte           `json:"nonce"`
	Ciphertext   []byte           `json:"ciphertext"`
	Enabled      bool             `json:"enabled"`
	LastAttempt  time.Time        `json:"last_attempt,omitzero"`
	State        recoveryState    `json:"state"`
	TestProfile  bool             `json:"test_profile,omitempty"`
	Headed       bool             `json:"headed,omitempty"`
	LastEvidence *LoginEvidence   `json:"last_evidence,omitempty"`
}

type RecoveryStatus struct {
	Enrolled     bool             `json:"enrolled"`
	Enabled      bool             `json:"enabled"`
	Bound        bool             `json:"bound"`
	Provider     recoveryProvider `json:"provider,omitempty"`
	State        recoveryState    `json:"state"`
	LastAttempt  time.Time        `json:"last_attempt,omitzero"`
	LastEvidence *LoginEvidence   `json:"last_evidence,omitempty"`
}

func recoveryHome() (string, error) {
	h, err := filepath.Abs(HomeDir())
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(h)
}

// Ciphertext is bound to its profile, principal, session and key provider.
func (r *recoveryRecord) aad() []byte {
	b, _ := json.Marshal([]string{r.ID, r.Home, r.SessionID, r.Realm, r.Email, string(r.Provider), r.KeyFile, r.GPGRecipient})
	return b
}

func readPrivateFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !recoveryFileOwned(info) || info.Size() > limit {
		return nil, errRecoveryInvalid
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, errRecoveryInvalid
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return nil, errRecoveryInvalid
	}
	return io.ReadAll(io.LimitReader(f, limit+1))
}

func loadRecovery() (*recoveryRecord, error) {
	b, err := readPrivateFile(filepath.Join(HomeDir(), recoveryFile), 65536)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrRecoveryDisabled
	}
	if err != nil {
		return nil, errRecoveryInvalid
	}
	var r recoveryRecord
	if json.Unmarshal(b, &r) != nil || r.Version != 1 || len(r.ID) != 32 || r.SessionID == "" || r.Realm == "" || r.Email == "" {
		return nil, errRecoveryInvalid
	}
	h, err := recoveryHome()
	if err != nil || r.Home != h {
		return nil, errRecoveryInvalid
	}
	return &r, nil
}

func writeRecovery(r *recoveryRecord) error {
	b, err := json.Marshal(r)
	if err != nil {
		return errRecoveryInvalid
	}
	f, err := os.CreateTemp(HomeDir(), ".login-recovery-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), filepath.Join(HomeDir(), recoveryFile))
}

var recoveryNativeKey = nativeRecoveryKey
var recoveryDeleteNativeKey = deleteNativeRecoveryKey

var runRecoveryGPG = func(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gpg", args...)
	cmd.Stdin = bytes.NewReader(input)
	// No stderr forwarding: a provider's errors may echo secret material.
	return cmd.Output()
}

func (r *recoveryRecord) key(ctx context.Context, create []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	switch r.Provider {
	case recoveryNative:
		return recoveryNativeKey(ctx, r.ID, create)
	case recoveryKeyFile:
		path := r.KeyFile
		if override := os.Getenv("QB_RECOVERY_KEY_FILE"); override != "" {
			resolved, err := filepath.EvalSymlinks(override)
			if err != nil || !filepath.IsAbs(override) || resolved != override || override == r.Home || strings.HasPrefix(override, r.Home+string(os.PathSeparator)) {
				return nil, ErrRecoveryKeyUnavailable
			}
			path = override
		}
		key, err := readPrivateFile(path, 32)
		if err != nil || len(key) != 32 {
			return nil, ErrRecoveryKeyUnavailable
		}
		return key, nil
	case recoveryGPG:
		args := []string{"--batch", "--no-tty", "--no-auto-key-retrieve", "--no-encrypt-to", "--pinentry-mode", "error"}
		args = append([]string{"--no-options"}, args...)
		if create != nil {
			out, err := runRecoveryGPG(ctx, create, append(args, "--encrypt", "--recipient", r.GPGRecipient)...)
			if err != nil || len(out) == 0 || len(out) > 16384 {
				return nil, ErrRecoveryKeyUnavailable
			}
			r.EncryptedKey = out
		}
		key, err := runRecoveryGPG(ctx, r.EncryptedKey, append(args, "--decrypt")...)
		if err != nil || len(key) != 32 {
			return nil, ErrRecoveryKeyUnavailable
		}
		return key, nil
	default:
		return nil, errRecoveryInvalid
	}
}

func (r *recoveryRecord) decrypt(ctx context.Context) (*RecoverySecrets, error) {
	key, err := r.key(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, ErrRecoveryKeyUnavailable
	}
	aead, _ := cipher.NewGCM(block)
	if len(r.Nonce) != aead.NonceSize() {
		return nil, errRecoveryInvalid
	}
	plain, err := aead.Open(nil, r.Nonce, r.Ciphertext, r.aad())
	if err != nil {
		return nil, errRecoveryInvalid
	}
	defer clear(plain)
	var secrets RecoverySecrets
	if json.Unmarshal(plain, &secrets) != nil || secrets.Username == "" || secrets.Password == "" {
		return nil, errRecoveryInvalid
	}
	if _, err := parseTOTP(secrets.TOTP); err != nil {
		return nil, errRecoveryInvalid
	}
	return &secrets, nil
}

func recoveryBound(r *recoveryRecord, tok *TokenSet) bool {
	return SupportsNativeTicket(tok) && tok.SessionID == r.SessionID && tok.RealmID == r.Realm && strings.EqualFold(tok.Email, r.Email)
}

func LoginRecoveryStatus() (RecoveryStatus, error) {
	r, err := loadRecovery()
	if errors.Is(err, ErrRecoveryDisabled) {
		return RecoveryStatus{State: recoveryNotEnrolled}, nil
	}
	if err != nil {
		return RecoveryStatus{}, err
	}
	tok, _ := Load()
	return RecoveryStatus{Enrolled: true, Enabled: r.Enabled, Bound: recoveryBound(r, tok), Provider: r.Provider, State: r.State, LastAttempt: r.LastAttempt, LastEvidence: r.LastEvidence}, nil
}

// Enrollment requires a freshly verified managed capture before persisting secrets.
func EnrollLoginRecovery(ctx context.Context, secrets RecoverySecrets, opts RecoveryOptions) error {
	if IsHarness() {
		return ErrRecoveryDisabled
	}
	if len(secrets.Username) == 0 || len(secrets.Username) > 320 || len(secrets.Password) == 0 || len(secrets.Password) > 4096 {
		return errRecoveryInvalid
	}
	if _, err := parseTOTP(secrets.TOTP); err != nil {
		return err
	}
	if opts.KeyFile != "" && opts.GPGRecipient != "" {
		return errRecoveryInvalid
	}
	if opts.TestProfile && !opts.Bootstrap {
		return errors.New("--test-profile requires --bootstrap in a fresh QB_HOME")
	}
	if opts.Bootstrap {
		return bootstrapLoginRecovery(ctx, secrets, opts)
	}
	ctx = withManagedHeaded(ctx, opts.Headed)
	tok, err := EnsureSession()
	if err != nil {
		return err
	}
	if !SupportsNativeTicket(tok) || tok.Email == "" || tok.RealmID == "" {
		return errors.New("enrollment requires a verified managed login or pinned existing Ego source")
	}
	if !strings.EqualFold(secrets.Username, tok.Email) {
		return ErrSessionChanged
	}
	unlock, err := LockProfile(ctx, HomeDir(), "renewal")
	if err != nil {
		return err
	}
	defer unlock()
	if _, err = loadRecovery(); !errors.Is(err, ErrRecoveryDisabled) {
		return errors.New("recovery enrollment already exists or is invalid; use qb auth recovery forget before re-enrolling")
	}
	cap, err := recoveryCapture(ctx, tok, nil)
	if err != nil {
		return err
	}
	if err = verifyRecoveryIdentity(tok, cap); err != nil {
		return err
	}
	h, err := recoveryHome()
	if err != nil {
		return err
	}
	var id [16]byte
	if _, err = rand.Read(id[:]); err != nil {
		return err
	}
	r := &recoveryRecord{Version: 1, ID: hex.EncodeToString(id[:]), Home: h, SessionID: tok.SessionID, Realm: tok.RealmID, Email: tok.Email, Provider: recoveryNative, Enabled: true, State: recoveryEnrolled, Headed: opts.Headed}
	if opts.KeyFile != "" {
		r.Provider = recoveryKeyFile
		r.KeyFile, err = filepath.Abs(opts.KeyFile)
		if err != nil {
			return errRecoveryInvalid
		}
		resolved, e := filepath.EvalSymlinks(r.KeyFile)
		if e != nil || resolved != r.KeyFile || r.KeyFile == h || strings.HasPrefix(r.KeyFile, h+string(os.PathSeparator)) {
			return errors.New("--key-file must be an absolute regular private file outside QB_HOME, without symlinks")
		}
	}
	if opts.GPGRecipient != "" {
		if !regexp.MustCompile(`^[A-Fa-f0-9]{40}([A-Fa-f0-9]{24})?$`).MatchString(opts.GPGRecipient) {
			return errors.New("--gpg-recipient requires a full 40- or 64-character encryption-key fingerprint")
		}
		r.Provider = recoveryGPG
		r.GPGRecipient = strings.ToUpper(opts.GPGRecipient)
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return err
	}
	defer clear(key)
	key, err = r.key(ctx, key)
	if err != nil {
		return err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		return ErrRecoveryKeyUnavailable
	}
	aead, _ := cipher.NewGCM(block)
	r.Nonce = make([]byte, aead.NonceSize())
	if _, err = rand.Read(r.Nonce); err != nil {
		return err
	}
	plain, _ := json.Marshal(secrets)
	defer clear(plain)
	r.Ciphertext = aead.Seal(nil, r.Nonce, plain, r.aad())
	lock, err := credentialLock(HomeDir())
	if err != nil {
		return err
	}
	defer lock()
	current, err := Load()
	if err != nil || !SameSession(tok, current) {
		return ErrSessionChanged
	}
	return writeRecovery(r)
}

// Disable and forget share the commit lock with recovery, so neither can be undone by an in-flight login.
func DisableLoginRecovery(ctx context.Context, forget bool) error {
	if IsHarness() {
		return ErrRecoveryDisabled
	}
	unlock, err := credentialLock(HomeDir())
	if err != nil {
		return err
	}
	defer unlock()
	r, err := loadRecovery()
	if errors.Is(err, ErrRecoveryDisabled) {
		return nil
	}
	if err != nil {
		return err
	}
	r.Enabled = false
	r.State = recoveryDisabled
	if err = writeRecovery(r); err != nil {
		return err
	}
	if !forget {
		return nil
	}
	if r.Provider == recoveryNative {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err = recoveryDeleteNativeKey(ctx, r.ID); err != nil {
			return err
		}
	}
	return os.Remove(filepath.Join(HomeDir(), recoveryFile))
}

// CreateRecoveryKeyFile never overwrites an existing secret or creates a key
// alongside the encrypted enrollment. Provision it once under the service user.
func CreateRecoveryKeyFile(ctx context.Context, path string) error {
	if IsHarness() {
		return ErrRecoveryDisabled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(path) {
		return errors.New("key path must be absolute")
	}
	home, err := filepath.Abs(HomeDir())
	if err != nil {
		return err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil || parent != filepath.Dir(path) || path == home || strings.HasPrefix(path, home+string(os.PathSeparator)) {
		return errors.New("key path must be outside QB_HOME in an existing private directory without symlinks")
	}
	info, err := os.Stat(parent)
	if err != nil || info.Mode().Perm()&0077 != 0 || !recoveryFileOwned(info) {
		return errRecoveryInvalid
	}
	key := make([]byte, 32)
	if _, err = rand.Read(key); err != nil {
		return err
	}
	defer clear(key)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("could not create key file; existing files are never overwritten")
	}
	_, err = f.Write(key)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
