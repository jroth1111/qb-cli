package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	pageproto "github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

type loginStep string

const (
	stepApp            loginStep = "app"
	stepUsername       loginStep = "username"
	stepPassword       loginStep = "password"
	stepPasswordChoice loginStep = "password-choice"
	stepCompanyChoice  loginStep = "company-choice"
	stepTOTP           loginStep = "totp"
	stepAuthenticator  loginStep = "authenticator"
	stepUnknown        loginStep = "unknown"
	stepChallenge      loginStep = "challenge"
)

type loginSnapshot struct {
	URL            string    `json:"url"`
	Step           loginStep `json:"step"`
	Fields         []string  `json:"fields,omitempty"`
	Buttons        []string  `json:"buttons,omitempty"`
	Heading        string    `json:"heading,omitempty"`
	VisibleCaptcha bool      `json:"visible_captcha,omitempty"`
	VisibleAlert   bool      `json:"visible_alert,omitempty"`
	Frames         []string  `json:"frames,omitempty"`
}

// This inventory excludes input values and unrestricted page text.
const loginSnapshotJS = `(() => {
 const visible = e => e && !e.disabled && e.getClientRects().length > 0;
 const pick = s => [...document.querySelectorAll(s)].filter(visible);
 const text = (document.body?.innerText || '').toLowerCase();
 const label=s=>String(s||'').replace(/[\w.+-]+@[\w.-]+\.[a-z]{2,}/ig,'[email]').replace(/\s+/g,' ').trim().slice(0,100);
 const out = {url: location.href, step: 'unknown',
 fields:[...document.querySelectorAll('input')].filter(visible).map(e=>label([e.id,e.type,e.autocomplete].join(':'))),
 buttons:[...document.querySelectorAll('button,input[type="submit"]')].filter(visible).map(e=>label([e.id,e.type,e.tagName==='BUTTON'?e.innerText:'submit'].join(':'))),
 heading:label(document.querySelector('h1,h2,[role="heading"]')?.innerText),
 visible_captcha:[...document.querySelectorAll('iframe')].some(e=>visible(e)&&/captcha|recaptcha|hcaptcha/.test(e.src)),
 visible_alert:[...document.querySelectorAll('[role="alert"]')].some(e=>visible(e)&&e.innerText.trim()),
 frames:[...document.querySelectorAll('iframe')].filter(visible).map(e=>{try{const u=new URL(e.src);return u.origin+u.pathname}catch(_){return 'relative-frame'}}),
 };
 const blocked = /verify you are human|complete.{0,20}captcha|unusual activity|account.{0,20}locked|too many attempts|incorrect password|wrong password|approve.{0,30}(phone|device)|check.{0,30}(phone|email)/.test(text) ||
  [...document.querySelectorAll('iframe')].some(e => visible(e) && /captcha|recaptcha|hcaptcha/.test(e.src)) ||
  [...document.querySelectorAll('[role="alert"]')].some(e => visible(e) && e.innerText.trim());
 if (blocked) {out.step='challenge'; return out;}
 if (/choose your company/.test(text)) {out.step='company-choice';return out;}
 if (pick('#ius-password, #iux-password-field, input[type="password"]').length === 1) out.step='password';
 else if (pick('#ius-userid, #iux-identifier-field, #iux-identifier-first-international-email-user-id-input, input[autocomplete="username"], input[type="email"]').length === 1) out.step='username';
 else if (pick('#ius-mfa-confirm-code, #iux-mfa-soft-token-verification-code, input[autocomplete="one-time-code"]').length === 1) {
  out.step = (pick('#iux-mfa-soft-token-verification-code').length===1 || /authenticator|authentication app/.test(text)) && !/(code|text).{0,40}(sent|email|sms)|sent.{0,40}(phone|email)/.test(text) ? 'totp' : 'challenge';
 } else if ([...document.querySelectorAll('button')].some(e=>visible(e)&&/^enter password(?:[\s,]|$)/i.test(e.innerText.trim()))) out.step='password-choice';
 else if ([...document.querySelectorAll('button')].some(e => visible(e) && /^(use )?(an? )?authenticator app(?:[\s,]|$)/i.test(e.innerText.trim()))) out.step='authenticator';
 return out;
})()`

