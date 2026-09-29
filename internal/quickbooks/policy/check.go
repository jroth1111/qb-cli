// Journal-payload validation against the company policy rules.
//
// check-journal is read-only: it inspects a journal payload — either the
// --items-json array shape accepted by `accounting journal create`, or a raw
// v3 JournalEntry object as returned by `accounting journal get` — and reports
// findings against the documented MSA treatment rules (paired accounts, class
// parity, fee recompute, cleaning carry-forward, margin evidence). It never
// dials QBO and never asserts that a journal should be posted; unresolved
// evidence stays unresolved.
package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"
	"time"
)

// Account paths verified against chart-of-accounts.md.
const (
	AcctMgmtFeeDebit   = "Client Property Expenses:Short-Term Business Operating Expenses:Management Fee"
	AcctCleaningDebit  = "Client Property Expenses:Short-Term Business Operating Expenses:Commercial Cleaning Services"
	AcctOwnerDist      = "Client Property Expenses:Owner Distribution"
	AcctMgmtIncome     = "Management Income"
	AcctCleaningIncome = "Cleaning Amenities Income"
	cpePrefix          = "Client Property Expenses:"
)

// knownAccountIDs maps the policy-relevant QBO account ids to their names so
// id-only payloads (v3 lines or items-json) can still be checked.
var knownAccountIDs = map[string]string{
	"89":  AcctMgmtFeeDebit,
	"71":  AcctCleaningDebit,
	"88":  AcctOwnerDist,
	"1":   AcctMgmtIncome,
	"151": AcctCleaningIncome,
}

// landlordClassIDs maps the two landlord-management class ids to names
// (chart-of-accounts.md class inventory).
var landlordClassIDs = map[string]string{
	"3700000000000906240": "10 Toorak Melody",
	"3700000000000906241": "14 Agatha",
}

// CheckLine is one normalized journal line regardless of source format.
type CheckLine struct {
	PostingType string  `json:"posting_type"` // Debit | Credit
	Account     string  `json:"account,omitempty"`
	AccountID   string  `json:"account_id,omitempty"`
	Amount      float64 `json:"amount"`
	Class       string  `json:"class,omitempty"`
	ClassID     string  `json:"class_id,omitempty"`
	Description string  `json:"description,omitempty"`
}

// CheckInput is a journal normalized for checking, plus optional evidence the
// checker needs (fee revenue base, prior approved cleaning amount, supplier
// cost) that cannot be derived from the payload itself.
type CheckInput struct {
	Format         string      `json:"format"` // "journal-entry" | "items"
	TxnDate        string      `json:"txn_date,omitempty"`
	KindHint       string      `json:"kind_hint,omitempty"`
	Revenue        *float64    `json:"revenue,omitempty"`
	ExpectedAmount *float64    `json:"expected_amount,omitempty"`
	SupplierCost   *float64    `json:"supplier_cost,omitempty"`
	Lines          []CheckLine `json:"lines"`
}

// CheckOpts carries evidence supplied via CLI flags; flags override
// payload-embedded values. Company names the profile used for account-id
// resolution; empty means the sole embedded profile.
type CheckOpts struct {
	Kind           string
	Revenue        *float64
	ExpectedAmount *float64
	SupplierCost   *float64
	Company        string
}

// Finding is one check outcome.
type Finding struct {
	Severity string `json:"severity"` // error | warning | unresolved | info
	Check    string `json:"check"`
	Detail   string `json:"detail"`
}

// CheckResult is the full verdict for one journal.
type CheckResult struct {
	Format       string    `json:"format"`
	Kind         string    `json:"kind,omitempty"`
	DetectedKind string    `json:"detected_kind,omitempty"`
	Pairs        []string  `json:"pairs,omitempty"`
	TxnDate      string    `json:"txn_date,omitempty"`
	Balanced     bool      `json:"balanced"`
	TotalDebit   float64   `json:"total_debit"`
	TotalCredit  float64   `json:"total_credit"`
	Findings     []Finding `json:"findings"`
	Verdict      string    `json:"verdict"` // pass | fail | unverified
	OK           bool      `json:"ok"`
}

