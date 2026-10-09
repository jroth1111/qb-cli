package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// The browser path is selected before sending, never as a retry after a direct
// request. An interrupted browser write still requires read-only recovery.
const egoTransportScript = `const fs = await import('node:fs/promises');
let fetchStarted = false;
const refuse = code => { const error = new Error(code); error.qbCode = code; throw error; };
try {
const task = await ` + auth.ExistingEgoTaskJS + `(request.space,taskSpace,typeof listTaskSpaces==='function'?listTaskSpaces:undefined);
if(task.ownership!=='agent')refuse('user-control');
const tabs = await task.tabs();
const selected = tabs.filter(t => t.label === request.target || t.targetId === request.target);
if (selected.length !== 1 || !selected[0].label) refuse('pinned-page-unavailable');
const page = task.page(selected[0].label);
if((await page.info()).dialog)refuse('dialog-open');
const pageURL = new URL(await page.url());
if(pageURL.origin==='https://accounts.intuit.com'&&!pageURL.username&&!pageURL.password)refuse('need-login');
if (pageURL.origin !== 'https://qbo.intuit.com' || pageURL.username || pageURL.password || !pageURL.pathname.startsWith('/app/')) refuse('pinned-page-origin-changed');
async function hydratedIdentity() {
 let current = await page.evaluate(identityExpression);
 if (!current?.realm || !current?.email) {
  // Native ticket maintenance can reload this same pinned SPA. Wait only
  // for absent bootstrap fields; a populated wrong identity still fails.
  await page.waitForFunction(expression => {
   try { const value=(0,eval)(expression); return !!value?.realm && !!value?.email; }
   catch (_) { return false; }
  }, identityExpression, {timeout:30000});
  current=await page.evaluate(identityExpression);
 }
 const u=new URL(await page.url());
 if(u.origin!=='https://qbo.intuit.com'||u.username||u.password||!u.pathname.startsWith('/app/')) refuse('pinned-page-origin-changed');
 return current;
}
const before = await hydratedIdentity();
if (before.realm !== request.realm || (before.email || '').toLowerCase() !== request.email.toLowerCase()) refuse('identity-before-request');
const options = {method:request.method,headers:request.headers,credentials:'include',redirect:'error',cache:'no-store',timeout:request.timeout,saveAs:request.out+'.body'};
if (request.body && !request.binaryBody && request.method !== 'GET' && request.method !== 'HEAD') options.body = Buffer.from(request.body,'base64').toString('utf8');
fetchStarted = true;
let result;
if(request.binaryBody){
 result=await page.evaluate((0,eval)(byteFetchExpression),{endpoint:request.url,method:request.method,headers:request.headers,bodyBase64:request.body,timeoutMs:request.timeout});
 await fs.writeFile(request.out+'.body',Buffer.from(result.bodyBase64,'base64'),{mode:0o600});
}else{result=await page.fetch(request.url,options);}
const after = await hydratedIdentity();
if (after.realm !== before.realm || (after.email || '').toLowerCase() !== before.email.toLowerCase()) refuse('identity-after-request');
// The inline SDK text response is capped; saveAs preserves the actual bytes.
// Both files live inside the caller's private 0700 temporary directory.
await fs.chmod(request.out+'.body',0o600);
const body = await fs.readFile(request.out+'.body');
await fs.writeFile(request.out,JSON.stringify({status:result.status,headers:result.headers,bodyBase64:body.toString('base64')}),{mode:0o600});
} catch (error) {
const message = String(error?.message || '');
const code = error.qbCode || (/user has taken control|space is inactive|unassigned/i.test(message) ? 'user-control' : /timeout|timed out|deadline/i.test(message) ? 'timeout' : /network|fetch|connection/i.test(message) ? 'network' : 'browser-runtime');
await fs.writeFile(request.out,JSON.stringify({error_code:code,fetch_started:fetchStarted}),{mode:0o600});
console.error('qb pinned browser request failed: '+code);
process.exitCode = 1;
}
`

type egoHTTPRequest struct {
	Space      string            `json:"space"`
	Target     string            `json:"target"`
	Realm      string            `json:"realm"`
	Email      string            `json:"email"`
	URL        string            `json:"url"`
	Method     string            `json:"method"`
	Headers    map[string]string `json:"headers"`
	Body       string            `json:"body"`
	Out        string            `json:"out"`
	Timeout    int64             `json:"timeout"`
	BinaryBody bool              `json:"binaryBody,omitempty"`
}

var errEgoReadTransient = errors.New("pinned browser read transport interrupted")

