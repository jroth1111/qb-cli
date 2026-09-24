package auth

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestExistingEgoCaptureContract(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	for _, scenario := range []string{"success", "tracked-no-label", "untracked", "numeric-space", "homepage", "redirect-other-origin", "dialog", "ambiguous", "identity-drift", "user-control"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			script := filepath.Join(dir, "capture.mjs")
			if err := os.WriteFile(script, existingEgoScript, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(node, "--input-type=module", "-e", existingEgoMock, script, filepath.Join(dir, "out.json"), scenario)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
		})
	}
}

const existingEgoMock = `
import fs from 'node:fs/promises';
import assert from 'node:assert/strict';
const [script, out, scenario] = process.argv.slice(1);
const identity = {realm:'123', email:'person@example.com'};
let reads = 0, ticks = 0, reloaded = false, navigations = 0;
let currentURL = scenario === 'homepage' ? 'https://qbo.intuit.com/app/homepage' : 'https://qbo.intuit.com/app/banking';
const events = [{method:'Network.requestWillBeSent',params:{requestId:'1',request:{url:'https://qbo.intuit.com/api/test',headers:{Authorization:'Intuit_APIKey test'}}}},
 {method:'Network.requestWillBeSent',params:{requestId:'2',request:{url:'https://qbo.intuit.com/api/v4/graphql',headers:{apikey:'secondary'}}}}];
const tab={targetId:'target',label:'p5',url:currentURL};
if (scenario==='tracked-no-label' || scenario==='untracked') delete tab.label;
tab.page = scenario==='untracked' ? {targetId:'target',spaceId:3} : {fetch:async()=>{throw new Error('capture must not fetch')}};
const ego={helpers:{
 switchTaskSpace:async id=>{ assert.equal(id,3); },
 useOrCreateTaskSpace:async()=> {
   assert.notEqual(scenario,'numeric-space');
   if(scenario==='user-control') throw new Error('user control');
   return {id:3};
 },
 listTabs:async()=>scenario==='ambiguous'?[tab,tab]:[tab],
 switchTab:async(target)=>{ assert.equal(target,'target'); return tab; },
 pageInfo:async()=>({url:currentURL,dialog:scenario==='dialog'}),
 gotoAndWait:async url=>{ assert.equal(url,'https://qbo.intuit.com/app/banking');currentURL=url;navigations++; },
 js:async()=> (++reads > 1 && scenario==='identity-drift' ? {...identity,email:'other@example.com'} : identity),
 drainEvents:async()=> reloaded ? events : [],
 cdp:async(method,params)=> {
   if (method==='Page.reload') { reloaded=true;if(scenario==='redirect-other-origin')currentURL='https://evil.example/app/banking'; return {}; }
   if (method==='Network.enable') return {};
   assert.equal(method,'Network.getCookies');
   assert.deepEqual(params.urls,['https://qbo.intuit.com/']);
   return {cookies:[{name:'ticket',value:'secret',domain:'.qbo.intuit.com',path:'/',expires:-1}]};
 },
}};
const fakeProcess={env:{QB_CAPTURE_OUT:out,QB_TIMEOUT_MS:'90000',QB_EGO_SPACE:scenario==='numeric-space'?'3':'qb-login'}};
const logs=[];
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
const run=new AsyncFunction('ego','process','captureTarget','identityExpression','Date','setTimeout','console',await fs.readFile(script,'utf8'));
let failure;
try { await run(ego,fakeProcess,scenario==='success'?'p5':'target','identity',{now:()=>ticks+=5000},f=>f(),{log:x=>logs.push(x)}); } catch(e) { failure=e; }
if(['success','tracked-no-label','untracked','numeric-space','homepage'].includes(scenario)) {
 assert.equal(failure,undefined);
 const result=JSON.parse(await fs.readFile(out,'utf8'));
 assert.equal(result.cookies.length,1); assert.equal(result.identity.realm,'123');
 assert.equal(JSON.stringify(logs).includes('secret'),false);
 assert.equal(navigations,scenario==='homepage'?1:0);
} else {
 assert.ok(failure); await assert.rejects(fs.access(out));
 if(scenario==='user-control'||scenario==='ambiguous'||scenario==='dialog') assert.equal(reloaded,false);
}
`
