package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type egoLoginDriver struct {
	expected  *TokenSet
	authorize func() error
	redact    []string
	purposes  *[]string
}

const egoLoginActionScript = `const fs=await import('node:fs/promises');
async function finish(value){await fs.writeFile(output,JSON.stringify(value),{mode:0o600});}
try {
 const task=await taskSpace(Number(arg.space));
 if(task.ownership!=='agent'){await finish({error:'user_control'});}
 else {
  const tabs=(await task.tabs()).filter(t=>t.label===arg.target||t.targetId===arg.target);
  if(tabs.length!==1||!tabs[0].label)throw Error('target');
  const page=task.page(tabs[0].label);const info=await page.info();
  if(info.dialog)throw Error('dialog');
  const url=new URL(await page.url());
  if(url.protocol!=='https:'||url.username||url.password||url.port&&url.port!=='443'||!['accounts.intuit.com','qbo.intuit.com'].includes(url.hostname))throw Error('origin');
  const visible=e=>e&&!e.disabled&&e.getClientRects().length>0;
  if(arg.action==='snapshot') {
   const snapshot=await page.evaluate(snapshotScript);
   if(url.hostname==='qbo.intuit.com'&&url.pathname.startsWith('/app/'))snapshot.step='app';
   else {
    const choice=await page.evaluate(({email})=>{
     const visible=e=>e&&!e.disabled&&e.getClientRects().length>0;
     const cards=[...document.querySelectorAll('li button')].filter(e=>visible(e)&&
      (e.innerText.match(/[\w.+-]+@[\w.-]+\.[a-z]{2,}/ig)||[]).some(v=>v.toLowerCase()===email.toLowerCase()));
     const alerts=[...document.querySelectorAll('[role="alert"]')].filter(e=>visible(e)&&e.innerText.trim());
     return cards.length===1&&alerts.every(e=>/^your session timed out\.?\s*sign in again to continue$/i.test(e.innerText.trim()));
    },{email:arg.email});
    if(choice&&!snapshot.visible_captcha&&['unknown','challenge'].includes(snapshot.step))snapshot.step='account-choice';
   }
   await finish({snapshot});
  } else {
   if(url.hostname!=='accounts.intuit.com')throw Error('origin');
   if(arg.action==='initialize') {
    await page.cdp('Page.addScriptToEvaluateOnNewDocument',{source:passkeyScript});
    await page.evaluate(passkeyScript);
   } else if(arg.action==='submit') {
    const selectors={username:'#ius-userid, #iux-identifier-field, #iux-identifier-first-international-email-user-id-input, input[autocomplete="username"], input[type="email"]',password:'#ius-password, #iux-password-field, input[type="password"]',totp:'#ius-mfa-confirm-code, #iux-mfa-soft-token-verification-code, input[autocomplete="one-time-code"]'};
    const selector=selectors[arg.step];if(!selector)throw Error('step');
    const marked=await page.evaluate(({selector,email,step})=>{
     const visible=e=>e&&!e.disabled&&e.getClientRects().length>0;
     const input=[...document.querySelectorAll(selector)].filter(visible);
     const shown=[...document.querySelectorAll('input[name="Email"],input[type="email"]')].filter(e=>e.getClientRects().length&&e.value);
     if(step!=='username'&&shown.some(e=>e.value.toLowerCase()!==email.toLowerCase()))return false;
     if(input.length!==1)return false;
     const buttons=[...(input[0].form||document).querySelectorAll('button[type="submit"],#ius-userid-submit-btn,#ius-sign-in-submit-btn,#ius-mfa-code-submit-btn')].filter(visible);
     if(buttons.length!==1)return false;
     document.querySelectorAll('[data-qb-ego-input],[data-qb-ego-submit]').forEach(e=>{e.removeAttribute('data-qb-ego-input');e.removeAttribute('data-qb-ego-submit')});
     input[0].setAttribute('data-qb-ego-input','true');buttons[0].setAttribute('data-qb-ego-submit','true');return true;
    },{selector,email:arg.email,step:arg.step});
    if(!marked)throw Error('controls');
    await page.fill('[data-qb-ego-input="true"]',arg.value);
    await page.click('[data-qb-ego-submit="true"]');
   } else {
    if(arg.action==='password')await page.evaluate('(async()=>{await window.__qbPasskeyCancellation?.cancel()})()');
    const marked=await page.evaluate(arg=>{
     const visible=e=>e&&!e.disabled&&e.getClientRects().length>0;
     const buttons=[...document.querySelectorAll('button')].filter(visible);
     let selected=[];
     if(arg.action==='password')selected=buttons.filter(e=>/^enter password(?:[\s,]|$)/i.test(e.innerText.trim()));
     if(arg.action==='authenticator')selected=buttons.filter(e=>/^(use )?(an? )?authenticator app(?:[\s,]|$)/i.test(e.innerText.trim()));
     if(arg.action==='account')selected=[...document.querySelectorAll('li button')].filter(e=>visible(e)&&(e.innerText.match(/[\w.+-]+@[\w.-]+\.[a-z]{2,}/ig)||[]).some(v=>v.toLowerCase()===arg.email.toLowerCase()));
     if(arg.action==='company')selected=[...document.querySelectorAll('li button')].filter(e=>visible(e)&&e.name!=='buttonToOpen'&&(!arg.company||e.innerText.trim()===arg.company));
     if(selected.length!==1)return false;
     document.querySelectorAll('[data-qb-ego-choice]').forEach(e=>e.removeAttribute('data-qb-ego-choice'));
     selected[0].setAttribute('data-qb-ego-choice','true');return true;
    },{action:arg.action,email:arg.email,company:arg.company});
    if(!marked)throw Error('choice');await page.click('[data-qb-ego-choice="true"]');
   }
   await finish({ok:true});
  }
 }
}catch(error){await finish({error:/user.*control|inactive|unassigned/i.test(String(error))?'user_control':'attention'});}
`

