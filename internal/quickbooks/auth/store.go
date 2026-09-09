package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNoCredentials is returned by Load when no credentials file exists.
// It is a typed error so callers (e.g. `qb auth status`) can distinguish
// "never logged in" from a corrupt file or a filesystem failure.
var ErrNoCredentials = errors.New("no saved credentials: run `qb login`")

// ErrEmptyTokenSet is returned by Save when the TokenSet has neither cookies
// nor an access/refresh token. Persisting an empty set would silently wipe a
// known-good session on the next Load, so Save refuses.
var ErrEmptyTokenSet = errors.New("refusing to save empty token set: no cookies and no tokens")

// credentialsFile is the filename written inside HomeDir().
const credentialsFile = "credentials.json"

// HomeDir returns the qb configuration root: $QB_HOME when set, otherwise
// ~/.config/qb. The directory is NOT created here; callers that need it on
// disk use Save (which mkdir's 0700) or os.MkdirAll directly.
func HomeDir() string {
	if v := os.Getenv("QB_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		// Fall back to a relative path rather than panic; Save will then
		// surface the mkdir error if the cwd is unwritable.
		return filepath.Join(".config", "qb")
	}
	return filepath.Join(home, ".config", "qb")
}

// CredPath returns the absolute-ish path to the credentials JSON file.
func CredPath() string {
	return filepath.Join(HomeDir(), credentialsFile)
}

// Save persists t to CredPath() atomically with mode 0600, creating the
// parent directory with mode 0700. It rejects an empty TokenSet
// (no cookies and no access/refresh token) with ErrEmptyTokenSet so a
// failed capture can never overwrite a known-good session.
func Save(t *TokenSet) error {
	if t == nil {
		return ErrEmptyTokenSet
	}
	if !t.HasUsableCredential() {
		return ErrEmptyTokenSet
	}
	if t.Version == 0 {
		t.Version = CurrentVersion
	}

	dir := HomeDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating qb home %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}

	final := CredPath()
	tmp, err := os.CreateTemp(dir, ".credentials-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp credentials file: %w", err)
	}
	tmpPath := tmp.Name()
	// Best-effort cleanup if the write or rename fails below.
	defer func() {
		if tmpPath != "" {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("writing credentials: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing credentials temp file: %w", err)
	}
	// Force 0600 even if CreateTemp's umask left it more open.
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return fmt.Errorf("setting credentials mode: %w", err)
	}
	if err := os.Rename(tmpPath, final); err != nil {
		return fmt.Errorf("installing credentials: %w", err)
	}
	tmpPath = "" // rename consumed it; don't double-remove.
	return nil
}

// Load reads and decodes the persisted TokenSet. A missing file yields
// ErrNoCredentials; any other read or decode failure is wrapped.
func Load() (*TokenSet, error) {
	data, err := os.ReadFile(CredPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNoCredentials
		}
		return nil, fmt.Errorf("reading credentials: %w", err)
	}
	var t TokenSet
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("parsing credentials: %w", err)
	}
	return &t, nil
}

// Delete removes the persisted credentials file. A missing file is not an
// error (logout is idempotent).
func Delete() error {
	if err := os.Remove(CredPath()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("deleting credentials: %w", err)
	}
	return nil
}
