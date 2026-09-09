package client

import (
	"errors"
	"testing"
)

// TestCheckLineItemsJSONRejectsCSV is the negative guard for the silent-$1
// defect: CSV rows were dropped and a $1 default line recorded instead.
// Every non-JSON payload must fail fast; valid JSON arrays and empty pass.
func TestCheckLineItemsJSONRejectsCSV(t *testing.T) {
	for _, raw := range []string{
		"Services,1,7.77,GST",
		"8,1,0.01,GST",
		"just-a-string",
		"{not json}",
		"[]",
		"  [ ]  ",
	} {
		err := CheckLineItemsJSON(map[string]string{"line-items": raw})
		if !errors.Is(err, ErrCSVLineItems) {
			t.Errorf("payload %q accepted (err=%v), want ErrCSVLineItems", raw, err)
		}
	}
	for _, raw := range []string{
		"",
		"   ",
		`[{"Amount":3.0,"DetailType":"SalesItemLineDetail"}]`,
	} {
		if err := CheckLineItemsJSON(map[string]string{"line-items": raw}); err != nil {
			t.Errorf("payload %q rejected: %v", raw, err)
		}
	}
	if err := CheckLineItemsJSON(map[string]string{"lines": "a,b,c"}); !errors.Is(err, ErrCSVLineItems) {
		t.Errorf("--lines alias not validated: %v", err)
	}
}
