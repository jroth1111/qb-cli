package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
)

const egoNativeTicketScript = `const fs=await import('node:fs/promises');
async function result(value){await fs.writeFile(output,JSON.stringify(value),{mode:0o600});}
try {
 const task=await taskSpace(Number(space));
 if(task.ownership!=='agent'){await result({error:'user_control'});}
 else {
  const matches=(await task.tabs()).filter(t=>t.label===target||t.targetId===target);
  if(matches.length!==1||!matches[0].label){await result({error:'target_unavailable'});}
  else {
   const page=task.page(matches[0].label);
   const info=await page.info();
   if(info.dialog){await result({error:'dialog'});}
   else {
    const before=await page.evaluate(identityScript);
    const u=new URL(await page.url());
    if(u.protocol!=='https:'||u.hostname!=='qbo.intuit.com'||u.username||u.password||u.port&&u.port!=='443'||!u.pathname.startsWith('/app/')){await result({error:'needs_login'});}
    else if(before.realm!==realm||String(before.email||'').toLowerCase()!==email.toLowerCase()){await result({error:'identity_mismatch'});}
    else {
     const extension=await page.evaluate(extensionScript);
     const after=await page.evaluate(identityScript);
     if(task.ownership!=='agent'){await result({error:'user_control'});}
     else if(after.realm!==before.realm||String(after.email||'').toLowerCase()!==email.toLowerCase()){await result({error:'identity_mismatch'});}
     else {await result(extension);}
    }
   }
  }
 }
} catch(error) {
 await result({error:/user.*control|inactive|unassigned/i.test(String(error))?'user_control':'browser_failure'});
}`

func captureEgoNativeTicket(ctx context.Context, expected *TokenSet) (*nativeTicketResult, error) {
	space, err := strconv.ParseInt(expected.EgoSpace, 10, 64)
	if err != nil || space <= 0 || expected.EgoTargetID == "" {
		return nil, ErrRemintNeedsLogin
	}
	if _, err := exec.LookPath("ego-browser"); err != nil {
		return nil, ErrEgoMissing
	}
	// Hydrate the pinned banking context before asking its native plugin to
	// extend. This warm capture never resolves enrollment secrets.
	proofCtx := context.WithValue(ctx, egoAPIProofKey{}, true)
	warm, err := CaptureATSFromEgo(WithExistingEgo(proofCtx, expected.EgoSpace, expected.EgoTargetID), BankingCaptureURL, BankingCaptureURL)
	if err != nil {
		return nil, err
	}
	if err = verifyRecoveryIdentity(expected, warm); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "qb-ego-native-ticket-*.json")
	if err != nil {
		return nil, err
	}
	path := file.Name()
	_ = file.Close()
	defer func() { _ = os.Remove(path) }()
	script := "const output=" + strconv.Quote(path) + ",space=" + strconv.Quote(expected.EgoSpace) + ",target=" + strconv.Quote(expected.EgoTargetID) + ",realm=" + strconv.Quote(expected.RealmID) + ",email=" + strconv.Quote(expected.Email) + ",identityScript=" + strconv.Quote(IdentityJS) + ",extensionScript=" + strconv.Quote(nativeTicketJS) + ";\n" + egoNativeTicketScript
	cmd := exec.CommandContext(ctx, "ego-browser", "nodejs")
	cmd.Stdin = bytes.NewBufferString(script)
	// Native scripts contain no login secrets. Still never return unrestricted
	// child errors or browser text as a diagnostic surface.
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, ErrRecoveryAttention
	}
	raw, err := readPrivateFile(path, 1<<20)
	if err != nil {
		return nil, err
	}
	var reply struct {
		nativeTicketResult
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &reply) != nil {
		return nil, ErrRecoveryAttention
	}
	switch reply.Error {
	case "user_control":
		return nil, ErrEgoUserControl
	case "needs_login":
		return nil, ErrRemintNeedsLogin
	case "identity_mismatch":
		return nil, ErrSessionChanged
	case "":
	default:
		return nil, ErrRecoveryAttention
	}
	if !reply.Extended {
		return nil, ErrRecoveryAttention
	}
	cap, err := CaptureATSFromEgo(WithExistingEgo(proofCtx, expected.EgoSpace, expected.EgoTargetID), BankingCaptureURL, BankingCaptureURL)
	if err != nil {
		return nil, err
	}
	if err = verifyRecoveryIdentity(expected, cap); err != nil {
		return nil, err
	}
	reply.Capture = cap
	return &reply.nativeTicketResult, nil
}
