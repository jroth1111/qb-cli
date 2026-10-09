package auth

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestBrowserByteFetchPreservesBinaryRequestAndResponse(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	script := fmt.Sprintf(`
import assert from 'node:assert/strict';
const bytes=Uint8Array.from({length:70000},(_,index)=>index%%256);
const encoded=Buffer.from(bytes).toString('base64');
let calls=0;
globalThis.fetch=async(url,options)=>{
 calls++;
 assert.equal(url,'https://qbo.intuit.com/fixture');
 assert.equal(options.credentials,'include');assert.equal(options.redirect,'error');assert.equal(options.cache,'no-store');
 assert.equal(options.headers['x-fixture'],'kept');
 if(options.method==='POST')assert.deepEqual(options.body,bytes);
 else assert.equal(options.body,undefined);
 return new Response(bytes,{status:200,headers:{'content-type':'application/octet-stream'}});
};
const execute=%s;
for(const method of ['GET','HEAD','POST']){
 const response=await execute({endpoint:'https://qbo.intuit.com/fixture',method,headers:{'x-fixture':'kept'},bodyBase64:encoded,timeoutMs:30000});
 assert.equal(response.status,200);assert.deepEqual(Buffer.from(response.bodyBase64,'base64'),Buffer.from(bytes));
 assert.equal(response.headers['content-type'],'application/octet-stream');
}
assert.equal(calls,3);
`, BrowserByteFetchJS)
	if output, err := exec.Command(node, "--input-type=module", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("byte preservation failed: %v: %s", err, output)
	}
}
