package gql

import (
	"strings"
	"testing"
)

func TestBillsRequestUsesCompleteUnrestrictedQuery(t *testing.T) {
	w, err := BillDateWindow.ParseWindow("10/09/2026", "12/09/2026")
	if err != nil {
		t.Fatal(err)
	}
	r := BillsRequest(w)
	if err := r.Op.ValidateVars(r.Variables); err != nil {
		t.Fatal(err)
	}
	if r.Op.DetectWalkStyle() != WalkOffset || !strings.Contains(r.Op.Document, "status='ALL'") {
		t.Fatal("bill walk is incomplete")
	}
	if !strings.Contains(r.Op.Document, "fragment txnListFragment") || !strings.Contains(r.Variables["filterBy"].(string), "2026-09-10") {
		t.Fatal("bill selection or filter missing")
	}
}
