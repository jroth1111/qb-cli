package client

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"strings"
	"testing"
)

// Compare the serialized request body used by ReplayMutate, including presence
// of numeric zero fields. No credentials or network transport are needed.
func assertTimeRequestJSON(t *testing.T, body map[string]any, wantJSON string) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var got, want any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("request = %s\nwant = %s", raw, wantJSON)
	}
}

func timeFieldsExisting(t *testing.T) map[string]any {
	t.Helper()
	var existing map[string]any
	err := json.Unmarshal([]byte(`{"Id":"7","SyncToken":"3","NameOf":"Employee","EmployeeRef":{"value":"10"},"CustomerRef":{"value":"20"},"ItemRef":{"value":"30"},"Hours":2,"Minutes":45,"HourlyRate":18.5,"TxnDate":"2026-09-10","Description":"original","BillableStatus":"Billable"}`), &existing)
	if err != nil {
		t.Fatal(err)
	}
	return existing
}

func TestTimeFieldsCreateRequest(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags map[string]string
		hours int
		mins  int
		rate  float64
	}{
		{"separate duration", map[string]string{"hours": "1", "minutes": "30", "rate": "12.75"}, 1, 30, 12.75},
		{"minutes only", map[string]string{"minutes": "30"}, 0, 30, 0},
		{"explicit zeros", map[string]string{"hours": "0", "minutes": "0", "rate": "0"}, 0, 0, 0},
		{"zero hours", map[string]string{"hours": "0"}, 0, 0, 0},
		{"zero minutes", map[string]string{"minutes": "0"}, 0, 0, 0},
		{"omitted duration", nil, 1, 0, 0},
		{"unset CLI flags", map[string]string{"hours": "", "minutes": "", "rate": ""}, 1, 0, 0},
		{"last minute", map[string]string{"hours": "3", "minutes": "59"}, 3, 59, 0},
		{"trimmed fields", map[string]string{"hours": " 0 ", "minutes": " 5 ", "rate": " 0.25 "}, 0, 5, 0.25},
	} {
		t.Run(tc.name, func(t *testing.T) {
			flags := map[string]string{"employee": "10", "date": "10/09/2026", "billable": "false"}
			maps.Copy(flags, tc.flags)
			body, err := buildCreateBody("TimeActivity", flags)
			if err != nil {
				t.Fatal(err)
			}
			assertTimeRequestJSON(t, body, fmt.Sprintf(`{"NameOf":"Employee","EmployeeRef":{"value":"10"},"TxnDate":"2026-09-10","Hours":%d,"Minutes":%d,"HourlyRate":%g,"BillableStatus":"NotBillable"}`, tc.hours, tc.mins, tc.rate))
		})
	}
}

func TestTimeFieldsUpdateForwardsRequest(t *testing.T) {
	existing := timeFieldsExisting(t)
	before, err := json.Marshal(existing)
	if err != nil {
		t.Fatal(err)
	}
	body, err := buildUpdateBody("TimeActivity", map[string]string{
		"hours": "0", "minutes": "0", "rate": "0", "billable": "false",
		"date": "11/09/2026", "description": "updated", "employee": "11",
		"name-of": "Employee", "customer-id": "21", "item-id": "31",
	}, existing)
	if err != nil {
		t.Fatal(err)
	}
	assertTimeRequestJSON(t, body, `{"Id":"7","SyncToken":"3","sparse":true,"NameOf":"Employee","EmployeeRef":{"value":"11"},"CustomerRef":{"value":"21"},"ItemRef":{"value":"31"},"Hours":0,"Minutes":0,"HourlyRate":0,"TxnDate":"2026-09-11","Description":"updated","BillableStatus":"NotBillable"}`)
	assertTimeRequestJSON(t, existing, string(before))
}

func TestTimeFieldsPartialUpdatePreservesOmittedValues(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags map[string]string
		hours int
		mins  int
		rate  float64
	}{
		{"omitted", nil, 2, 45, 18.5},
		{"unset CLI flags", map[string]string{"hours": "", "minutes": "", "rate": "", "billable": "", "employee": ""}, 2, 45, 18.5},
		{"hours zero", map[string]string{"hours": "0"}, 0, 45, 18.5},
		{"minutes zero", map[string]string{"minutes": "0"}, 2, 0, 18.5},
		{"rate zero", map[string]string{"rate": "0"}, 2, 45, 0},
		{"new duration", map[string]string{"hours": "1", "minutes": "15"}, 1, 15, 18.5},
		{"new rate", map[string]string{"rate": "25.75"}, 2, 45, 25.75},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, err := buildUpdateBody("TimeActivity", tc.flags, timeFieldsExisting(t))
			if err != nil {
				t.Fatal(err)
			}
			assertTimeRequestJSON(t, body, fmt.Sprintf(`{"Id":"7","SyncToken":"3","sparse":true,"NameOf":"Employee","EmployeeRef":{"value":"10"},"CustomerRef":{"value":"20"},"ItemRef":{"value":"30"},"Hours":%d,"Minutes":%d,"HourlyRate":%g,"TxnDate":"2026-09-10","Description":"original","BillableStatus":"Billable"}`, tc.hours, tc.mins, tc.rate))
		})
	}
	// A sparse update must not invent numeric defaults when absent upstream.
	body, err := buildUpdateBody("TimeActivity", map[string]string{"description": "note"}, map[string]any{"Id": "7", "SyncToken": "3"})
	if err != nil {
		t.Fatal(err)
	}
	assertTimeRequestJSON(t, body, `{"Id":"7","SyncToken":"3","sparse":true,"Description":"note"}`)
}

