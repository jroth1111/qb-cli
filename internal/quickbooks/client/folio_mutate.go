package client

// CUSTOM_CREATE / MANAGEMENT_CREATE / MANAGEMENT_EDIT — folio-backed report
// CRUD on universalreportinsights.api.intuit.com, proven on TC2 2026-09-19
// against /app/customreports and /app/managementreports:
//
//	GET    /v1/folio?locale=en-au&subType=CRB_GROUP   custom report groups
//	GET    /v1/folio?locale=en-au                    management folios
//	GET    /v1/folio/{id}                            single folio (200)
//	POST   /v1/folio                                 create (200)
//	PUT    /v1/folio/{id}                            update — full object (200)
//	DELETE /v1/folio/{id}                            soft-delete, active:false (200)
//
// A "custom report" in the new UI is a CRB_GROUP folio whose pages reference
// report tokens (standard tokens like PANDL, or sbg: ids of saved custom
// report instances — those pages carry customReport:true). A "management
// report" is a FOLIO folio with a COVER_PAGE front page plus report pages.
// DELETE renames the folio to "X (deleted - n)" and drops it from the list.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const folioMutateURL = "https://universalreportinsights.api.intuit.com/v1/folio"

// PlannedFolioMutateURL is the dry-run URL for folio create/update.
func PlannedFolioMutateURL() string { return folioMutateURL }

// folioPage is one folioDataRequest.pages[] entry (spelling preserved).
type folioPage struct {
	PageType          string            `json:"pageType"`
	Title             string            `json:"title"`
	Type              string            `json:"type"`
	HideTemplate      bool              `json:"hideTemplate"`
	PageID            string            `json:"pageId,omitempty"`
	SubTitle          string            `json:"subTitle,omitempty"`
	PreparedDate      string            `json:"preparedDate,omitempty"`
	PreparedBy        string            `json:"preparedBy,omitempty"`
	ReportPeriod      string            `json:"reportPeriod,omitempty"`
	ShowLogo          *bool             `json:"showLogo,omitempty"`
	IncludeEndNotes   *bool             `json:"includeEndNotes,omitempty"`
	ReportToken       string            `json:"reportToken,omitempty"`
	ReportCustomAttrs map[string]string `json:"reportCustomizationAttributes,omitempty"`
	ReportDateMacro   string            `json:"reportDateMacro,omitempty"`
	StartDate         string            `json:"startDate,omitempty"`
	EndDate           string            `json:"endDate,omitempty"`
	CustomReport      *bool             `json:"customReport,omitempty"`
	ShowPyComparison  *bool             `json:"showPyComparison,omitempty"`
	ShowPpComparison  *bool             `json:"showPpComparison,omitempty"`
}

// folio is the universalreportinsights folio shape (spelling preserved).
type folio struct {
	ReportKindEnum     string          `json:"reportKindEnum"`
	ID                 string          `json:"id,omitempty"`
	Name               string          `json:"name"`
	Active             bool            `json:"active"`
	ReportingNamespace string          `json:"reportingNamespace,omitempty"`
	RealmID            string          `json:"realmId,omitempty"`
	VisibleToCustomer  bool            `json:"visibleToCustomer"`
	FolioDataRequest   json.RawMessage `json:"folioDataRequest"`
}

// reportPage builds the URI_REPORT page referencing a report token. start
// and end pin a custom reporting period on the page (ISO YYYY-MM-DD); when
// they are set the page macro is "custom", matching live folios.
func reportPage(title, token, dateMacro, start, end string) folioPage {
	// sbg: tokens are saved custom reports; all-numeric tokens are classic
	// mem_rpt_ids — live management folios mark those pages
	// customReport:true (verified 2026-09 on tokens 41/42/59/70/162/558).
	custom := strings.HasPrefix(token, "sbg:") || numericToken(token)
	f := false
	return folioPage{
		PageType:    "report",
		Title:       title,
		Type:        "URI_REPORT",
		ReportToken: token,
		ReportCustomAttrs: map[string]string{
			"subcol_py": "false",
			"subcol_pp": "false",
		},
		ReportDateMacro:  dateMacro,
		StartDate:        start,
		EndDate:          end,
		CustomReport:     &custom,
		ShowPyComparison: &f,
		ShowPpComparison: &f,
	}
}

