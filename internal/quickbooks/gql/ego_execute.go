package gql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// ErrEgoMissing is returned when the ego-browser CLI is not on PATH.
var ErrEgoMissing = errors.New("ego-browser not found on PATH: install ego lite (ego-browser onboarding)")

// egoSpaceName is the isolated ego-browser task space gql execution drives.
// It inherits the user's QBO login state; the space persists across calls.
const egoSpaceName = "qb-gql"

// egoLoginURL is opened when the space has no authenticated app tab yet.
const egoLoginURL = "https://qbo.intuit.com/app/banking"

func selectedEgoSpace() string {
	if space := os.Getenv("QB_EGO_SPACE_ID"); space != "" {
		return space
	}
	if space := os.Getenv("QB_EGO_SPACE"); space != "" {
		return space
	}
	return egoSpaceName
}

const egoExecuteScript = `const fs = await import('node:fs');
const outPath = process.env.QB_EGO_OUT;
const endpoint = process.env.QB_EGO_ENDPOINT;
const method = process.env.QB_EGO_METHOD || 'POST';
const headers = JSON.parse(process.env.QB_EGO_HEADERS || '{}');
const body = process.env.QB_EGO_BODY || '';
const loginURL = process.env.QB_EGO_LOGIN_URL;
const spaceKey = process.env.QB_EGO_SPACE || 'qb-gql';
const targetKey = process.env.QB_EGO_TARGET || '';
const expectedRealm = process.env.QB_EGO_REALM || '';
const expectedEmail = process.env.QB_EGO_EMAIL || '';
function isQBOApp(value) {
 try { const u = new URL(value); return u.protocol === 'https:' && u.hostname === 'qbo.intuit.com' && !u.username && !u.password && u.pathname.startsWith('/app/') && !value.includes('UNAUTHENTICATED'); }
 catch (_) { return false; }
}
function done(obj) { fs.writeFileSync(outPath, JSON.stringify(obj), {mode:0o600}); }
function fail(msg, code) { try { done({ok:false,error:msg}); } catch (_) {} process.exit(code || 2); }
if (!outPath || !endpoint) fail('missing browser request parameters', 2);
const task = await taskSpace(/^\d+$/.test(spaceKey) ? Number(spaceKey) : spaceKey);
const tabs = await task.tabs();
const candidates = tabs.filter(t => isQBOApp(t.url) && (!targetKey || t.targetId === targetKey || t.label === targetKey));
if (targetKey && candidates.length !== 1) fail('need-login: pinned app tab unavailable', 3);
const existing = candidates[0];
let page;
if (existing) {
 if (existing.label) page = task.page(existing.label);
 else if (existing.page && typeof existing.page.fetch === 'function') page = existing.page;
 else page = await task.adopt(existing.page);
}
else {
 const blank = tabs.find(t => t.label && t.url === 'about:blank');
 page = blank ? task.page(blank.label) : await task.newPage();
 await page.goto(loginURL);
}
const info = await page.info();
if (info && info.dialog) fail('need-login: dialog open', 3);
const url = await page.url();
if (!isQBOApp(url) || url.includes('sign-in')) fail('need-login: no authenticated app tab', 3);
headers['content-type'] = 'application/json';
headers['accept'] = 'application/json';
const verifyIdentity = async () => {
 if (!expectedRealm) return;
 const identity = await page.evaluate(process.env.QB_EGO_IDENTITY);
 if (identity.realm !== expectedRealm || (identity.email || '').toLowerCase() !== expectedEmail.toLowerCase()) fail('browser identity differs from captured session', 3);
};
await verifyIdentity();
const options = {method, headers, credentials:'include', redirect:'error', cache:'no-store', saveAs:outPath+'.body', timeout:Math.min(30000,Number(process.env.QB_EGO_TIMEOUT_MS || 30000))};
if (method !== 'GET' && method !== 'HEAD' && body) options.body = body;
let result;
try { result = await page.fetch(endpoint, options); }
catch (_) { fail('browser fetch failed', 2); }
await verifyIdentity();
fs.chmodSync(outPath+'.body',0o600);
const responseBody = fs.readFileSync(outPath+'.body','utf8');
done({ok:true,status:result.status,body:responseBody || ''});
`

// ExecuteEgo posts req inside the ego-browser QBO space via an in-page
// fetch: same origin, cookies, and SPA auth context as the retired relay
// path, but through the sanctioned ego helper channel (no Target.* CDP).
func ExecuteEgo(ctx context.Context, req Request) (*Response, error) {
	if req.Op == nil {
		return nil, fmt.Errorf("gql: missing operation")
	}
	endpoint := req.Endpoint
	if endpoint == "" {
		endpoint = req.Op.Endpoint
	}
	if endpoint == "" {
		endpoint = EndpointDefault
	}
	vars := req.Variables
	if vars == nil {
		vars = map[string]any{}
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": req.Op.Name,
		"variables":     vars,
		"query":         req.Op.Document,
	})
	if err != nil {
		return nil, fmt.Errorf("gql: encoding request: %w", err)
	}
	return FetchEgo(ctx, endpoint, "POST", authHeadersFor(endpoint, req.Op.Name), payload)
}

// FetchEgo sends an exact request through the selected ego-browser app page.
// GraphQL and browser-session REST surfaces share this transport.
func FetchEgo(ctx context.Context, endpoint, method string, headers map[string]string, payload []byte) (*Response, error) {
	if _, pinned := ctx.Value(executionSessionKey{}).(*auth.TokenSet); !pinned {
		tok, err := auth.Load()
		if err != nil {
			return nil, err
		}
		ctx = context.WithValue(ctx, executionSessionKey{}, tok)
	}
	return retryReadResponse(ctx, method == http.MethodGet || method == http.MethodHead, method != http.MethodHead, func() (*Response, error) {
		return fetchEgoOnce(ctx, endpoint, method, headers, payload)
	})
}

