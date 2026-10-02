package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// NativeTicketStatus contains no credentials. It is saved with the login so a
// logout/new login naturally discards its scheduling state.
type NativeTicketStatus struct {
	State        NativeTicketState `json:"state"`
	LastAttempt  time.Time         `json:"last_attempt,omitzero"`
	LastExtended time.Time         `json:"last_extended,omitzero"`
	NextDue      time.Time         `json:"next_due,omitzero"`
}

type NativeTicketState string

const (
	NativeTicketExtended NativeTicketState = "extended"
	NativeTicketFailed   NativeTicketState = "extension_failed"
)

type nativeTicketResult struct {
	Extended   bool        `json:"extended"`
	NextExpiry int64       `json:"nextExpiry"`
	Stage      string      `json:"stage"`
	Capture    *ATSCapture `json:"-"`
}

var nativeTicketCapture = captureNativeTicket

// This is the deployed TicketManager's own SDK path, not an invented response
// or a replay of captured bearer headers. pingServer checks a real PERMIT for
// the sessionextender read before advancing lastExtendedTime.
const nativeTicketJS = `(async()=>{let stage='sdk_unavailable';
 try {
  if(location.origin!=='https://qbo.intuit.com'||typeof window.require!=='function')return {extended:false,stage:'sdk_unavailable'};
  const work=(async()=>{
   stage='loading_context';
   const getContext=()=>new Promise((resolve,reject)=>window.require(['shell/base/appContext'],resolve,reject));
   let ctx=await getContext();const readyDeadline=Date.now()+20000;
   // Some shell variants publish an initialize-only stub before replacing it
   // with the real core context. Never initialize that stub ourselves.
   while(typeof ctx?.getCorePlugin!=='function'&&Date.now()<readyDeadline){await new Promise(resolve=>setTimeout(resolve,100));ctx=await getContext()}
   if(typeof ctx?.getCorePlugin!=='function')return {extended:false,stage:'context_unavailable'};
   stage='loading_plugin';
   const ticket=await ctx.getCorePlugin('ticketManager');
   if(typeof ticket?.pingServer!=='function'||typeof ticket.onTicketRenewalHandler!=='function'||!(ticket.lastExtendedTime instanceof Date))return {extended:false,stage:'plugin_unavailable'};
   const before=ticket.lastExtendedTime.getTime();
   // A timer/storage update is not a successful server authorization. Observe
   // the native success callback, which pingServer invokes only for PERMIT.
   let permitted=false;const original=ticket.onTicketRenewalHandler;
   const observe=function(...args){const value=Reflect.apply(original,this,args);permitted=true;return value};
   ticket.onTicketRenewalHandler=observe;
   stage='requesting_extension';
   try{await ticket.pingServer(true)}finally{if(ticket.onTicketRenewalHandler===observe)ticket.onTicketRenewalHandler=original}
   return {extended:permitted&&location.origin==='https://qbo.intuit.com'&&ticket.lastExtendedTime.getTime()>before,nextExpiry:ticket.expiryTime?.getTime()||0,stage:permitted?'extended':'not_permitted'};
  })();
  let timer;try{return await Promise.race([work,new Promise((_,reject)=>{timer=setTimeout(()=>reject(new Error('extension timeout')),30000)})])}finally{clearTimeout(timer)}
 }catch(_){return {extended:false,stage}}
})()`

func nativeTicketDue(state *NativeTicketStatus, now time.Time) bool {
	if state == nil {
		return true
	}
	if !state.LastAttempt.IsZero() && now.Sub(state.LastAttempt) < 5*time.Minute {
		return false
	}
	// The keeper wakes before this runtime threshold even if its banking probe
	// interval is longer. Leave one minute for browser startup/network latency.
	return state.NextDue.IsZero() || !now.Before(state.NextDue.Add(-time.Minute))
}