// ---------------------------------------------------------------------------
// Payload parsing
// ---------------------------------------------------------------------------

// checkItem mirrors the `accounting journal create --items-json` shape plus
// optional check metadata (names, class, per-item evidence). Extra keys are
// ignored by journal create; the checker uses them when present.
type checkItem struct {
	Date            string   `json:"date"`
	Amount          float64  `json:"amount"`
	FromAccount     string   `json:"from-account"`
	ToAccount       string   `json:"to-account"`
	FromAccountName string   `json:"from-account-name"`
	ToAccountName   string   `json:"to-account-name"`
	Class           string   `json:"class"`
	ClassName       string   `json:"class-name"`
	Kind            string   `json:"kind"`
	Memo            string   `json:"memo"`
	Revenue         *float64 `json:"revenue"`
	ExpectedAmount  *float64 `json:"expected-amount"`
	SupplierCost    *float64 `json:"supplier-cost"`
}

// itemRef splits an item field into (name, id): an explicit name field wins;
// a bare ref value counts as a name when it is not all digits (agents often
// write names where create wants ids — the checker should still see them).
func itemRef(ref, name string) (string, string) {
	if name != "" {
		return name, ref
	}
	if ref == "" {
		return "", ""
	}
	for _, r := range ref {
		if r < '0' || r > '9' {
			return ref, ""
		}
	}
	return "", ref
}

type v3Ref struct {
	Value string `json:"value"`
	Name  string `json:"name"`
}

type v3Line struct {
	Amount      float64 `json:"Amount"`
	Description string  `json:"Description"`
	Detail      struct {
		PostingType string `json:"PostingType"`
		AccountRef  v3Ref  `json:"AccountRef"`
		ClassRef    *v3Ref `json:"ClassRef"`
		Description string `json:"Description"`
	} `json:"JournalEntryLineDetail"`
}

type v3Entry struct {
	Id          string   `json:"Id"`
	TxnDate     string   `json:"TxnDate"`
	PrivateNote string   `json:"PrivateNote"`
	Line        []v3Line `json:"Line"`
}

// entryLines projects a v3 entry into checker lines.
func entryLines(e v3Entry) []CheckLine {
	var out []CheckLine
	for _, l := range e.Line {
		cl := CheckLine{
			PostingType: l.Detail.PostingType,
			Account:     l.Detail.AccountRef.Name,
			AccountID:   l.Detail.AccountRef.Value,
			Amount:      l.Amount,
			Description: l.Description,
		}
		if cl.Description == "" {
			cl.Description = l.Detail.Description
		}
		if l.Detail.ClassRef != nil {
			cl.Class = l.Detail.ClassRef.Name
			cl.ClassID = l.Detail.ClassRef.Value
		}
		out = append(out, cl)
	}
	return out
}

// entryFromObject unwraps a decoded JSON object to the raw JournalEntry
// when it sits inside a known envelope: {"JournalEntry":…},
// {"QueryResponse":{"JournalEntry":[…]}}, or the CLI's
// {"detail":{…}} QueryResult shape.
func entryFromObject(obj map[string]json.RawMessage) (json.RawMessage, bool) {
	if inner, ok := obj["JournalEntry"]; ok {
		var arr []json.RawMessage
		if err := json.Unmarshal(inner, &arr); err == nil && len(arr) > 0 {
			return arr[0], true
		}
		return inner, true
	}
	if qr, ok := obj["QueryResponse"]; ok {
		var q map[string]json.RawMessage
		if err := json.Unmarshal(qr, &q); err == nil {
			if inner, ok := q["JournalEntry"]; ok {
				var arr []json.RawMessage
				if err := json.Unmarshal(inner, &arr); err == nil && len(arr) > 0 {
					return arr[0], true
				}
				return inner, true
			}
		}
	}
	if det, ok := obj["detail"]; ok {
		return det, true
	}
	return nil, false
}

