package gql

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
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
function isQBOApp(value) {
 try { const u = new URL(value); return u.protocol === 'https:' && u.hostname === 'qbo.intuit.com' && !u.username && !u.password && u.pathname.startsWith('/app/') && !value.includes('UNAUTHENTICATED'); }
 catch (_) { return false; }
}
function done(obj) { fs.writeFileSync(outPath, JSON.stringify(obj), {mode:0o600}); }
function fail(msg, code) { try { done({ok:false,error:msg}); } catch (_) {} process.exit(code || 2); }
if (!outPath || !endpoint) fail('missing browser request parameters', 2);
const task = await taskSpace(/^\d+$/.test(spaceKey) ? Number(spaceKey) : spaceKey);
const tabs = await task.tabs();
const existing = tabs.find(t => isQBOApp(t.url));
let page;
if (existing) page = existing.label ? task.page(existing.label) : await task.adopt(existing.targetId);
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
const options = {method, headers, credentials:'include', timeout:Math.min(30000,Number(process.env.QB_EGO_TIMEOUT_MS || 30000))};
if (method !== 'GET' && method !== 'HEAD' && body) options.body = body;
let result;
try { result = await page.fetch(endpoint, options); }
catch (_) { fail('browser fetch failed', 2); }
const responseBody = typeof result.body === 'string' ? result.body : JSON.stringify(result.body);
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
	outFile, err := os.CreateTemp("", "qb-gql-ego-*.json")
	if err != nil {
		return nil, fmt.Errorf("gql: creating result file: %w", err)
	}
	outPath := outFile.Name()
	_ = outFile.Close()
	defer func() { _ = os.Remove(outPath) }()
	_ = os.Chmod(outPath, 0o600)

	timeoutMs := int64(120 * time.Second / time.Millisecond)
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl); rem > 0 {
			timeoutMs = rem.Milliseconds()
		}
	}
	space := selectedEgoSpace()
	preamble := "process.env.QB_EGO_OUT = " + strconv.Quote(outPath) + ";\n" +
		"process.env.QB_EGO_ENDPOINT = " + strconv.Quote(endpoint) + ";\n" +
		"process.env.QB_EGO_HEADERS = " + strconv.Quote(string(hdrJSON)) + ";\n" +
		"process.env.QB_EGO_BODY = " + strconv.Quote(string(payload)) + ";\n" +
		"process.env.QB_EGO_LOGIN_URL = " + strconv.Quote(egoLoginURL) + ";\n" +
		"process.env.QB_EGO_METHOD = " + strconv.Quote(method) + ";\n" +
		"process.env.QB_EGO_SPACE = " + strconv.Quote(space) + ";\n" +
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
