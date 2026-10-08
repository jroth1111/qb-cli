package gql

import (
	"encoding/json"
	"strings"
	"testing"
)

func nativeInspectionFixture(t *testing.T, taxable any) *Response {
	t.Helper()
	request, err := TransactionInspectionRequest("12", "7", false)
	if err != nil {
		t.Fatal(err)
	}
	node := map[string]any{"id": request.Variables["id_0"], "type": "PURCHASE", "entityVersion": "3",
		"header":      map[string]any{"amount": "12.34", "txnDate": "2025-01-01", "cleared": nil},
		"qboAppData":  map[string]any{"hasPurchaseTax": false},
		"traits":      map[string]any{"tax": map[string]any{"taxType": "NOT_APPLICABLE", "totalTaxAmount": nil}, "payment": map[string]any{"paymentType": "CREDIT_CARD"}},
		"lines":       map[string]any{"accountLines": map[string]any{"edges": []any{map[string]any{"node": map[string]any{"amount": "12.34", "sequence": "1", "traits": map[string]any{"tax": taxable}}}}}},
		"stageEntity": map[string]any{"edges": []any{map[string]any{"node": map[string]any{"matchMode": "MANUAL_MATCH", "qboAppData": map[string]any{"ruleId": nil, "matchModeDisplayName": "Manually matched"}}}}},
	}
	raw, _ := json.Marshal(map[string]any{"data": map[string]any{"node": node}})
	return &Response{Status: 200, Body: raw}
}

func TestNativeInspectionPreservesTaxNullAndBoolean(t *testing.T) {
	for _, tax := range []any{nil, map[string]any{"taxable": nil}, map[string]any{"taxable": true}, map[string]any{"taxable": false}, map[string]any{}} {
		response := nativeInspectionFixture(t, tax)
		result, err := ParseTransactionInspection(response, "12", "7", false)
		if err != nil {
			t.Fatal(err)
		}
		if !result.RecordIdentityVerified || !result.ClearingStateRequiresRegister || result.MetadataEffectsCertified {
			t.Fatal("inspection overstated its evidence")
		}
		var line struct {
			Traits struct {
				Tax any `json:"tax"`
			} `json:"traits"`
		}
		if err = json.Unmarshal(result.AccountLines[0], &line); err != nil {
			t.Fatal(err)
		}
		want, _ := json.Marshal(tax)
		got, _ := json.Marshal(line.Traits.Tax)
		if string(want) != string(got) {
			t.Fatalf("normalized tax trait: %s -> %s", want, got)
		}
		if !strings.Contains(string(result.Header), `"cleared":null`) {
			t.Fatal("null cleared lost")
		}
		if result.ObservationPresence["headerCleared"] != "null" {
			t.Fatal("nullable clearing observation not explicit")
		}
		if !strings.Contains(string(result.BankLinks[0]), `"ruleId":null`) {
			t.Fatal("null rule ID lost")
		}
		if len(result.MatchControls) != 1 || result.MatchControls[0].UnmatchPath != unmatchImmediate {
			t.Fatal("manual match immediate-effect warning missing")
		}
	}
}

func TestNativeInspectionRejectsOmittedRequestedFields(t *testing.T) {
	response := nativeInspectionFixture(t, nil)
	var envelope map[string]any
	if err := json.Unmarshal(response.Body, &envelope); err != nil {
		t.Fatal(err)
	}
	node := envelope["data"].(map[string]any)["node"].(map[string]any)
	delete(node, "stageEntity")
	response.Body, _ = json.Marshal(envelope)
	if _, err := ParseTransactionInspection(response, "12", "7", false); err == nil {
		t.Fatal("omitted link field converted into empty links")
	}
	node["stageEntity"] = nil
	response.Body, _ = json.Marshal(envelope)
	result, err := ParseTransactionInspection(response, "12", "7", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.BankLinks != nil || result.ObservationPresence["stageEntity"] != "null" {
		t.Fatal("null links converted into empty links")
	}
}

func TestNativeInspectionUnknownMatchModeDoesNotInventBehavior(t *testing.T) {
	response := nativeInspectionFixture(t, nil)
	response.Body = []byte(strings.Replace(string(response.Body), "MANUAL_MATCH", "UNSEEN_MODE", 1))
	result, err := ParseTransactionInspection(response, "12", "7", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.MatchControls[0].UnmatchPath != unmatchUnknown {
		t.Fatal("invented behavior for unknown mode")
	}
	response.Body = []byte(strings.Replace(string(response.Body), "UNSEEN_MODE", "AUTO_ADD_AND_MATCH_RULE", 1))
	result, err = ParseTransactionInspection(response, "12", "7", false)
	if err != nil {
		t.Fatal(err)
	}
	if result.MatchControls[0].UnmatchPath != unmatchStageUpdate {
		t.Fatal("added-rule branch not distinguished")
	}
}

func TestNativeInspectionRejectsWrongIdentityErrorsAndType(t *testing.T) {
	for _, change := range []func(*Response){
		func(r *Response) { r.Status = 401 },
		func(r *Response) { r.Body = []byte(strings.Replace(string(r.Body), ":7\"", ":8\"", 1)) },
		func(r *Response) { r.Body = []byte(strings.Replace(string(r.Body), "PURCHASE", "INVOICE", 1)) },
		func(r *Response) { r.Body = []byte(`{"data":{"node":null}}`) },
		func(r *Response) { r.Body = []byte(`{"errors":[{"message":"denied"}]}`) },
		func(r *Response) { r.Errors = []GraphQLError{{Message: "partial read"}} },
	} {
		response := nativeInspectionFixture(t, nil)
		change(response)
		if _, err := ParseTransactionInspection(response, "12", "7", false); err == nil {
			t.Fatal("accepted invalid native observation")
		}
	}
	if _, err := ParseTransactionInspection(nativeInspectionFixture(t, nil), "13", "7", false); err == nil {
		t.Fatal("accepted wrong company")
	}
	if _, err := ParseTransactionInspection(nativeInspectionFixture(t, nil), "12", "7", true); err == nil {
		t.Fatal("accepted debit as credit")
	}
}

func TestTransactionInspectionRequestIsReadOnlyAndInputBound(t *testing.T) {
	for _, id := range []string{"", "7'", "olb-7", "7:8", "-7", "0"} {
		if _, err := TransactionInspectionRequest("12", id, false); err == nil {
			t.Fatalf("accepted %q", id)
		}
	}
	r, err := TransactionInspectionRequest("12", "7", true)
	if err != nil {
		t.Fatal(err)
	}
	if r.Op.Kind != KindQuery || !strings.HasPrefix(strings.TrimSpace(r.Op.Document), "query ") {
		t.Fatal("not a read-only query")
	}
	if r.Variables["with"] != "type='PURCHASE' && paymentType='CREDIT_CARD_CREDIT' && txnType='PURCHASE'" {
		t.Fatal("credit form filter drift")
	}
}
