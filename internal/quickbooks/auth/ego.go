package auth

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const egoCaptureScript = "ego_capture.js"

//go:embed ego_existing.js
var existingEgoScript []byte

type existingEgoKey struct{}
type egoAPIProofKey struct{}
type existingEgoOptions struct{ space, target string }

// WithExistingEgo selects an already authenticated tab. It never claims a
// user-controlled space or launches a login flow.
func WithExistingEgo(ctx context.Context, space, target string) context.Context {
	return context.WithValue(ctx, existingEgoKey{}, existingEgoOptions{space, target})
}

// ErrEgoMissing is returned when the ego-browser CLI is not on PATH.
var ErrEgoMissing = errors.New("ego-browser not found on PATH: install ego lite (ego-browser onboarding) or use --from-mitm / --no-open")

var ErrEgoUserControl = errors.New("QBO Ego space is user-controlled; browser access stopped, explicitly continue/claim it before remint")

// CaptureATSFromEgo runs the bundled ego_capture.js inside an isolated
// ego-browser Space, waits for an authenticated QBO tab, intercepts one
// ATS Intuit_APIKey request, and returns the header map plus Intuit cookies.
// Secrets are written only to a 0600 temp file that this function reads
// and then deletes. stdout from ego is redacted JSON only.
func CaptureATSFromEgo(ctx context.Context, loginURL, bankingURL string) (*ATSCapture, error) {
	if IsHarness() {
		return nil, ErrRemintNeedsLogin
	}
	if _, err := exec.LookPath("ego-browser"); err != nil {
		return nil, ErrEgoMissing
	}
	dir, err := scriptDir()
	if err != nil {
		return nil, fmt.Errorf("locating ego capture script: %w", err)
	}
	scriptName := egoCaptureScript
	existing, existingOnly := ctx.Value(existingEgoKey{}).(existingEgoOptions)
	if existingOnly {
		scriptName = "ego_existing.js"
	}
	scriptPath := filepath.Join(dir, scriptName)
	script := existingEgoScript
	if !existingOnly {
		script, err = os.ReadFile(scriptPath)
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", scriptPath, err)
	}
	if loginURL == "" {
		loginURL = DefaultLoginURL
	}
	if bankingURL == "" {
		bankingURL = BankingCaptureURL
	}

	outFile, err := os.CreateTemp("", "qb-ego-capture-*.json")
	if err != nil {
		return nil, fmt.Errorf("creating capture file: %w", err)
	}
	outPath := outFile.Name()
	_ = outFile.Close()
	defer func() { _ = os.Remove(outPath) }()
	_ = os.Chmod(outPath, 0o600)

	timeoutMs := int64(10 * time.Minute / time.Millisecond)
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl); rem > 0 {
			timeoutMs = rem.Milliseconds()
		}
	}
	auditCapture := timeoutMs > 30_000

	// ego-browser nodejs does not reliably forward parent env into the
	// script VM. Stamp paths into the source so capture cannot miss them.
	// QB_EGO_SPACE_ID/QB_EGO_SPACE select the capture Space (default
	// "qb-login"); reuse the gql exec convention so an authenticated exec
	// Space can also serve remint.
	space := os.Getenv("QB_EGO_SPACE_ID")
	if space == "" {
		space = os.Getenv("QB_EGO_SPACE")
	}
	if existingOnly {
		space = existing.space
	}
	preamble := "process.env.QB_CAPTURE_OUT = " + strconv.Quote(outPath) + ";\n" +
		"process.env.QB_KEEP_CAPTURE_SPACE = " + strconv.Quote(strconv.FormatBool(retainBrowser(ctx))) + ";\n" +
		"process.env.QB_LOGIN_URL = " + strconv.Quote(loginURL) + ";\n" +
		"process.env.QB_BANKING_URL = " + strconv.Quote(bankingURL) + ";\n" +
		"process.env.QB_EGO_SPACE = " + strconv.Quote(space) + ";\n" +
		"process.env.QB_CAPTURE_AUDIT = " + strconv.Quote(strconv.FormatBool(auditCapture)) + ";\n" +
		"process.env.QB_TIMEOUT_MS = " + strconv.Quote(strconv.FormatInt(timeoutMs, 10)) + ";\n"
	if existingOnly {
		preamble += "const captureTarget = " + strconv.Quote(existing.target) + ";\n" +
			"const identityExpression = " + strconv.Quote(IdentityJS) + ";\n"
		requireProof, _ := ctx.Value(egoAPIProofKey{}).(bool)
		preamble += "const requireAPIProof = " + strconv.FormatBool(requireProof) + ";\n"
	}
	cmd := exec.CommandContext(ctx, "ego-browser", "nodejs")
	cmd.Stdin = bytes.NewReader(append([]byte(preamble), script...))
	cmd.Env = append(os.Environ(),
		"QB_KEEP_CAPTURE_SPACE="+strconv.FormatBool(retainBrowser(ctx)),
		"QB_CAPTURE_OUT="+outPath,
		"QB_LOGIN_URL="+loginURL,
		"QB_BANKING_URL="+bankingURL,
		"QB_EGO_SPACE="+space,
		"QB_CAPTURE_AUDIT="+strconv.FormatBool(auditCapture),
		"QB_TIMEOUT_MS="+strconv.FormatInt(timeoutMs, 10),
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		if bytes.Contains(bytes.ToLower(out), []byte("user has taken control")) {
			return nil, ErrEgoUserControl
		}
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ego-browser capture stopped before authentication could be verified: %w", ctx.Err())
		}
		return nil, fmt.Errorf("ego-browser capture failed: %w%s", err, trimOutput(out))
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("reading ego capture: %w", err)
	}
	var file struct {
		Headers            map[string]string            `json:"headers"`
		APIHeaders         map[string]string            `json:"api_headers"`
		HostHeaders        map[string]map[string]string `json:"host_headers"`
		AuditAuthorization string                       `json:"audit_authorization"`
		Cookies            []Cookie                     `json:"cookies"`
		Identity           Identity                     `json:"identity"`
		APIProof           bool                         `json:"api_proof"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("parsing ego capture: %w", err)
	}
	cap := &ATSCapture{Headers: file.Headers, SecondaryHeaders: file.APIHeaders, HostHeaders: file.HostHeaders, AuditAuthorization: file.AuditAuthorization, Cookies: file.Cookies, Identity: file.Identity}
	if required, _ := ctx.Value(egoAPIProofKey{}).(bool); required {
		if !file.APIProof {
			return nil, ErrSessionIdentityUnverified
		}
		cap.Evidence = &LoginEvidence{APIProof: true, IdentityVerified: true}
	}
	if !isIntuitAPIKey(headerGet(cap.Headers, "authorization")) {
		return nil, ErrNoATSAuthorization
	}
	return cap, nil
}
