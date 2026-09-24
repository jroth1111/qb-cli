package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Regression tests for the captured api.intuit.com read surfaces added from
// the sales/expenses overview sweep: universalreportsgraphql allSales,
// dashboardframework EXPENSES_DASHBOARD, salestxnrisksvc eligibility,
// bill-pay-onboarding eligibility, stagetransactions StageEntity, b2bbillpay
// paymentApproval, b2bnetworkservice attributes, taxconfig IndirectTaxTaxGropups.

func TestSalesOverviewAggregates(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{
		"unbilled":{"aggregate":{"total":{"value":1200},"count":{"value":3}}},
		"estimates":{"aggregate":{"total":{"value":500},"count":{"value":2}}},
		"overdueInvoice":{"aggregate":{"total":{"value":0},"count":{"value":0}},"dueStatus":"OVERDUE"},
		"openInvoices":{"aggregate":{"total":{"value":900},"count":{"value":4}}},
		"last30daysPaid":{"total":{"value":3000},"count":{"value":7}}
	}}`)
	interceptHTTP(t, srv.URL)
	res, err := ReplaySalesOverview(context.Background(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 5 {
		t.Fatalf("got status=%d items=%d", res.Status, len(res.Items))
	}
	calls, meth, path, body := srv.snap()
	if calls != 1 || meth != http.MethodPost || !strings.Contains(path, "/graphql") {
		t.Fatalf("call = %s %s x%d", meth, path, calls)
	}
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		vars, _ := m["variables"].(map[string]any)
		if _, ok := vars["unbilledFilter"]; !ok {
			t.Errorf("missing unbilledFilter in variables")
		}
	}
}

func TestExpensesOverviewDashboard(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `[{"id":"sbg:1","name":"EXPENSES_DASHBOARD","panels":[{"name":"bills-status-funnel"}]}]`)
	interceptHTTP(t, srv.URL)
	res, err := ReplayExpensesOverview(context.Background(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 1 {
		t.Fatalf("got status=%d items=%d", res.Status, len(res.Items))
	}
	calls, meth, path, _ := srv.snap()
	if calls != 1 || meth != http.MethodGet || !strings.Contains(path, "/v1/dashboards") {
		t.Fatalf("call = %s %s x%d", meth, path, calls)
	}
}

func TestSalesTxnRiskEligibility(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"eligible":false,"reasons":["NO_PAYMENTS"]}`)
	interceptHTTP(t, srv.URL)
	res, err := ReplaySalesTxnRisk(context.Background(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 1 {
		t.Fatalf("got status=%d items=%d", res.Status, len(res.Items))
	}
	_, meth, path, _ := srv.snap()
	if meth != http.MethodGet || !strings.Contains(path, "/v2/risk/eligibility") {
		t.Fatalf("call = %s %s", meth, path)
	}
}

func TestBillPayOnboardingEligibility(t *testing.T) {
	saveUsableURIHost(t)
	// b2bbillpay/bill-pay-onboarding answer XML, not JSON.
	srv := newDomServer(t, http.StatusOK, `<EligibilityResponse><eligible>false</eligible></EligibilityResponse>`)
	interceptHTTP(t, srv.URL)
	res, err := ReplayBillPayOnboarding(context.Background(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 1 {
		t.Fatalf("got status=%d items=%d", res.Status, len(res.Items))
	}
}

func TestStageTransactionsQuery(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `[{"$type":"/Result","data":{"entities":[{"id":"st1","state":"TO_BE_REVIEWED"}],"totalCount":1}}]`)
	interceptHTTP(t, srv.URL)
	res, err := ReplayStageTransactions(context.Background(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK {
		t.Fatalf("got status=%d", res.Status)
	}
	calls, meth, path, body := srv.snap()
	if calls != 1 || meth != http.MethodPost || path != "/stage/entities" {
		t.Fatalf("call = %s %s x%d", meth, path, calls)
	}
	if !strings.Contains(string(body), "StageEntity") {
		t.Errorf("body missing StageEntity query: %s", body)
	}
}

func TestB2BBillpayXML(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `<ApprovalIndicatorsResponse><billPayApprovalWorkflowEnabled>false</billPayApprovalWorkflowEnabled></ApprovalIndicatorsResponse>`)
	interceptHTTP(t, srv.URL)
	res, err := ReplayB2BBillpay(context.Background(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || len(res.Items) < 1 {
		t.Fatalf("got status=%d items=%d", res.Status, len(res.Items))
	}
}

func TestB2BNetworkAttributes(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"attributes":[{"key":"tradeCredit","value":"eligible"}]}`)
	interceptHTTP(t, srv.URL)
	res, err := ReplayB2BNetwork(context.Background(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK {
		t.Fatalf("got status=%d", res.Status)
	}
	_, meth, path, _ := srv.snap()
	if meth != http.MethodGet || !strings.Contains(path, "/v2/directory/attributes") {
		t.Fatalf("call = %s %s", meth, path)
	}
}

func TestTaxConfigGroupsEdges(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, domGQL(`{"indirectTaxTaxGroups":{"edges":[
		{"node":{"globalId":"g1","legacyName":"GST","activeStatus":"ACTIVE"}},
		{"node":{"globalId":"g2","legacyName":"GST Free","activeStatus":"ACTIVE"}}
	]}}`))
	interceptHTTP(t, srv.URL)
	res, err := ReplayTaxConfigGroups(context.Background(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 2 {
		t.Fatalf("got status=%d items=%d", res.Status, len(res.Items))
	}
	if res.Items[0].ID != "g1" {
		t.Errorf("item id = %q, want g1", res.Items[0].ID)
	}
}

func TestReconciliationNodeRead(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"node":{"id":"djQuMTo6MTIzNDU6Y2VjNGZjZTgyZg:44","__typename":"Integration_Reconciliation","inProgress":true,"statementEndingDate":"2026-09-16","account":{"id":"a44","name":"CR-Test-Account"}}}}`)
	interceptHTTP(t, srv.URL)
	res, err := ReplayReconciliation(context.Background(), "44", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 1 {
		t.Fatalf("got status=%d items=%d", res.Status, len(res.Items))
	}
	_, meth, path, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(path, "/api/v4/graphql") {
		t.Fatalf("call = %s %s", meth, path)
	}
	// The node id is base64("v4.1:<realm>:<typeHash>") + ":" + accountID.
	if !strings.Contains(string(body), "OjQ0") && !strings.Contains(string(body), `:44`) {
		t.Errorf("body missing account-keyed node id: %s", body)
	}
	// Missing account id must fail fast before any network.
	if _, err := ReplayReconciliation(context.Background(), "", "", 20); err == nil {
		t.Error("empty account id should error")
	}
}

func TestReconcileSessionMutate(t *testing.T) {
	saveUsableURIHost(t)
	srv := &domServer{t: t}
	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		srv.mu.Lock()
		srv.calls++
		srv.lastMeth, srv.lastPath, srv.lastBody = r.Method, r.URL.Path, body
		srv.mu.Unlock()
		inProgress := strings.Contains(string(body), `"reconcileAction":"BEGIN"`)
		var req struct {
			Variables struct {
				Input struct {
					Rec struct {
						ID string `json:"id"`
					} `json:"integrationReconciliation"`
				} `json:"input_0"`
			} `json:"variables"`
		}
		_ = json.Unmarshal(body, &req)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"updateIntegration_Reconciliation":{"clientMutationId":"qb-cli","integrationReconciliation":{"id":"` + req.Variables.Input.Rec.ID + `","inProgress":` + map[bool]string{true: "true", false: "false"}[inProgress] + `,"statementEndingBalance":"0.00","statementEndingDate":"2026-11-30"}}}}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	res, err := ReplayReconcileSession(context.Background(), "BEGIN", "45", "0.00", "2026-11-30")
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || res.Op != "begin" {
		t.Fatalf("got status=%d op=%s", res.Status, res.Op)
	}
	_, meth, path, body := srv.snap()
	if meth != http.MethodPost || !strings.Contains(path, "qbonline-aws") && !strings.Contains(path, "/v4/graphql") {
		t.Fatalf("call = %s %s", meth, path)
	}
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		vars, _ := m["variables"].(map[string]any)
		in, _ := vars["input_0"].(map[string]any)
		rec, _ := in["integrationReconciliation"].(map[string]any)
		if rec["reconcileAction"] != "BEGIN" {
			t.Errorf("reconcileAction = %v, want BEGIN", rec["reconcileAction"])
		}
		if rec["statementEndingBalance"] != "0.00" || rec["statementEndingDate"] != "2026-11-30" {
			t.Errorf("statement fields = %v %v", rec["statementEndingBalance"], rec["statementEndingDate"])
		}
		if pi, _ := in["pluginInfo"].(map[string]any); pi == nil {
			t.Error("missing pluginInfo envelope")
		}
	}
	// CANCEL needs no statement fields.
	res2, err := ReplayReconcileSession(context.Background(), "CANCEL", "45", "", "")
	if err != nil || res2.Op != "cancel" {
		t.Fatalf("cancel: %v op=%s", err, res2.Op)
	}
	// Validation: missing account id, missing BEGIN fields, unknown action.
	if _, err := ReplayReconcileSession(context.Background(), "BEGIN", "", "0", "2026-01-01"); err == nil {
		t.Error("empty account id should error")
	}
	if _, err := ReplayReconcileSession(context.Background(), "BEGIN", "45", "", ""); err == nil {
		t.Error("BEGIN without ending fields should error")
	}
	if _, err := ReplayReconcileSession(context.Background(), "FINISH", "45", "", ""); err == nil {
		t.Error("unproven action should error")
	}
}

// financeAssets rides assetservice.api.intuit.com/v1/graphql; live-proven on
// TC2 with zero assets (typed Finance_AssetConnection, edges=[]).
func TestFinanceAssetsEdges(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"financeAssets":{"edges":[
		{"node":{"assetId":"fa-9","name":"Delivery Van","status":"ACTIVE","purchaseValue":{"currency":"AUD","value":45000}}},
		{"node":{"assetId":"fa-10","name":"Lathe","status":"DISPOSED","purchaseValue":{"currency":"AUD","value":12000}}}
	],"pageInfo":{"hasNextPage":false}}}}`)
	interceptHTTP(t, srv.URL)
	res, err := ReplayFinanceAssets(context.Background(), "", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 2 {
		t.Fatalf("got status=%d items=%d", res.Status, len(res.Items))
	}
	if res.Items[0].ID != "fa-9" || res.Items[0].Name != "Delivery Van" {
		t.Fatalf("node projection = %+v", res.Items[0])
	}
	calls, meth, path, body := srv.snap()
	if calls != 1 || meth != http.MethodPost || !strings.Contains(path, "/v1/graphql") {
		t.Fatalf("call = %s %s x%d", meth, path, calls)
	}
	var m map[string]any
	if json.Unmarshal(body, &m) == nil {
		vars, _ := m["variables"].(map[string]any)
		if _, ok := vars["financeAssetFilter"]; !ok {
			t.Errorf("missing financeAssetFilter in variables")
		}
	}
	// --id narrows to the matching assetId.
	res, err = ReplayFinanceAssets(context.Background(), "fa-10", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "fa-10" {
		t.Fatalf("id filter items = %+v", res.Items)
	}
}

// Fixed-asset mutations ride the captured createAsset/updateAsset/deleteAsset
// documents; create→rename→delete was proven live on TC2 2026-09-17.
func TestFixedAssetMutate(t *testing.T) {
	saveUsableURIHost(t)
	srv := &domServer{t: t}
	srv.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		srv.mu.Lock()
		srv.calls++
		srv.lastMeth, srv.lastPath, srv.lastBody = r.Method, r.URL.Path, body
		srv.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), "financeDeleteAsset") {
			_, _ = w.Write([]byte(`{"data":{"financeDeleteAsset":{"assetId":"fa-new"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"financeCreateAsset":{"asset":{"assetId":"fa-new","name":"Lathe","status":"DEPRECIATING"}}}}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	res, err := ReplayFixedAssetMutate(context.Background(), "create", map[string]string{
		"name": "Lathe", "price": "12000", "life-years": "5",
		"asset-account": "31", "dep-expense-account": "1150040003",
		"purchase-date": "2026-09-17",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Op != "create" || res.Item.ID != "fa-new" {
		t.Fatalf("got %+v", res)
	}
	var sent map[string]any
	_, _, _, body := srv.snap()
	if json.Unmarshal(body, &sent) != nil {
		t.Fatal("no body captured")
	}
	vars, _ := sent["variables"].(map[string]any)
	model, _ := vars["model"].(map[string]any)
	assoc, _ := model["associatedIds"].(map[string]any)
	if assoc["coa_asset"] != "31" || assoc["coa_depreciation_expense"] != "1150040003" {
		t.Fatalf("associatedIds = %v", assoc)
	}
	if model["name"] != "Lathe" || model["assetId"] != "" {
		t.Fatalf("model = %v", model)
	}
	// delete sends the assetId as a bare String variable.
	_, err = ReplayFixedAssetMutate(context.Background(), "delete", map[string]string{"id": "fa-new"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, body = srv.snap()
	if json.Unmarshal(body, &sent) != nil {
		t.Fatal("no body captured")
	}
	if vars, _ := sent["variables"].(map[string]any); vars["model"] != "fa-new" {
		t.Fatalf("delete vars = %v", vars)
	}
	// create requires the COA link ids.
	if _, err := ReplayFixedAssetMutate(context.Background(), "create", map[string]string{"name": "X", "price": "1", "life-years": "5"}); err == nil {
		t.Fatal("missing-account create should fail")
	}
}

// Settings update rides settingsfacade updateQbAppFoundationQbSettings with
// optimistic-concurrency entityVersion — proven live on TC2 (round-trip
// accountNumbersEnabled, versions 7→9).
func TestSettingsUpdateVersion(t *testing.T) {
	saveUsableURIHost(t)
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		b, _ := io.ReadAll(r.Body)
		if strings.Contains(string(b), "accountingCoreSettings { entityVersion }") && !strings.Contains(string(b), "mutation") {
			_, _ = w.Write([]byte(`{"data":{"qbAppFoundationQbSettings":{"finance":{"accounting":{"accountingCore":{"accountingCoreSettings":{"entityVersion":"8"}}}}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":{"updateQbAppFoundationQbSettings":{"finance":{"accounting":{"accountingCore":{"accountingCoreSettings":{"entityVersion":"9"}}}},"identity":{"company":{"profile":{"entityVersion":null}}}}}}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	res, err := ReplaySettingsUpdate(context.Background(), map[string]any{"accountNumbersEnabled": true})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("expected version-read + update = 2 calls, got %d", calls)
	}
	if res.Op != "update" || !strings.Contains(res.Note, "8 -> 9") {
		t.Fatalf("got %+v", res)
	}
	if _, err := ReplaySettingsUpdate(context.Background(), map[string]any{}); err == nil {
		t.Fatal("empty patch should fail")
	}
}

// SALES_SETTINGS_GET — captured readSettings on salesettings.api.intuit.com.
// TC2 answers a null salesSettings row (no row provisioned); the contract is
// still proven: POST path, csrf override, null-vs-row projection, gql+HTTP errors.
func TestSalesSettingsNullRow(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"company":{"id":"9341457769756854","salesSettings":null}}}`)
	interceptHTTP(t, srv.URL)
	res, err := ReplaySalesSettings(context.Background(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != http.StatusOK || len(res.Items) != 0 || !strings.Contains(res.Note, "no sales-settings row") {
		t.Fatalf("null projection = %+v", res)
	}
	calls, meth, path, body := srv.snap()
	if calls != 1 || meth != http.MethodPost || !strings.Contains(path, "/v4/graphql") {
		t.Fatalf("call = %s %s x%d", meth, path, calls)
	}
	var m map[string]any
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal("no JSON body")
	}
	if !strings.Contains(m["query"].(string), "readSettings") || !strings.Contains(m["query"].(string), "showFullyConsumedLines") {
		t.Error("query doc missing readSettings/fields")
	}
}

func TestSalesSettingsRow(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"company":{"id":"r1","salesSettings":{"entityVersion":"4","showFullyConsumedLines":true}}}}`)
	interceptHTTP(t, srv.URL)
	res, err := ReplaySalesSettings(context.Background(), "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Items) != 1 || res.Items[0].ID != "r1" || !strings.Contains(res.Items[0].Name, "entityVersion=4") {
		t.Fatalf("row projection = %+v", res.Items)
	}
	if res.Prefs == nil {
		t.Error("Prefs should carry the settings map")
	}
}

func TestSalesSettingsErrors(t *testing.T) {
	saveUsableURIHost(t)
	srv := newDomServer(t, http.StatusOK, `{"data":{"company":null},"errors":[{"message":"company unresolved"}]}`)
	interceptHTTP(t, srv.URL)
	if _, err := ReplaySalesSettings(context.Background(), "", 20); err == nil || !strings.Contains(err.Error(), "company unresolved") {
		t.Fatalf("gql error = %v", err)
	}
	srv2 := newDomServer(t, http.StatusForbidden, `{"error":"nope"}`)
	interceptHTTP(t, srv2.URL)
	if _, err := ReplaySalesSettings(context.Background(), "", 20); err == nil {
		t.Error("403 should error")
	}
}