func (d egoLoginDriver) action(ctx context.Context, action string, step loginStep, value string) (loginSnapshot, error) {
	var empty loginSnapshot
	if d.authorize != nil {
		if err := d.authorize(); err != nil {
			return empty, err
		}
	}
	space, err := strconv.ParseInt(d.expected.EgoSpace, 10, 64)
	if err != nil || space <= 0 || d.expected.EgoTargetID == "" {
		return empty, ErrRecoveryAttention
	}
	file, err := os.CreateTemp("", "qb-ego-login-step-*.json")
	if err != nil {
		return empty, err
	}
	path := file.Name()
	_ = file.Close()
	defer func() { _ = os.Remove(path) }()
	arg, _ := json.Marshal(map[string]string{"action": action, "step": string(step), "value": value, "space": d.expected.EgoSpace, "target": d.expected.EgoTargetID, "email": d.expected.Email, "company": d.expected.CompanyName})
	script := []byte("const output=" + strconv.Quote(path) + ",arg=" + string(arg) + ",snapshotScript=" + strconv.Quote(loginSnapshotJS) + ",passkeyScript=" + strconv.Quote(managedPasskeyCancellationJS) + ";\n" + egoLoginActionScript)
	defer clear(script)
	cmd := exec.CommandContext(ctx, "ego-browser", "nodejs")
	cmd.Stdin = bytes.NewReader(script)
	// Credential values travel only through the private process stdin pipe.
	// Never persist source or return unrestricted child output on failure.
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return empty, ctx.Err()
		}
		return empty, ErrRecoveryAttention
	}
	raw, err := readPrivateFile(path, 1<<20)
	if err != nil {
		return empty, err
	}
	var result struct {
		Snapshot loginSnapshot `json:"snapshot"`
		Error    string        `json:"error"`
		OK       bool          `json:"ok"`
	}
	if json.Unmarshal(raw, &result) != nil {
		return empty, ErrRecoveryAttention
	}
	if result.Error == "user_control" {
		return empty, ErrEgoUserControl
	}
	if result.Error != "" {
		return empty, ErrRecoveryAttention
	}
	for _, secret := range d.redact {
		if secret != "" {
			result.Snapshot.Heading = strings.ReplaceAll(result.Snapshot.Heading, secret, "[redacted]")
			result.Snapshot.URL = strings.ReplaceAll(result.Snapshot.URL, secret, "[redacted]")
			for _, values := range [][]string{result.Snapshot.Fields, result.Snapshot.Buttons, result.Snapshot.Frames} {
				for i := range values {
					values[i] = strings.ReplaceAll(values[i], secret, "[redacted]")
				}
			}
		}
	}
	if action != "snapshot" && !result.OK {
		return empty, ErrRecoveryAttention
	}
	if action == "submit" && d.purposes != nil {
		*d.purposes = append(*d.purposes, string(step))
	}
	return result.Snapshot, nil
}
func (d egoLoginDriver) snapshot(ctx context.Context) (loginSnapshot, error) {
	return d.action(ctx, "snapshot", "", "")
}
func (d egoLoginDriver) submit(ctx context.Context, step loginStep, value string) error {
	_, err := d.action(ctx, "submit", step, value)
	return err
}
func (d egoLoginDriver) choosePassword(ctx context.Context) error {
	_, err := d.action(ctx, "password", "", "")
	return err
}
func (d egoLoginDriver) chooseAuthenticator(ctx context.Context) error {
	_, err := d.action(ctx, "authenticator", "", "")
	return err
}
func (d egoLoginDriver) chooseCompany(ctx context.Context) error {
	_, err := d.action(ctx, "company", "", "")
	return err
}
func (d egoLoginDriver) chooseAccount(ctx context.Context, email string) error {
	if !strings.EqualFold(email, d.expected.Email) {
		return ErrSessionChanged
	}
	_, err := d.action(ctx, "account", "", "")
	return err
}

