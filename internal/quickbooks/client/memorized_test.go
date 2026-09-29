package client

import (
	"encoding/json"
	"testing"
)

// The captured /app/customreports payload: top-level [{$type,data}] where
// data is an OBJECT keyed "0" (not the array form listsPrefs uses).
const memorizedFixture = `[{"$type":"/Result","data":{"0":{"company":{"reportDefinitions":{"edges":[
{"node":{"id":"djQuMToxOTM1MTQzMDEwOTkzNTI6NDgzYjZkNTY0Yg:42","reportName":"Melo Detailed Statement","reportType":"MEMORIZED","entityVersion":"13","createdBy":"Jacob Rothfield","shareInfo":{"shareType":"NONE"},"attributes":[{"name":"reportPeriod","value":"Last Month"}],"groups":{"edges":[]}}},
{"node":{"id":"djQuMToxOTM1MTQzMDEwOTkzNTI6NDgzYjZkNTY0Yg:41","reportName":"Gingelly Detailed Statement","reportType":"MEMORIZED","entityVersion":"10","createdBy":"Jacob Rothfield","shareInfo":{"shareType":"NONE"},"attributes":[],"groups":{"edges":[]}}},
{"node":{"id":"djQuMToxOTM1MTQzMDEwOTkzNTI6NDgzYjZkNTY0Yg:70","reportName":"Melo Gingelly Owner Distributions","reportType":"MEMORIZED","entityVersion":"0","createdBy":"Jacob Rothfield","shareInfo":{"shareType":"NONE"},"attributes":[],"groups":{"edges":[]}}}
]}}}}}]`

func TestMemorizedEdgesUnwrapObjectKeyedData(t *testing.T) {
	edges, err := memorizedEdges([]byte(memorizedFixture))
	if err != nil {
		t.Fatalf("memorizedEdges: %v", err)
	}
	if len(edges) != 3 {
		t.Fatalf("edges=%d, want 3", len(edges))
	}
	if edges[0].Node.ReportName != "Melo Detailed Statement" {
		t.Fatalf("first report = %q", edges[0].Node.ReportName)
	}
}

func TestMemorizedEdgesArrayData(t *testing.T) {
	// alternate envelope: data as array of {company:{...}}
	body := `[{"$type":"/Result","data":[{"company":{"reportDefinitions":{"edges":[{"node":{"id":"x:9","reportName":"R","reportType":"MEMORIZED"}}]}}}]}]`
	edges, err := memorizedEdges([]byte(body))
	if err != nil || len(edges) != 1 {
		t.Fatalf("array-data edges=%d err=%v", len(edges), err)
	}
}

func TestMemorizedEdgesRejectsFalseEmpty(t *testing.T) {
	for _, body := range []string{
		`[]`, `null`, `[{"$type":"/Error","data":null}]`,
		`[{"data":[{"errors":[{"message":"denied"}]}]}]`,
		`[{"data":{"0":{"company":{"reportDefinitions":{}}}}}]`,
		`[{"data":{"0":{"company":{"reportDefinitions":{"edges":null}}}}}]`,
		`[{"data":{"0":{"company":{"reportDefinitions":{"edges":[{"node":null}]}}}}}]`,
	} {
		if rows, err := memorizedEdges([]byte(body)); err == nil {
			t.Fatalf("invalid registry accepted: %s => %+v", body, rows)
		}
	}
	for _, body := range []string{
		`[{"data":[{"company":{"reportDefinitions":{"edges":[]}}}]}]`,
		`[{"data":{"0":{"company":{"reportDefinitions":{"edges":[]}}}}}]`,
	} {
		if rows, err := memorizedEdges([]byte(body)); err != nil || len(rows) != 0 {
			t.Fatalf("valid empty registry rejected: %+v, %v", rows, err)
		}
	}
}

func TestMemorizedIDMatch(t *testing.T) {
	id := "djQuMToxOTM1MTQzMDEwOTkzNTI6NDgzYjZkNTY0Yg:42"
	for _, want := range []string{id, "42"} {
		if !memorizedIDMatch(id, want) {
			t.Fatalf("id %q did not match %q", id, want)
		}
	}
	if memorizedIDMatch(id, "43") || memorizedIDMatch(id, "420") {
		t.Fatalf("id %q overmatched", id)
	}
}

func TestMemorizedFixtureParsesCompositeID(t *testing.T) {
	edges, _ := memorizedEdges([]byte(memorizedFixture))
	var seen string
	for _, e := range edges {
		if e.Node != nil && memorizedIDMatch(e.Node.ID, "41") {
			seen = e.Node.ReportName
		}
	}
	if seen != "Gingelly Detailed Statement" {
		t.Fatalf("mem_rpt_id 41 resolved %q, want Gingelly Detailed Statement", seen)
	}
}

// memorizedRunFixture mirrors the captured createReports_Report response for
// mem_rpt_id 42 — envelope, resolvedOptions, one data row, one summary row.
const memorizedRunFixture = `[{"$type":"/Result","data":[{"createReports_Report":{"clientMutationId":"0","reportsReportEdge":{"node":{"reportDefinition":{"reportName":"TransactionDetailByAccount","reportOptions":[{"id":"token","value":"TX_DET_BY_ACCT"},{"id":"account","value":"46,47,69"},{"id":"class","value":"3700000000000906240,3700000000000906240"},{"id":"accounting_method","value":"yes"},{"id":"start_date","value":"01/08/2026"},{"id":"end_date","value":"31/08/2026"}]},"attributes":[{"name":"reportBasis","value":"Cash"},{"name":"startPeriod","value":"2026-08-01"}],"columns":[{"value":"DATE"},{"value":"MEMO"},{"value":"AMOUNT"}],"rows":[{"id":"0","parentId":null,"cells":[{"value":"Client Property Revenue","attributes":[{"name":"indent","value":"0"}]}]},{"id":"1","parentId":"0","cells":[{"value":"24/08/2026"},{"value":"Judith Carpenter"},{"value":"6,104.70"}]}]}}}}]}]`

func TestMemorizedRunNodeFromEnvelope(t *testing.T) {
	node, err := memorizedRunNodeFromEnvelope([]byte(memorizedRunFixture))
	if err != nil || node == nil {
		t.Fatalf("node=%v err=%v", node, err)
	}
	if node.ReportDefinition.ReportName != "TransactionDetailByAccount" {
		t.Fatalf("reportName %q", node.ReportDefinition.ReportName)
	}
	resolved := map[string]string{}
	for _, o := range node.ReportDefinition.ReportOptions {
		resolved[o.ID] = o.Value
	}
	if resolved["account"] != "46,47,69" || resolved["token"] != "TX_DET_BY_ACCT" {
		t.Fatalf("resolvedOptions %v", resolved)
	}
	var rows []json.RawMessage
	if err := json.Unmarshal(node.Rows, &rows); err != nil || len(rows) != 2 {
		t.Fatalf("rows=%d err=%v", len(rows), err)
	}
}

func TestMemorizedRunHelpers(t *testing.T) {
	if got := auDate("2026-04-01"); got != "01/04/2026" {
		t.Fatalf("auDate %q", got)
	}
	if got := isoDate("30/04/2026"); got != "2026-04-30" {
		t.Fatalf("isoDate %q", got)
	}
	if got := dedupeCSV("3700000000000906240,3700000000000906240,3700000000000906240"); got != "3700000000000906240" {
		t.Fatalf("dedupeCSV %q", got)
	}
}
