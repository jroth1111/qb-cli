package client

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Captured 2026-08-19 on /app/formstyles (open-39 follow-up).
// GET txnsrendering.api.intuit.com/v2/customizations?fetchRethinkTemplate=true
// → 200 list items=2 (id, printPrefName, templateType). Honest form-style list — not Preferences.
const formStyleURL = "https://txnsrendering.api.intuit.com/v2/customizations?fetchRethinkTemplate=true"
const formStyleHost = "txnsrendering.api.intuit.com"

func PlannedFormStyleURL() string { return formStyleURL }

// PlannedFormStyleUpdateURL reports the PUT URL for dry-run plans.
func PlannedFormStyleUpdateURL(id string) string {
	return "https://txnsrendering.api.intuit.com/v2/customizations/" + id
}

// FORM_STYLE_EDIT rides a full-document read-modify-write on txnsrendering.
// Captured 2026-09-18 from the Standard template editor (Design → colour →
// Done): PUT /v2/customizations/1125188 with the whole customization doc.
// The GET answers templateEditsequence as a number; the PUT sends it back
// as "templateEditSequence" (capital S) plus a static clientMutationId:"0".
// Only the "Standard"-schema (airy) template is editable — the modernised
// BUSINESS_MODE_SALES template is view-only per QBO docs.
func ReplayFormStyleUpdate(ctx context.Context, id, name, color string) (*MutateResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("form-style update requires --id")
	}
	name = strings.TrimSpace(name)
	color = strings.TrimSpace(color)
	if name == "" && color == "" {
		return nil, fmt.Errorf("form-style update needs a change: --name and/or --color")
	}
	if color != "" && !isHexColor(color) {
		return nil, fmt.Errorf("--color %q: want #RRGGBB", color)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	getURL := PlannedFormStyleUpdateURL(id)
	resp, err := ac.doURIHost(ctx, http.MethodGet, getURL, formStyleHost, nil)
	if err != nil {
		return nil, fmt.Errorf("form-style read: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("reading form-style: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("form-style doc: %w", err)
	}
	var seq string
	switch v := doc["templateEditsequence"].(type) {
	case float64:
		seq = strconv.FormatInt(int64(v), 10)
	case string:
		seq = v
	}
	changed := []string{}
	if name != "" && doc["customizationName"] != name {
		doc["customizationName"] = name
		changed = append(changed, "name")
	}
	if color != "" {
		styles := nestedMap(doc, "templateCustomization", "design", "stylePrefs", "generalStyles")
		if styles == nil {
			return nil, fmt.Errorf("form-style %s has no design.stylePrefs.generalStyles — cannot set color", id)
		}
		if styles["foregroundColor"] != color {
			styles["foregroundColor"] = color
			changed = append(changed, "color")
		}
	}
	if len(changed) == 0 {
		return &MutateResult{
			Status: http.StatusOK, Op: "form-style update", Entity: "FormStyle",
			Item: QueryItem{Type: "FormStyle", ID: id, Name: fmt.Sprint(doc["customizationName"])},
			Note: "already at requested values",
		}, nil
	}
	// Re-shape the version echo: GET's numeric templateEditsequence becomes
	// PUT's string templateEditSequence; the raw GET key is dropped.
	delete(doc, "templateEditsequence")
	doc["templateEditSequence"] = seq
	doc["clientMutationId"] = "0"
	body, err := json.Marshal(doc)
	if err != nil {
		return nil, err
	}
	resp, err = ac.doURIHost(ctx, http.MethodPut, getURL, formStyleHost, body)
	if err != nil {
		return nil, fmt.Errorf("form-style update: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err = readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading form-style update: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var receipt struct {
		ID flexibleString `json:"id"`
	}
	if json.Unmarshal(raw, &receipt) != nil || receipt.ID.String() != id {
		return nil, fmt.Errorf("form-style update receipt unverified; inspect before retrying")
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "form-style update",
		Entity: "FormStyle",
		Item:   QueryItem{Type: "FormStyle", ID: id, Name: fmt.Sprint(doc["customizationName"])},
		Note:   "txnsrendering PUT customizations/" + id + " (" + strings.Join(changed, ",") + ")",
	}, nil
}

// nestedMap walks a decoded JSON object through path keys, returning nil if
// any hop is missing or not an object.
func nestedMap(m map[string]any, path ...string) map[string]any {
	cur := m
	for _, k := range path {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return nil
		}
		cur = next
	}
	return cur
}

func isHexColor(s string) bool {
	if len(s) != 7 || s[0] != '#' {
		return false
	}
	for _, c := range s[1:] {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func replayFormStyles(ctx context.Context, _, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodGet, formStyleURL, formStyleHost, nil)
	if err != nil {
		return nil, fmt.Errorf("form-style customizations: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading form-style customizations: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectFormStyles(raw, query, limit)
	res.Status = resp.StatusCode
	return res, nil
}

func projectFormStyles(body []byte, query string, limit int) *QueryResult {
	note := "txnsrendering GET /v2/customizations"
	var wrap struct {
		Customizations []struct {
			ID            string `json:"id"`
			PrintPrefName string `json:"printPrefName"`
			TemplateType  string `json:"templateType"`
			Active        *bool  `json:"active"`
		} `json:"customizations"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "FormStyle", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (unparsed)"}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(wrap.Customizations))
	for _, c := range wrap.Customizations {
		it := QueryItem{ID: c.ID, Name: c.PrintPrefName, Type: c.TemplateType, Active: c.Active}
		if q != "" && !strings.Contains(strings.ToLower(it.ID+" "+it.Name+" "+it.Type), q) {
			continue
		}
		items = append(items, it)
		if len(items) >= limit {
			break
		}
	}
	note = fmt.Sprintf("%s n=%d items=%d", note, len(wrap.Customizations), len(items))
	return &QueryResult{
		Entity: "FormStyle",
		Counts: map[string]int{"items": len(items), "totalCount": len(wrap.Customizations)},
		Items:  items,
		Note:   note,
	}
}

// ReplayFormStyleDelete deletes a custom form style through
// DELETE txnsrendering /v2/customizations/{id}?templateEditSequence={seq}&forceDelete=true,
// captured 2026-09-19 from the formstyles kebab → Delete → confirm flow on TC2
// (custom style 1000000003 removed). The GET first answers the doc's
// templateEditsequence so the delete carries the real version, and a missing
// doc fails fast instead of reporting a phantom delete.
func ReplayFormStyleDelete(ctx context.Context, templateID string) (*MutateResult, error) {
	templateID = strings.TrimSpace(templateID)
	if templateID == "" {
		return nil, fmt.Errorf("form-style delete requires --template-id")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	getURL := PlannedFormStyleUpdateURL(templateID)
	resp, err := ac.doURIHost(ctx, http.MethodGet, getURL, formStyleHost, nil)
	if err != nil {
		return nil, fmt.Errorf("form-style read: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("reading form-style: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("form-style doc: %w", err)
	}
	var seq string
	switch v := doc["templateEditsequence"].(type) {
	case float64:
		seq = strconv.FormatInt(int64(v), 10)
	case string:
		seq = v
	}
	if seq == "" {
		seq = "0"
	}
	name := fmt.Sprint(doc["customizationName"])
	delURL := getURL + "?templateEditSequence=" + seq + "&forceDelete=true"
	resp, err = ac.doURIHost(ctx, http.MethodDelete, delURL, formStyleHost, nil, freshTraceHeaders())
	if err != nil {
		return nil, fmt.Errorf("form-style delete: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err = readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading form-style delete: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var receipt struct {
		Status string `json:"status"`
	}
	if json.Unmarshal(raw, &receipt) != nil || receipt.Status != "Deleted Successfully" {
		return nil, fmt.Errorf("form-style deletion unverified; inspect before retrying")
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "form-style delete",
		Entity: "FormStyle",
		Item:   QueryItem{Type: "FormStyle", ID: templateID, Name: name},
		Note:   "txnsrendering DELETE customizations/" + templateID + " seq=" + seq + " forceDelete",
	}, nil
}

// freshTraceHeaders mints request-scoped tracing ids. txnsrendering rejects
// replayed intuit_tid / x-b3 ids on writes — the jar's captured values are
// consumed nonces and the service answers them with a generic
// TRS_V2_INTERNAL_DB_ERROR. Proven live on TC2 2026-09-19: identical delete
// 500s with stale ids, 200 {"status":"Deleted Successfully"} with fresh ones.
// x-csrf-token is not validated on this surface.
func freshTraceHeaders() map[string]string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return map[string]string{}
	}
	tid := fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
	var s [8]byte
	if _, err := rand.Read(s[:]); err != nil {
		return map[string]string{}
	}
	return map[string]string{
		"intuit_tid":   tid,
		"x-b3-traceid": strings.ReplaceAll(tid, "-", ""),
		"x-b3-spanid":  fmt.Sprintf("%016x", s[:]),
		"x-b3-sampled": "1",
	}
}
