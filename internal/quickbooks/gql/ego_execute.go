package gql

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
	"unicode/utf8"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// ErrEgoMissing is returned when the ego-browser CLI is not on PATH.
var ErrEgoMissing = errors.New("ego-browser not found on PATH: install ego lite (ego-browser onboarding)")

// egoSpaceName is the isolated ego-browser task space gql execution drives.
// It inherits the user's QBO login state; the space persists across calls.
const egoSpaceName = "qb-gql"

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
const {outPath,endpoint,method,headers,body,spaceKey,targetKey,expectedRealm,expectedEmail,identityScript,timeoutMs}=request;
function isQBOApp(value) {
 try { const u = new URL(value); return u.origin === 'https://qbo.intuit.com' && !u.username && !u.password && u.pathname.startsWith('/app/') && !value.includes('UNAUTHENTICATED'); }
 catch (_) { return false; }
}
function done(obj) { fs.writeFileSync(outPath, JSON.stringify(obj), {mode:0o600}); }
function fail(msg, code) { try { done({ok:false,error:msg}); } catch (_) {} process.exit(code || 2); }
if (!outPath || !endpoint) fail('missing browser request parameters', 2);
let task;
try{task=await ` + auth.ExistingEgoTaskJS + `(spaceKey,taskSpace,typeof listTaskSpaces==='function'?listTaskSpaces:undefined);}
catch(error){fail(error.qbCode==='user-control'||/user.*control|inactive|unassigned/i.test(String(error))?'user-control':'pinned-page-unavailable',3);}
if(task.ownership!=='agent')fail('user-control',3);
const tabs = await task.tabs();
const candidates = tabs.filter(t => targetKey && (t.targetId === targetKey || t.label === targetKey));
if(candidates.length!==1||!candidates[0].label)fail('pinned-page-unavailable',3);
const page=task.page(candidates[0].label);
const info = await page.info();
if (info && info.dialog) fail('dialog-open', 3);
const url = await page.url();
const location=new URL(url);
if(location.origin==='https://accounts.intuit.com'&&!location.username&&!location.password)fail('need-login',3);
if (!isQBOApp(url)) fail('pinned-page-origin-changed', 3);
const verifyIdentity = async () => {
 const identity = await page.evaluate(identityScript);
 if (!expectedRealm || !expectedEmail || identity.realm !== expectedRealm || (identity.email || '').toLowerCase() !== expectedEmail.toLowerCase()) fail('identity-mismatch', 3);
 if(!isQBOApp(await page.url()))fail('pinned-page-origin-changed',3);
};
await verifyIdentity();
const options = {method, headers, credentials:'include', redirect:'error', cache:'no-store', saveAs:outPath+'.body', timeout:Math.min(30000,timeoutMs)};
if (method !== 'GET' && method !== 'HEAD' && body&&!request.binaryBody) options.body = body;
let result;
try {
 if(request.binaryBody){
  result=await page.evaluate((0,eval)(request.byteFetchScript),{...request,timeoutMs:Math.min(30000,request.timeoutMs)});
  fs.writeFileSync(outPath+'.body',Buffer.from(result.bodyBase64,'base64'),{mode:0o600});
 }else{result = await page.fetch(endpoint, options);}
}
catch (_) { fail('browser fetch failed', 2); }
await verifyIdentity();
fs.chmodSync(outPath+'.body',0o600);
const responseBody = fs.readFileSync(outPath+'.body');
done({ok:true,status:result.status,bodyBase64:responseBody.toString('base64')});
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
	headers := authHeadersFor(endpoint, req.Op.Name)
	if headers == nil {
		headers = map[string]string{}
	}
	headers["content-type"], headers["accept"] = "application/json", "application/json"
	return FetchEgo(ctx, endpoint, "POST", headers, payload)
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
	read := method == http.MethodGet || method == http.MethodHead
	jsonBody := method != http.MethodHead && expectsJSONResponse(headers)
	response, err := retryReadResponse(ctx, read, jsonBody, func() (*Response, error) {
		return fetchEgoOnce(ctx, endpoint, method, headers, payload)
	})
	rejected := response != nil && response.Status == 401 || errors.Is(err, auth.ErrRemintNeedsLogin)
	if !read || !rejected {
		return response, err
	}
	expected := ctx.Value(executionSessionKey{}).(*auth.TokenSet)
	rctx, cancel := context.WithTimeout(ctx, 100*time.Second)
	defer cancel()
	if err = remintGQL(rctx); err != nil {
		return nil, err
	}
	fresh, err := auth.Load()
	if err != nil || !auth.SameBrowserSession(expected, fresh) {
		return nil, auth.ErrSessionChanged
	}
	updated := refreshedBrowserHeaders(expected, fresh, endpoint, headers)
	return retryReadResponse(ctx, true, jsonBody, func() (*Response, error) { return fetchEgoOnce(ctx, endpoint, method, updated, payload) })
}