func captureSourceRecovery(ctx context.Context, expected *TokenSet, secrets *RecoverySecrets) (*ATSCapture, error) {
	if expected.Source != "ego-existing" {
		return captureManagedRecovery(ctx, expected, secrets)
	}
	driver := egoLoginDriver{expected: expected}
	purposes := []string{}
	driver.purposes = &purposes
	driver.authorize = func() error {
		current, err := Load()
		if err != nil || !SameSession(expected, current) || current.CredentialGeneration != expected.CredentialGeneration || current.Source != expected.Source || current.EgoSpace != expected.EgoSpace || current.EgoTargetID != expected.EgoTargetID {
			return ErrSessionChanged
		}
		if secrets != nil {
			r, err := loadRecovery()
			grant, _ := ctx.Value(recoveryGrantKey{}).(string)
			if err != nil || !r.Enabled || r.ID != grant || !recoveryBound(r, current) {
				return ErrRecoveryDisabled
			}
		}
		return nil
	}
	if secrets != nil {
		if !strings.EqualFold(secrets.Username, expected.Email) {
			return nil, ErrSessionChanged
		}
		driver.redact = []string{secrets.Password, secrets.TOTP, secrets.Username}
		if _, err := driver.action(ctx, "initialize", "", ""); err != nil {
			return nil, err
		}
	}
	if err := authenticateManaged(ctx, driver, secrets); err != nil {
		return nil, err
	}
	proofCtx := context.WithValue(ctx, egoAPIProofKey{}, true)
	cap, err := CaptureATSFromEgo(WithExistingEgo(proofCtx, expected.EgoSpace, expected.EgoTargetID), BankingCaptureURL, BankingCaptureURL)
	if err != nil {
		return nil, err
	}
	if err = verifyRecoveryIdentity(expected, cap); err != nil {
		return nil, err
	}
	if cap.Evidence != nil {
		cap.Evidence.CredentialPurposes = purposes
	}
	return cap, nil
}
