package client

import "testing"

// folioFixture mirrors the live Melo and Gingelly folio shape (captured
// 2026-09-29): cover + custom page + two detail reports + combined summary +
// all-time distributions page.
func folioFixture() map[string]any {
	return map[string]any{
		"id":   "sbg:test",
		"name": "Melo and Gingelly Statement",
		"folioDataRequest": map[string]any{
			"dateMacro": "custom",
			"pages": []any{
				map[string]any{"pageType": "page", "type": "COVER_PAGE", "title": "Melo and Gingelly Property Statement", "pageId": "CP_4", "preparedBy": "Jacob Rothfield", "preparedDate": "29 September 2026"},
				map[string]any{"pageType": "page", "type": "CUSTOM_PAGE", "hideTemplate": true},
				map[string]any{"pageType": "report", "type": "REPORT", "title": "Melo Detailed Statement", "reportToken": "42", "reportDateMacro": "custom", "startDate": "2026-05-01", "endDate": "2026-05-31"},
				map[string]any{"pageType": "report", "type": "REPORT", "title": "Gingelly Detailed Statement", "reportToken": "41", "reportDateMacro": "custom", "startDate": "2026-05-01", "endDate": "2026-05-31"},
				map[string]any{"pageType": "report", "type": "REPORT", "title": "Combined Summary Statement", "reportToken": "59", "reportDateMacro": "custom", "startDate": "2026-05-01", "endDate": "2026-05-31"},
				map[string]any{"pageType": "report", "type": "REPORT", "title": "Owner Distributions", "reportToken": "70", "reportDateMacro": "all"},
			},
		},
	}
}

func TestFolioPageItems(t *testing.T) {
	items := folioPageItems(folioFixture())
	if len(items) != 6 {
		t.Fatalf("expected 6 page items, got %d", len(items))
	}
	// Cover exposes preparedBy, not a token.
	if items[0].Type != "COVER_PAGE" || items[0].SubType != "Jacob Rothfield" || items[0].DocNumber != "" {
		t.Fatalf("cover page misprojected: %+v", items[0])
	}
	// Report pages: ID and DocNumber carry the mem_rpt_id so the token can
	// feed `reports memorized run --id` directly.
	if items[2].ID != "42" || items[2].DocNumber != "42" || items[2].SubType != "custom" {
		t.Fatalf("detail page misprojected: %+v", items[2])
	}
	if items[2].Date != "2026-05-01→2026-05-31" {
		t.Fatalf("custom page dates missing: %q", items[2].Date)
	}
	// All-dates page has no startDate/endDate — Date must stay empty, not
	// emit a dangling arrow.
	if items[5].DocNumber != "70" || items[5].SubType != "all" || items[5].Date != "" {
		t.Fatalf("all-time page misprojected: %+v", items[5])
	}
}

func TestFolioPageItemsDegenerate(t *testing.T) {
	// Missing folioDataRequest → empty, not a crash.
	if got := folioPageItems(map[string]any{}); len(got) != 0 {
		t.Fatalf("empty folio produced %d items", len(got))
	}
	// Malformed page entries are skipped; page order is preserved.
	obj := folioFixture()
	fdr := obj["folioDataRequest"].(map[string]any)
	fdr["pages"] = append([]any{"junk", 42}, fdr["pages"].([]any)...)
	items := folioPageItems(obj)
	if len(items) != 6 || items[0].Type != "COVER_PAGE" {
		t.Fatalf("malformed pages not skipped: %+v", items)
	}
	// Duplicate reportToken on two pages: both must index for resolution.
	fdr["pages"] = append(fdr["pages"].([]any), map[string]any{
		"pageType": "report", "type": "URI_REPORT", "title": "Dup", "reportToken": "42",
	})
	items = folioPageItems(obj)
	if len(items) != 7 || items[6].DocNumber != "42" {
		t.Fatalf("duplicate token page lost: %+v", items[6])
	}
}
