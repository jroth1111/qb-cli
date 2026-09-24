package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRESTMutationRejectsEmptyReceipt(t *testing.T) {
	for _, name := range []string{"workflow", "folio", "performance", "proposal", "report", "formstyle"} {
		t.Run(name, func(t *testing.T) {
			saveUsableURIHost(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if name == "formstyle" && r.Method == http.MethodGet {
					_, _ = w.Write([]byte(formStyleDoc))
					return
				}
				_, _ = w.Write([]byte(`{}`))
			}))
			defer srv.Close()
			interceptHTTP(t, srv.URL)
			ac, err := newAPIClient()
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "workflow":
				_, err = workflowDo(context.Background(), ac, "create", http.MethodPost, appconnectSubsURL, nil, "")
			case "folio":
				_, err = folioDo(context.Background(), ac, "create", http.MethodPost, PlannedFolioMutateURL(), nil, "")
			case "performance":
				_, err = perfDo(context.Background(), ac, "create", http.MethodPost, PlannedDashboardsURL(), nil, "")
			case "proposal":
				_, err = ReplayProposalCreate(context.Background(), "1", "CUSTOMER", "")
			case "report":
				_, err = ReplaySavedReportMutate(context.Background(), "create", "", "Report", "", "")
			case "formstyle":
				_, err = ReplayFormStyleUpdate(context.Background(), "1125188", "Renamed", "")
			}
			if err == nil {
				t.Fatal("empty receipt accepted")
			}
		})
	}
}

func TestProposalDeleteRejectsContradictoryReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := newProposalServer(t)
	srv.deleteBody = `{"id":1002,"isDeleted":false}`
	interceptHTTP(t, srv.URL)
	if _, err := ReplayProposalDelete(context.Background(), "1002"); err == nil {
		t.Fatal("undeleted proposal accepted")
	}
}

func TestSavedReportReceiptIdentityAndState(t *testing.T) {
	if validateSavedReportReceipt([]byte(`{"id":"x","active":false}`), "x", false) == nil {
		t.Fatal("update accepted inactive report")
	}
	for _, raw := range []string{`{}`, `null`, `{"id":"other","active":false}`, `{"id":"x","active":true}`, `{"id":"x"}`} {
		if validateSavedReportReceipt([]byte(raw), "x", true) == nil {
			t.Errorf("accepted deletion receipt %s", raw)
		}
	}
	if err := validateSavedReportReceipt([]byte(`{"report":{"id":"x","active":false}}`), "x", true); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowActionRequiresExpectedState(t *testing.T) {
	for _, action := range []string{"activate", "deactivate"} {
		t.Run(action, func(t *testing.T) {
			saveUsableURIHost(t)
			srv := newWFServer(t)
			srv.wfStatus = 200
			if action == "deactivate" {
				srv.wfBody = `{"id":"wf-7","status":"active"}`
			}
			interceptHTTP(t, srv.URL)
			if _, err := ReplayWorkflowMutate(context.Background(), "update", "wf-7", "", "", action); err == nil {
				t.Fatal("contradictory workflow state accepted")
			}
		})
	}
}
