package client

import (
	"encoding/json"
	"testing"
)

func TestApplyTPARDateRangeSetsCustomWindow(t *testing.T) {
	var payload map[string]any
	if err := json.Unmarshal(tparInstancesBodyJSON, &payload); err != nil {
		t.Fatal(err)
	}
	applyTPARDateRange(payload, "2026-07-01", "2026-08-20")
	filters := payload["filters"].([]any)
	val := filters[0].(map[string]any)["value"].(map[string]any)
	if val["macroName"] != "CUSTOM" {
		t.Fatalf("macro %v", val["macroName"])
	}
	if val["startDate"] != "2026-07-01" || val["endDate"] != "2026-08-20" {
		t.Fatalf("dates %v %v", val["startDate"], val["endDate"])
	}
}