type loginDriver interface {
	snapshot(context.Context) (loginSnapshot, error)
	submit(context.Context, loginStep, string) error
	chooseAuthenticator(context.Context) error
	choosePassword(context.Context) error
	chooseCompany(context.Context) error
}

type managedLoginDriver struct {
	ctx       context.Context
	authorize func() error
	company   string
}

// Keep optional WebAuthn requests cancellable when the operator has selected
// password recovery. This only aborts a request; it never supplies a credential,
// reports authentication success, or changes the server's MFA requirements.
const managedPasskeyCancellationJS = `(() => {
 let owner=window;
 try{while(owner.parent!==owner&&owner.parent.location.origin==='https://accounts.intuit.com')owner=owner.parent;}catch(_){}
 if(location.origin!=='https://accounts.intuit.com'&&!(location.href==='about:blank'&&owner!==window))return;
 if(Object.prototype.hasOwnProperty.call(window,'__qbPasskeyCancellationInstalled'))return;
 const credentials=navigator.credentials;if(!credentials)return;
	 const original=credentials.get;const pending=new Set();let passwordSelected=false;let started=0;const waiters=new Set();
	 const state=owner.__qbPasskeyCancellation||{waitForRequest:timeout=>started?Promise.resolve():new Promise(resolve=>{const done=()=>{clearTimeout(timer);waiters.delete(done);resolve()};const timer=setTimeout(done,timeout);waiters.add(done)}),track:request=>{started++;pending.add(request);for(const done of [...waiters])done();if(passwordSelected)request.controller.abort();},remove:request=>pending.delete(request),cancel:async()=>{passwordSelected=true;const requests=[...pending];
   for(const request of requests)request.controller.abort();
   await Promise.allSettled(requests.map(request=>request.promise));return requests.length;}};
 if(!owner.__qbPasskeyCancellation)Object.defineProperty(owner,'__qbPasskeyCancellation',{value:state});
 if(window!==owner)Object.defineProperty(window,'__qbPasskeyCancellation',{value:state});
 Object.defineProperty(window,'__qbPasskeyCancellationInstalled',{value:true});
 credentials.get=function(options){
   if(!options?.publicKey)return Reflect.apply(original,this,[options]);
   const controller=new AbortController();
   const signal=options.signal?AbortSignal.any([options.signal,controller.signal]):controller.signal;
   const request={controller,promise:null};
   request.promise=Promise.resolve().then(()=>Reflect.apply(original,this,[{...options,signal}])).finally(()=>state.remove(request));
   state.track(request);return request.promise;
 };
})()`

func (d managedLoginDriver) chooseCompany(ctx context.Context) error {
	if d.authorize != nil {
		if err := d.authorize(); err != nil {
			return err
		}
	}
	company, _ := json.Marshal(d.company)
	var marked bool
	script := `(()=>{if(location.origin!=='https://accounts.intuit.com')return false;
 const name=` + string(company) + `;const candidates=[...document.querySelectorAll('li button')].filter(e=>!e.disabled&&e.getClientRects().length&&!e.getAttribute('aria-label')?.startsWith('Actions for')&&e.name!=='buttonToOpen');
 const selected=name?candidates.filter(e=>e.innerText.trim()===name):candidates;
 if(selected.length!==1)return false;selected[0].setAttribute('data-qb-auth-company','true');return true;})()`
	if chromedp.Run(d.ctx, chromedp.Evaluate(script, &marked)) != nil || !marked {
		return fmt.Errorf("%w: company selection is ambiguous", ErrRecoveryAttention)
	}
	return chromedp.Run(d.ctx, chromedp.Click(`[data-qb-auth-company="true"]`, chromedp.ByQuery))
}

func (d managedLoginDriver) snapshot(ctx context.Context) (loginSnapshot, error) {
	var s loginSnapshot
	if d.authorize != nil {
		if err := d.authorize(); err != nil {
			return s, err
		}
	}
	if ctx.Err() != nil {
		return s, ctx.Err()
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(loginSnapshotJS, &s)); err != nil {
		return s, ErrRecoveryAttention
	}
	if AuthenticatedURL(s.URL) {
		s.Step = stepApp
	}
	if evidence := loginEvidence(ctx); evidence != nil {
		copy := s
		if u, err := url.Parse(s.URL); err == nil {
			copy.URL = u.Scheme + "://" + u.Host + u.Path
		}
		evidence.LastSnapshot = &copy
	}
	return s, nil
}

