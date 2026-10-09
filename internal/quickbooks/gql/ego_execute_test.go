package gql

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func TestSelectedEgoSpacePrecedence(t *testing.T) {
	t.Setenv("QB_EGO_SPACE_ID", "")
	t.Setenv("QB_EGO_SPACE", "")
	if selectedEgoSpace() != "qb-gql" {
		t.Fatal("wrong default")
	}
	t.Setenv("QB_EGO_SPACE", "named-space")
	if selectedEgoSpace() != "named-space" {
		t.Fatal("named space ignored")
	}
	t.Setenv("QB_EGO_SPACE_ID", "47")
	if selectedEgoSpace() != "47" {
		t.Fatal("explicit ID must win")
	}
}

func TestEgoExistingTabContract(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	for _, scenario := range []string{"untracked", "tracked", "tracked-no-label", "spoofed-page", "user-control", "pinned", "wrong-target", "wrong-identity", "changed-identity", "signed-out", "dialog", "binary"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			script := filepath.Join(dir, "execute.mjs")
			if err := os.WriteFile(script, []byte(egoExecuteScript), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(node, "--input-type=module", "-e", egoMock, script, filepath.Join(dir, "out.json"), scenario, auth.BrowserByteFetchJS)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
		})
	}
}

const egoMock = `
import fs from 'node:fs/promises';
import assert from 'node:assert/strict';
const [script,out,scenario,byteFetchScript]=process.argv.slice(1);
let fetched=false,adopted=false;
const binaryBytes=Buffer.from(Array.from({length:70000},(_,index)=>index%256));
globalThis.fetch=async(endpoint,options)=>{assert.equal(scenario,'binary');assert.deepEqual(Buffer.from(options.body),binaryBytes);fetched=true;return new Response(binaryBytes,{status:200});};
const good='https://qbo.intuit.com/app/banking';
const bad='https://evil.example/qbo.intuit.com/app/banking';
const page={info:async()=>({dialog:scenario==='dialog'}),url:async()=>scenario==='spoofed-page'?bad:scenario==='signed-out'?'https://accounts.intuit.com/app/sign-in':good,
 evaluate:async(expression,argument)=>{if(typeof expression==='function')return await expression(argument);if(argument!==undefined)throw Error('string expression does not accept an argument');return {realm:scenario==='wrong-identity'||scenario==='changed-identity'&&fetched?'13':'12',email:'owner@example.test'};},
 fetch:async(url,options)=>{fetched=true;assert.equal(options.redirect,'error');assert.equal(options.cache,'no-store');await fs.writeFile(options.saveAs,JSON.stringify({data:{padding:'x'.repeat(2*1024*1024)}}));return {status:200,body:'truncated-inline-text'}}};
const untracked={targetId:'T1',spaceId:3,openedBy:'unknown'};
const task={ownership:scenario==='user-control'?'user':'agent',tabs:async()=>{
 const tab={targetId:'T1',page:scenario.startsWith('tracked')?page:untracked,url:good};
 if(scenario!=='untracked'&&scenario!=='tracked-no-label')tab.label='p1';
 return [tab];
},page:label=>{assert.equal(label,'p1');return page;},
 adopt:async handle=>{assert.equal(handle,untracked);adopted=true;return page;}};
const proc={env:{QB_EGO_OUT:out,QB_EGO_ENDPOINT:'https://qbo.intuit.com/api/v4/graphql',QB_EGO_SPACE:'named-space'},exit:()=>{throw new Error('exited');}};
const request={outPath:out,endpoint:'https://qbo.intuit.com/api/v4/graphql',method:'POST',headers:{},body:'{}',binaryBody:scenario==='binary',bodyBase64:binaryBytes.toString('base64'),byteFetchScript,spaceKey:'named-space',targetKey:scenario==='wrong-target'?'T2':'T1',expectedRealm:'12',expectedEmail:'owner@example.test',identityScript:'identity',timeoutMs:30000};
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
const run=new AsyncFunction('taskSpace','process','request','listTaskSpaces',await fs.readFile(script,'utf8'));
let error;
try{await run(async id=>{assert.equal(id,41);return task},proc,request,async()=>[{id:41,name:'named-space',ownership:'agent'}])}catch(e){error=e}
if(['tracked','pinned','binary'].includes(scenario)){
 assert.equal(error,undefined);assert.equal(adopted,false);assert.equal(fetched,true);
 assert.equal(JSON.parse(await fs.readFile(out,'utf8')).status,200);
 const body=Buffer.from(JSON.parse(await fs.readFile(out,'utf8')).bodyBase64,'base64');
 if(scenario==='binary')assert.deepEqual(body,binaryBytes);else assert.ok(body.length>2*1024*1024);
 assert.equal((await fs.stat(out+'.body')).mode&0o777,0o600);
}else{assert.ok(error);assert.equal(adopted,false);assert.equal(fetched,scenario==='changed-identity');}
`