// decodeJournalStream reads every journal entry from a reader that may carry
// a JSON array, one object, or concatenated JSON objects (e.g. the natural
// `journal get --id … --json` loop output). Envelopes are unwrapped.
func decodeJournalStream(r io.Reader) ([]v3Entry, error) {
	dec := json.NewDecoder(r)
	var out []v3Entry
	seen := map[string]bool{}
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("journal stream does not decode: %w", err)
		}
		var trimmed = raw
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &obj); err == nil && obj != nil {
			if inner, ok := entryFromObject(obj); ok {
				trimmed = inner
			}
		}
		var e v3Entry
		if err := json.Unmarshal(trimmed, &e); err != nil {
			var arr []v3Entry
			if err2 := json.Unmarshal(trimmed, &arr); err2 == nil {
				for _, e2 := range arr {
					if e2.Id == "" || !seen[e2.Id] {
						seen[e2.Id] = true
						out = append(out, e2)
					}
				}
				continue
			}
			return nil, fmt.Errorf("journal stream entry does not decode: %w", err)
		}
		if e.Id != "" && seen[e.Id] {
			continue
		}
		seen[e.Id] = true
		out = append(out, e)
	}
	return out, nil
}

// ParseCheckInputs detects the payload format and returns one CheckInput per
// journal: a JSON array is treated as items-json (each item → one journal);
// an object is unwrapped from {"JournalEntry":{...}} or
// {"QueryResponse":{"JournalEntry":[{...}]}} when present.
func ParseCheckInputs(raw []byte) ([]CheckInput, error) {
	trim := strings.TrimSpace(string(raw))
	if trim == "" {
		return nil, fmt.Errorf("empty journal payload")
	}
	if strings.HasPrefix(trim, "[") {
		var items []checkItem
		if err := json.Unmarshal(raw, &items); err != nil {
			return nil, fmt.Errorf("payload looks like an items array but does not decode: %w", err)
		}
		if len(items) == 0 {
			return nil, fmt.Errorf("items array is empty")
		}
		out := make([]CheckInput, 0, len(items))
		for i, it := range items {
			if it.Amount == 0 {
				return nil, fmt.Errorf("item %d: amount must be non-zero", i)
			}
			if (it.FromAccount == "" && it.FromAccountName == "") || (it.ToAccount == "" && it.ToAccountName == "") {
				return nil, fmt.Errorf("item %d: from-account and to-account (or their *-name forms) are required", i)
			}
			fromAcct, fromID := itemRef(it.FromAccount, it.FromAccountName)
			toAcct, toID := itemRef(it.ToAccount, it.ToAccountName)
			className, classID := itemRef(it.Class, it.ClassName)
			out = append(out, CheckInput{
				Format:         "items",
				TxnDate:        it.Date,
				KindHint:       it.Kind,
				Revenue:        it.Revenue,
				ExpectedAmount: it.ExpectedAmount,
				SupplierCost:   it.SupplierCost,
				Lines: []CheckLine{
					{PostingType: "Debit", Account: fromAcct, AccountID: fromID, Amount: it.Amount, Class: className, ClassID: classID},
					{PostingType: "Credit", Account: toAcct, AccountID: toID, Amount: it.Amount, Class: className, ClassID: classID},
				},
			})
		}
		return out, nil
	}

	// Object: unwrap known envelopes.
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("journal payload must be an items array or a JournalEntry object: %w", err)
	}
	entryRaw := raw
	if inner, ok := entryFromObject(obj); ok {
		entryRaw = inner
	}
	var entry v3Entry
	if err := json.Unmarshal(entryRaw, &entry); err != nil {
		return nil, fmt.Errorf("JournalEntry payload does not decode: %w", err)
	}
	if len(entry.Line) == 0 {
		return nil, fmt.Errorf("JournalEntry carries no lines")
	}
	in := CheckInput{Format: "journal-entry", TxnDate: entry.TxnDate}
	in.Lines = entryLines(entry)
	return []CheckInput{in}, nil
}

// ---------------------------------------------------------------------------
// Checking
// ---------------------------------------------------------------------------

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func normalizeAcct(s string) string {
	parts := strings.Split(s, ":")
	for i := range parts {
		parts[i] = strings.Join(strings.Fields(parts[i]), " ")
	}
	return strings.Join(parts, ":")
}

