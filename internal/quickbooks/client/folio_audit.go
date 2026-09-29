package client

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AuditFolio executes every REPORT page of a management folio for each month
// in [from,to] (YYYY-MM) and reconciles the constituents. A statement is
// verified against the delivered PDF — this audit proves what the folio's
// pages compute *today* for each period and whether they tie out.
//
// Page roles are derived, never assumed by title:
//   - reportDateMacro "all"           → alltime page (owner-distributions
//     ledger); run once per class, sliced per period by row date
//   - resolved class list >1 distinct → combined page (covers several units)
//   - exactly 1 distinct class        → unit page (per-property detail)
//   - 0 classes                       → company page (flagged: unscoped)
//
// Invariants checked per period: Σ unit-page nets == combined-page net;
// whitelist coverage (accounts in the combined page missing from a unit page
// and vice versa); class coverage (union of unit classes vs combined classes).
// Cumulative position per unit = Σ period nets − Σ distributions in window.
func AuditFolio(ctx context.Context, folioID, from, to string) (*FolioAudit, error) {
	folioID = strings.TrimSpace(folioID)
	if folioID == "" {
		return nil, fmt.Errorf("management audit requires --id (folio id, e.g. sbg:…)")
	}
	periods, err := monthRange(from, to)
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	u := "https://" + managementFolioHost + "/v1/folio/" + url.PathEscape(folioID) + "?locale=en-au"
	resp, err := ac.doURIHost(ctx, http.MethodGet, u, managementFolioHost, nil)
	if err != nil {
		return nil, fmt.Errorf("management folio GET /v1/folio/%s: %w", folioID, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var obj map[string]any
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("malformed folio %.200s", raw)
	}

	audit := &FolioAudit{
		FolioID:   folioID,
		FolioName: strOfMap(obj, "name"),
		From:      from,
		To:        to,
	}

	// Resolve each REPORT page's saved definition once (its stored filters).
	var pages []*folioAuditPage
	fdr, _ := obj["folioDataRequest"].(map[string]any)
	rawPages, _ := fdr["pages"].([]any)
	for i, rp := range rawPages {
		b, _ := json.Marshal(rp)
		var p folioReadPage
		if json.Unmarshal(b, &p) != nil || p.ReportToken == "" {
			continue
		}
		if p.Type != "REPORT" && p.Type != "URI_REPORT" {
			continue
		}
		ap := &folioAuditPage{Seq: i, Title: p.Title, Token: p.ReportToken,
			Macro: p.ReportDateMacro, Start: p.StartDate, End: p.EndDate}
		if err := ap.resolve(ctx); err != nil {
			audit.Warnings = append(audit.Warnings,
				fmt.Sprintf("page p%d %q token %s: %v", i, p.Title, p.ReportToken, err))
			continue
		}
		ap.classify()
		pages = append(pages, ap)
		audit.Pages = append(audit.Pages, *ap)
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("folio %s has no executable REPORT pages", folioID)
	}
	audit.checkCoverage()

	for _, ap := range pages {
		if ap.Kind == "alltime" {
			if err := ap.runDistributions(ctx, from, to, audit); err != nil {
				audit.Warnings = append(audit.Warnings, fmt.Sprintf("distributions %q: %v", ap.Title, err))
			}
			continue
		}
		for _, pr := range periods {
			tot, terr := ap.runPeriod(ctx, pr)
			if terr != nil {
				audit.Warnings = append(audit.Warnings,
					fmt.Sprintf("%s %s: %v", ap.Title, pr.Label, terr))
				continue
			}
			audit.record(pr.Label, ap, tot)
		}
	}
	audit.reconcile()
	return audit, nil
}

// FolioAudit is the full statement-bundle audit for one folio over a window.
type FolioAudit struct {
	FolioID       string             `json:"folioId"`
	FolioName     string             `json:"folioName"`
	From          string             `json:"from"`
	To            string             `json:"to"`
	Pages         []folioAuditPage   `json:"pages"`
	Periods       []folioAuditPeriod `json:"periods"`
	Distributions []folioAuditDist   `json:"distributions,omitempty"`
	Position      map[string]float64 `json:"position,omitempty"` // unit title → earned − distributed
	Warnings      []string           `json:"warnings,omitempty"`
}

type folioAuditPage struct {
	Seq       int      `json:"seq"`
	Title     string   `json:"title"`
	Token     string   `json:"reportToken"` // mem_rpt_id
	SavedName string   `json:"savedName,omitempty"`
	Kind      string   `json:"kind"` // unit | combined | alltime | company
	Macro     string   `json:"dateMacro"`
	Start     string   `json:"startDate,omitempty"`
	End       string   `json:"endDate,omitempty"`
	Report    string   `json:"report"` // v3 report the token maps to
	Basis     string   `json:"basis"`
	Classes   []string `json:"classes,omitempty"`
	Accounts  []string `json:"accounts,omitempty"`

	opts map[string]string `json:"-"` // resolved saved options
}

type folioAuditPeriod struct {
	Label       string             `json:"period"`
	Units       map[string]float64 `json:"units"`           // unit-page title → net
	Combined    map[string]float64 `json:"combined"`        // combined-page title → net
	Other       map[string]float64 `json:"other,omitempty"` // unscoped/company pages
	UnitSum     float64            `json:"unitSum"`
	CombinedSum float64            `json:"combinedSum"`
	Delta       float64            `json:"delta"`
	OK          bool               `json:"ok"`
}

// folioAuditDist is one all-time page's distribution totals for one class.
type folioAuditDist struct {
	Page    string             `json:"page"`
	Class   string             `json:"class"`
	AllTime float64            `json:"allTime"`
	InRange float64            `json:"inRange"`
	ByMonth map[string]float64 `json:"byMonth,omitempty"`
}

type auditPeriod struct {
	Label, Start, End string
}

// monthRange expands YYYY-MM..YYYY-MM into day-precise periods.
func monthRange(from, to string) ([]auditPeriod, error) {
	parse := func(s string) (time.Time, error) {
		return time.Parse("2006-01", strings.TrimSpace(s))
	}
	f, err := parse(from)
	if err != nil {
		return nil, fmt.Errorf("--from must be YYYY-MM: %q", from)
	}
	t, err := parse(to)
	if err != nil {
		return nil, fmt.Errorf("--to must be YYYY-MM: %q", to)
	}
	if t.Before(f) {
		return nil, fmt.Errorf("--to %s precedes --from %s", to, from)
	}
	var out []auditPeriod
	for cur := f; !cur.After(t); cur = cur.AddDate(0, 1, 0) {
		end := cur.AddDate(0, 1, -1)
		out = append(out, auditPeriod{
			Label: cur.Format("2006-01"),
			Start: cur.Format("2006-01-02"),
			End:   end.Format("2006-01-02"),
		})
	}
	return out, nil
}

// resolve executes the saved report once (no override) to harvest its stored
// filter set — token, class list, account whitelist, basis.
func (ap *folioAuditPage) resolve(ctx context.Context) error {
	res, err := ReplayMemorizedRun(ctx, ap.Token, MemorizedRunOptions{})
	if err != nil {
		return err
	}
	ap.SavedName = res.Report
	opts, _ := res.Header["resolvedOptions"].(map[string]string)
	if opts == nil {
		return fmt.Errorf("no resolvedOptions — saved definition unreadable")
	}
	for _, c := range strings.Split(opts["class"], ",") {
		if c = strings.TrimSpace(c); c != "" && !contains(ap.Classes, c) {
			ap.Classes = append(ap.Classes, c)
		}
	}
	for _, a := range strings.Split(opts["account"], ",") {
		if a = strings.TrimSpace(a); a != "" {
			ap.Accounts = append(ap.Accounts, a)
		}
	}
	ap.Report = memorizedV3Report[opts["token"]]
	if ap.Report == "" {
		return fmt.Errorf("token %q has no v3 equivalent", opts["token"])
	}
	ap.Basis = "Cash"
	if opts["accounting_method"] == "no" {
		ap.Basis = "Accrual"
	}
	ap.opts = opts
	return nil
}

// classify derives the page role from its resolved scope, not its title.
func (ap *folioAuditPage) classify() {
	switch {
	case strings.EqualFold(ap.Macro, "all"):
		ap.Kind = "alltime"
	case len(ap.Classes) > 1:
		ap.Kind = "combined"
	case len(ap.Classes) == 1:
		ap.Kind = "unit"
	default:
		ap.Kind = "company"
	}
}

// filters rebuilds the saved filter set for a v3 replay; klass overrides the
// class list when set (per-class distribution runs).
func (ap *folioAuditPage) filters(klass string) ReportOptions {
	f := map[string]string{}
	if v := dedupeCSV(ap.opts["class"]); v != "" {
		f["klass"] = v
	}
	if klass != "" {
		f["klass"] = klass
	}
	if v := ap.opts["account"]; v != "" {
		f["account"] = v
	}
	if strings.Contains(ap.opts["group_by"], "Account/") {
		f["group_by"] = "Account"
	}
	if v := ap.opts["sort_order"]; v != "" {
		f["sort_order"] = v
	}
	return ReportOptions{AccountingMethod: ap.Basis, Filters: f}
}

// runPeriod executes the page's saved filters for one month.
func (ap *folioAuditPage) runPeriod(ctx context.Context, p auditPeriod) (*auditTotals, error) {
	res, err := ReplayReport(ctx, ap.Report, p.Start, p.End, ap.filters(""))
	if err != nil {
		return nil, err
	}
	return v3ReportTotals(res.Rows)
}

// runDistributions runs an alltime page once per resolved class with
// date_macro=All, then slices the same row set into the audit window —
// one call yields both the cumulative figure and the per-month split.
// Falls back to a windowed run when the page returns no txn rows.
func (ap *folioAuditPage) runDistributions(ctx context.Context, from, to string, audit *FolioAudit) error {
	classes := ap.Classes
	if len(classes) == 0 {
		classes = []string{""} // unscoped page: single company-wide run
	}
	t, _ := time.Parse("2006-01", to)
	winStart := from + "-01"
	winEnd := t.AddDate(0, 1, -1).Format("2006-01-02")
	for _, cls := range classes {
		d := folioAuditDist{Page: ap.Title, Class: cls, ByMonth: map[string]float64{}}
		opts := ap.filters(cls)
		opts.Filters["date_macro"] = "All"
		all, err := ReplayReport(ctx, ap.Report, "", "", opts)
		if err != nil {
			return err
		}
		if rows, werr := v3TxnRows(all.Columns, all.Rows); werr == nil {
			for _, r := range rows {
				d.AllTime += r.amount
				if r.date >= winStart && r.date <= winEnd {
					d.InRange += r.amount
					d.ByMonth[r.date[:7]] += r.amount
				}
			}
		} else {
			// Not txn-shaped: fall back to summary totals (window only).
			win, werr := ReplayReport(ctx, ap.Report, winStart, winEnd, ap.filters(cls))
			if werr != nil {
				return werr
			}
			if wt, _ := v3ReportTotals(win.Rows); wt != nil {
				d.InRange = wt.Total
			}
			if at, _ := v3ReportTotals(all.Rows); at != nil {
				d.AllTime = at.Total
			}
		}
		d.AllTime = roundCents(d.AllTime)
		d.InRange = roundCents(d.InRange)
		audit.Distributions = append(audit.Distributions, d)
	}
	return nil
}

func (a *FolioAudit) record(label string, ap *folioAuditPage, tot *auditTotals) {
	var pr *folioAuditPeriod
	for i := range a.Periods {
		if a.Periods[i].Label == label {
			pr = &a.Periods[i]
			break
		}
	}
	if pr == nil {
		a.Periods = append(a.Periods, folioAuditPeriod{
			Label: label, Units: map[string]float64{}, Combined: map[string]float64{},
		})
		pr = &a.Periods[len(a.Periods)-1]
	}
	switch ap.Kind {
	case "unit":
		pr.Units[ap.Title] = tot.Net
	case "combined":
		pr.Combined[ap.Title] = tot.Net
	default:
		if pr.Other == nil {
			pr.Other = map[string]float64{}
		}
		pr.Other[ap.Title] = tot.Net // company/unscoped: reported, never tied
	}
}

// reconcile computes per-period deltas and the cumulative position.
// Single-class pages are leaves: several leaves can share one class (a
// single-property folio carries detail + summary for the same unit), so the
// unit sum counts each class once via a representative leaf — Detail-shaped
// reports preferred. Leaves on the same class must agree with each other.
// Delta = combined − Σ class nets; folios without a combined page reconcile
// purely on leaf agreement.
func (a *FolioAudit) reconcile() {
	repTitle := map[string]string{}
	repIsDetail := map[string]bool{}
	titleClass := map[string]string{}
	hasCombined := false
	for _, p := range a.Pages {
		if p.Kind == "combined" {
			hasCombined = true
		}
		if p.Kind != "unit" || len(p.Classes) != 1 {
			continue
		}
		c := p.Classes[0]
		titleClass[p.Title] = c
		d := strings.Contains(p.Report, "Detail")
		if _, ok := repTitle[c]; !ok || (d && !repIsDetail[c]) {
			repTitle[c] = p.Title
			repIsDetail[c] = d
		}
	}
	pairWarned := map[string]bool{}
	earned := map[string]float64{} // class → Σ representative net
	for i := range a.Periods {
		pr := &a.Periods[i]
		netsByClass := map[string][]struct {
			t string
			v float64
		}{}
		for title, v := range pr.Units {
			c := titleClass[title]
			netsByClass[c] = append(netsByClass[c], struct {
				t string
				v float64
			}{title, v})
		}
		pairsOK := true
		for c, list := range netsByClass {
			for j := 1; j < len(list); j++ {
				if math.Abs(list[j].v-list[0].v) > 0.005 {
					pairsOK = false
					if !pairWarned[c] {
						pairWarned[c] = true
						a.Warnings = append(a.Warnings, fmt.Sprintf(
							"period %s: unit pages on class %s disagree — %q=%.2f vs %q=%.2f",
							pr.Label, c, list[0].t, list[0].v, list[j].t, list[j].v))
					}
				}
			}
		}
		for c, t := range repTitle {
			pr.UnitSum += pr.Units[t]
			earned[c] += pr.Units[t]
		}
		pr.UnitSum = roundCents(pr.UnitSum)
		for _, v := range pr.Combined {
			pr.CombinedSum += v
		}
		pr.CombinedSum = roundCents(pr.CombinedSum)
		if hasCombined {
			pr.Delta = roundCents(pr.CombinedSum - pr.UnitSum)
			pr.OK = pr.Delta == 0
		} else {
			pr.OK = pairsOK
		}
	}
	// Position: earned per class (via its representative leaf) minus
	// distributions sliced to the same class; without unit pages, report per
	// class id and flag it.
	a.Position = map[string]float64{}
	distByClass := map[string]float64{}
	for _, d := range a.Distributions {
		distByClass[d.Class] += d.InRange
	}
	for c, t := range repTitle {
		a.Position[t] = roundCents(earned[c] - distByClass[c])
	}
	if len(repTitle) == 0 && len(distByClass) > 0 {
		a.Warnings = append(a.Warnings, "no unit pages to bind distributions to — position reported per class id")
		for cls, v := range distByClass {
			a.Position["class:"+cls] = roundCents(-v)
		}
	}
}

// checkCoverage cross-checks class and account whitelists across pages —
// divergences that break the tie-out before a single number is compared.
func (a *FolioAudit) checkCoverage() {
	var units, combined []*folioAuditPage
	for i := range a.Pages {
		p := &a.Pages[i]
		switch p.Kind {
		case "unit":
			units = append(units, p)
		case "combined":
			combined = append(combined, p)
		}
	}
	unionClasses := map[string]bool{}
	for _, u := range units {
		for _, c := range u.Classes {
			unionClasses[c] = true
		}
	}
	for _, cb := range combined {
		for _, c := range cb.Classes {
			if !unionClasses[c] {
				a.Warnings = append(a.Warnings,
					fmt.Sprintf("combined page %q covers class %s with no unit page — combined total includes activity the detail pages never show", cb.Title, c))
			}
		}
		for _, u := range units {
			for _, c := range u.Classes {
				if !contains(cb.Classes, c) {
					a.Warnings = append(a.Warnings,
						fmt.Sprintf("unit page %q class %s absent from combined page %q — unit activity missing from the summary", u.Title, c, cb.Title))
				}
			}
			for _, acc := range cb.Accounts {
				if !contains(u.Accounts, acc) {
					a.Warnings = append(a.Warnings,
						fmt.Sprintf("account %s is in combined page %q but not in unit page %q — a posting on that class there breaks the tie-out", acc, cb.Title, u.Title))
				}
			}
		}
	}
	for _, u := range units {
		for _, cb := range combined {
			for _, acc := range u.Accounts {
				if !contains(cb.Accounts, acc) {
					a.Warnings = append(a.Warnings,
						fmt.Sprintf("account %s is in unit page %q but not in combined page %q — detail shows activity the summary drops", acc, u.Title, cb.Title))
				}
			}
		}
	}
}

// auditTotals carries the section totals one report run produces.
type auditTotals struct {
	Income   float64
	Expenses float64
	Net      float64
	Total    float64 // Net, else Σ summaries — for distribution-style pages
}

// v3ReportTotals walks a v3 report's Rows tree — top level is usually a
// single "Ordinary Income/Expenses" section whose Summary is the "Net Income"
// row, with "Income"/"Expenses" sections nested inside — and extracts the
// canonical totals. Exact labels only: "Total for Income", "Total for
// Expenses", "Net *" — nested group summaries ("Total for Client Property
// Expenses") must not fold into the top figures.
func v3ReportTotals(rows json.RawMessage) (*auditTotals, error) {
	var rmap struct {
		Row []map[string]any `json:"Row"`
	}
	if err := json.Unmarshal(rows, &rmap); err != nil {
		return nil, fmt.Errorf("report rows unreadable: %w", err)
	}
	t := &auditTotals{}
	var sawNet bool
	var sectionSum float64
	var walk func(n any)
	walk = func(n any) {
		m, _ := n.(map[string]any)
		if m == nil {
			return
		}
		// A section's Summary row is its total; matched once per label.
		if node, ok := m["Summary"].(map[string]any); ok {
			if label, amt, ok := colDataTotal(node["ColData"]); ok {
				l := strings.ToLower(label)
				switch {
				case l == "total for income":
					t.Income = amt
				case l == "total for expenses":
					t.Expenses = amt
				case strings.HasPrefix(l, "net"):
					if !sawNet {
						t.Net = amt
						sawNet = true
					}
				default:
					sectionSum += amt
				}
			}
		}
		if sub, ok := m["Rows"].(map[string]any); ok {
			list, _ := sub["Row"].([]any)
			for _, r := range list {
				walk(r)
			}
		}
		// trailing plain rows carry the total as ColData (no Summary node)
		if _, hasSummary := m["Summary"]; !hasSummary {
			if _, hasRows := m["Rows"]; !hasRows {
				if label, amt, ok := colDataTotal(m["ColData"]); ok {
					l := strings.ToLower(label)
					if strings.HasPrefix(l, "net") && !sawNet {
						t.Net = amt
						sawNet = true
					}
				}
			}
		}
	}
	for _, r := range rmap.Row {
		walk(r)
	}
	if !sawNet && t.Income != 0 {
		t.Net = roundCents(t.Income - t.Expenses)
	}
	t.Total = t.Net
	if t.Total == 0 && !sawNet {
		t.Total = roundCents(sectionSum) // e.g. a bare account-detail page
	}
	return t, nil
}

// colDataTotal extracts (label, amount) from a Summary/ColData array. The
// amount is the rightmost parseable numeric — wide detail reports pad Summary
// rows with trailing empty cells, so the last cell is often blank.
func colDataTotal(cd any) (string, float64, bool) {
	list, _ := cd.([]any)
	if len(list) < 2 {
		return "", 0, false
	}
	label, _ := list[0].(map[string]any)["value"].(string)
	if strings.TrimSpace(label) == "" {
		return "", 0, false
	}
	for i := len(list) - 1; i > 0; i-- {
		s, _ := list[i].(map[string]any)["value"].(string)
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
		if err != nil {
			continue // a non-numeric trailing cell (e.g. a name) — keep scanning
		}
		return strings.TrimSpace(label), f, true
	}
	return "", 0, false
}

// auditTxn is one transaction leaf row: normalised date + amount + cells.
type auditTxn struct {
	date   string // YYYY-MM-DD
	amount float64
	cells  []string
}

// v3TxnRows extracts transaction leaf rows from a v3 detail report. Leaf rows
// are ColData arrays; the date is the first cell. The amount comes from the
// column titled "Amount" when the report advertises columns; otherwise the
// tail heuristic — a trailing numeric cell is the running BALANCE, so when the
// last two numerics differ the amount is the second-from-right.
func v3TxnRows(cols []string, rows json.RawMessage) ([]auditTxn, error) {
	amtIdx := -1
	for i, c := range cols {
		l := strings.ToLower(strings.TrimSpace(c))
		if l == "amount" {
			amtIdx = i
			break
		}
	}
	var rmap map[string]any
	if err := json.Unmarshal(rows, &rmap); err != nil {
		return nil, err
	}
	var out []auditTxn
	var walk func(n any)
	walk = func(n any) {
		m, _ := n.(map[string]any)
		if m == nil {
			return
		}
		if sub, ok := m["Rows"].(map[string]any); ok {
			if list, _ := sub["Row"].([]any); len(list) > 0 {
				for _, r := range list {
					walk(r)
				}
				return
			}
		}
		cd, _ := m["ColData"].([]any)
		if len(cd) < 2 {
			return
		}
		var cells []string
		for _, c := range cd {
			v, _ := c.(map[string]any)["value"].(string)
			cells = append(cells, strings.TrimSpace(v))
		}
		d := parseTxnDate(cells[0])
		if d == "" {
			return
		}
		numAt := func(i int) (float64, bool) {
			f, err := strconv.ParseFloat(strings.ReplaceAll(cells[i], ",", ""), 64)
			return f, err == nil
		}
		var amt float64
		var found bool
		if amtIdx >= 0 && amtIdx < len(cells) {
			amt, found = numAt(amtIdx)
		}
		if !found {
			var tail []int
			for i := len(cells) - 1; i >= 0 && len(tail) < 2; i-- {
				if _, ok := numAt(i); ok {
					tail = append(tail, i)
				}
			}
			switch {
			case len(tail) == 0:
				return
			case len(tail) == 1:
				amt, _ = numAt(tail[0])
			default:
				a, _ := numAt(tail[1])
				b, _ := numAt(tail[0])
				if a != b {
					amt = a // two distinct trailing numerics: amount, then balance
				} else {
					amt = b
				}
			}
			found = true
		}
		out = append(out, auditTxn{date: d, amount: amt, cells: cells})
	}
	if top, ok := rmap["Row"].([]any); ok {
		for _, r := range top {
			walk(r)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no transaction rows")
	}
	return out, nil
}

func parseTxnDate(s string) string {
	s = strings.TrimSpace(s)
	for _, f := range []string{"2006-01-02", "02/01/2006"} {
		if t, err := time.Parse(f, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func strOfMap(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func roundCents(f float64) float64 {
	return math.Round(f*100) / 100
}
