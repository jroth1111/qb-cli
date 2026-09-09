package client

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func TestTPARRawDump(t *testing.T) {
	if os.Getenv("TPAR_PEEL") == "" {
		t.Skip("set TPAR_PEEL=1")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	ac, err := newAPIClient()
	if err != nil {
		t.Fatal(err)
	}
	v5, err := fetchV3(ctx, ac, "vendor", "5")
	if err != nil {
		t.Logf("vendor5 err %v", err)
	} else {
		t.Logf("vendor5 keys=%v Vendor1099=%v", keysOf(v5), v5["Vendor1099"])
	}
	bill, err := fetchV3(ctx, ac, "bill", "120")
	if err != nil {
		t.Logf("bill120 err %v", err)
	} else {
		t.Logf("bill120 keys=%v", keysOf(bill))
		t.Logf("bill120 VendorRef=%v TotalAmt=%v Balance=%v TxnTaxDetail=%v", bill["VendorRef"], bill["TotalAmt"], bill["Balance"], bill["TxnTaxDetail"])
		raw, _ := json.Marshal(bill["Line"])
		preview := string(raw)
		if len(preview) > 600 {
			preview = preview[:600]
		}
		t.Logf("bill120 Line=%s", preview)
	}
	prefs, err := ReplayQuery(ctx, "Preferences", "", "", 5)
	if err != nil {
		t.Logf("prefs query err %v", err)
	} else {
		t.Logf("prefs n=%d note=%s prefs=%v", prefs.Counts["items"], prefs.Note, prefs.Prefs)
	}
}

func TestTPARBillsAndTax(t *testing.T) {
	if os.Getenv("TPAR_PEEL") == "" {
		t.Skip("set TPAR_PEEL=1")
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
		t.Fatalf("tax raw %v", err)
	}
	raw, _ := readBody(resp)
	drainAndClose(resp)
	preview := string(raw)
	if len(preview) > 700 {
		preview = preview[:700]
	}
	t.Logf("tax raw status=%d body=%s", resp.StatusCode, preview)

	u2 := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/query?minorversion=73&query=select+*+from+Preferences", ac.realm)
	resp2, err := ac.getJSON(ctx, u2, "")
	if err != nil {
		t.Fatalf("prefs raw %v", err)
	}
	raw2, _ := readBody(resp2)
	drainAndClose(resp2)
	var wrap map[string]any
	_ = json.Unmarshal(raw2, &wrap)
	qr, _ := wrap["QueryResponse"].(map[string]any)
	t.Logf("prefs raw status=%d qrKeys=%v", resp2.StatusCode, keysOf(qr))
	if arr, ok := qr["Preferences"].([]any); ok && len(arr) > 0 {
		if p, ok := arr[0].(map[string]any); ok {
			t.Logf("prefs keys=%v", keysOf(p))
			if ai, ok := p["AccountingInfoPrefs"].(map[string]any); ok {
				t.Logf("AccountingInfoPrefs=%v", ai)
			}
			if tp, ok := p["TaxPrefs"].(map[string]any); ok {
				t.Logf("TaxPrefs=%v", tp)
			}
		}
	}
}