// lineAccount resolves a line's account to a name — from the name field, the
// policy-id map, or the company's chart of accounts for id-only lines.
// "" when unresolvable.
func lineAccount(company string, l CheckLine) string {
	if l.Account != "" {
		return normalizeAcct(l.Account)
	}
	if n, ok := knownAccountIDs[l.AccountID]; ok {
		return n
	}
	if company != "" {
		return normalizeAcct(AccountNameForID(company, l.AccountID))
	}
	return ""
}

// classIdentity resolves a line's class to a name when possible.
func classIdentity(l CheckLine) (name string, resolved bool) {
	if l.Class != "" {
		return l.Class, true
	}
	if n, ok := landlordClassIDs[l.ClassID]; ok {
		return n, true
	}
	if l.ClassID != "" {
		return l.ClassID, false
	}
	return "", false
}

// debitLines/creditLines partition by posting type.
func splitLines(lines []CheckLine) (dr, cr []CheckLine) {
	for _, l := range lines {
		switch strings.ToLower(l.PostingType) {
		case "debit":
			dr = append(dr, l)
		case "credit":
			cr = append(cr, l)
		}
	}
	return
}

// inferKind detects the journal kind from its account names. Returns the kind
// or "" when the accounts match no documented pattern. "mixed" when lines
// disagree about which pattern they belong to.
func inferKind(company string, lines []CheckLine) string {
	dr, cr := splitLines(lines)
	kinds := map[string]bool{}
	for _, d := range dr {
		acct := lineAccount(company, d)
		var kind string
		switch {
		case acct == AcctMgmtFeeDebit:
			kind = "management-fee"
		case acct == AcctCleaningDebit:
			kind = "cleaning"
		case acct == AcctOwnerDist:
			kind = "owner-distribution"
		case strings.HasPrefix(acct, cpePrefix):
			kind = "cost-recharge"
		default:
			continue
		}
		// Debit side determines kind; a matching credit confirms.
		for _, c := range cr {
			ca := lineAccount(company, c)
			switch kind {
			case "management-fee", "cost-recharge":
				if ca == AcctMgmtIncome {
					kinds[kind] = true
				}
			case "cleaning":
				if ca == AcctCleaningIncome {
					kinds[kind] = true
				}
			case "owner-distribution":
				kinds[kind] = true
			}
		}
	}
	if len(kinds) == 0 {
		return ""
	}
	if len(kinds) > 1 {
		return "mixed"
	}
	for k := range kinds {
		return k
	}
	return ""
}

// decomposePairs partitions a journal's lines into balanced Dr/Cr pairs,
// matching each debit to a credit with equal amount and the same resolved
// class. Returns nil when the lines are not a clean set of pairs (split
// entries, unmatched amounts) — callers then keep the mixed-kind error.
func decomposePairs(company string, lines []CheckLine) [][]CheckLine {
	dr, cr := splitLines(lines)
	if len(lines) < 4 || len(dr) == 0 || len(dr) != len(cr) || len(dr)+len(cr) != len(lines) {
		return nil
	}
	key := func(l CheckLine) string {
		cn, _ := classIdentity(l)
		return fmt.Sprintf("%.2f|%s", round2(l.Amount), cn)
	}
	avail := map[string][]int{}
	for i, c := range cr {
		k := key(c)
		avail[k] = append(avail[k], i)
	}
	used := make([]bool, len(cr))
	pairs := make([][]CheckLine, 0, len(dr))
	for _, d := range dr {
		matched := -1
		for _, ci := range avail[key(d)] {
			if !used[ci] {
				matched = ci
				break
			}
		}
		if matched < 0 {
			return nil
		}
		used[matched] = true
		pairs = append(pairs, []CheckLine{d, cr[matched]})
	}
	return pairs
}

// isMonthEnd parses dd/MM/yyyy or yyyy-MM-dd and reports whether it is the
// last day of its month. Unparseable dates return false (caller skips).
func isMonthEnd(date string) bool {
	for _, layout := range []string{"2006-01-02", "02/01/2006"} {
		if t, err := time.Parse(layout, date); err == nil {
			return t.AddDate(0, 0, 1).Day() == 1
		}
	}
	return false
}

