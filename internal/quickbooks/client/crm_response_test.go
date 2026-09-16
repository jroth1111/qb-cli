package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func crmResponseStub(t *testing.T, status int, body string) {
	t.Helper()
	old := executeCRM
	executeCRM = func(context.Context, *RequestPlan) ([]byte, int, error) { return []byte(body), status, nil }
	t.Cleanup(func() { executeCRM = old })
}

func crmResponseCalls() map[string]func() error {
	ctx := context.Background()
	return map[string]func() error{
		"list":               func() error { _, e := ReplayLeadList(ctx, 0, 25, false); return e },
		"get":                func() error { _, e := ReplayLeadGet(ctx, "L1"); return e },
		"search":             func() error { _, e := ReplayLeadSearch(ctx, "lead", 0, 25); return e },
		"create":             func() error { _, e := ReplayLeadCreate(ctx, CrmLeadInput{DisplayName: "Lead"}); return e },
		"update":             func() error { _, e := ReplayLeadUpdate(ctx, "L1", CrmLeadInput{DisplayName: "Lead"}); return e },
		"delete":             func() error { _, e := ReplayLeadDelete(ctx, "L1"); return e },
		"count":              func() error { _, e := ReplayLeadCount(ctx, ""); return e },
		"bulk":               func() error { _, e := ReplayLeadBulk(ctx, "", []CrmLeadInput{{DisplayName: "Lead"}}); return e },
		"convert":            func() error { _, e := ReplayLeadConvert(ctx, "L1"); return e },
		"associations":       func() error { _, e := ReplayAssociationList(ctx, ""); return e },
		"opportunities":      func() error { _, e := ReplayOpportunityList(ctx, 0, 25); return e },
		"opportunity-create": func() error { _, e := ReplayOpportunityCreate(ctx, CrmOpportunityInput{Name: "Opportunity"}); return e },
	}
}

func TestCrmResponseRejectsHTTPFailures(t *testing.T) {
	for name, call := range crmResponseCalls() {
		t.Run(name, func(t *testing.T) {
			for _, status := range []int{0, 199, 300, 401, 403, 404, 409, 422, 500} {
				t.Run(fmt.Sprint(status), func(t *testing.T) {
					crmResponseStub(t, status, `{"error":"private-server-detail"}`)
					err := call()
					var replay *ReplayError
					if !errors.As(err, &replay) || replay.Status != status {
						t.Fatalf("got %v, want ReplayError status %d", err, status)
					}
					if strings.Contains(err.Error(), "private-server-detail") {
						t.Fatal("raw error body leaked")
					}
				})
			}
		})
	}
}

func TestCrmResponseRejectsMalformedAndErrorEnvelopes(t *testing.T) {
	for name, call := range crmResponseCalls() {
		t.Run(name, func(t *testing.T) {
			for i, body := range []string{"", "<html>denied</html>", `{"id":`, "null", `{"errors":[{"message":"denied"}]}`, `{"error":"denied"}`, `{"Fault":{"Error":[{}]}}`, `{"success":false}`, `{"ok":false}`, `{"status":500}`, `{"status":"FAILED"}`} {
				t.Run(fmt.Sprint(i), func(t *testing.T) {
					crmResponseStub(t, 200, body)
					if err := call(); err == nil {
						t.Fatalf("accepted failing response %q", body)
					}
				})
			}
		})
	}
}

func TestCrmResponseNoContentIsNotRecordAcknowledgement(t *testing.T) {
	validEmpty := map[string]bool{"list": true, "search": true, "associations": true, "opportunities": true, "delete": true}
	for name, call := range crmResponseCalls() {
		t.Run(name, func(t *testing.T) {
			crmResponseStub(t, 204, "")
			err := call()
			if (err == nil) != validEmpty[name] {
				t.Fatalf("HTTP 204 error = %v, empty allowed %v", err, validEmpty[name])
			}
		})
	}
	t.Run("unexpected-body", func(t *testing.T) {
		crmResponseStub(t, 204, `{"id":"L1"}`)
		if _, err := ReplayLeadDelete(context.Background(), "L1"); err == nil {
			t.Fatal("accepted body on 204")
		}
	})
}