func TestTimeFieldsRejectInvalidNumbersInCreateAndUpdate(t *testing.T) {
	invalid := map[string][]string{
		"hours":   {"-1", "1.5", "1.0", "1e2", "two", "NaN", "Inf", "99999999999999999999999"},
		"minutes": {"-1", "60", "90", "0.5", "1.0", "1e1", "ten", "NaN", "Inf", "99999999999999999999999"},
		"rate":    {"-1", "-0.01", "NaN", "Inf", "+Inf", "-Inf", "1e309", "free", "$1"},
	}
	for flag, values := range invalid {
		for _, value := range values {
			for _, op := range []string{"create", "update"} {
				t.Run(op+"/"+flag+"/"+value, func(t *testing.T) {
					flags := map[string]string{"employee": "10", "hours": "1", "minutes": "0", "rate": "0", flag: value}
					var body map[string]any
					var err error
					if op == "create" {
						body, err = buildCreateBody("TimeActivity", flags)
					} else {
						body, err = buildUpdateBody("TimeActivity", flags, timeFieldsExisting(t))
					}
					if err == nil || !strings.Contains(err.Error(), "--"+flag) || body != nil {
						t.Fatalf("invalid --%s %q returned body=%v, err=%v", flag, value, body, err)
					}
				})
			}
		}
	}
}

func TestTimeFieldsReferenceAndMetadataForwarding(t *testing.T) {
	created, err := buildCreateBody("TimeActivity", map[string]string{
		"name-of": "Vendor", "supplier": "40", "hours": "0", "minutes": "15",
		"date": "10/09/2026", "customer": "20", "item": "30", "description": "contractor", "billable": "true",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertTimeRequestJSON(t, created, `{"NameOf":"Vendor","VendorRef":{"value":"40"},"Hours":0,"Minutes":15,"HourlyRate":0,"TxnDate":"2026-09-10","CustomerRef":{"value":"20"},"ItemRef":{"value":"30"},"Description":"contractor","BillableStatus":"Billable"}`)
	for _, tc := range []struct {
		nameOf string
		flags  map[string]string
		ref    string
	}{
		{"Vendor", map[string]string{"name-of": "vendor", "vendor": "41"}, `"VendorRef":{"value":"41"}`},
		{"Vendor", map[string]string{"supplier": "42"}, `"VendorRef":{"value":"42"}`},
		{"Employee", map[string]string{"employee": "12"}, `"EmployeeRef":{"value":"12"}`},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			body, err := buildUpdateBody("TimeActivity", tc.flags, timeFieldsExisting(t))
			if err != nil {
				t.Fatal(err)
			}
			assertTimeRequestJSON(t, body, fmt.Sprintf(`{"Id":"7","SyncToken":"3","sparse":true,"NameOf":%q,%s,"CustomerRef":{"value":"20"},"ItemRef":{"value":"30"},"Hours":2,"Minutes":45,"HourlyRate":18.5,"TxnDate":"2026-09-10","Description":"original","BillableStatus":"Billable"}`, tc.nameOf, tc.ref))
		})
	}
	for _, flags := range []map[string]string{{"name-of": "Vendor"}, {"name-of": "invalid"}} {
		if _, err := buildUpdateBody("TimeActivity", flags, timeFieldsExisting(t)); err == nil {
			t.Fatalf("invalid reference change accepted: %v", flags)
		}
	}
}

func TestTimeFieldsDoNotValidateOtherEntityNumbers(t *testing.T) {
	flags := map[string]string{"customer": "20", "amount": "2.5", "hours": "invalid", "minutes": "invalid", "rate": "NaN"}
	for _, op := range []string{"create", "update"} {
		var body map[string]any
		var err error
		if op == "create" {
			body, err = buildCreateBody("Invoice", flags)
		} else {
			body, err = buildUpdateBody("Invoice", flags, map[string]any{"Id": "1", "SyncToken": "0"})
		}
		if err != nil {
			t.Fatalf("%s Invoice: %v", op, err)
		}
		for _, key := range []string{"Hours", "Minutes", "HourlyRate", "BillableStatus"} {
			if _, exists := body[key]; exists {
				t.Errorf("%s Invoice received TimeActivity field %s", op, key)
			}
		}
		if op == "update" && body["TotalAmt"] != 2.5 {
			t.Fatalf("Invoice amount contract changed: %v", body)
		}
	}
}