// CheckJournal validates one normalized journal against the policy rules.
// Severity semantics: error = violates a documented rule; warning = suspicious
// but not proven wrong; unresolved = evidence required but not supplied.
// Verdict: fail (any error) > unverified (any unresolved) > pass.
func CheckJournal(in CheckInput, opts CheckOpts) CheckResult {
	res := CheckResult{Format: in.Format, TxnDate: in.TxnDate}
	company, _ := resolveCompanyForCOA(opts.Company)
	add := func(sev, check, format string, args ...any) {
		res.Findings = append(res.Findings, Finding{Severity: sev, Check: check, Detail: fmt.Sprintf(format, args...)})
	}

	if opts.Kind != "" {
		in.KindHint = opts.Kind
	}
	if opts.Revenue != nil {
		in.Revenue = opts.Revenue
	}
	if opts.ExpectedAmount != nil {
		in.ExpectedAmount = opts.ExpectedAmount
	}
	if opts.SupplierCost != nil {
		in.SupplierCost = opts.SupplierCost
	}

	dr, cr := splitLines(in.Lines)
	for i, line := range in.Lines {
		switch strings.ToLower(line.PostingType) {
		case "debit", "credit":
		default:
			add("error", "line_type", "line %d requires PostingType Debit or Credit", i+1)
		}
		if line.Amount < 0 || math.IsNaN(line.Amount) || math.IsInf(line.Amount, 0) {
			add("error", "line_amount", "line %d requires a finite non-negative amount", i+1)
		}
	}
	for _, l := range dr {
		res.TotalDebit += l.Amount
	}
	for _, l := range cr {
		res.TotalCredit += l.Amount
	}
	res.TotalDebit = round2(res.TotalDebit)
	res.TotalCredit = round2(res.TotalCredit)

	if len(in.Lines) < 2 {
		add("error", "line_count", "journal carries %d line(s); assess as a complete balanced pair, never line-by-line", len(in.Lines))
	}
	res.Balanced = math.Abs(res.TotalDebit-res.TotalCredit) < 0.005
	if res.Balanced {
		add("info", "balance", "balanced: debits %.2f = credits %.2f", res.TotalDebit, res.TotalCredit)
	} else {
		add("error", "balance", "unbalanced: debits %.2f != credits %.2f", res.TotalDebit, res.TotalCredit)
	}

	// Kind resolution: explicit hint wins; inference conflict is surfaced.
	detected := inferKind(company, in.Lines)
	res.DetectedKind = detected
	kind := in.KindHint
	switch {
	case kind == "" && detected == "":
		add("unresolved", "kind", "cannot infer journal kind from accounts; pass --kind (management-fee|cleaning|owner-distribution|cost-recharge)")
	case kind == "" && detected == "mixed":
		// A balanced entry that decomposes into clean Dr/Cr pairs (equal
		// amount and class on each side) is the documented "complete
		// balanced pairs" convention — MSA posts fee+cleaning as one entry.
		// Check each pair against its own template instead of failing.
		pairs := decomposePairs(company, in.Lines)
		if pairs == nil {
			add("error", "kind", "lines mix multiple journal kinds; assess each pair separately")
			break
		}
		res.Pairs = []string{}
		for i, p := range pairs {
			sub := CheckJournal(CheckInput{Format: in.Format, TxnDate: in.TxnDate, Lines: p}, opts)
			pk := sub.Kind
			if pk == "" {
				pk = sub.DetectedKind
			}
			if pk == "" {
				pk = "unknown"
			}
			res.Pairs = append(res.Pairs, pk)
			for _, f := range sub.Findings {
				if f.Check == "balance" || f.Check == "line_count" {
					continue
				}
				f.Detail = fmt.Sprintf("pair %d (%s): %s", i+1, pk, f.Detail)
				res.Findings = append(res.Findings, f)
			}
		}
		add("info", "pairs", "entry decomposes into %d balanced pairs (%s); each was checked against its own template", len(pairs), strings.Join(res.Pairs, ", "))
	case kind == "":
		kind = detected
	case detected != "" && detected != "mixed" && detected != kind:
		add("warning", "kind", "declared kind %q but accounts match %q — verifying declared kind, review the mismatch", kind, detected)
	}
	res.Kind = kind

	// Account mapping + class parity against the template.
	if kind != "" && kind != "mixed" {
		tpl, terr := JournalTemplateFor(kind)
		if terr != nil {
			add("error", "kind", "unknown kind %q", kind)
		} else {
			checkAccounts(company, in, *tpl, dr, cr, add)
			// The property class belongs on both sides of every paired fee
			// journal; kinds that don't require one skip the check.
			if tpl.ClassRequired {
				checkClass(in, add)
			}
		}
	}

	// Kind-specific amount verification.
	checkAmount(company, in, kind, cr, add)

	// Timing: management-fee journals are usually month-end.
	if kind == "management-fee" && in.TxnDate != "" && !isMonthEnd(in.TxnDate) {
		add("info", "timing", "TxnDate %s is not month-end; management-fee journals are usually month-end pairs — confirm the date-period logic", in.TxnDate)
	}

	// Description parity on v3 entries. Mixed entries that decomposed into
	// pairs were already checked per pair; legitimate pair memos differ.
	if in.Format == "journal-entry" && len(in.Lines) >= 2 && res.Pairs == nil {
		descs := map[string]bool{}
		for _, l := range in.Lines {
			if l.Description != "" {
				descs[l.Description] = true
			}
		}
		if len(descs) > 1 {
			add("warning", "description_parity", "lines carry different descriptions; the documented pattern uses the same description on both sides")
		}
	}

	errs, unres := 0, 0
	for _, f := range res.Findings {
		switch f.Severity {
		case "error":
			errs++
		case "unresolved":
			unres++
		}
	}
	switch {
	case errs > 0:
		res.Verdict = "fail"
	case unres > 0:
		res.Verdict = "unverified"
	default:
		res.Verdict = "pass"
	}
	res.OK = res.Verdict == "pass"
	return res
}

