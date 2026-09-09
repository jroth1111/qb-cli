package auth

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// ErrMitmProxyMissing is returned by ImportMitmDump when the `mitmdump`
// executable cannot be found on PATH. mitmproxy provides it; install with
// `brew install mitmproxy` on macOS.
var ErrMitmProxyMissing = errors.New("mitmdump not found on PATH: install mitmproxy (brew install mitmproxy)")

// ErrNoUsableCredential is returned by ImportMitmDump when the mitm dump was
// processed and credentials.json was written, but the resulting TokenSet has
// no usable credential (no Authorization header, no Intuit_APIKey access
// token, and no qbn.ticket/qbo.ticket cookie). This usually means the dump
// did not contain a completed QBO login.
var ErrNoUsableCredential = errors.New("mitm import produced no usable credential: dump has no QBO login")

// mitmImportScript is the filename of the mitmdump addon that extracts QBO
// credentials from a captured flow. It lives next to this source file.
const mitmImportScript = "mitm_import.py"

// scriptDir returns the directory holding this source file at runtime, so
// ImportMitmDump can locate mitm_import.py without depending on the caller's
// working directory.
func scriptDir() (string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("runtime.Caller: cannot determine source file path")
	}
	return filepath.Dir(file), nil
}

// ImportMitmDump replays a mitmproxy capture (`.mitm` flow file) through the
// bundled mitm_import.py addon, which writes credentials.json into QB_HOME.
// After the addon runs, ImportMitmDump loads the persisted TokenSet and
// requires it to hold a usable credential.
//
// QB_HOME is inherited from the environment so the addon writes to the same
// credentials store the rest of the package uses. The mitmdump process runs
// with no network (`-nr`); it only parses the on-disk dump.
//
// Errors:
//   - ErrMitmProxyMissing: `mitmdump` is not installed.
//   - wrapped fs/exec error: the dump path cannot be read or mitmdump exited
//     non-zero (e.g. the dump file is missing or corrupt).
//   - ErrNoUsableCredential: the dump parsed but contained no QBO login.
func ImportMitmDump(dumpPath string) error {
	if _, err := exec.LookPath("mitmdump"); err != nil {
		return ErrMitmProxyMissing
	}

	dir, err := scriptDir()
	if err != nil {
		return fmt.Errorf("locating mitm import script: %w", err)
	}
	scriptPath := filepath.Join(dir, mitmImportScript)

	// Fail fast on a missing dump before spawning mitmdump so the error
	// names the path the user gave us, not an opaque addon failure.
	if _, err := os.Stat(dumpPath); err != nil {
		return fmt.Errorf("mitm dump not found at %s: %w", dumpPath, err)
	}

	cmd := exec.Command("mitmdump",
		"-nr", dumpPath,
		"-s", scriptPath,
		"--quiet",
		"--set", "flow_detail=0",
	)
	// QB_HOME is inherited so the addon writes into the same store Load()
	// reads from. All other env (PATH, HOME, etc.) is inherited too.
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("mitmdump failed on %s: %w%s", dumpPath, err, trimOutput(out))
	}

	ts, err := Load()
	if err != nil {
		return fmt.Errorf("loading credentials after mitm import: %w", err)
	}
	if !ts.HasUsableCredential() {
		return ErrNoUsableCredential
	}
	return nil
}

// trimOutput keeps mitmdump stderr out of error messages unless it is short,
// so a failure surfaces something useful without dumping a wall of text (and
// never a secret — the addon never prints secret values).
func trimOutput(out []byte) string {
	const max = 512
	s := string(out)
	if len(s) == 0 {
		return ""
	}
	if len(s) > max {
		s = s[:max] + "…[truncated]"
	}
	return "\n" + s
}
