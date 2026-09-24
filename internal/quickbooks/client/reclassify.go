package client

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Reclassify rides the qbo.intuit.com datarequest BFF — not a GraphQL host.
// Captured live on TC2 2026-09-18 from the Accountant "Reclassify
// transactions" tool (Accounting and bookkeeping → Office expenses, 3 lines,
// then reversed):
//
//	GET  /api/datarequest/6377689576571494/reclassifyTxn/getTxnData
//	     ?basis=accrual&fromDate=..&toDate=..&accountId=<src>&...
//	POST /api/datarequest/6377689576571494/reclassifyTxn/reclassify
//	     {"txnInfos":[{txnId,sequence,editSequence,txnType}…],"accountId":"<dst>"}
//
// The read answers a JS literal (unquoted new Date()/qbo.amount calls), so
// txnInfos are extracted per-item with regexes rather than JSON decode.
// The route id is the observed test-company plugin route, not a company id.
// Other UI sessions may expose a different plugin route; capture that contract
// before assuming a failed call requires a transaction-type conversion.
const reclassifyRouteID = "6377689576571494"

var reclassifyOverrides = map[string]string{
	"Referer":          "https://qbo.intuit.com/app/reclassify-transaction",
	"intuit-plugin-id": "accounting-transactions-reclassify-ui",
	"Content-Type":     "application/text; charset=UTF-8",
}

var (
	reTxnID        = regexp.MustCompile(`"txnId"\s*:\s*(\d+)`)
	reSequence     = regexp.MustCompile(`"sequence"\s*:\s*(\d+)`)
	reEditSequence = regexp.MustCompile(`"editSequence"\s*:\s*(\d+)`)
	reTypeID       = regexp.MustCompile(`"typeId"\s*:\s*(\d+)`)
	reItem         = regexp.MustCompile(`\{[^{}]*"txnId"[^{}]*\}`)
	reNotEditable  = regexp.MustCompile(`"editable"\s*:\s*false`)
	reTaxLine      = regexp.MustCompile(`"isTaxLine"\s*:\s*true`)
	reEmptyItems   = regexp.MustCompile(`"items"\s*:\s*\[\s*\]`)
)

// ReclassifyTxn is one reclassifiable transaction line from getTxnData.
type ReclassifyTxn struct {
	TxnID        int64
	Sequence     int
	EditSequence int
	TxnType      int
}

