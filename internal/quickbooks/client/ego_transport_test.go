package client

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func TestEgoRequestPreservesAPIAndPinsCompany(t *testing.T) {
	tok := &auth.TokenSet{Source: "ego-existing", EgoSpace: "52", EgoTargetID: "p1", RealmID: "123", Email: "fixture@example.test"}
	req, _ := http.NewRequest("POST", "https://qbo.intuit.com/api/neo/v1/company/123/olb/ng/batchAcceptTransactions", strings.NewReader(`{"fixture":true}`))
	for k, v := range map[string]string{"Cookie": "secret", "Authorization": "synthetic", "X-Range": "items=0-299", "Sec-Ch-Ua": "browser", "Content-Type": "application/json"} {
		req.Header.Set(k, v)
	}
	got, err := buildEgoHTTPRequest(req, tok)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := base64.StdEncoding.DecodeString(got.Body)
	if got.Space != "52" || got.Target != "p1" || got.Method != "POST" || string(body) != `{"fixture":true}` || got.Headers["Authorization"] != "synthetic" || got.Headers["X-Range"] != "items=0-299" {
		t.Fatalf("request fields not preserved")
	}
	if _, ok := got.Headers["Cookie"]; ok {
		t.Fatal("browser must own cookies")
	}
	if _, ok := got.Headers["Sec-Ch-Ua"]; ok {
		t.Fatal("browser must own client hints")
	}
	for _, url := range []string{"https://qbo.intuit.com/api/v3/company/456/purchase/1", "https://evil.example/company/123/x", "http://qbo.intuit.com/company/123/x", "https://qbo.intuit.com:443/company/123/x"} {
		r, _ := http.NewRequest("GET", url, nil)
		if _, err := buildEgoHTTPRequest(r, tok); err == nil {
			t.Errorf("accepted unsafe endpoint %s", url)
		}
	}
}

func TestEgoRetriesOnlyTransientReads(t *testing.T) {
	for _, method := range []string{"GET", "HEAD", "POST", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			req, _ := http.NewRequest(method, "https://example.test", nil)
			calls := 0
			_, _ = readReliably(req, func(*http.Request) (*http.Response, error) { calls++; return nil, errEgoReadTransient })
			want := 1
			if method == "GET" || method == "HEAD" {
				want = 3
			}
			if calls != want {
				t.Fatalf("calls=%d want=%d", calls, want)
			}
		})
	}
	req, _ := http.NewRequest("GET", "https://example.test", nil)
	calls := 0
	_, _ = readReliably(req, func(*http.Request) (*http.Response, error) { calls++; return nil, auth.ErrEgoUserControl })
	if calls != 1 {
		t.Fatal("user control retried")
	}
}

func TestEgoRequestAcceptsCapturedNamedSpace(t *testing.T) {
	tok := &auth.TokenSet{Source: "ego-existing", EgoSpace: "qb-gql", EgoTargetID: "p2", RealmID: "123", Email: "fixture@example.test"}
	req, _ := http.NewRequest("GET", "https://qbo.intuit.com/api/v3/company/123/purchase/1", nil)
	got, err := buildEgoHTTPRequest(req, tok)
	if err != nil || got.Space != "qb-gql" || got.Target != "p2" {
		t.Fatalf("captured named space rejected: %v", err)
	}
	for _, space := range []string{"", "0", "-1", " qb-gql ", "9999999999999999999999999999"} {
		tok.EgoSpace = space
		if _, err := buildEgoHTTPRequest(req, tok); err == nil {
			t.Errorf("invalid space accepted: %q", space)
		}
	}
}

func TestEgoScriptUsesOneFetchAndRejectsIdentityDrift(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node unavailable")
	}
	for _, scenario := range []string{"success", "identity-before", "identity-after", "wrong-target", "fetch-error"} {
		t.Run(scenario, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "result.json")
			setup := fmt.Sprintf(`
const scenario=%q;
const disk=await import('node:fs/promises');
let calls=0, identities=0;
const page={url:async()=>"https://qbo.intuit.com/app/banking",evaluate:async()=>({realm:scenario==="identity-before"||scenario==="identity-after"&&identities++>0?"456":"123",email:"fixture@example.test"}),fetch:async(url,options)=>{calls++;if(options.redirect!=="error"||options.body!=="fixture"||!options.saveAs)throw Error("request changed");if(scenario==="fetch-error")throw Error("network");await disk.writeFile(options.saveAs,JSON.stringify({padding:'x'.repeat(2*1024*1024)}));return {status:200,headers:{"content-type":"application/json"},body:'truncated-inline-text'}}};
const taskSpace=async n=>{if(n!==52)throw Error("wrong space");return {tabs:async()=>[{label:scenario==="wrong-target"?"p2":"p1"}],page:()=>page}};
const request={space:"52",target:"p1",realm:"123",email:"fixture@example.test",url:"https://qbo.intuit.com/api/v3/company/123/purchase/1",method:"POST",headers:{},body:Buffer.from("fixture").toString("base64"),out:%q,timeout:30000};
const identityExpression="fixture";
try {await (new (Object.getPrototypeOf(async function(){}).constructor)("taskSpace","request","identityExpression",%q))(taskSpace,request,identityExpression);if(scenario!=="success")throw Error("unexpected success");}catch(e){if(scenario==="success")throw e;}
if(calls!==(scenario==="identity-before"||scenario==="wrong-target"?0:1))throw Error("write replayed or incorrectly sent");
if((process.exitCode||0)!==(scenario==="success"?0:1))throw Error("incorrect transport exit status");process.exitCode=0;
`, scenario, out, egoTransportScript)
			cmd := exec.Command("node", "--input-type=module", "-e", setup)
			if data, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, data)
			}
			_, err := os.Stat(out)
			if err != nil {
				t.Fatal("missing safe diagnostic envelope")
			}
			raw, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			var envelope map[string]any
			if err := json.Unmarshal(raw, &envelope); err != nil {
				t.Fatal(err)
			}
			if scenario == "success" && envelope["error_code"] != nil || scenario != "success" && envelope["error_code"] == nil {
				t.Fatal("response accepted despite failure")
			}
			if scenario == "success" {
				body, _ := envelope["body"].(string)
				if len(body) < 2*1024*1024 || !json.Valid([]byte(body)) {
					t.Fatal("full response body was not preserved")
				}
				info, err := os.Stat(out + ".body")
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatal("response body is not private")
				}
			}
			if strings.Contains(string(raw), "fixture@example.test") {
				t.Fatal("diagnostic leaked principal")
			}
		})
	}
}
