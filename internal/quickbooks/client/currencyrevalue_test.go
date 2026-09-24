package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// The currency-revalue contract (Settings > Currencies > home row > Revalue
// currency, captured on TC2 2026-09-19):
//
//	GET  /api/neo/v1/company/{realm}/multicurrency/calculateHomeCurrencyAdjustment
//	     ?currencyCode={home}&exchangeRate={rate}&revaluedDate={dd/MM/yyyy}
//	     → bare entity array
//	POST /api/neo/v1/company/{realm}/multicurrency/doHomeCurrencyAdjustment
//	     {adjustmentDate, currency, exchangeRate, memo, selectedEntityInfoList}
//	     → bare JE id
//
// The POST needs the multicurrency plugin's own apikey + plugin-id + revalue
// Referer + fresh csrftoken + Accept */* — Accept:application/json 500s
// (bisected live).
var revalueCalcPath string // recorded per-request for the calc-param test

func revalueServer(t *testing.T, calcResp, postResp string, postStatus int) *domServer {
	t.Helper()
	saveUsableURIHost(t)
	revalueCalcPath = ""
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		s.mu.Lock()
		s.calls++
		s.lastMeth = r.Method
		s.lastPath = r.URL.Path + "?" + r.URL.RawQuery
		s.lastBody = b
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "calculateHomeCurrencyAdjustment"):
			revalueCalcPath = r.URL.Path + "?" + r.URL.RawQuery
			_, _ = w.Write([]byte(calcResp))
		case strings.Contains(r.URL.Path, "doHomeCurrencyAdjustment"):
			if postStatus != 0 && postStatus != 200 {
				w.WriteHeader(postStatus)
				_, _ = w.Write([]byte(`{"message":"System is unable to process your request","code":-1}`))
				return
			}
			_, _ = w.Write([]byte(postResp))
		case strings.Contains(r.URL.Path, "/query"):
			q, _ := url.QueryUnescape(r.URL.RawQuery)
			if strings.Contains(q, "from Preferences") {
				_, _ = w.Write([]byte(`{"QueryResponse":{"Preferences":[{` +
					`"CurrencyPrefs":{"HomeCurrency":{"value":"AUD"},"MultiCurrencyEnabled":true}}]}}`))
				return
			}
			_, _ = w.Write([]byte(`{"QueryResponse":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

const revalueEntities = `[{"entityType":"Current assets","entityId":30,"entityTypeId":4,"entityName":"Undeposited funds","currentCurrencyBalance":"50.00","currentHomeBalance":"50.00","adjustedHomeBalance":"50.00","exchangeGainLoss":"0.00"},{"entityType":"Accounts receivable (A&#x2F;R)","entityId":48,"entityTypeId":4,"entityName":"Accounts Receivable (A&#x2F;R)","currentCurrencyBalance":"60.00","currentHomeBalance":"60.00","adjustedHomeBalance":"60.00","exchangeGainLoss":"0.00"}]`

func TestCurrencyRevalueWireShape(t *testing.T) {
	srv := revalueServer(t, revalueEntities, "204", 200)
	res, err := ReplayCurrencyRevalue(context.Background(), "26/09/2026", "")
	if err != nil {
		t.Fatalf("revalue: %v", err)
	}
	if res.Op != "revalue" || res.Entity != "HomeCurrencyAdjustment" || res.Item.ID != "204" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(pathq, "doHomeCurrencyAdjustment") {
		t.Fatalf("last call = %s %s", meth, pathq)
	}
	var sent struct {
		AdjustmentDate string            `json:"adjustmentDate"`
		Currency       string            `json:"currency"`
		ExchangeRate   string            `json:"exchangeRate"`
		Memo           string            `json:"memo"`
		Entities       []json.RawMessage `json:"selectedEntityInfoList"`
	}
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("sent body: %v", err)
	}
	if sent.AdjustmentDate != "2026-09-26" || sent.Currency != "AUD" || sent.ExchangeRate != "1" {
		t.Fatalf("sent = %+v", sent)
	}
	if len(sent.Entities) != 2 {
		t.Fatalf("entities = %d, want 2 (verbatim from calculate)", len(sent.Entities))
	}
	// entity rows pass through verbatim (json.Marshal re-escapes & as &
	// — JSON-equivalent after decode, proven live by JE 204)
	if !strings.Contains(string(sent.Entities[1]), `"entityId":48`) {
		t.Fatalf("entity rows should pass through verbatim: %s", sent.Entities[1])
	}
}

func TestCurrencyRevalueCalcParams(t *testing.T) {
	revalueServer(t, revalueEntities, "9", 200)
	if _, err := ReplayCurrencyRevalue(context.Background(), "26/09/2026", "1.55"); err != nil {
		t.Fatalf("revalue: %v", err)
	}
	if revalueCalcPath == "" {
		t.Fatalf("no calculate call made")
	}
	if !strings.Contains(revalueCalcPath, "exchangeRate=1.55") || !strings.Contains(revalueCalcPath, "revaluedDate=26%2F09%2F2026") {
		t.Fatalf("calc query = %s", revalueCalcPath)
	}
}

func TestCurrencyRevalueValidation(t *testing.T) {
	revalueServer(t, revalueEntities, "9", 200)
	if _, err := ReplayCurrencyRevalue(context.Background(), "", ""); err == nil || !strings.Contains(err.Error(), "--date") {
		t.Fatalf("missing date should fail, got %v", err)
	}
	if _, err := ReplayCurrencyRevalue(context.Background(), "bogus", ""); err == nil || !strings.Contains(err.Error(), "--date") {
		t.Fatalf("bad date should fail, got %v", err)
	}
	if _, err := ReplayCurrencyRevalue(context.Background(), "26/09/2026", "abc"); err == nil || !strings.Contains(err.Error(), "--rate") {
		t.Fatalf("bad rate should fail, got %v", err)
	}
	if _, err := ReplayCurrencyRevalue(context.Background(), "26/09/2026", "0"); err == nil || !strings.Contains(err.Error(), "--rate") {
		t.Fatalf("zero rate should fail, got %v", err)
	}
}

func TestCurrencyRevalueEmptyEntities(t *testing.T) {
	revalueServer(t, `[]`, "9", 200)
	_, err := ReplayCurrencyRevalue(context.Background(), "26/09/2026", "")
	if err == nil || !strings.Contains(err.Error(), "no revalue-eligible balances") {
		t.Fatalf("empty entity table should fail, got %v", err)
	}
}

func TestCurrencyRevalueHTTPError(t *testing.T) {
	revalueServer(t, revalueEntities, "", 500)
	_, err := ReplayCurrencyRevalue(context.Background(), "26/09/2026", "")
	if err == nil || !strings.Contains(err.Error(), "unable to process") {
		t.Fatalf("HTTP 500 should surface, got %v", err)
	}
}