// ReplayReclassify moves every editable transaction line in a source account
// to a target account over a date range. source/target accept a numeric v3
// account Id or an exact account name resolved through the v3 query service.
func ReplayReclassify(ctx context.Context, target, source, dest, from, to, basis string) (*MutateResult, error) {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return nil, fmt.Errorf("reclassify requires --target <account|class|location|gst-code>")
	}
	if target != "account" {
		return nil, fmt.Errorf("reclassify --target %s: only 'account' is wired — the captured contract carries accountId only", target)
	}
	fromISO, toISO := normalizeDate(from), normalizeDate(to)
	if from == "" && to == "" {
		fromISO = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
		toISO = time.Now().Format("2006-01-02")
	} else {
		a, e1 := time.Parse(time.DateOnly, fromISO)
		b, e2 := time.Parse(time.DateOnly, toISO)
		if e1 != nil || e2 != nil || b.Before(a) {
			return nil, fmt.Errorf("reclassify requires a valid ordered --from and --to pair; no mutation sent")
		}
	}
	basis = strings.ToLower(strings.TrimSpace(basis))
	if basis == "" {
		basis = "accrual"
	}
	if basis != "cash" && basis != "accrual" {
		return nil, fmt.Errorf("reclassify basis must be cash or accrual")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	srcID, err := reclassifyAccountID(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("from-account: %w", err)
	}
	dstID, err := reclassifyAccountID(ctx, dest)
	if err != nil {
		return nil, fmt.Errorf("to-account: %w", err)
	}
	if srcID == dstID {
		return nil, fmt.Errorf("from-account and to-account resolve to the same account id %s", srcID)
	}
	txns, err := reclassifyTxnList(ctx, ac, srcID, fromISO, toISO, basis)
	if err != nil {
		return nil, err
	}
	if len(txns) == 0 {
		return &MutateResult{
			Status: http.StatusOK, Op: "reclassify", Entity: "Transaction",
			Item: QueryItem{Type: "Account", ID: srcID, Name: "0 lines"},
			Note: "no editable transactions in source account for the date range",
		}, nil
	}
	infos := make([]map[string]any, 0, len(txns))
	for _, t := range txns {
		infos = append(infos, map[string]any{
			"txnId":        t.TxnID,
			"sequence":     t.Sequence,
			"editSequence": t.EditSequence,
			"txnType":      t.TxnType,
		})
	}
	body, err := json.Marshal(map[string]any{"txnInfos": infos, "accountId": dstID})
	if err != nil {
		return nil, err
	}
	postURL := "https://qbo.intuit.com/api/datarequest/" + reclassifyRouteID + "/reclassifyTxn/reclassify"
	resp, err := ac.doURIHost(ctx, http.MethodPost, postURL, "qbo.intuit.com", body, reclassifyOverridesFor(ac))
	if err != nil {
		return nil, fmt.Errorf("reclassify: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading reclassify: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	if strings.Contains(string(raw), `"error"`) && strings.Contains(string(raw), `"message"`) {
		return nil, fmt.Errorf("reclassify: %.400s", raw)
	}
	if err := validateReclassifyReceipt(raw, txns); err != nil {
		return nil, err
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "reclassify",
		Entity: "Transaction",
		Item:   QueryItem{Type: "Account", ID: dstID, Name: fmt.Sprintf("%d lines → %s", len(txns), dstID)},
		Note:   "datarequest reclassifyTxn " + srcID + "→" + dstID,
	}, nil
}

// Require one positive, correlated receipt for every submitted transaction
// line. A valid JSON acknowledgement alone says nothing about the outcome.
func validateReclassifyReceipt(raw []byte, expected []ReclassifyTxn) error {
	if err := reclassifyReceiptFailure(raw); err != nil {
		return err
	}
	var rows []struct {
		TxnID    int64  `json:"txnId"`
		Sequence int    `json:"sequence"`
		Result   string `json:"result"`
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		var envelope struct {
			Items   json.RawMessage `json:"items"`
			Results json.RawMessage `json:"results"`
		}
		if json.Unmarshal(raw, &envelope) != nil {
			return fmt.Errorf("reclassify: missing line receipts; inspect before retrying")
		}
		data := envelope.Items
		if len(data) == 0 {
			data = envelope.Results
		}
		if json.Unmarshal(data, &rows) != nil {
			return fmt.Errorf("reclassify: missing line receipts; inspect before retrying")
		}
	}
	want := make(map[[2]int64]bool, len(expected))
	for _, t := range expected {
		want[[2]int64{t.TxnID, int64(t.Sequence)}] = true
	}
	if len(rows) != len(expected) || len(want) != len(expected) {
		return fmt.Errorf("reclassify: receipt count mismatch; inspect all target readbacks before retrying")
	}
	for _, row := range rows {
		key := [2]int64{row.TxnID, int64(row.Sequence)}
		if !want[key] || !strings.EqualFold(strings.TrimSpace(row.Result), "Success") {
			return fmt.Errorf("reclassify: uncorrelated or unsuccessful receipt; inspect all target readbacks before retrying")
		}
		delete(want, key)
	}
	return nil
}

// HTTP success can contain per-line failures, including a partially applied
// batch. Surface these as inspection stops rather than inviting a blind retry.
func reclassifyReceiptFailure(raw []byte) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("reclassify: invalid receipt; inspect readback before retry: %w", err)
	}
	var inspect func(any) error
	inspect = func(value any) error {
		switch v := value.(type) {
		case []any:
			for _, item := range v {
				if err := inspect(item); err != nil {
					return err
				}
			}
		case map[string]any:
			if result, present := v["result"]; present {
				status, ok := result.(string)
				if !ok || !strings.EqualFold(strings.TrimSpace(status), "Success") {
					return fmt.Errorf("reclassify: transaction %v sequence %v returned %v (%v); inspect all target readbacks before retry", v["txnId"], v["sequence"], result, v["additionalMessage"])
				}
			}
			for _, key := range []string{"items", "results"} {
				if nested, ok := v[key]; ok {
					if err := inspect(nested); err != nil {
						return err
					}
				}
			}
		}
		return nil
	}
	return inspect(value)
}

