package cli

import (
	"strings"
	"testing"
)

func TestParamsForPreservesCatalogAndFileContracts(t *testing.T) {
	const id = "QBO.ADVANCED.BATCH_RUN"
	params := paramsFor(id)
	if len(params) == 0 {
		t.Fatal("missing batch parameters")
	}
	if !strings.Contains(params[0].Help, "JSON") || strings.Contains(params[0].Help, "CSV") {
		t.Fatalf("batch file must describe JSON, not bank-statement CSV: %s", params[0].Help)
	}
	original := firstParams(id)[0].Help
	params[0].Help = "caller mutation"
	if firstParams(id)[0].Help != original || paramsFor(id)[0].Help != original {
		t.Fatal("returned parameters alias the shared catalog")
	}
	for _, param := range paramsFor("QBO.FEED.TXN_IMPORT") {
		if param.Name == "file" {
			if !strings.Contains(param.Help, "CSV") {
				t.Fatal("bank import lost CSV help")
			}
			return
		}
	}
	t.Fatal("bank import has no file parameter")
}

func TestTimeHelpMatchesNativeDurationAndRate(t *testing.T) {
	for _, id := range []string{"QBO.EXPENSES.TIME_ACTIVITY_CREATE", "QBO.EXPENSES.TIME_ACTIVITY_EDIT", "QBO.PAYROLL.TIMESHEET_CREATE", "QBO.PAYROLL.TIMESHEET_EDIT"} {
		found := map[string]string{}
		for _, p := range paramsFor(id) {
			found[p.Name] = p.Help
		}
		if !strings.Contains(found["minutes"], "0 through 59") || !strings.Contains(found["rate"], "zero on create") {
			t.Fatalf("%s: %v", id, found)
		}
	}
}
