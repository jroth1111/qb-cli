package client

import (
	"strings"
	"testing"
)

// TestTermPaymentMethodCreateBodies locks the Step 3 parity-gap contract:
// Term and PaymentMethod creates require --name and post a Name-only body
// (no TxnDate pollution from the generic default branch, which always sets
// TxnDate and would send an invalid field on list entities).
//
// Kill denominator: 3. Rejects (1) a missing entity case (no --name error,
// falling through to the default branch), (2) TxnDate leaking into the body,
// (3) a dropped Name.
func TestTermPaymentMethodCreateBodies(t *testing.T) {
	for _, entity := range []string{"Term", "PaymentMethod"} {
		if _, err := buildCreateBody(entity, map[string]string{}); err == nil {
			t.Errorf("%s create without --name should error, got nil", entity)
		} else if !strings.Contains(err.Error(), "--name") {
			t.Errorf("%s create error = %q, want it to name --name", entity, err)
		}

		body, err := buildCreateBody(entity, map[string]string{"name": "Step3 Probe"})
		if err != nil {
			t.Fatalf("%s create with --name failed: %v", entity, err)
		}
		if body["Name"] != "Step3 Probe" {
			t.Errorf("%s body Name = %v, want %q", entity, body["Name"], "Step3 Probe")
		}
		if _, ok := body["TxnDate"]; ok {
			t.Errorf("%s body must not carry TxnDate, got %v", entity, body["TxnDate"])
		}
	}
}

// TestTermCreateMapsDueDays locks the TC2 2026-09-08 server correction:
// Name-only Term creates 400 ("select the day of the month for
// 'Date-driven'"), so --due-days/--type map to DueDays/Type. Rejects a
// string DueDays and a dropped Type.
func TestTermCreateMapsDueDays(t *testing.T) {
	body, err := buildCreateBody("Term", map[string]string{"name": "Net 15", "type": "Standard", "due-days": "15"})
	if err != nil {
		t.Fatalf("buildCreateBody: %v", err)
	}
	if body["Type"] != "Standard" {
		t.Errorf("Type = %v, want Standard", body["Type"])
	}
	if body["DueDays"] != 15 {
		t.Errorf("DueDays = %v (%T), want int 15", body["DueDays"], body["DueDays"])
	}
}

// TestTermUpdateCarriesDueDays locks the TC2 2026-09-08 correction: renames
// must carry the schedule fields or the server 400s demanding day-of-month.
func TestTermUpdateCarriesDueDays(t *testing.T) {
	existing := map[string]any{"Id": "1000000031", "SyncToken": "0", "DueDays": float64(30)}
	body, err := buildUpdateBody("Term", map[string]string{"name": "ParityTerm0809b"}, existing)
	if err != nil {
		t.Fatalf("buildUpdateBody: %v", err)
	}
	if body["Name"] != "ParityTerm0809b" {
		t.Errorf("Name = %v, want ParityTerm0809b", body["Name"])
	}
	if body["DueDays"] != float64(30) {
		t.Errorf("DueDays = %v, want carried-over 30", body["DueDays"])
	}
}

// TestCompanyInfoUpdateMapsNameToCompanyName locks the Step 4 parity-gap
// contract: CompanyInfo has no Name/DisplayName field, so --name must map to
// CompanyName on sparse update; the generic default (out["Name"]) would send
// an unknown field the server ignores, making the edit a silent no-op.
//
// Kill denominator: 2. Rejects (1) the default-branch Name mapping and
// (2) a dropped CompanyName.
func TestCompanyInfoUpdateMapsNameToCompanyName(t *testing.T) {
	existing := map[string]any{"Id": "1", "SyncToken": "0"}
	body, err := buildUpdateBody("CompanyInfo", map[string]string{"name": "Step4 Pty Ltd"}, existing)
	if err != nil {
		t.Fatalf("buildUpdateBody: %v", err)
	}
	if body["CompanyName"] != "Step4 Pty Ltd" {
		t.Errorf("CompanyName = %v, want %q", body["CompanyName"], "Step4 Pty Ltd")
	}
	if _, ok := body["Name"]; ok {
		t.Errorf("body must not carry Name, got %v", body["Name"])
	}
	if body["Id"] != "1" || body["SyncToken"] != "0" {
		t.Errorf("body lost Id/SyncToken: %v", body)
	}
}
