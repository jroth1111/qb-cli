package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"
)

func tparPost(t *testing.T, ctx context.Context, ac *apiClient, path string, body map[string]any) (int, []byte) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/%s?minorversion=73", ac.realm, path)
	resp, err := ac.postJSON(ctx, u, raw)
	if err != nil {
		t.Fatalf("post %s: %v", path, err)
	}
	got, _ := readBody(resp)
	drainAndClose(resp)
	return resp.StatusCode, got
}

func TestTPARCompanyRewrite(t *testing.T) {
	if os.Getenv("TPAR_REWRITE") == "" {
		t.Skip("set TPAR_REWRITE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	// Preferences
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/query?minorversion=73&query=select+*+from+Preferences", ac.realm)
	resp, err := ac.getJSON(ctx, u, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := readBody(resp)
	drainAndClose(resp)
	var wrap map[string]any
	_ = json.Unmarshal(raw, &wrap)
	qr, _ := wrap["QueryResponse"].(map[string]any)
	arr, _ := qr["Preferences"].([]any)
	prefs, _ := arr[0].(map[string]any)
	t.Logf("prefs before TaxPrefs=%v", prefs["TaxPrefs"])

	other, _ := prefs["OtherPrefs"].(map[string]any)
	nvs, _ := other["NameValue"].([]any)
	for i, nv := range nvs {
		m, _ := nv.(map[string]any)
		if fmt.Sprint(m["Name"]) == "Vendor1099Enabled" {
			m["Value"] = "true"
			nvs[i] = m
		}
	}
	if other != nil {
		other["NameValue"] = nvs
	}
	prefBody := map[string]any{
		"Id":         prefs["Id"],
		"SyncToken":  prefs["SyncToken"],
		"sparse":     true,
		"TaxPrefs":   map[string]any{"UsingSalesTax": true},
		"OtherPrefs": other,
	}
	st, got := tparPost(t, ctx, ac, "preferences", prefBody)
	t.Logf("prefs write status=%d preview=%s", st, clip(got, 400))

	// CompanyInfo industry
	u2 := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/companyinfo/%s?minorversion=73", ac.realm, ac.realm)
	resp2, err := ac.getJSON(ctx, u2, "")
	if err != nil {
		t.Fatal(err)
	}
	raw2, _ := readBody(resp2)
	drainAndClose(resp2)
	var ciWrap map[string]any
	_ = json.Unmarshal(raw2, &ciWrap)
	ci, _ := ciWrap["CompanyInfo"].(map[string]any)
	nvs2, _ := ci["NameValue"].([]any)
	set := map[string]string{
		"IndustryType":    "Construction",
		"IndustryCode":    "23",
		"QBOIndustryType": "Construction",
	}
	for i, nv := range nvs2 {
		m, _ := nv.(map[string]any)
		name := fmt.Sprint(m["Name"])
		if v, ok := set[name]; ok {
			m["Value"] = v
			nvs2[i] = m
			delete(set, name)
		}
	}
	for name, v := range set {
		nvs2 = append(nvs2, map[string]any{"Name": name, "Value": v})
	}
	ciBody := map[string]any{
		"Id":        ci["Id"],
		"SyncToken": ci["SyncToken"],
		"sparse":    true,
		"NameValue": nvs2,
	}
	st, got = tparPost(t, ctx, ac, "companyinfo", ciBody)
	t.Logf("companyinfo write status=%d preview=%s", st, clip(got, 400))

	// Vendor TPAR
	res, err := ReplayMutate(ctx, "Vendor", "update", "5", map[string]string{"tpar": "true"})
	if err != nil {
		t.Logf("vendor tpar err %v", err)
	} else {
		t.Logf("vendor tpar status=%d id=%s", res.Status, res.Item.ID)
	}

	v5, err := fetchV3(ctx, ac, "vendor", "5")
	if err != nil {
		t.Logf("vendor5 refetch err %v", err)
	} else {
		t.Logf("vendor5 Vendor1099=%v", v5["Vendor1099"])
	}
}

func clip(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n]
	}
	return s
}

func TestTPARRewriteSanity(t *testing.T) {
	if os.Getenv("TPAR_REWRITE") == "" {
		t.Skip("set TPAR_REWRITE=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/query?minorversion=73&query=select+*+from+TaxCode+maxresults+20", ac.realm)
	resp, err := ac.getJSON(ctx, u, "")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := readBody(resp)
	drainAndClose(resp)
	t.Logf("taxcodes status=%d body=%s", resp.StatusCode, clip(raw, 300))

	u2 := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/query?minorversion=73&query=select+*+from+Preferences", ac.realm)
	resp2, err := ac.getJSON(ctx, u2, "")
	if err != nil {
		t.Fatal(err)
	}
	raw2, _ := readBody(resp2)
	drainAndClose(resp2)
	var wrap map[string]any
	_ = json.Unmarshal(raw2, &wrap)
	qr, _ := wrap["QueryResponse"].(map[string]any)
	arr, _ := qr["Preferences"].([]any)
	p, _ := arr[0].(map[string]any)
	t.Logf("TaxPrefs=%v", p["TaxPrefs"])
	if op, ok := p["OtherPrefs"].(map[string]any); ok {
		for _, nv := range op["NameValue"].([]any) {
			m := nv.(map[string]any)
			if fmt.Sprint(m["Name"]) == "Vendor1099Enabled" {
				t.Logf("Vendor1099Enabled=%v", m["Value"])
			}
		}
	}
	u3 := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/companyinfo/%s?minorversion=73", ac.realm, ac.realm)
	resp3, err := ac.getJSON(ctx, u3, "")
	if err != nil {
		t.Fatal(err)
	}
	raw3, _ := readBody(resp3)
	drainAndClose(resp3)
	var ciWrap map[string]any
	_ = json.Unmarshal(raw3, &ciWrap)
	ci, _ := ciWrap["CompanyInfo"].(map[string]any)
	for _, nv := range ci["NameValue"].([]any) {
		m := nv.(map[string]any)
		name := fmt.Sprint(m["Name"])
		if name == "IndustryType" || name == "IndustryCode" || name == "QBOIndustryType" {
			t.Logf("%s=%v", name, m["Value"])
		}
	}
	_ = http.StatusOK
}