func TestCrmResponseLeadAliasesAndFields(t *testing.T) {
	crmResponseStub(t, 201, `{"nameId":"L1","displayName":"Lead","organizationName":"Org","email":"lead@example.invalid","phone":"555","status":"HOT","createdAt":"2026-01-01","updatedAt":"2026-01-02","futureField":true}`)
	got, err := ReplayLeadCreate(context.Background(), CrmLeadInput{DisplayName: "request must not leak into result"})
	want := &CrmLead{ID: "L1", DisplayName: "Lead", OrganizationName: "Org", Email: "lead@example.invalid", Phone: "555", Status: "HOT", CreatedAt: "2026-01-01", UpdatedAt: "2026-01-02"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, %v; want %+v", got, err, want)
	}
	for _, body := range []string{`{"customerId":"L1","name":"Lead"}`, `{"id":"L1","companyName":"Lead"}`, `{"nameId":"L1","organizationName":"Lead"}`} {
		row, err := projectCrmLead(json.RawMessage(body))
		if err != nil || row.ID != "L1" || row.DisplayName != "Lead" {
			t.Fatalf("alias %s: %+v %v", body, row, err)
		}
	}
}

func TestCrmResponseOpportunityFields(t *testing.T) {
	for _, key := range []string{"name", "displayName"} {
		t.Run(key, func(t *testing.T) {
			crmResponseStub(t, 201, fmt.Sprintf(`{"id":"O1",%q:"Opportunity","stage":"OPEN","status":"ACTIVE","amount":12.5,"customerId":"C1"}`, key))
			got, err := ReplayOpportunityCreate(context.Background(), CrmOpportunityInput{Name: "request"})
			want := &CrmOpportunity{ID: "O1", Name: "Opportunity", Stage: "OPEN", Status: "ACTIVE", Amount: 12.5, CustomerID: "C1"}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("got %+v %v, want %+v", got, err, want)
			}
		})
	}
}

func TestCrmResponseKnownListEnvelopes(t *testing.T) {
	for _, key := range []string{"content", "items", "data", "array"} {
		t.Run(key, func(t *testing.T) {
			body := `[{"nameId":"L1","displayName":"Lead","email":"lead@example.invalid"}]`
			if key != "array" {
				body = fmt.Sprintf(`{%q:%s,"totalElements":7}`, key, body)
			}
			crmResponseStub(t, 200, body)
			got, err := ReplayLeadSearch(context.Background(), "Lead", 2, 10)
			wantTotal := 7
			if key == "array" {
				wantTotal = 1
			}
			if err != nil || got.Count != 1 || got.Total != wantTotal || got.Page != 2 || got.Size != 10 || got.Leads[0].Email != "lead@example.invalid" {
				t.Fatalf("got %+v %v", got, err)
			}
		})
	}
	// These are REST search envelopes; the lead list itself has the separate
	// typed DataAccessLeads GraphQL contract.
	for i, body := range []string{`[]`, `{"content":[],"totalElements":0}`, `{"items":[],"errors":[ ],"error":{ }}`, `{"data":[]}`} {
		t.Run(fmt.Sprintf("empty-%d", i), func(t *testing.T) {
			crmResponseStub(t, 200, body)
			got, err := ReplayLeadSearch(context.Background(), "Lead", 0, 25)
			if err != nil || got.Leads == nil || got.Count != 0 {
				t.Fatalf("got %+v %v", got, err)
			}
		})
	}
}

func TestCrmResponseRejectsMissingOrInvalidRecords(t *testing.T) {
	for name, call := range crmResponseCalls() {
		if name == "delete" {
			continue
		} // HTTP 200 JSON is a completed delete response; HTTP 204 can omit it.
		t.Run(name, func(t *testing.T) {
			for i, body := range []string{`{}`, `{"message":"not a record"}`, `{"data":null}`} {
				t.Run(fmt.Sprint(i), func(t *testing.T) {
					crmResponseStub(t, 200, body)
					if err := call(); err == nil {
						t.Fatalf("accepted absent acknowledgement %s", body)
					}
				})
			}
		})
	}
	for _, body := range []string{`{"id":12}`, `{"id":"L1","email":[]}`, `{"nameId":"L1","error":"denied"}`} {
		if _, err := projectCrmLead(json.RawMessage(body)); err == nil {
			t.Fatalf("accepted invalid lead %s", body)
		}
	}
	for _, body := range []string{`{"id":"O1","amount":"invalid"}`, `{"name":"Opportunity","success":false}`} {
		if _, err := projectCrmOpportunity(json.RawMessage(body)); err == nil {
			t.Fatalf("accepted invalid opportunity %s", body)
		}
	}
	for i, body := range []string{`{"content":null}`, `{"items":{}}`, `{"data":[null]}`, `{"items":[{}]}`, `{"content":[{"id":"L1"},{"error":"denied"}]}`, `{"content":[],"totalElements":"invalid"}`} {
		t.Run(fmt.Sprintf("bad-list-%d", i), func(t *testing.T) {
			crmResponseStub(t, 200, body)
			if _, err := ReplayLeadList(context.Background(), 0, 25, false); err == nil {
				t.Fatalf("accepted invalid list %s", body)
			}
		})
	}
}