func fetchEgoOnce(ctx context.Context, endpoint, method string, headers map[string]string, payload []byte) (*Response, error) {
	tok, loadErr := auth.Load()
	if loadErr != nil {
		return nil, loadErr
	}
	if expected, ok := ctx.Value(executionSessionKey{}).(*auth.TokenSet); ok && !auth.SameBrowserSession(expected, tok) {
		return nil, auth.ErrSessionChanged
	}
	if tok.Source == "managed-profile" {
		return fetchManagedSession(ctx, tok, endpoint, method, headers, payload)
	}
	if headers == nil {
		headers = map[string]string{}
	}
	if err := auth.ValidateBrowserDestination(endpoint, tok, headers); err != nil {
		return nil, err
	}
	if tok.Source != "ego-existing" || tok.EgoSpace == "" || tok.EgoTargetID == "" {
		return nil, errors.New("browser transport requires an explicitly pinned Ego login; run qb login --source ego with --ego-space and --target-id")
	}
	if _, err := exec.LookPath("ego-browser"); err != nil {
		return nil, ErrEgoMissing
	}
	if endpoint == "" {
		return nil, fmt.Errorf("browser request requires an endpoint")
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
	space := tok.EgoSpace
	if explicit := os.Getenv("QB_EGO_SPACE_ID"); explicit != "" && explicit != tok.EgoSpace {
		return nil, fmt.Errorf("explicit Ego space differs from captured session")
	}
	if explicit := os.Getenv("QB_EGO_SPACE"); explicit != "" && explicit != tok.EgoSpace {
		return nil, fmt.Errorf("explicit Ego space differs from captured session")
	}
	data, err := json.Marshal(map[string]any{"outPath": outPath, "endpoint": endpoint, "method": method, "headers": headers, "body": string(payload), "bodyBase64": base64.StdEncoding.EncodeToString(payload), "binaryBody": !utf8.Valid(payload), "byteFetchScript": auth.BrowserByteFetchJS, "spaceKey": space, "targetKey": tok.EgoTargetID, "expectedRealm": tok.RealmID, "expectedEmail": tok.Email, "identityScript": auth.IdentityJS, "timeoutMs": timeoutMs})
	if err != nil {
		return nil, errors.New("encoding browser request")
	}
	preamble := "const request = " + string(data) + ";\n"
	cmd := exec.CommandContext(ctx, "ego-browser", "nodejs")
	cmd.Stdin = bytes.NewReader(append([]byte(preamble), []byte(egoExecuteScript)...))
	cmd.Env = append(os.Environ(), "QB_EGO_SPACE="+space, "QB_EGO_OUT="+outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		if bytes.Contains(out, []byte("user has taken control")) || bytes.Contains(out, []byte("space is inactive")) {
			return nil, auth.ErrEgoUserControl
		}
		// The script records its failure reason in the result file before
		// exiting non-zero; prefer it over the bare process error.
		if raw, rerr := os.ReadFile(outPath); rerr == nil {
			var file struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			}
			if jerr := json.Unmarshal(raw, &file); jerr == nil && file.Error != "" {
				return nil, egoExecutionFailure(file.Error)
			}
		}
		return nil, errors.New("ego execution failed; raw provider output withheld; verify state before retrying a write")
	}
	raw, err := os.ReadFile(outPath)
	if err != nil {
		return nil, fmt.Errorf("gql: reading ego result: %w", err)
	}
	var file struct {
		OK         bool    `json:"ok"`
		Status     int     `json:"status"`
		Body       string  `json:"body"`
		BodyBase64 *string `json:"bodyBase64"`
		Error      string  `json:"error"`
	}
	if err := json.Unmarshal(raw, &file); err != nil {
		return nil, fmt.Errorf("gql: parsing ego result: %w", err)
	}
	if !file.OK {
		return nil, egoExecutionFailure(file.Error)
	}
	if file.Status < 100 || file.Status > 599 {
		return nil, errors.New("browser returned no valid HTTP response")
	}
	body := []byte(file.Body)
	if file.BodyBase64 != nil {
		body, err = base64.StdEncoding.DecodeString(*file.BodyBase64)
		if err != nil {
			return nil, errors.New("invalid binary browser response")
		}
	}
	res := &Response{Status: file.Status, Body: json.RawMessage(body)}
	var envelope struct {
		Errors []GraphQLError `json:"errors"`
	}
	if err := json.Unmarshal(res.Body, &envelope); err == nil {
		res.Errors = envelope.Errors
	}
	return res, nil
}

func egoExecutionFailure(code string) error {
	switch code {
	case "need-login":
		return fmt.Errorf("%w: %w", ErrNeedsLogin, auth.ErrRemintNeedsLogin)
	case "user-control":
		return auth.ErrEgoUserControl
	case "dialog-open":
		return auth.ErrRecoveryAttention
	case "pinned-page-unavailable", "pinned-page-origin-changed", "identity-mismatch":
		return auth.ErrSessionChanged
	case "browser fetch failed":
		return errBrowserQueryTransport
	default:
		return errors.New("ego request failed; raw provider output withheld")
	}
}