func buildEgoHTTPRequest(req *http.Request, tok *auth.TokenSet) (egoHTTPRequest, error) {
	var out egoHTTPRequest
	if req.URL == nil || req.URL.Scheme != "https" || req.URL.Host != "qbo.intuit.com" || req.URL.User != nil {
		return out, fmt.Errorf("ego HTTP transport only supports first-party QBO HTTPS requests")
	}
	if tok == nil || tok.Source != "ego-existing" || tok.RealmID == "" || tok.Email == "" || tok.EgoTargetID == "" {
		return out, fmt.Errorf("ego HTTP transport requires a pinned ego-existing QBO session")
	}
	if tok.EgoSpace == "" || strings.TrimSpace(tok.EgoSpace) != tok.EgoSpace {
		return out, fmt.Errorf("ego HTTP transport requires a captured space ID or name")
	}
	if space, err := strconv.Atoi(tok.EgoSpace); err == nil && space < 1 || err != nil && strings.Trim(tok.EgoSpace, "0123456789") == "" {
		return out, fmt.Errorf("ego HTTP transport requires a valid captured space")
	}
	if !strings.Contains(req.URL.Path, "/company/"+tok.RealmID+"/") {
		return out, fmt.Errorf("ego request does not match captured QBO company")
	}
	if err := auth.ValidateBrowserDestination(req.URL.String(), tok, nil); err != nil {
		return out, err
	}
	out = egoHTTPRequest{Space: tok.EgoSpace, Target: tok.EgoTargetID, Realm: tok.RealmID, Email: tok.Email, URL: req.URL.String(), Method: req.Method, Headers: map[string]string{}, Timeout: httpTimeout.Milliseconds()}
	if req.Method == http.MethodGet || req.Method == http.MethodHead {
		out.Timeout = (90 * time.Second).Milliseconds()
	}
	for key, vals := range req.Header {
		lower := strings.ToLower(key)
		if skipReplayHeader(key) || strings.HasPrefix(lower, "sec-") || lower == "origin" || lower == "referer" || lower == "user-agent" {
			continue
		}
		out.Headers[key] = strings.Join(vals, ", ")
	}
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return out, fmt.Errorf("reading browser request body: %w", err)
		}
		_ = req.Body.Close()
		out.Body = base64.StdEncoding.EncodeToString(body)
		out.BinaryBody = !utf8.Valid(body)
	}
	return out, nil
}

func egoSessionDo(req *http.Request) (*http.Response, error) { return egoSessionDoOnce(req) }

func egoSessionDoOnce(req *http.Request) (*http.Response, error) {
	tok, err := auth.Load()
	if err != nil {
		return nil, err
	}
	input, err := buildEgoHTTPRequest(req, tok)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "qb-http-ego-*")
	if err != nil {
		return nil, err
	}
	input.Out = filepath.Join(dir, "response.json")
	defer func() {
		_ = os.Remove(input.Out)
		_ = os.Remove(input.Out + ".body")
		_ = os.Remove(dir)
	}()
	payload, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	script := "const request = " + string(payload) + ";\nconst identityExpression = " + strconv.Quote(auth.IdentityJS) + ";\nconst byteFetchExpression = " + strconv.Quote(auth.BrowserByteFetchJS) + ";\n" + egoTransportScript
	ctx, cancel := context.WithTimeout(req.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ego-browser", "nodejs")
	cmd.Stdin = bytes.NewBufferString(script)
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		if bytes.Contains(output, []byte("user has taken control")) || bytes.Contains(output, []byte("space is inactive")) {
			return nil, auth.ErrEgoUserControl
		}
		var failure struct {
			Code         string `json:"error_code"`
			FetchStarted bool   `json:"fetch_started"`
		}
		if raw, readErr := os.ReadFile(input.Out); readErr == nil && json.Unmarshal(raw, &failure) == nil && failure.Code != "" {
			if failure.Code == "user-control" {
				return nil, auth.ErrEgoUserControl
			}
			if failure.Code == "need-login" {
				return nil, auth.ErrRemintNeedsLogin
			}
			if failure.Code == "dialog-open" {
				return nil, auth.ErrRecoveryAttention
			}
			if failure.Code == "pinned-page-unavailable" || failure.Code == "pinned-page-origin-changed" || failure.Code == "identity-before-request" || failure.Code == "identity-after-request" {
				return nil, auth.ErrSessionChanged
			}
			phase := "before request dispatch"
			if failure.FetchStarted {
				phase = "after fetch started; outcome may be unverified"
			}
			if (req.Method == http.MethodGet || req.Method == http.MethodHead) && (failure.Code == "timeout" || failure.Code == "network") {
				return nil, fmt.Errorf("%w: %s", errEgoReadTransient, failure.Code)
			}
			return nil, fmt.Errorf("pinned Ego request failed (%s, %s); no automatic replay", failure.Code, phase)
		}
		return nil, fmt.Errorf("pinned Ego request failed; no automatic replay; inspect live state for writes: %w", runErr)
	}
	raw, err := os.ReadFile(input.Out)
	if err != nil {
		return nil, err
	}
	var result struct {
		Status     int               `json:"status"`
		Headers    map[string]string `json:"headers"`
		Body       string            `json:"body"`
		BodyBase64 *string           `json:"bodyBase64"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	if result.Status < 100 || result.Status > 599 {
		return nil, fmt.Errorf("browser returned no valid HTTP response")
	}
	headers := http.Header{}
	for k, v := range result.Headers {
		headers.Set(k, v)
	}
	body := []byte(result.Body)
	if result.BodyBase64 != nil {
		body, err = base64.StdEncoding.DecodeString(*result.BodyBase64)
		if err != nil {
			return nil, errors.New("invalid binary browser response")
		}
	}
	return &http.Response{StatusCode: result.Status, Status: fmt.Sprintf("%d %s", result.Status, http.StatusText(result.Status)), Header: headers, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)), Request: req}, nil
}
