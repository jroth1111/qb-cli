//go:build live

package client

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/url"
	"os"
	"testing"
	"time"
)

// TestLiveSweepProbe captures a raw read response only when explicitly requested.
// It never writes to QBO; the output is private local sweep evidence.
func TestLiveSweepProbe(t *testing.T) {
	if os.Getenv("QB_LIVE_PROBE") != "1" {
		t.Skip("explicit sweep probe only")
	}
	c, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	if want := os.Getenv("QB_EXPECTED_COMPANY"); want == "" || c.tok.CompanyName != want {
		t.Fatal("test company not verified")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := c.getJSON(ctx, "https://qbo.intuit.com/api/v3/company/"+c.realm+"/query?minorversion=73&query="+url.QueryEscape("select * from CompanyInfo maxresults 5"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		t.Fatal(err)
	}
	if out := os.Getenv("QB_PROBE_OUT"); out != "" {
		if err := os.WriteFile(out, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var obj map[string]any
	if json.Unmarshal(body, &obj) == nil {
		keys := []string{}
		for k := range obj {
			keys = append(keys, k)
		}
		t.Logf("status=%d JSON keys=%v", resp.StatusCode, keys)
		return
	}
	var root struct{ XMLName xml.Name }
	_ = xml.Unmarshal(body, &root)
	t.Logf("status=%d contentType=%s bytes=%d xmlRoot=%s", resp.StatusCode, resp.Header.Get("Content-Type"), len(body), root.XMLName.Local)
	t.Fatal("read response was not JSON")
}