// checkAccounts verifies the journal's accounts against the template pair.
func checkAccounts(company string, in CheckInput, tpl JournalTemplate, dr, cr []CheckLine, add func(string, string, string, ...any)) {
	wantDr := normalizeAcct(tpl.DebitAccount)
	wantCr := normalizeAcct(tpl.CreditAccount)
	drWildcard := strings.HasSuffix(wantDr, "*")
	crWildcard := strings.HasSuffix(wantCr, "*")

	matchAcct := func(l CheckLine, want string, wildcard bool) bool {
		got := lineAccount(company, l)
		if got == "" {
			return false
		}
		if wildcard {
			return strings.HasPrefix(got, strings.TrimSuffix(want, "*"))
		}
		return got == want
	}

	foundDr, foundCr := false, false
	for _, l := range dr {
		if matchAcct(l, wantDr, drWildcard) {
			foundDr = true
		}
	}
	for _, l := range cr {
		if matchAcct(l, wantCr, crWildcard) {
			foundCr = true
		}
	}
	if !foundDr {
		add("error", "account_mapping", "no debit line matches expected account %q", tpl.DebitAccount)
	}
	if !foundCr {
		add("error", "account_mapping", "no credit line matches expected account %q", tpl.CreditAccount)
	}

	// Lines outside the pair: fine only for cost-recharge wildcard debits and
	// owner-distribution's open credit leg.
	for _, l := range dr {
		if !matchAcct(l, wantDr, drWildcard) {
			add("warning", "account_mapping", "debit line on %q is outside the %s pair — verify each pair separately", accountLabel(l), tpl.Kind)
		}
	}
	for _, l := range cr {
		if !matchAcct(l, wantCr, crWildcard) {
			add("warning", "account_mapping", "credit line on %q is outside the %s pair — verify each pair separately", accountLabel(l), tpl.Kind)
		}
	}

	// Owner distribution must never be booked against an income account —
	// it is not an expense journal and never contractor pay.
	if tpl.Kind == "owner-distribution" {
		for _, l := range cr {
			ca := lineAccount(company, l)
			if ca == AcctMgmtIncome || ca == AcctCleaningIncome {
				add("error", "account_mapping", "owner distribution credits %q: recorded separately, never an expense/income pair", ca)
			}
		}
	}
}