// reclassifyTxnList fetches the editable transaction lines for an account.
func reclassifyTxnList(ctx context.Context, ac *apiClient, accountID, from, to, basis string) ([]ReclassifyTxn, error) {
	q := url.Values{
		"basis":              {basis},
		"fromDate":           {from},
		"toDate":             {to},
		"show":               {"all"},
		"includeJE":          {"true"},
		"includeAttachments": {"false"},
		"getAllTxns":         {"true"},
		"limit":              {"150"},
		"offset":             {"0"},
		"hasErrors":          {"false"},
		"isValid":            {"true"},
		"accountId":          {accountID},
	}
	u := "https://qbo.intuit.com/api/datarequest/" + reclassifyRouteID + "/reclassifyTxn/getTxnData?" + q.Encode()
	resp, err := ac.doURIHost(ctx, http.MethodGet, u, "qbo.intuit.com", nil, reclassifyOverridesFor(ac))
	if err != nil {
		return nil, fmt.Errorf("reclassify txn list: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reclassify txn list: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var out []ReclassifyTxn
	items := reItem.FindAll(raw, -1)
	if len(items) == 0 && !reEmptyItems.Match(raw) {
		return nil, fmt.Errorf("reclassify source list unrecognized; no mutation sent")
	}
	if len(items) >= 150 {
		return nil, fmt.Errorf("reclassify source reached page limit; narrow the date range before mutating")
	}
	for _, item := range items {
		if reNotEditable.Match(item) || reTaxLine.Match(item) {
			continue
		}
		if !reSequence.Match(item) || !reEditSequence.Match(item) || !reTypeID.Match(item) {
			return nil, fmt.Errorf("reclassify source line lacks required identity/version; no mutation sent")
		}
		var t ReclassifyTxn
		if m := reTxnID.FindSubmatch(item); len(m) == 2 {
			t.TxnID, _ = strconv.ParseInt(string(m[1]), 10, 64)
		}
		if m := reSequence.FindSubmatch(item); len(m) == 2 {
			t.Sequence, _ = strconv.Atoi(string(m[1]))
		}
		if m := reEditSequence.FindSubmatch(item); len(m) == 2 {
			t.EditSequence, _ = strconv.Atoi(string(m[1]))
		}
		if m := reTypeID.FindSubmatch(item); len(m) == 2 {
			t.TxnType, _ = strconv.Atoi(string(m[1]))
		}
		if t.TxnID != 0 {
			out = append(out, t)
		}
	}
	return out, nil
}

// reclassifyAccountID resolves a numeric account id or an exact account name
// to its v3 account Id.
func reclassifyAccountID(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty account reference")
	}
	if _, err := strconv.ParseInt(ref, 10, 64); err == nil {
		return ref, nil
	}
	res, err := ReplayQuery(ctx, "Account", "", ref, 10)
	if err != nil {
		return "", err
	}
	for _, it := range res.Items {
		if strings.EqualFold(it.Name, ref) {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("no account named %q", ref)
}

func reclassifyOverridesFor(ac *apiClient) map[string]string {
	o := map[string]string{}
	maps.Copy(o, reclassifyOverrides)
	if ac.realm != "" {
		o["intuit-company-id"] = ac.realm
	}
	return o
}

// PlannedReclassifyURL reports the mutation URL for dry-run plans.
func PlannedReclassifyURL() string {
	return "https://qbo.intuit.com/api/datarequest/" + reclassifyRouteID + "/reclassifyTxn/reclassify"
}
