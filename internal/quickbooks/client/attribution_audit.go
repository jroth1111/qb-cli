package client

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// Business rule (per owner instruction): money collected under cancellation,
// break-lease, Airbnb resolution/claim, or guest-fee semantics belongs to MSA
// (Management Income), never to the property owner. Lines carrying those
// semantics inside a Client Property Revenue account are misattributions:
// positive amounts overpay the owner, negative amounts charge the owner for
// costs that were MSA's.
//
// This audit is detection-only — it reclassifies nothing. Ordinary
// refund/adjustment/write-off lines are reported at `review` severity because
// direction depends on whether the underlying revenue was MSA's or the
// owner's.

// msaRevenueSemantics matches descriptions that are unambiguously MSA revenue
// under the rule. Phrase-level on purpose: a bare "fee" would catch
// "Accommodation fee" boilerplate and surnames like "Breakey" catch "break".
var msaRevenueSemantics = regexp.MustCompile(`(?i)\bcancel(?:l?ed|l?ing|lation)?\b|break[ -]lease|early[ -]term|resolution|claim|charged to guest|guest fee|cleaning fee|penalt|damage claim|extra guest|late check`)

// reviewSemantics matches descriptions where the correct account depends on
// what the underlying money was — a refund of ordinary stay revenue is the
// owner's; a refund of a cancellation payout is MSA's.
var reviewSemantics = regexp.MustCompile(`(?i)\brefund|\badjustment\b|write ?off|writeoff|partially refund|booking adjustment`)

// strongMSA excludes the fee-phrase subset (cleaning fee, late check-in) that
// can appear inside predominantly-accommodation lines, e.g. "Accommodation
// 5th Dec - 20th Dec (including cleaning fee)". A line matching only the
// fee-ish phrases AND mentioning accommodation is demoted to review — it
// contains MSA money, but flagging the whole line as misbooked overstates it.
var strongMSA = regexp.MustCompile(`(?i)\bcancel(?:l?ed|l?ing|lation)?\b|break[ -]lease|early[ -]term|resolution|claim|charged to guest|guest fee|penalt|damage claim|extra guest`)

var accommodationMention = regexp.MustCompile(`(?i)accommodation`)

// AttributionFinding is one revenue line whose description carries
// cancellation/fee/claim (or reviewable refund/adjustment) semantics inside an
// owner-revenue account.
type AttributionFinding struct {
	Entity      string  `json:"entity"` // Deposit or Invoice
	EntityID    string  `json:"entity_id"`
	LineID      string  `json:"line_id"`
	TxnDate     string  `json:"txn_date"`
	Class       string  `json:"class"`
	Account     string  `json:"account"` // revenue account id the line posts to
	Amount      float64 `json:"amount"`
	Direction   string  `json:"direction"` // owner_overpaid | owner_underpaid
	Severity    string  `json:"severity"`  // msa | review
	Description string  `json:"description"`
}

// AttributionAudit is the scan result for one window.
type AttributionAudit struct {
	From           string               `json:"from"`
	To             string               `json:"to"`
	RevenueAccount string               `json:"revenue_account"`
	Findings       []AttributionFinding `json:"findings"`
	Scanned        int                  `json:"scanned"` // revenue lines examined
}

// ScanAttributionFindings is the pure core: given decoded v3 entities
// ("Deposit" or "Invoice") it returns every line posting to revenueAccount
// whose description carries MSA-revenue or reviewable semantics. Kept free of
// I/O so tests drive it with fixtures.
func ScanAttributionFindings(kind string, entities []map[string]any, revenueAccount string) []AttributionFinding {
	var out []AttributionFinding
	for _, e := range entities {
		eid := strOfMap(e, "Id")
		txnDate := strOfMap(e, "TxnDate")
		lines, _ := e["Line"].([]any)
		for _, raw := range lines {
			L, _ := raw.(map[string]any)
			if L == nil {
				continue
			}
			sd, _ := L["DepositLineDetail"].(map[string]any)
			if sd == nil {
				sd, _ = L["SalesItemLineDetail"].(map[string]any)
			}
			if sd == nil {
				continue
			}
			acct := refValue(sd, "AccountRef")
			if acct == "" {
				acct = refValue(sd, "ItemAccountRef")
			}
			if acct != revenueAccount {
				continue
			}
			desc := strOfMap(L, "Description")
			var sev string
			switch {
			case strongMSA.MatchString(desc):
				sev = "msa"
			case msaRevenueSemantics.MatchString(desc):
				sev = "msa"
				if accommodationMention.MatchString(desc) {
					sev = "review" // mixed accommodation+fee line: partial MSA money
				}
			case reviewSemantics.MatchString(desc):
				sev = "review"
			default:
				continue
			}
			amt, _ := L["Amount"].(float64)
			dir := "owner_overpaid"
			if amt < 0 {
				dir = "owner_underpaid"
			}
			out = append(out, AttributionFinding{
				Entity:      kind,
				EntityID:    eid,
				LineID:      strOfMap(L, "Id"),
				TxnDate:     txnDate,
				Class:       refName(sd, "ClassRef"),
				Account:     acct,
				Amount:      amt,
				Direction:   dir,
				Severity:    sev,
				Description: desc,
			})
		}
	}
	return out
}