func fetchEgoOnce(ctx context.Context, endpoint, method string, headers map[string]string, payload []byte) (*Response, error) {
	tok, loadErr := auth.Load()
	if loadErr != nil {
		return nil, loadErr
	}
	if expected, ok := ctx.Value(executionSessionKey{}).(*auth.TokenSet); ok && !auth.SameSession(expected, tok) {
		return nil, auth.ErrSessionChanged
	}
	if tok.Source == "managed-profile" {
		return fetchManagedSession(ctx, tok, endpoint, method, headers, payload)
	}
	if _, err := exec.LookPath("ego-browser"); err != nil {
		return nil, ErrEgoMissing
	}
	if endpoint == "" {
		return nil, fmt.Errorf("browser request requires an endpoint")
	}
	hdrJSON, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("gql: encoding headers: %w", err)
	}
	dir, err := os.MkdirTemp("", "qb-gql-ego-*")
	if err != nil {
		return nil, fmt.Errorf("gql: creating result file: %w", err)
	}
	outPath := filepath.Join(dir, "response.json")
	defer func() { _ = os.Remove(outPath); _ = os.Remove(outPath + ".body"); _ = os.Remove(dir) }()

	timeoutMs := int64(120 * time.Second / time.Millisecond)
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl); rem > 0 {
			timeoutMs = rem.Milliseconds()
		}
	}
	space := selectedEgoSpace()
	target, realm, email := "", "", ""
	if tok.Source == "ego-existing" && tok.EgoSpace != "" && tok.EgoTargetID != "" {
		if explicit := os.Getenv("QB_EGO_SPACE_ID"); explicit != "" && explicit != tok.EgoSpace {
			return nil, fmt.Errorf("explicit Ego space differs from captured session")
		}
		if explicit := os.Getenv("QB_EGO_SPACE"); explicit != "" && explicit != tok.EgoSpace {
			return nil, fmt.Errorf("explicit Ego space differs from captured session")
		}
		space, target, realm, email = tok.EgoSpace, tok.EgoTargetID, tok.RealmID, tok.Email
	}
	preamble := "process.env.QB_EGO_OUT = " + strconv.Quote(outPath) + ";\n" +
		"process.env.QB_EGO_ENDPOINT = " + strconv.Quote(endpoint) + ";\n" +
		"process.env.QB_EGO_HEADERS = " + strconv.Quote(string(hdrJSON)) + ";\n" +
		"process.env.QB_EGO_BODY = " + strconv.Quote(string(payload)) + ";\n" +
		"process.env.QB_EGO_LOGIN_URL = " + strconv.Quote(egoLoginURL) + ";\n" +
		"process.env.QB_EGO_METHOD = " + strconv.Quote(method) + ";\n" +
		"process.env.QB_EGO_SPACE = " + strconv.Quote(space) + ";\n" +
		"process.env.QB_EGO_TARGET = " + strconv.Quote(target) + ";\n" +
		"process.env.QB_EGO_REALM = " + strconv.Quote(realm) + ";\n" +
		"process.env.QB_EGO_EMAIL = " + strconv.Quote(email) + ";\n" +
		"process.env.QB_EGO_IDENTITY = " + strconv.Quote(auth.IdentityJS) + ";\n" +
		"process.env.QB_EGO_TIMEOUT_MS = " + strconv.Quote(strconv.FormatInt(timeoutMs, 10)) + ";\n"
	cmd := exec.CommandContext(ctx, "ego-browser", "nodejs")
	cmd.Stdin = bytes.NewReader(append([]byte(preamble), []byte(egoExecuteScript)...))
	cmd.Env = append(os.Environ(), "QB_EGO_SPACE="+space, "QB_EGO_OUT="+outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		// The script records its failure reason in the result file before
		// exiting non-zero; prefer it over the bare process error.
		if raw, rerr := os.ReadFile(outPath); rerr == nil {
			var file struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if jerr := json.Unmarshal(raw, &file); jerr == nil && file.Error != "" {
				if file.Error == "need-login: dialog open" || file.Error == "need-login: no authenticated app tab" {
					return nil, fmt.Errorf("%w: %s", ErrNeedsLogin, file.Error)
				}
				if file.Error == "browser fetch failed" {
					return nil, errBrowserQueryTransport
				}
				return nil, fmt.Errorf("gql: ego execution failed: %s", file.Error)
			}
		}
		return nil, fmt.Errorf("gql: ego-browser execution failed: %w%s", err, trimEgoOutput(out))
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("gql: reading ego result: %w", err)
	}
	var file struct {
		OK     bool   `json:"ok"`
		Status int    `json:"status"`
		Body   string `json:"body"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("gql: parsing ego result: %w", err)
	}
	if !file.OK {
		if file.Error == "need-login: dialog open" || file.Error == "need-login: no authenticated app tab" {
			return nil, fmt.Errorf("%w: %s", ErrNeedsLogin, file.Error)
		}
		return nil, fmt.Errorf("gql: ego execution failed: %s", file.Error)
	}
	res := &Response{Status: file.Status, Body: json.RawMessage(file.Body)}
	var envelope struct {
		Errors []GraphQLError `json:"errors"`
	}
	if err := json.Unmarshal(res.Body, &envelope); err == nil {
		res.Errors = envelope.Errors
	}
	return res, nil
}

func trimEgoOutput(out []byte) string {
	const max = 300
	s := string(out)
	if len(s) > max {
		s = s[len(s)-max:]
	}
	if s == "" {
		return ""
	}
	return ": " + s
}