func accountOrigin(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host == "accounts.intuit.com" && u.User == nil
}

func (d managedLoginDriver) submit(ctx context.Context, step loginStep, value string) error {
	if d.authorize != nil {
		if err := d.authorize(); err != nil {
			return err
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	selectors := map[loginStep]string{
		stepUsername: `#ius-userid, #iux-identifier-field, #iux-identifier-first-international-email-user-id-input, input[autocomplete="username"], input[type="email"]`,
		stepPassword: `#ius-password, #iux-password-field, input[type="password"]`,
		stepTOTP:     `#ius-mfa-confirm-code, #iux-mfa-soft-token-verification-code, input[autocomplete="one-time-code"]`,
	}
	selector, ok := selectors[step]
	if !ok {
		return ErrRecoveryAttention
	}
	arg, _ := json.Marshal(selector)
	script := `(() => {
 if (location.origin !== 'https://accounts.intuit.com') return false;
 const selector = ` + string(arg) + `;
 const visible=e=>e && !e.disabled && e.getClientRects().length>0;
 const inputs=[...document.querySelectorAll(selector)].filter(visible);
 if (inputs.length !== 1) return false;
 const el=inputs[0];
 const scope=el.form || document;
 const buttons=[...scope.querySelectorAll('button[type="submit"], #ius-userid-submit-btn, #ius-sign-in-submit-btn, #ius-mfa-code-submit-btn')].filter(visible);
 if (buttons.length !== 1) return false;
 document.querySelectorAll('[data-qb-auth-input]').forEach(e=>e.removeAttribute('data-qb-auth-input'));
 el.setAttribute('data-qb-auth-input','true');el.value='';
 document.querySelectorAll('[data-qb-auth-submit]').forEach(e=>e.removeAttribute('data-qb-auth-submit'));
 buttons[0].setAttribute('data-qb-auth-submit','true'); return true;
})()`
	var submitted bool
	if chromedp.Run(d.ctx, chromedp.Evaluate(script, &submitted)) != nil || !submitted {
		return fmt.Errorf("%w: %s field/submit control could not be identified", ErrRecoveryAttention, step)
	}
	if err := chromedp.Run(d.ctx, chromedp.SendKeys(`[data-qb-auth-input="true"]`, value, chromedp.ByQuery)); err != nil {
		return fmt.Errorf("%w: %s input failed", ErrRecoveryAttention, step)
	}
	if err := chromedp.Run(d.ctx, chromedp.Click(`[data-qb-auth-submit="true"]`, chromedp.ByQuery)); err != nil {
		return fmt.Errorf("%w: %s submit click failed", ErrRecoveryAttention, step)
	}
	return nil
}

func (d managedLoginDriver) chooseAuthenticator(ctx context.Context) error {
	if d.authorize != nil {
		if err := d.authorize(); err != nil {
			return err
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	var clicked bool
	script := `(() => {if(location.origin!=='https://accounts.intuit.com') return false;
 const buttons=[...document.querySelectorAll('button')].filter(e=>!e.disabled && e.getClientRects().length && /^(use )?(an? )?authenticator app(?:[\s,]|$)/i.test(e.innerText.trim()));
 if(buttons.length!==1) return false;buttons[0].click();return true;})()`
	if chromedp.Run(d.ctx, chromedp.Evaluate(script, &clicked)) != nil || !clicked {
		return ErrRecoveryAttention
	}
	return nil
}

func (d managedLoginDriver) choosePassword(ctx context.Context) error {
	if d.authorize != nil {
		if err := d.authorize(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var selected struct {
		Ready     bool `json:"ready"`
		Cancelled int  `json:"cancelled"`
	}
	script := `(async()=>{if(location.origin!=='https://accounts.intuit.com')return false;
 const buttons=[...document.querySelectorAll('button')].filter(e=>!e.disabled&&e.getClientRects().length&&/^enter password( |,|$)/i.test(e.innerText.replace(/\s+/g,' ').trim()));
 if(buttons.length!==1)return false;
 if(!window.__qbPasskeyCancellation)return false;
 // The site's automatic method selection runs after rendering its chooser.
 // Wait for that optional browser request before selecting another method;
 // otherwise its late completion overwrites the password selection.
 await window.__qbPasskeyCancellation.waitForRequest(10000);
 const cancelled=await window.__qbPasskeyCancellation.cancel();
 await new Promise(resolve=>requestAnimationFrame(()=>requestAnimationFrame(resolve)));
 const live=[...document.querySelectorAll('button')].filter(e=>!e.disabled&&e.getClientRects().length&&/^enter password( |,|$)/i.test(e.innerText.replace(/\s+/g,' ').trim()));
 if(live.length!==1)return {ready:false,cancelled};
 live[0].click();return {ready:true,cancelled};})()`
	if chromedp.Run(d.ctx, chromedp.Evaluate(script, &selected, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })) != nil || !selected.Ready {
		return fmt.Errorf("%w: password method button could not be identified", ErrRecoveryAttention)
	}
	if evidence := loginEvidence(ctx); evidence != nil {
		evidence.PasskeyRequestsCancelled += selected.Cancelled
	}
	return nil
}

func waitLoginStep(ctx context.Context, driver loginDriver, previous loginStep) (loginSnapshot, error) {
	wctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		s, err := driver.snapshot(wctx)
		if err != nil {
			return s, err
		}
		if s.Step == stepApp {
			return s, nil
		}
		if s.URL != "about:blank" && !accountOrigin(s.URL) {
			if u, e := url.Parse(s.URL); e != nil || u.Scheme != "https" || u.Host != "qbo.intuit.com" || u.User != nil {
				return s, ErrRecoveryAttention
			}
		}
		if accountOrigin(s.URL) && s.Step != stepUnknown && s.Step != previous {
			return s, nil
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-wctx.Done():
			timer.Stop()
			if ctx.Err() != nil {
				return s, ctx.Err()
			}
			return s, ErrRecoveryAttention
		case <-timer.C:
		}
	}
}

func authenticateManaged(ctx context.Context, driver loginDriver, secrets *RecoverySecrets) error {
	s, err := waitLoginStep(ctx, driver, stepUnknown)
	if err != nil {
		return err
	}
	if s.Step == stepApp {
		return nil
	}
	if secrets == nil {
		if s.Step == stepCompanyChoice {
			if err := driver.chooseCompany(ctx); err != nil {
				return err
			}
			s, err = waitLoginStep(ctx, driver, s.Step)
			if err != nil {
				return err
			}
			if s.Step == stepApp {
				return nil
			}
		}
		if s.Step == stepUsername || s.Step == stepPassword || s.Step == stepPasswordChoice || s.Step == stepTOTP || s.Step == stepAuthenticator {
			return ErrRemintNeedsLogin
		}
		return ErrRecoveryAttention
	}
	totp, err := parseTOTP(secrets.TOTP)
	if err != nil {
		return err
	}
	used := map[loginStep]bool{}
	for range 7 {
		if s.Step == stepApp {
			return nil
		}
		if !accountOrigin(s.URL) || s.Step == stepChallenge || used[s.Step] {
			return ErrRecoveryAttention
		}
		used[s.Step] = true
		switch s.Step {
		case stepUsername:
			err = driver.submit(ctx, s.Step, secrets.Username)
		case stepPassword:
			err = driver.submit(ctx, s.Step, secrets.Password)
		case stepPasswordChoice:
			err = driver.choosePassword(ctx)
		case stepCompanyChoice:
			err = driver.chooseCompany(ctx)
		case stepAuthenticator:
			if totp == nil {
				return ErrRecoveryAttention
			}
			err = driver.chooseAuthenticator(ctx)
		case stepTOTP:
			if totp == nil {
				return ErrRecoveryAttention
			}
			now := time.Now()
			remaining := time.Duration(totp.Period)*time.Second - time.Duration(now.UnixNano()%(int64(totp.Period)*int64(time.Second)))
			if remaining < 5*time.Second {
				timer := time.NewTimer(remaining + 100*time.Millisecond)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			}
			code, e := totp.code(time.Now())
			if e != nil {
				return e
			}
			err = driver.submit(ctx, s.Step, code)
		default:
			return ErrRecoveryAttention
		}
		if err != nil {
			return err
		}
		if evidence := loginEvidence(ctx); evidence != nil && s.Step != stepAuthenticator && s.Step != stepPasswordChoice && s.Step != stepCompanyChoice {
			evidence.CredentialPurposes = append(evidence.CredentialPurposes, string(s.Step))
		}
		s, err = waitLoginStep(ctx, driver, s.Step)
		if err != nil {
			return err
		}
	}
	return ErrRecoveryAttention
}

func withManagedBrowser(ctx context.Context, run func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(ctx, managedRemintCeiling)
	defer cancel()
	if IsHarness() || os.Getenv(managedDisableEnv) == "1" {
		return ErrRemintNeedsLogin
	}
	unlock, err := LockProfile(ctx, HomeDir(), "browser")
	if err != nil {
		return err
	}
	defer unlock()
	if err = prepareManagedProfile(); err != nil {
		return err
	}
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	headed, _ := ctx.Value(managedHeadedKey{}).(bool)
	opts = append(opts, chromedp.UserDataDir(ManagedProfileDir()), chromedp.Flag("headless", !headed))
	alloc, stop := chromedp.NewExecAllocator(ctx, opts...)
	defer stop()
	page, closePage := chromedp.NewContext(alloc)
	defer closePage()
	if evidence := loginEvidence(ctx); evidence != nil {
		listenerCtx, stopListening := context.WithCancel(page)
		var workers sync.WaitGroup
		var listenerMu sync.Mutex
		closed := false
		defer func() {
			stopListening()
			listenerMu.Lock()
			closed = true
			listenerMu.Unlock()
			workers.Wait()
		}()
		chromedp.ListenTarget(listenerCtx, func(event any) {
			listenerMu.Lock()
			defer listenerMu.Unlock()
			if closed {
				return
			}
			switch e := event.(type) {
			case *network.EventRequestWillBeSent:
				u, err := url.Parse(e.Request.URL)
				if err == nil && u.Hostname() == "accounts.intuit.com" && u.Path == "/identity-api/signin/graphql" {
					requestID := e.RequestID
					workers.Go(func() {
						requestCtx, cancel := context.WithTimeout(page, 5*time.Second)
						defer cancel()
						body, err := network.GetRequestPostData(requestID).Do(cdp.WithExecutor(requestCtx, chromedp.FromContext(page).Target))
						if err != nil {
							return
						}
						var payload struct {
							OperationName string         `json:"operationName"`
							Query         string         `json:"query"`
							Variables     map[string]any `json:"variables"`
						}
						if json.Unmarshal([]byte(body), &payload) != nil {
							return
						}
						operation := payload.OperationName
						if operation == "" {
							operation = regexp.MustCompile(`\bidentity[A-Za-z]+`).FindString(payload.Query)
						}
						requestSummary := loginResponse{Path: "graphql-request", RequestID: string(requestID), Operation: operation}
						var describe func(any, string, int)
						describe = func(value any, path string, depth int) {
							if depth > 5 {
								return
							}
							switch v := value.(type) {
							case map[string]any:
								for key, item := range v {
									next := path + "." + key
									requestSummary.Schema = append(requestSummary.Schema, next)
									if s, ok := item.(string); ok && (key == "credentialType" || key == "optionType" || key == "type") {
										if s == "FIDO2" || s == "PASSWORD" || s == "TOTP" {
											requestSummary.Enums = append(requestSummary.Enums, next+"="+s)
										}
									}
									describe(item, next, depth+1)
								}
							case []any:
								for _, item := range v {
									describe(item, path+"[]", depth+1)
								}
							}
						}
						describe(payload.Variables, "variables", 0)
						evidence.mu.Lock()
						evidence.Responses = append(evidence.Responses, requestSummary)
						evidence.mu.Unlock()
					})
				}
			case *network.EventResponseReceived:
				u, err := url.Parse(e.Response.URL)
				if err == nil && u.Hostname() == "accounts.intuit.com" {
					evidence.mu.Lock()
					if len(evidence.Responses) < 80 {
						path := "other"
						if u.Path == "/app/sign-in" || u.Path == "/identity-api/signin/graphql" {
							path = u.Path
						}
						evidence.Responses = append(evidence.Responses, loginResponse{Path: path, Status: int(e.Response.Status)})
					}
					evidence.mu.Unlock()
					if u.Path == "/identity-api/signin/graphql" {
						requestID := e.RequestID
						workers.Go(func() {
							requestCtx, cancel := context.WithTimeout(page, 5*time.Second)
							defer cancel()
							body, err := network.GetResponseBody(requestID).Do(cdp.WithExecutor(requestCtx, chromedp.FromContext(page).Target))
							if err != nil {
								return
							}
							var value any
							if json.Unmarshal(body, &value) != nil {
								return
							}
							summary := loginResponse{Path: "graphql-schema", Status: int(e.Response.Status), RequestID: string(requestID)}
							var visit func(any, string, int)
							visit = func(value any, path string, depth int) {
								if depth > 7 || len(summary.Schema) > 80 {
									return
								}
								switch v := value.(type) {
								case map[string]any:
									for key, item := range v {
										next := path + "." + key
										summary.Schema = append(summary.Schema, fmt.Sprintf("%s (%T)", next, item))
										if s, ok := item.(string); ok && recordedLoginEnum(key, s) {
											safe := len(s) < 80
											for _, r := range s {
												if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' && r != '-' {
													safe = false
												}
											}
											if safe {
												summary.Enums = append(summary.Enums, next+"="+s)
											}
										}
										visit(item, next, depth+1)
									}
								case []any:
									for index, item := range v {
										if index > 4 {
											break
										}
										visit(item, fmt.Sprintf("%s[%d]", path, index), depth+1)
									}
								}
							}
							visit(value, "response", 0)
							evidence.mu.Lock()
							evidence.Responses = append(evidence.Responses, summary)
							evidence.mu.Unlock()
						})
					}
				}
			case *runtime.EventExceptionThrown:
				evidence.mu.Lock()
				evidence.Exceptions++
				evidence.mu.Unlock()
			}
		})
	}
	if err = chromedp.Run(page, chromedp.ActionFunc(func(ctx context.Context) error {
		_, err := pageproto.AddScriptToEvaluateOnNewDocument(managedPasskeyCancellationJS).Do(ctx)
		return err
	}), chromedp.Navigate(BankingCaptureURL)); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("managed browser could not reach QBO")
	}
	return run(page)
}

// Never record arbitrary response codes/messages: a provider can echo input in
// them. Only schema typenames and known authentication enums are diagnostics.
func recordedLoginEnum(key, value string) bool {
	if key == "__typename" {
		return strings.HasPrefix(value, "Identity_")
	}
	if key == "optionType" {
		switch value {
		case "FIDO2", "PASSWORD", "TIME_BASED_ONE_TIME_PASSWORD", "TEXT_MESSAGE_ONE_TIME_PASSWORD", "VOICE_CALL_ONE_TIME_PASSWORD", "TEXT_MESSAGE_IDENTITY_PROOFING":
			return true
		}
	}
	if key == "successCode" {
		switch value {
		case "SOMETHING_ELSE_REQUIRED", "NOTHING_ELSE_REQUIRED", "FIDO2_SUCCESS":
			return true
		}
	}
	return false
}

func waitManagedIdentity(ctx context.Context, expected *TokenSet) error {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		var identity Identity
		if chromedp.Run(ctx, chromedp.Evaluate(IdentityJS, &identity)) != nil {
			return ErrSessionIdentityUnverified
		}
		if identity.Realm != "" && identity.Email != "" {
			if identity.Realm != expected.RealmID || !strings.EqualFold(identity.Email, expected.Email) {
				return ErrSessionChanged
			}
			return nil
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ErrSessionIdentityUnverified
		case <-timer.C:
		}
	}
}

func captureManagedRecovery(ctx context.Context, expected *TokenSet, secrets *RecoverySecrets) (*ATSCapture, error) {
	grantID, _ := ctx.Value(recoveryGrantKey{}).(string)
	if secrets != nil && grantID == "" {
		return nil, ErrRecoveryDisabled
	}
	trace := &LoginEvidence{}
	ctx = context.WithValue(ctx, loginEvidenceKey{}, trace)
	var cap *ATSCapture
	err := withManagedBrowser(ctx, func(page context.Context) error {
		driver := managedLoginDriver{ctx: page, company: expected.CompanyName}
		if secrets != nil {
			driver.authorize = func() error {
				current, err := Load()
				r, rerr := loadRecovery()
				if err != nil || rerr != nil || !SameSession(expected, current) || current.CredentialGeneration != expected.CredentialGeneration || !r.Enabled || r.ID != grantID || !recoveryBound(r, current) {
					return ErrSessionChanged
				}
				return nil
			}
		}
		if err := authenticateManaged(page, driver, secrets); err != nil {
			// A keeper may have rotated exported cookies while Chromium was
			// closed. Restore them only after observing a sign-in wall, never
			// over a live browser identity or a newer explicit login.
			if secrets != nil || !errors.Is(err, ErrRemintNeedsLogin) {
				return err
			}
			if err = restoreManagedCookies(page, expected); err != nil {
				return err
			}
			trace.CookieRestore = true
			if err = authenticateManaged(page, managedLoginDriver{ctx: page}, nil); err != nil {
				return err
			}
		}
		headers, secondary, hosts, err := interceptManagedATS(page, BankingCaptureURL)
		if err != nil {
			return err
		}
		var identity Identity
		if chromedp.Run(page, chromedp.Evaluate(IdentityJS, &identity)) != nil {
			return ErrSessionIdentityUnverified
		}
		cookies, err := snapshotManagedCookies(page)
		if err != nil {
			return ErrSessionIdentityUnverified
		}
		cap = &ATSCapture{Headers: headers, SecondaryHeaders: secondary, HostHeaders: hosts, Identity: identity, Cookies: cookies, Evidence: trace}
		if err = verifyRecoveryIdentity(expected, cap); err != nil {
			return err
		}
		trace.IdentityVerified = true
		if err = proveManagedCompany(page, expected, cap); err != nil {
			return err
		}
		trace.APIProof = true
		return nil
	})
	if err != nil {
		return cap, &LoginFailure{Cause: err, Evidence: trace}
	}
	return cap, nil
}

func restoreManagedCookies(ctx context.Context, expected *TokenSet) error {
	current, err := Load()
	if err != nil || !SameSession(expected, current) || current.Source != "managed-profile" || current.CredentialGeneration != expected.CredentialGeneration {
		return ErrSessionChanged
	}
	var cookies []*network.CookieParam
	for _, cookie := range current.Cookies {
		if cookie.Name == "" || cookie.Value == "" || !intuitHost(cookieDomain(cookie)) || !cookie.Expires.IsZero() && !cookie.Expires.After(time.Now()) {
			continue
		}
		param := &network.CookieParam{Name: cookie.Name, Value: cookie.Value, Path: cookiePath(cookie), Secure: cookie.Secure, HTTPOnly: cookie.HTTPOnly}
		if cookie.HostOnly || !strings.HasPrefix(cookie.Domain, ".") {
			param.URL = "https://" + cookieDomain(cookie) + cookiePath(cookie)
		} else {
			param.Domain = cookie.Domain
		}
		if !cookie.Expires.IsZero() {
			expiry := cdp.TimeSinceEpoch(cookie.Expires)
			param.Expires = &expiry
		}
		cookies = append(cookies, param)
	}
	if len(cookies) == 0 {
		return ErrRemintNeedsLogin
	}
	if chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error { return network.SetCookies(cookies).Do(ctx) }), chromedp.Navigate(BankingCaptureURL)) != nil {
		return ErrRecoveryAttention
	}
	return nil
}