func accountLabel(l CheckLine) string {
	if l.Account != "" {
		return l.Account
	}
	return "id:" + l.AccountID
}

// checkClass enforces same-property-class on both lines where the template
// requires it.
func checkClass(in CheckInput, add func(string, string, string, ...any)) {
	identities := map[string]bool{}
	missing, unresolvable := 0, 0
	for _, l := range in.Lines {
		name, resolved := classIdentity(l)
		switch {
		case l.Class == "" && l.ClassID == "":
			missing++
		case !resolved:
			unresolvable++
		default:
			identities[name] = true
		}
	}
	if missing > 0 {
		add("error", "class_parity", "%d line(s) carry no class; the property class belongs on both sides of every paired journal", missing)
	}
	if len(identities) > 1 {
		var names []string
		for n := range identities {
			names = append(names, n)
		}
		sort.Strings(names)
		add("error", "class_parity", "lines carry different classes (%s); same property class required on both sides", strings.Join(names, ", "))
	}
	for n := range identities {
		if !landlordClasses[n] {
			add("warning", "class_parity", "class %q is not one of the landlord-management properties these rules are evidenced for", n)
		}
	}
	if missing == 0 && len(identities) == 1 {
		for n := range identities {
			add("info", "class_parity", "consistent property class %q on all lines", n)
		}
	}
	if unresolvable > 0 {
		add("unresolved", "class_parity", "%d line(s) carry a class id not in the known map; verify it names the journal's property", unresolvable)
	}
}

// checkAmount applies the kind-specific amount rules.
func checkAmount(company string, in CheckInput, kind string, cr []CheckLine, add func(string, string, string, ...any)) {
	switch kind {
	case "management-fee":
		if in.Revenue == nil {
			add("unresolved", "amount", "fee not verified: supply --revenue (Client Property Revenue for the period, same class) to recompute 20.9%%")
			return
		}
		fee := round2(*in.Revenue * 0.209)
		var credit float64
		for _, l := range cr {
			if lineAccount(company, l) == AcctMgmtIncome {
				credit += l.Amount
			}
		}
		credit = round2(credit)
		if math.Abs(credit-fee) < 0.005 {
			add("info", "amount", "fee verified: %.2f = revenue %.2f x 20.9%%", credit, *in.Revenue)
		} else {
			add("error", "amount", "fee mismatch: credit %.2f but revenue %.2f x 20.9%% = %.2f (difference %+.2f); check revenue base and rounding", credit, *in.Revenue, fee, credit-fee)
		}
	case "cleaning":
		if in.ExpectedAmount == nil {
			add("unresolved", "amount", "cleaning fee not verified: rule MSA.CLEANING_FEE_CARRY_FORWARD requires the prior month's approved amount; pass --expected-amount")
			return
		}
		var debit float64
		dr, _ := splitLines(in.Lines)
		for _, l := range dr {
			if lineAccount(company, l) == AcctCleaningDebit {
				debit += l.Amount
			}
		}
		debit = round2(debit)
		if math.Abs(debit-*in.ExpectedAmount) < 0.005 {
			add("info", "amount", "cleaning fee matches prior approved amount %.2f", debit)
		} else {
			add("error", "amount", "cleaning fee %.2f differs from prior approved amount %.2f; if carry-forward and activity pricing conflict, present both and ask — do not choose silently", debit, *in.ExpectedAmount)
		}
	case "cost-recharge":
		if in.SupplierCost == nil {
			add("unresolved", "amount", "margin unverified: locate the underlying supplier cost before asserting margin (rule MSA.COST_MARGIN_EVIDENCE); pass --supplier-cost")
			return
		}
		var charge float64
		for _, l := range cr {
			if lineAccount(company, l) == AcctMgmtIncome {
				charge += l.Amount
			}
		}
		margin := round2(charge - *in.SupplierCost)
		sev := "info"
		if margin < 0 {
			sev = "warning"
		}
		add(sev, "amount", "recharge %.2f - supplier cost %.2f = margin %.2f; verify against a comparable approved entry", charge, *in.SupplierCost, margin)
	case "owner-distribution":
		add("info", "amount", "owner distribution is separate from the statement equation; reconcile statement net to distributions independently")
	}
}
