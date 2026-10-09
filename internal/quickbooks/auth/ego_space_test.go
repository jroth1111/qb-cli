package auth

import (
	"fmt"
	"os/exec"
	"testing"
)

func TestExistingEgoNamedSpacesNeverCreateOrClaim(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node unavailable")
	}
	script := fmt.Sprintf(`
import assert from 'node:assert/strict';
const resolve=%s;
let opens=0;
const open=async id=>{opens++;assert.equal(id,41);return {spaceId:id}};
for(const spaces of [[],[{id:41,name:'existing',ownership:'user'}],[{id:41,name:'existing',ownership:'agent'},{id:42,name:'existing',ownership:'agent'}]]){
 await assert.rejects(resolve('existing',open,async()=>spaces));assert.equal(opens,0);
}
const task=await resolve('existing',open,async()=>[{id:41,name:'existing',ownership:'agent'}]);
assert.equal(task.spaceId,41);assert.equal(opens,1);
await resolve('41',open,()=>{throw Error('numeric resolution should not list spaces')});
assert.equal(opens,2);
for(const key of ['0','9999999999999999999999999'])await assert.rejects(resolve(key,open));
assert.equal(opens,2);
`, ExistingEgoTaskJS)
	if output, err := exec.Command(node, "--input-type=module", "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("existing-space resolution failed: %v: %s", err, output)
	}
}