// ExtendNativeTicket only maintains an existing owned session. It never reads
// password/TOTP secrets, opens a user browser, or starts credential recovery.
func ExtendNativeTicket(ctx context.Context, expected *TokenSet, force bool) error {
	if IsHarness() {
		return ErrRecoveryDisabled
	}
	if expected == nil || expected.Source != "managed-profile" {
		return ErrRemintNeedsLogin
	}
	unlock, err := LockProfile(ctx, HomeDir(), "renewal")
	if err != nil {
		return err
	}
	defer unlock()
	current, err := Load()
	if err != nil || !SameSession(expected, current) || current.CredentialGeneration != expected.CredentialGeneration || current.Source != "managed-profile" {
		return ErrSessionChanged
	}
	now := time.Now().UTC()
	if !force && !nativeTicketDue(current.NativeTicket, now) {
		return nil
	}
	result, err := nativeTicketCapture(ctx, current)
	if err == nil && result == nil {
		err = errors.New("native ticket extension returned no verification")
	}
	state := &NativeTicketStatus{State: NativeTicketFailed, LastAttempt: now}
	if current.NativeTicket != nil {
		state.LastExtended = current.NativeTicket.LastExtended
		state.NextDue = current.NativeTicket.NextDue
	}
	if err == nil {
		expiry := time.UnixMilli(result.NextExpiry).UTC()
		if !result.Extended || result.Capture == nil || !expiry.After(now.Add(time.Minute)) || expiry.After(now.Add(24*time.Hour)) {
			err = errors.New("native ticket extension was not independently verified")
		} else if err = SaveRenewedCapture(current, result.Capture, "managed-profile"); err == nil {
			state.State = NativeTicketExtended
			state.LastExtended = time.Now().UTC()
			state.NextDue = expiry
		}
	}
	lock, lerr := credentialLock(HomeDir())
	if lerr != nil {
		return lerr
	}
	defer lock()
	latest, lerr := Load()
	if lerr != nil || !SameSession(current, latest) || current.CredentialGeneration != latest.CredentialGeneration {
		return ErrSessionChanged
	}
	latest.NativeTicket = state
	if lerr = saveAt(HomeDir(), latest); lerr != nil {
		return lerr
	}
	return err
}

func captureNativeTicket(ctx context.Context, expected *TokenSet) (*nativeTicketResult, error) {
	var result nativeTicketResult
	err := withManagedBrowser(ctx, func(page context.Context) error {
		if err := authenticateManaged(page, managedLoginDriver{ctx: page, company: expected.CompanyName}, nil); err != nil {
			if !errors.Is(err, ErrRemintNeedsLogin) {
				return err
			}
			if err = restoreManagedCookies(page, expected); err != nil {
				return err
			}
			if err = authenticateManaged(page, managedLoginDriver{ctx: page, company: expected.CompanyName}, nil); err != nil {
				return err
			}
		}
		// A committed /app URL and SSR bootstrap are not a hydrated native SDK.
		// Wait for the real banking request before inspecting identity/plugins.
		preflight, _, _, err := interceptManagedATS(page, BankingCaptureURL)
		if err != nil {
			return err
		}
		if realm := headerGet(preflight, "intuit-company-id"); realm != "" && realm != expected.RealmID {
			return ErrSessionChanged
		}
		if err := waitManagedIdentity(page, expected); err != nil {
			return fmt.Errorf("native preflight identity: %w", err)
		}
		current, err := Load()
		if err != nil || !SameSession(expected, current) || current.CredentialGeneration != expected.CredentialGeneration {
			return ErrSessionChanged
		}
		if err = chromedp.Run(page, chromedp.Evaluate(nativeTicketJS, &result, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })); err != nil || !result.Extended {
			return fmt.Errorf("native ticket extension did not complete: %s", result.Stage)
		}
		if err = waitManagedIdentity(page, expected); err != nil {
			return fmt.Errorf("native post-extension identity: %w", err)
		}
		headers, secondary, hosts, err := interceptManagedATS(page, BankingCaptureURL)
		if err != nil {
			return err
		}
		var identity Identity
		if err = chromedp.Run(page, chromedp.Evaluate(IdentityJS, &identity)); err != nil {
			return ErrSessionIdentityUnverified
		}
		cookies, err := snapshotManagedCookies(page)
		if err != nil {
			return err
		}
		cap := &ATSCapture{Headers: headers, SecondaryHeaders: secondary, HostHeaders: hosts, Identity: identity, Cookies: cookies}
		if err = verifyRecoveryIdentity(expected, cap); err != nil {
			return fmt.Errorf("native recaptured identity: %w", err)
		}
		if err = proveManagedCompany(page, expected, cap); err != nil {
			return err
		}
		result.Capture = cap
		return nil
	})
	return &result, err
}