func TestCrmResponseConversionAcknowledgement(t *testing.T) {
	for _, tc := range []struct {
		body, state, id string
		ok              bool
	}{
		{`{"status":"CONVERTED","nameId":"C1"}`, "CONVERTED", "C1", true},
		{`{"state":"CONVERTED","customerID":"C1"}`, "CONVERTED", "C1", true},
		{`{"customerId":"C1"}`, "", "C1", true},
		{`{"state":"FAILED","customerId":"C1"}`, "", "", false},
		{`{"status":"PENDING"}`, "", "", false},
		{`{"data":null}`, "", "", false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			crmResponseStub(t, 200, tc.body)
			got, err := ReplayLeadConvert(context.Background(), "L1")
			if (err == nil) != tc.ok {
				t.Fatalf("got %+v %v", got, err)
			}
			if tc.ok && (got.State != tc.state || got.CustomerID != tc.id) {
				t.Fatalf("invented/omitted acknowledgement: %+v", got)
			}
		})
	}
}

func TestCrmResponseCountsAndAssociations(t *testing.T) {
	for _, tc := range []struct {
		body string
		want int
		ok   bool
	}{{"0", 0, true}, {"7", 7, true}, {`{"count":3}`, 3, true}, {"-1", 0, false}, {"1.5", 0, false}, {`{"count":null}`, 0, false}, {`{"count":"7"}`, 0, false}} {
		t.Run(tc.body, func(t *testing.T) {
			crmResponseStub(t, 200, tc.body)
			got, err := ReplayLeadCount(context.Background(), "")
			if (err == nil) != tc.ok || (tc.ok && got != tc.want) {
				t.Fatalf("got %d %v", got, err)
			}
		})
	}
	t.Run("bulk", func(t *testing.T) {
		crmResponseStub(t, 201, `{"content":[{"id":"L1","displayName":"Lead"}]}`)
		got, err := ReplayLeadBulk(context.Background(), "", []CrmLeadInput{{}, {}})
		if err != nil || got.Created != 1 || got.SourceType != "CSV" {
			t.Fatalf("got %+v %v", got, err)
		}
	})
	t.Run("associations", func(t *testing.T) {
		crmResponseStub(t, 200, `{"items":[{"leadID":"L1","customerID":"C1"}]}`)
		got, err := ReplayAssociationList(context.Background(), "")
		if err != nil || got.Count != 1 || got.Associations[0].LeadID != "L1" || got.Associations[0].CustomerID != "C1" {
			t.Fatalf("got %+v %v", got, err)
		}
	})
}

func TestCrmResponsePreservesPlans(t *testing.T) {
	old := executeCRM
	t.Cleanup(func() { executeCRM = old })
	plans := []*RequestPlan{PlanLeadList(2, 5, true)}
	update, err := PlanLeadUpdate("L1", CrmLeadInput{Email: "lead@example.invalid"})
	if err != nil {
		t.Fatal(err)
	}
	plans = append(plans, update)
	for _, plan := range plans {
		executeCRM = func(_ context.Context, got *RequestPlan) ([]byte, int, error) {
			if !reflect.DeepEqual(got, plan) {
				t.Fatalf("request changed: %+v vs %+v", got, plan)
			}
			return []byte(`{"content":[]}`), http.StatusOK, nil
		}
		if _, _, err := crmResponse(context.Background(), plan); err != nil {
			t.Fatal(err)
		}
	}
}
