package auth

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEgoLoginActionUsesOnlyPinnedOwnedTrustedPage(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	for _, scenario := range []string{"submit", "wrong-email", "user", "dialog", "foreign", "snapshot", "account", "account-card", "account-card-snapshot", "account-card-ambiguous"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "action.js")
			if err := os.WriteFile(file, []byte(egoLoginActionScript), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(node, "--input-type=module", "-e", egoLoginActionMock, file, filepath.Join(dir, "out.json"), scenario)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, output)
			}
		})
	}
}

const egoLoginActionMock = `import fs from 'node:fs/promises';
const [,file,output,scenario]=process.argv;
let fills=0,clicks=0;
const email={value:scenario==='wrong-email'?'other@example.invalid':'agent@example.invalid',getClientRects:()=>[1]};
const button={innerText:'agent@example.invalid',getClientRects:()=>[1],setAttribute(){},removeAttribute(){}};
const input={getClientRects:()=>[1],setAttribute(){},form:{querySelectorAll:()=>[button]}};
globalThis.document={querySelectorAll:selector=>{
 if(selector.includes('input[name="Email"]'))return [email];
 if(selector.includes('input[type="password"]'))return [input];
 if(selector==='li button')return scenario.startsWith('account-card')?[]:[button];
 if(selector==='button')return scenario==='account-card-ambiguous'?[button,{...button}]:[button];
 return [];
}};
const page={info:async()=>({dialog:scenario==='dialog'}),url:async()=>scenario==='foreign'?'https://evil.invalid':'https://accounts.intuit.com/app/sign-in',
 evaluate:async(fn,arg)=>typeof fn==='function'?fn(arg):({url:'https://accounts.intuit.com/app/sign-in',step:scenario==='account-card-snapshot'?'unknown':'password',visible_captcha:false}),
 fill:async(selector,value)=>{if(value!=='fixture-secret')throw Error('wrong input');fills++;},click:async()=>{clicks++;},cdp:async()=>{}};
const task={ownership:scenario==='user'?'user':'agent',tabs:async()=>[{label:'p1',targetId:'target'}],page:()=>page};
globalThis.taskSpace=async()=>task;
const source=await fs.readFile(file,'utf8');
if(/claimTaskSpace|takeOverTaskSpace|newPage|\.goto\(/.test(source))throw Error('source switching');
const arg={action:['snapshot','account-card-snapshot'].includes(scenario)?'snapshot':['account','account-card','account-card-ambiguous'].includes(scenario)?'account':'submit',step:'password',value:'fixture-secret',space:'52',target:'p1',email:'agent@example.invalid'};
await new Function('output','arg','snapshotScript','passkeyScript','return(async()=>{'+source+'})()')(output,arg,'snapshot','passkey');
const raw=await fs.readFile(output,'utf8');const result=JSON.parse(raw);
if(raw.includes('fixture-secret'))throw Error('secret persisted');
if(scenario==='submit'&&(!result.ok||fills!==1||clicks!==1))throw Error('submission missing');
if(['account','account-card'].includes(scenario)&&(!result.ok||fills||clicks!==1))throw Error('account choice missing');
if(['user','dialog','foreign','wrong-email'].includes(scenario)&&(!result.error||fills||clicks))throw Error('unsafe submit');
if(scenario==='snapshot'&&!result.snapshot)throw Error('snapshot absent');
if(scenario==='account-card-snapshot'&&(result.snapshot?.step!=='account-choice'||fills||clicks))throw Error('non-list account card not detected');
if(scenario==='account-card-ambiguous'&&(!result.error||fills||clicks))throw Error('ambiguous account card selected');
`
