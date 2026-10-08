package gql

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
	for _, scenario := range []string{"untracked", "tracked", "tracked-no-label", "spoofed-page", "user-control", "pinned", "wrong-target", "wrong-identity", "changed-identity", "large-body"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			script := filepath.Join(dir, "execute.mjs")
			if err := os.WriteFile(script, []byte(egoExecuteScript), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(node, "--input-type=module", "-e", egoMock, script, filepath.Join(dir, "out.json"), scenario)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
		})
	}
}

const egoMock = `
import fs from 'node:fs/promises';
import assert from 'node:assert/strict';
const [script,out,scenario]=process.argv.slice(1);
let fetched=false,adopted=false;
const good='https://qbo.intuit.com/app/banking';
const bad='https://evil.example/qbo.intuit.com/app/banking';
const page={info:async()=>({}),url:async()=>scenario==='spoofed-page'?bad:good,
 evaluate:async()=>({realm:scenario==='wrong-identity'||scenario==='changed-identity'&&fetched?'13':'12',email:'owner@example.test'}),
 fetch:async(url,options)=>{fetched=true;const body=scenario==='large-body'?'{"data":{"value":"'+('x'.repeat(100000))+'"}}':'{"data":{}}';await fs.writeFile(options.saveAs,body);return {status:200,body:body.slice(0,10)}}};
const untracked={targetId:'T1',spaceId:3,openedBy:'unknown'};
const task={tabs:async()=>{
 if(scenario==='user-control')throw new Error('user control');
 const tab={targetId:'T1',page:scenario.startsWith('tracked')?page:untracked,url:good};
 if(scenario==='tracked')tab.label='p1';
 return [tab];
},page:label=>{assert.equal(label,'p1');return page;},
 adopt:async handle=>{assert.equal(handle,untracked);adopted=true;return page;}};
const proc={env:{QB_EGO_OUT:out,QB_EGO_ENDPOINT:'https://qbo.intuit.com/api/v4/graphql',QB_EGO_SPACE:'named-space'},exit:()=>{throw new Error('exited');}};
if(['pinned','wrong-target','wrong-identity','changed-identity'].includes(scenario)){proc.env.QB_EGO_TARGET=scenario==='wrong-target'?'T2':'T1';proc.env.QB_EGO_REALM='12';proc.env.QB_EGO_EMAIL='owner@example.test';proc.env.QB_EGO_IDENTITY='identity';}
const AsyncFunction=Object.getPrototypeOf(async function(){}).constructor;
const run=new AsyncFunction('taskSpace','process',await fs.readFile(script,'utf8'));
let error;
try{await run(async name=>{assert.equal(name,'named-space');return task},proc)}catch(e){error=e}
if(['untracked','tracked','tracked-no-label','pinned','large-body'].includes(scenario)){
 assert.equal(error,undefined);assert.equal(adopted,!scenario.startsWith('tracked'));assert.equal(fetched,true);
 assert.equal(JSON.parse(await fs.readFile(out,'utf8')).status,200);
 if(scenario==='large-body')assert.equal(JSON.parse(JSON.parse(await fs.readFile(out,'utf8')).body).data.value.length,100000);
}else{assert.ok(error);assert.equal(fetched,scenario==='changed-identity');}
`