// numericToken reports whether s is a nonempty all-numeric token (a classic
// mem_rpt_id such as "42").
func numericToken(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// pageMacros are reportDateMacro values a folio report page may carry. Only
// "custom" takes explicit startDate/endDate; every other macro is relative.
var pageMacros = map[string]bool{
	"all": true, "custom": true, "today": true, "yesterday": true,
	"thisweek": true, "lastweek": true, "thismonth": true, "lastmonth": true,
	"thisquarter": true, "lastquarter": true, "thisyear": true,
	"lastyear": true, "thisfiscalyear": true, "lastfiscalyear": true,
}

// splitTokenMacro splits "TOKEN:macro" when the suffix is a known page
// macro. sbg: tokens contain a colon but their tail is a UUID fragment, so
// they pass through unsplit; an unknown suffix stays part of the token and
// fails honestly at the server.
func splitTokenMacro(tok string) (token, macro string) {
	i := strings.LastIndexByte(tok, ':')
	if i < 0 || !pageMacros[strings.ToLower(tok[i+1:])] {
		return tok, ""
	}
	return tok[:i], strings.ToLower(tok[i+1:])
}

// coverPage builds the management-report COVER_PAGE.
func coverPage(preparedBy, preparedDate string) folioPage {
	t := true
	f := false
	return folioPage{
		PageType:        "page",
		Title:           "Management Report",
		Type:            "COVER_PAGE",
		PageID:          "CP_1",
		SubTitle:        "{CompanyName}",
		PreparedDate:    preparedDate,
		PreparedBy:      preparedBy,
		ReportPeriod:    "For the period ended {ReportEndDate}",
		ShowLogo:        &t,
		IncludeEndNotes: &f,
	}
}

// reportEndDate resolves {ReportEndDate} for the date macro.
func reportEndDate(dateMacro string, now time.Time) string {
	if strings.EqualFold(dateMacro, "thisyear") {
		return time.Date(now.Year(), 12, 31, 0, 0, 0, 0, time.UTC).Format("January 2, 2006")
	}
	return now.Format("January 2, 2006")
}

// folioCreateBody builds the POST body for one folio create. start/end pin
// an explicit reporting period (ISO YYYY-MM-DD): the folio and its report
// page carry startDate/endDate with dateMacro "custom", the same shape
// live management folios store for an issued statement.
func folioCreateBody(ac *apiClient, kind, name string, reports []string, title, dateMacro, start, end, preparedBy string) ([]byte, error) {
	if start != "" || end != "" {
		dateMacro = "custom"
	}
	// One report page per token, in flag order — a multi-property owner's
	// statement is per-unit details + combined summary + distributions.
	// A TOKEN:macro suffix overrides that page's period scope (e.g.
	// "70:all" keeps an owner-distributions page all-time, as live folios
	// store it, instead of pinning the statement window onto it).
	pages := make([]folioPage, 0, len(reports))
	for _, tok := range reports {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		tok, pageMacro := splitTokenMacro(tok)
		t := title
		if len(pages) > 0 || t == "" {
			t = tok // per-page title defaults to the token itself
		}
		macro, pStart, pEnd := dateMacro, start, end
		if pageMacro != "" {
			macro = pageMacro
			if pageMacro != "custom" {
				pStart, pEnd = "", ""
			}
		}
		pages = append(pages, reportPage(t, tok, macro, pStart, pEnd))
	}
	req := map[string]any{
		"templateName":           name,
		"templateType":           "USER",
		"showNonZeroRowsColumns": false,
		"pages":                  pages,
		"editSequence":           0,
	}
	kindEnum := "CRB_GROUP"
	if kind == "management" {
		kindEnum = "FOLIO"
		now := time.Now()
		pages = append([]folioPage{coverPage(preparedBy, now.Format("2 January 2006"))}, pages...)
		req["pages"] = pages
		req["dateMacro"] = dateMacro
		if start != "" {
			req["startDate"] = start
		}
		if end != "" {
			req["endDate"] = end
		}
		req["tocTitle"] = "Table of contents"
		req["includeTOC"] = true
		endDisp := reportEndDate(dateMacro, now)
		if end != "" {
			if t, perr := time.Parse("2006-01-02", end); perr == nil {
				endDisp = t.Format("2 January 2006")
			}
		}
		req["dynamicFields"] = map[string]string{
			"{CompanyName}":   ac.company,
			"{ReportEndDate}": endDisp,
		}
		req["customizationOptions"] = []map[string]any{
			{"key": "locale", "value": "en-au"},
			{"key": "title", "value": name},
			{"key": "showNonZeroRowsColumns", "value": false},
		}
	} else {
		req["includeTOC"] = false
	}
	body, err := json.Marshal(map[string]any{
		"reportKindEnum":     kindEnum,
		"name":               name,
		"active":             true,
		"reportingNamespace": "qbo",
		"realmId":            ac.realm,
		"visibleToCustomer":  false,
		"folioDataRequest":   req,
	})
	if err != nil {
		return nil, err
	}
	return body, nil
}

// ReplayFolioMutate creates or updates a folio-backed report.
//
//	custom create      --name X [--report PANDL]... [--title T] [--date-macro M]
//	management create  --name X [--report TOKEN]... [--date-macro M]
//	                   [--start-date YYYY-MM-DD --end-date YYYY-MM-DD] [--prepared-by P]
//	management update  --id sbg:... [--name X] [--start-date YYYY-MM-DD --end-date YYYY-MM-DD]
//
// Each --report produces one report page in flag order; a real owner
// statement is typically per-unit detailed tokens + combined summary +
// owner-distributions tokens.
func ReplayFolioMutate(ctx context.Context, kind, op, id, name string, reports []string, title, dateMacro, start, end, preparedBy string) (*MutateResult, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	title = strings.TrimSpace(title)
	dateMacro = strings.TrimSpace(dateMacro)
	start = strings.TrimSpace(start)
	end = strings.TrimSpace(end)
	preparedBy = strings.TrimSpace(preparedBy)
	if kind != "custom" && kind != "management" {
		return nil, fmt.Errorf("unknown folio kind %q", kind)
	}
	switch op {
	case "create":
		if name == "" {
			return nil, fmt.Errorf("%s create requires --name", kind)
		}
	case "update":
		if id == "" {
			return nil, fmt.Errorf("%s update requires --id (folio id, e.g. sbg:...)", kind)
		}
		if name == "" && start == "" && end == "" {
			return nil, fmt.Errorf("%s update requires --name or --start-date/--end-date", kind)
		}
		if (start == "") != (end == "") {
			return nil, fmt.Errorf("%s update requires --start-date and --end-date together", kind)
		}
	default:
		return nil, fmt.Errorf("unknown folio op %q", op)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	switch op {
	case "create":
		if len(reports) == 0 {
			reports = []string{"PANDL"}
		}
		if title == "" && len(reports) == 1 {
			title = "Profit and Loss"
		}
		if dateMacro == "" {
			if kind == "management" {
				dateMacro = "thisyear"
			} else {
				dateMacro = "thisyeartodate"
			}
		}
		if preparedBy == "" {
			preparedBy = ac.email
		}
		body, err := folioCreateBody(ac, kind, name, reports, title, dateMacro, start, end, preparedBy)
		if err != nil {
			return nil, err
		}
		return folioDo(ctx, ac, op, http.MethodPost, folioMutateURL, body, kind+" create "+name)
	case "update":
		u := folioMutateURL + "/" + url.PathEscape(id)
		resp, err := ac.doURIHost(ctx, http.MethodGet, u, insightsHost, nil)
		if err != nil {
			return nil, fmt.Errorf("folio GET %s: %w", id, err)
		}
		defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
		raw, err := readBody(resp)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
		}
		var cur folio
		if err := json.Unmarshal(raw, &cur); err != nil {
			return nil, fmt.Errorf("malformed folio %.200s", raw)
		}
		if name != "" {
			cur.Name = name
		}
		var req map[string]any
		if err := json.Unmarshal(cur.FolioDataRequest, &req); err == nil {
			if name != "" {
				req["templateName"] = name
			}
			if start != "" && end != "" {
				pinFolioPeriod(req, start, end)
			}
			cur.FolioDataRequest, _ = json.Marshal(req)
		}
		body, _ := json.Marshal(cur)
		return folioDo(ctx, ac, op, http.MethodPut, u, body, kind+" update "+id)
	}
	return nil, fmt.Errorf("unknown folio op %q", op)
}

// pinFolioPeriod rewrites the folio's reporting period to an explicit
// window: folioDataRequest gets dateMacro "custom" plus startDate/endDate,
// and every period-bound REPORT page gets the same pinned range. Pages
// whose reportDateMacro is "all" are deliberately untouched — they are
// running ledgers (e.g. Owner Distributions pages verified 2026-09-29 on
// Mega Style's live folios), not period reports.
func pinFolioPeriod(req map[string]any, start, end string) {
	req["dateMacro"] = "custom"
	req["startDate"] = start
	req["endDate"] = end
	pages, _ := req["pages"].([]any)
	for _, raw := range pages {
		p, _ := raw.(map[string]any)
		if p == nil {
			continue
		}
		macro, _ := p["reportDateMacro"].(string)
		if macro == "" || strings.EqualFold(macro, "all") {
			continue
		}
		p["reportDateMacro"] = "custom"
		p["startDate"] = start
		p["endDate"] = end
	}
}

// folioDo executes one universalreportinsights call and projects onto
// MutateResult. 200 responses carry the folio JSON.
func folioDo(ctx context.Context, ac *apiClient, op, method, url string, body []byte, note string) (*MutateResult, error) {
	resp, err := ac.doURIHost(ctx, method, url, insightsHost, body)
	if err != nil {
		return nil, fmt.Errorf("folio %s: %w", op, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	item := QueryItem{Type: "Folio"}
	var f folio
	if json.Unmarshal(raw, &f) == nil {
		item.ID = f.ID
		item.Name = f.Name
		if !f.Active {
			item.Type = "Folio (inactive)"
		}
	}
	if item.ID == "" {
		return nil, fmt.Errorf("folio receipt missing identity; inspect before retrying")
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     op,
		Entity: "Folio",
		Item:   item,
		Note:   "universalreportinsights " + note,
	}, nil
}
