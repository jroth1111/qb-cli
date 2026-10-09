package auth

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"
	"time"

	pageproto "github.com/chromedp/cdproto/page"
	cdpruntime "github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

func TestManagedPasswordSelectionCancelsOptionalPasskey(t *testing.T) {
	if os.Getenv("QB_BROWSER_TEST") != "1" || IsHarness() {
		t.Skip("set QB_BROWSER_TEST=1 for isolated browser proof")
	}
	// All connections terminate at this fixture, never the real account site.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/sessionextender" {
			w.Header().Set("Content-Type", "application/json")
			if r.URL.Query().Get("deny") == "true" {
				fmt.Fprint(w, `{"responses":[{"decision":"DENY"}]}`)
			} else {
				fmt.Fprint(w, `{"responses":[{"decision":"PERMIT"}]}`)
			}
			return
		}
		if r.Host == "qbo.intuit.com" {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, `<h1>Native ticket fixture</h1><script>
const ticket={lastExtendedTime:new Date(Date.now()-60000),expiryTime:new Date(Date.now()+1560000),onTicketRenewalHandler(){this.lastExtendedTime=new Date;this.expiryTime=new Date(Date.now()+1560000)},async pingServer(force){const r=await fetch('/sessionextender?deny='+!!window.deny,{method:'POST',body:JSON.stringify({resource:'sessionextender',action:'read'})});const result=await r.json();if(result.responses[0].decision==='PERMIT')this.onTicketRenewalHandler();else this.lastExtendedTime=new Date;}};
const readyAt=Date.now()+100;
window.require=(ids,resolve)=>resolve(Date.now()<readyAt?{initialize(){throw new Error('must not initialize a stub')}}:{getCorePlugin:async()=>ticket});
</script>`)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<h1>Verify it's you</h1><form><button type="submit">Use passkey</button><button type="submit" id="password">Enter password ,</button></form><script>
document.querySelector('form').onsubmit=e=>e.preventDefault();
document.querySelector('#password').onclick=e=>{e.preventDefault();document.body.dataset.trusted=String(e.isTrusted);document.body.dataset.cancelled=String(window.cancelled||0);document.body.innerHTML='<h1>Enter your Intuit password</h1><input type="password">';};
// Render the chooser first, then start the automatic passkey request late.
setTimeout(()=>{const frame=document.createElement('iframe');document.body.append(frame);frame.contentWindow.navigator.credentials.get({publicKey:{challenge:new Uint8Array(16)}}).catch(()=>{});
window.navigator.credentials.get({publicKey:{challenge:new Uint8Array(16)}}).catch(()=>{});},100);
</script>`)
	}))
	t.Cleanup(srv.Close)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			w.WriteHeader(502)
			return
		}
		upstream, err := net.DialTimeout("tcp", srv.Listener.Addr().String(), time.Second)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		defer upstream.Close()
		client, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		go func() { _, _ = io.Copy(upstream, rw) }()
		_, _ = io.Copy(client, upstream)
	}))
	t.Cleanup(proxy.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	t.Cleanup(cancel)
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.UserDataDir(t.TempDir()), chromedp.Flag("ignore-certificate-errors", true), chromedp.Flag("proxy-server", proxy.URL), chromedp.Flag("disable-quic", true), chromedp.Flag("host-resolver-rules", "MAP * 127.0.0.1, EXCLUDE localhost"))
	alloc, stop := chromedp.NewExecAllocator(ctx, opts...)
	t.Cleanup(stop)
	page, closePage := chromedp.NewContext(alloc)
	t.Cleanup(closePage)
	// Flush Chrome's profile writers before TempDir cleanup; killing the main
	// process can leave a renderer briefly writing into Default under -race.
	t.Cleanup(func() { _ = chromedp.Cancel(page) })
	// The fixture models a browser-owned request pending until its AbortSignal
	// fires. No credential or server authentication result is fabricated.
	mock := `(()=>{if(location.origin!=='https://accounts.intuit.com'&&location.href!=='about:blank')return;
 navigator.credentials.get=function(options){return new Promise((resolve,reject)=>{if(!options?.publicKey){resolve(null);return;}const abort=()=>{window.top.cancelled=(window.top.cancelled||0)+1;reject(options.signal.reason)};if(options.signal?.aborted)abort();else options.signal?.addEventListener('abort',abort,{once:true});});};})()`
	if err := chromedp.Run(page, chromedp.ActionFunc(func(ctx context.Context) error {
		if _, err := pageproto.AddScriptToEvaluateOnNewDocument(mock).Do(ctx); err != nil {
			return err
		}
		_, err := pageproto.AddScriptToEvaluateOnNewDocument(managedPasskeyCancellationJS).Do(ctx)
		return err
	}), chromedp.Navigate("https://accounts.intuit.com/app/sign-in")); err != nil {
		t.Fatal(err)
	}
	trace := &LoginEvidence{}
	page = context.WithValue(page, loginEvidenceKey{}, trace)
	driver := managedLoginDriver{ctx: page}
	if err := driver.choosePassword(page); err != nil {
		t.Fatal(err)
	}
	var state struct {
		Trusted   string `json:"trusted"`
		Cancelled string `json:"cancelled"`
		Password  bool   `json:"password"`
	}
	if err := chromedp.Run(page, chromedp.Evaluate(`({trusted:document.body.dataset.trusted,cancelled:document.body.dataset.cancelled,password:!!document.querySelector('input[type=password]')})`, &state)); err != nil {
		t.Fatal(err)
	}
	if state.Cancelled != "2" || !state.Password || trace.PasskeyRequestsCancelled != 2 {
		t.Fatalf("passkey cancellation/password transition failed: %+v, count=%d", state, trace.PasskeyRequestsCancelled)
	}
	if err := chromedp.Run(page, chromedp.Evaluate(`document.body.innerHTML='<h1>We need to double-check your identity</h1><button type="submit">Authenticator app ,</button>';document.querySelector('button').onclick=()=>{document.body.innerHTML='<h1>Enter your verification code</h1><form><input id="iux-mfa-soft-token-verification-code" autocomplete="off"><button type="submit">Continue</button></form>';document.querySelector('form').onsubmit=e=>{e.preventDefault();document.body.dataset.codeAccepted=String(document.querySelector('input').value==='123456');};}`, nil)); err != nil {
		t.Fatal(err)
	}
	snapshot, err := driver.snapshot(page)
	if err != nil || snapshot.Step != stepAuthenticator {
		t.Fatal("decorated authenticator label was not recognized")
	}
	if err = driver.chooseAuthenticator(page); err != nil {
		t.Fatal(err)
	}
	snapshot, err = driver.snapshot(page)
	if err != nil || snapshot.Step != stepTOTP {
		t.Fatal("authenticator selection did not reach the TOTP input")
	}
	if err = driver.submit(page, stepTOTP, "123456"); err != nil {
		t.Fatal(err)
	}
	var codeAccepted bool
	if err = chromedp.Run(page, chromedp.Evaluate(`document.body.dataset.codeAccepted==='true'`, &codeAccepted)); err != nil || !codeAccepted {
		t.Fatal("native TOTP input/submission failed")
	}
	if err = chromedp.Run(page, chromedp.Evaluate(`document.body.innerHTML='<div role="alert">We simplified the way you sign in to QuickBooks<br>Access all your products from one sign in page.</div><input id="iux-identifier-first-international-email-user-id-input"><iframe style="height:60px;width:300px;border:0" src="https://accounts.intuit.com/recaptcha"></iframe>'`, nil)); err != nil {
		t.Fatal(err)
	}
	snapshot, err = driver.snapshot(page)
	if err != nil || snapshot.Step != stepUsername || snapshot.VisibleCaptcha || snapshot.VisibleAlert {
		t.Fatal("informational banner or passive captcha badge blocked login")
	}
	if err = chromedp.Run(page, chromedp.Evaluate(`(()=>{const notice=document.querySelector('[role=alert]');notice.className='PageMessage-info-fixture';notice.innerText='General information about your account.'})()`, nil)); err != nil {
		t.Fatal(err)
	}
	snapshot, err = driver.snapshot(page)
	if err != nil || snapshot.Step != stepUsername || snapshot.VisibleAlert {
		t.Fatal("informational banner was not ignored")
	}
	if err = chromedp.Run(page, chromedp.Evaluate(`(()=>{const notice=document.querySelector('[role=alert]');notice.className='PageMessage-error-fixture';notice.innerText='Unexpected verification required'})()`, nil)); err != nil {
		t.Fatal(err)
	}
	snapshot, err = driver.snapshot(page)
	if err != nil || snapshot.Step != stepChallenge || !snapshot.VisibleAlert {
		t.Fatal("unknown alert was ignored")
	}
	if err = chromedp.Run(page, chromedp.Evaluate(`document.querySelector('[role=alert]').remove()`, nil)); err != nil {
		t.Fatal(err)
	}
	if err = chromedp.Run(page, chromedp.Evaluate(`document.querySelector('iframe').style.height='200px'`, nil)); err != nil {
		t.Fatal(err)
	}
	snapshot, err = driver.snapshot(page)
	if err != nil || snapshot.Step != stepChallenge || !snapshot.VisibleCaptcha {
		t.Fatal("interactive captcha was not rejected")
	}
	if err = chromedp.Run(page, chromedp.Evaluate(`document.body.innerHTML='<h1>Choose your company</h1><ul><li><button>First Company</button></li><li><button>Second Company</button></li></ul>';document.querySelectorAll('li button').forEach(e=>e.onclick=()=>document.body.dataset.chosenCompany=e.innerText);`, nil)); err != nil {
		t.Fatal(err)
	}
	selectionPage := context.WithValue(page, bootstrapCompanyKey{}, RecoveryOptions{ChooseCompany: func(_ context.Context, choices []string) (string, error) { return choices[1], nil }})
	if err = (managedLoginDriver{ctx: selectionPage}).chooseCompany(selectionPage); err != nil {
		t.Fatal(err)
	}
	var selectedCompany string
	if err = chromedp.Run(page, chromedp.Evaluate(`document.body.dataset.chosenCompany`, &selectedCompany)); err != nil || selectedCompany != "Second Company" {
		t.Fatal("dialog selection was not applied")
	}
	if err = chromedp.Run(page, chromedp.Evaluate(`document.body.innerHTML='<h1>Sign in</h1><button>owner@example.invalid , Last accessed on this device</button><button>other@example.invalid</button><button>Use a different account ,</button>';document.querySelectorAll('button').forEach(e=>e.onclick=()=>document.body.dataset.chosenAccount=e.innerText);`, nil)); err != nil {
		t.Fatal(err)
	}
	snapshot, err = driver.snapshot(page)
	if err != nil || snapshot.Step != stepAccountChoice {
		t.Fatal("remembered account choice was not recognized")
	}
	if err = driver.chooseAccount(page, "owner@example.invalid"); err != nil {
		t.Fatal(err)
	}
	var selectedAccount string
	if err = chromedp.Run(page, chromedp.Evaluate(`document.body.dataset.chosenAccount`, &selectedAccount)); err != nil || selectedAccount != "owner@example.invalid , Last accessed on this device" {
		t.Fatal("wrong remembered account was selected")
	}
	if err = driver.chooseAccount(page, "missing@example.invalid"); err == nil {
		t.Fatal("missing enrolled account did not fail closed")
	}
	if err = chromedp.Run(page, chromedp.Navigate("https://qbo.intuit.com/app/banking")); err != nil {
		t.Fatal(err)
	}
	var native nativeTicketResult
	if err = chromedp.Run(page, chromedp.Evaluate(nativeTicketJS, &native, func(p *cdpruntime.EvaluateParams) *cdpruntime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil || !native.Extended {
		t.Fatal("native SDK PERMIT was not observed")
	}
	if err = chromedp.Run(page, chromedp.Evaluate(`window.deny=true`, nil), chromedp.Evaluate(nativeTicketJS, &native, func(p *cdpruntime.EvaluateParams) *cdpruntime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil || native.Extended {
		t.Fatal("DENY with a changed local timer was accepted as native renewal")
	}
}

// This opt-in smoke uses a throwaway profile and a loopback-only fake site.
// It never reads a real browser profile or sends traffic to Intuit.
func TestManagedATSContinuationInIsolatedBrowser(t *testing.T) {
	if os.Getenv("QB_BROWSER_TEST") != "1" || IsHarness() {
		t.Skip("set QB_BROWSER_TEST=1 for isolated headless Chrome proof")
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ats/v1/company/1/banking/getTransactions" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"ok":true}`)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<body>loading<script>fetch('/ats/v1/company/1/banking/getTransactions',{headers:{Authorization:'Intuit_APIKey fixture-only','intuit-company-id':'1'}}).then(r=>r.json()).then(()=>document.body.innerText='continued')</script></body>`)
	}))
	defer srv.Close()
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect {
			w.WriteHeader(502)
			return
		}
		upstream, err := net.DialTimeout("tcp", srv.Listener.Addr().String(), time.Second)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		defer upstream.Close()
		client, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer client.Close()
		_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		_ = rw.Flush()
		go func() { _, _ = io.Copy(upstream, rw) }()
		_, _ = io.Copy(client, upstream)
	}))
	defer proxy.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	opts = append(opts, chromedp.UserDataDir(t.TempDir()), chromedp.Flag("ignore-certificate-errors", true), chromedp.Flag("proxy-server", proxy.URL), chromedp.Flag("disable-quic", true), chromedp.Flag("host-resolver-rules", "MAP * 127.0.0.1, EXCLUDE localhost"))
	if runtime.GOOS == "darwin" {
		path := "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
		if _, err := os.Stat(path); err != nil {
			t.Skip("Chrome unavailable")
		}
		opts = append(opts, chromedp.ExecPath(path))
	}
	alloc, closeAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer closeAlloc()
	page, closePage := chromedp.NewContext(alloc)
	defer closePage()
	if err := chromedp.Run(page, chromedp.Navigate("about:blank")); err != nil {
		t.Fatal("isolated browser startup failed")
	}
	headers, _, _, err := interceptManagedATS(page, BankingCaptureURL)
	if err != nil || !isIntuitAPIKey(headerGet(headers, "authorization")) {
		t.Fatal("native ATS capture failed on the loopback fixture")
	}
	var text string
	if err = chromedp.Run(page, chromedp.Text("body", &text)); err != nil || text != "continued" {
		t.Fatal("Fetch-paused request was not continued")
	}
}