func proveManagedCompany(ctx context.Context, expected *TokenSet, cap *ATSCapture) error {
	args, _ := json.Marshal(map[string]any{"url": "https://qbo.intuit.com/api/v3/company/" + url.PathEscape(expected.RealmID) + "/query?minorversion=73&query=select%20*%20from%20CompanyInfo%20maxresults%201", "headers": map[string]string{"Authorization": headerGet(cap.Headers, "authorization"), "intuit-company-id": expected.RealmID, "Accept": "application/json"}})
	script := `(async()=>{try {const arg=` + string(args) + `; if(location.origin!=='https://qbo.intuit.com')return false;
 const r=await fetch(arg.url,{method:'GET',headers:arg.headers,credentials:'include',redirect:'error'});
 if(r.status!==200)return false; const b=await r.json();
 const rows=b?.QueryResponse?.CompanyInfo; return Array.isArray(rows) && rows.length===1 && typeof rows[0]?.CompanyName==='string' && rows[0].CompanyName.length>0;
 }catch(_){return false;}})()`
	var ok bool
	if chromedp.Run(ctx, chromedp.Evaluate(script, &ok, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })) != nil || !ok {
		return ErrSessionIdentityUnverified
	}
	var after Identity
	if chromedp.Run(ctx, chromedp.Evaluate(IdentityJS, &after)) != nil || after.Realm != expected.RealmID || !strings.EqualFold(after.Email, expected.Email) {
		return ErrSessionChanged
	}
	return nil
}