// AuditRevenueAttribution scans deposit and invoice lines in [from,to]
// (YYYY-MM) for MSA-revenue semantics posted to the owner revenue account
// (default "47", Client Property Revenue).
func AuditRevenueAttribution(ctx context.Context, from, to, revenueAccount string) (*AttributionAudit, error) {
	if strings.TrimSpace(revenueAccount) == "" {
		revenueAccount = "47"
	}
	periods, err := monthRange(from, to)
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	audit := &AttributionAudit{From: from, To: to, RevenueAccount: revenueAccount}
	lo := periods[0].Start
	hi := periods[len(periods)-1].End
	for _, q := range []struct{ kind, entity string }{
		{"Deposit", "deposit"}, {"Invoice", "invoice"},
	} {
		entities, err := v3QueryAll(ctx, ac, q.entity,
			fmt.Sprintf("select * from %s where TxnDate >= '%s' and TxnDate <= '%s'", q.entity, lo, hi))
		if err != nil {
			return nil, err
		}
		var decoded []map[string]any
		for _, raw := range entities {
			var m map[string]any
			if json.Unmarshal(raw, &m) == nil {
				decoded = append(decoded, m)
			}
		}
		for _, e := range decoded {
			lines, _ := e["Line"].([]any)
			for _, raw := range lines {
				L, _ := raw.(map[string]any)
				sd, _ := L["DepositLineDetail"].(map[string]any)
				if sd == nil {
					sd, _ = L["SalesItemLineDetail"].(map[string]any)
				}
				if sd == nil {
					continue
				}
				acct := refValue(sd, "AccountRef")
				if acct == "" {
					acct = refValue(sd, "ItemAccountRef")
				}
				if acct == revenueAccount {
					audit.Scanned++
				}
			}
		}
		audit.Findings = append(audit.Findings, ScanAttributionFindings(q.kind, decoded, revenueAccount)...)
	}
	return audit, nil
}

// v3QueryAll pages a v3 select through STARTPOSITION until a short page.
// Rows are deduped on Id — a server that overlaps windows must not let a
// finding count twice.
func v3QueryAll(ctx context.Context, ac *apiClient, entity, stmt string) ([]json.RawMessage, error) {
	var all []json.RawMessage
	seen := map[string]bool{}
	for start := 1; ; start += queryPageSize {
		page := fmt.Sprintf("%s STARTPOSITION %d MAXRESULTS %d", stmt, start, queryPageSize)
		body, status, err := v3QueryGET(ctx, ac, entity, page)
		if err != nil {
			return nil, err
		}
		if status == 429 || status >= 500 {
			return nil, &ReplayError{Status: status, Message: errorMessage(body)}
		}
		if err := validateQueryEnvelope(entity, body); err != nil {
			return nil, err
		}
		var envelope struct {
			QueryResponse map[string]json.RawMessage `json:"QueryResponse"`
		}
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, fmt.Errorf("v3 query %s: malformed envelope", entity)
		}
		key := queryResponseEntity(entity)
		raw, ok := envelope.QueryResponse[key]
		if !ok {
			// pick the sole collection key if the name differs from the request
			for k, v := range envelope.QueryResponse {
				if k != "maxResults" && k != "startPosition" && k != "totalCount" {
					raw, ok = v, true
					break
				}
			}
		}
		var rows []json.RawMessage
		if ok {
			_ = json.Unmarshal(raw, &rows)
		}
		fresh := 0
		for _, row := range rows {
			var probe struct {
				ID string `json:"Id"`
			}
			if json.Unmarshal(row, &probe) == nil && probe.ID != "" {
				if seen[probe.ID] {
					continue
				}
				seen[probe.ID] = true
			}
			all = append(all, row)
			fresh++
		}
		// A full page contributes no new ids — the server is echoing the same
		// window regardless of STARTPOSITION; stop rather than walk forever.
		if len(rows) < queryPageSize || fresh == 0 {
			return all, nil
		}
	}
}

// auditPeriod bounds are produced by monthRange in folio_audit.go; the
// refValue/refName/strOfMap helpers come from chequebounce.go/folio_audit.go.
