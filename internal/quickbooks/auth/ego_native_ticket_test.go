package auth

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEgoNativeTicketScriptHonorsOwnershipIdentityAndPromptStops(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed")
	}
	for _, scenario := range []string{"ok", "user", "identity", "dialog", "login", "not-permitted", "identity-after"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "script.js")
			if err := os.WriteFile(file, []byte(egoNativeTicketScript), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(node, "--input-type=module", "-e", egoNativeTicketMock, file, filepath.Join(dir, "out.json"), scenario)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("scenario=%s err=%v output=%s", scenario, err, output)
			}
		})
	}
}

const egoNativeTicketMock = `import fs from 'node:fs/promises';
const [,file,output,scenario]=process.argv;
let extensions=0,identities=0;
const task={ownership:scenario==='user'?'user':'agent',tabs:async()=>[{label:'p1',targetId:'target'}],page:()=>({
info:async()=>({dialog:scenario==='dialog'}),url:async()=>scenario==='login'?'https://accounts.intuit.com/app/sign-in':'https://qbo.intuit.com/app/banking',
evaluate:async expression=>{
 if(expression==='identity'){identities++;return {realm:scenario==='identity'||scenario==='identity-after'&&identities===2?'other':'1',email:'agent@example.invalid'};}
 extensions++;return {extended:scenario!=='not-permitted',nextExpiry:Date.now()+3600000,stage:'extended'};
}})};
globalThis.taskSpace=async()=>task;
const source=await fs.readFile(file,'utf8');
if(/claimTaskSpace|takeOverTaskSpace|newPage|\.goto\(/.test(source))throw Error('source fallback or ownership takeover');
await new Function('output','space','target','realm','email','identityScript','extensionScript',
 'return(async()=>{'+source+'})()')(output,'52','p1','1','agent@example.invalid','identity','extension');
const result=JSON.parse(await fs.readFile(output,'utf8'));
const expected={user:'user_control',identity:'identity_mismatch',dialog:'dialog',login:'needs_login','identity-after':'identity_mismatch'};
if(expected[scenario]&&result.error!==expected[scenario])throw Error('wrong stop');
if(['user','identity','dialog','login'].includes(scenario)&&extensions)throw Error('extension after stop');
if(scenario==='ok'&&(!result.extended||extensions!==1))throw Error('missing native success');
if(scenario==='not-permitted'&&result.extended)throw Error('false native success');
`
