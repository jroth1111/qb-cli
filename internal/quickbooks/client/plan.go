package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// realmToken is the placeholder used in planned URLs so a dry-run never
// embeds a live company id.
const realmToken = "{realm}"

const (
	neoCompanyTmpl   = "https://qbo.intuit.com/api/neo/v1/company/" + realmToken
	atsBankingTmpl   = "https://qbo.intuit.com/ats/v1/company/" + realmToken + "/banking"
	excludePath      = "/olb/ng/excludeTransactions"
	undoPath         = "/olb/ng/undoTransactions"
	acceptPath       = "/olb/ng/acceptTransactions"
	batchAcceptPath  = "/olb/ng/batchAcceptTransactions"
	importPath       = "/olb/processCsvFile"
	trulesPath       = "/lists/olbrules/getRules"
	rulesPath        = "/lists/olbrules/getRules"
	auditPath        = "/audit/getInitialData"
	registerPath     = "/register/transactions/"
	accountsPath     = "/getInitialData"
	transactionsPath = "/getTransactions"
)

// RequestPlan is a secret-free description of an HTTP call that was not sent.
// Dial is always false. URL uses {realm}, never a live company id.
type RequestPlan struct {
	DryRun bool            `json:"dry_run"`
	Dial   bool            `json:"dial"`
	Method string          `json:"method"`
	URL    string          `json:"url"`
	Body   json.RawMessage `json:"body,omitempty"`
	Note   string          `json:"note,omitempty"`
	op     string
}

func newPOSTPlan(path string, body []byte, op, note string) *RequestPlan {
	return &RequestPlan{
		DryRun: true,
		Dial:   false,
		Method: http.MethodPost,
		URL:    neoCompanyTmpl + path,
		Body:   json.RawMessage(body),
		Note:   note,
		op:     op,
	}
}

// PlanExclude builds the excludeTransactions POST without opening a socket.
// Validation matches ReplayExclude: empty olbTxnIds are rejected.
func PlanExclude(accountID string, olbTxnIDs []string) (*RequestPlan, error) {
	ids := normalizeExcludeIDs(olbTxnIDs)
	if len(ids) == 0 {
		return nil, ErrEmptyExcludeIDs
	}
	if accountID == "" {
		accountID = DefaultAccountID
	}
	var req excludeRequest
	req.NextTxnInfo.AccountID = accountID
	req.NextTxnInfo.NextTransactionIndex = -1
	req.NextTxnInfo.ReviewState = "PENDING"
	req.NextTxnInfo.Sort = "-txnDate"
	req.TxnIDList.ExternalTxnIDs = []string{}
	req.TxnIDList.OlbTxnIDs = ids
	req.TxnIDList.TxnIDPairs = []any{}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding exclude request: %w", err)
	}
	return newPOSTPlan(excludePath, body, "excludeTransactions", "captured POST; not sent"), nil
}

// planUndoState builds the undoTransactions POST for one review state
// (EXCLUDED restores excluded rows; ACCEPTED unposts accepted rows).
func planUndoState(accountID string, olbTxnIDs []string, reviewState string) (*RequestPlan, error) {
	ids := normalizeExcludeIDs(olbTxnIDs)
	if len(ids) == 0 {
		return nil, ErrEmptyUndoIDs
	}
	if accountID == "" {
		accountID = DefaultAccountID
	}
	var req undoRequest
	req.NextTxnInfo.AccountID = accountID
	req.NextTxnInfo.NextTransactionIndex = -1
	req.NextTxnInfo.ReviewState = reviewState
	req.NextTxnInfo.Sort = "-txnDate"
	req.TxnIDList.ExternalTxnIDs = []string{}
	req.TxnIDList.OlbTxnIDs = ids
	req.TxnIDList.TxnIDPairs = []any{}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding undo request: %w", err)
	}
	return newPOSTPlan(undoPath, body, "undoTransactions", "captured POST; not sent"), nil
}

// PlanUndo builds the undoTransactions POST without opening a socket.
func PlanUndo(accountID string, olbTxnIDs []string) (*RequestPlan, error) {
	return planUndoState(accountID, olbTxnIDs, "EXCLUDED")
}

// PlanUnpost builds the undoTransactions POST with reviewState ACCEPTED —
// the Posted tab's Undo contract — without opening a socket.
func PlanUnpost(accountID string, olbTxnIDs []string) (*RequestPlan, error) {
	return planUndoState(accountID, olbTxnIDs, "ACCEPTED")
}

// PlanCategorise builds the categorise POST without opening a socket. The
// live contract folds categorise into batchAcceptTransactions?acceptOnly=true
// with the category on addAsQboTxn.details[0].categoryId; the plan body shows
// stub olbTxns entries — the live feed rows hydrate at send time. Validation
// matches ReplayCategorise: empty olbTxnIds and an empty categoryRef are rejected.
func PlanCategorise(accountID string, olbTxnIDs []string, detail TransactionDetail) (*RequestPlan, error) {
	ids := normalizeExcludeIDs(olbTxnIDs)
	if len(ids) == 0 {
		return nil, ErrEmptyCategoriseIDs
	}
	if detail.CategoryRef == nil || strings.TrimSpace(detail.CategoryRef.Value) == "" {
		return nil, ErrEmptyCategory
	}
	if accountID == "" {
		accountID = DefaultAccountID
	}
	olbTxns := make([]any, 0, len(ids))
	for _, id := range ids {
		line := map[string]any{"categoryId": detail.CategoryRef.Value}
		add := map[string]any{"details": []any{line}}
		applyCategoriseAnnotations(add, line, detail)
		olbTxns = append(olbTxns, map[string]any{
			"olbTxnId":    id,
			"acceptType":  "ADD",
			"addAsQboTxn": add,
		})
	}
	body, err := buildAcceptBody(accountID, olbTxns)
	if err != nil {
		return nil, fmt.Errorf("encoding categorise request: %w", err)
	}
	return newPOSTPlan(batchAcceptPath+batchAcceptQuery, body, "batchAcceptTransactions", "captured POST /olb/ng/batchAcceptTransactions?acceptOnly=true; not sent"), nil
}

// cleanMatchTxns drops empty match ids without mutating the caller's slice.
// The txn type no longer defaults: the register's recorded txnTypeId is the
// only value the acceptTransactions contract accepts.
func cleanMatchTxns(in []MatchTxn) []MatchTxn {
	clean := make([]MatchTxn, 0, len(in))
	for _, m := range in {
		if strings.TrimSpace(m.TxnID) == "" {
			continue
		}
		clean = append(clean, m)
	}
	return clean
}

// PlanMatch builds the acceptTransactions POST without opening a socket.
// Validation matches ReplayMatch: empty olbTxnIds and an empty matchTxns
// list are rejected. The plan body shows stub entries — feed-row fields and
// register-resolved matchedTxns fields hydrate at send time.
func PlanMatch(accountID string, olbTxnIDs []string, matchTxns []MatchTxn) (*RequestPlan, error) {
	ids := normalizeExcludeIDs(olbTxnIDs)
	if len(ids) == 0 {
		return nil, ErrEmptyMatchIDs
	}
	clean := cleanMatchTxns(matchTxns)
	if len(clean) == 0 {
		return nil, ErrEmptyMatchTxns
	}
	matched := make([]any, 0, len(clean))
	for _, m := range clean {
		matched = append(matched, map[string]any{"qboTxnId": m.TxnID})
	}
	olbTxns := make([]any, 0, len(ids))
	for _, id := range ids {
		olbTxns = append(olbTxns, map[string]any{
			"olbTxnId":   id,
			"acceptType": "MATCH",
			"selectedMatches": map[string]any{
				"matchedTxns":  matched,
				"addAdjQboTxn": nil,
				"addAsQboTxn":  nil,
			},
		})
	}
	body, err := json.Marshal(map[string]any{"olbTxns": olbTxns})
	if err != nil {
		return nil, fmt.Errorf("encoding match request: %w", err)
	}
	return newPOSTPlan(acceptPath, body, "acceptTransactions", "captured POST /olb/ng/acceptTransactions; not sent"), nil
}

// PlanBatchAccept builds the batchAcceptTransactions?acceptOnly=true POST
// without opening a socket. Validation matches ReplayBatchAccept: empty
// olbTxnIds are rejected. The plan body shows stub entries — live feed rows
// hydrate at send time.
func PlanBatchAccept(accountID string, olbTxnIDs []string) (*RequestPlan, error) {
	ids := normalizeExcludeIDs(olbTxnIDs)
	if len(ids) == 0 {
		return nil, ErrEmptyBatchAcceptIDs
	}
	if accountID == "" {
		accountID = DefaultAccountID
	}
	olbTxns := make([]any, 0, len(ids))
	for _, id := range ids {
		olbTxns = append(olbTxns, map[string]any{"olbTxnId": id, "acceptType": "ADD"})
	}
	body, err := buildAcceptBody(accountID, olbTxns)
	if err != nil {
		return nil, fmt.Errorf("encoding batch accept request: %w", err)
	}
	return newPOSTPlan(batchAcceptPath+batchAcceptQuery, body, "batchAcceptTransactions", "captured POST /olb/ng/batchAcceptTransactions?acceptOnly=true; not sent"), nil
}

// PlanSplit builds the split POST without opening a socket. The live contract
// folds split into batchAcceptTransactions?acceptOnly=true with the lines on
// addAsQboTxn.details; the plan body shows stub olbTxns entries — the live
// feed rows hydrate at send time. Validation matches ReplaySplit: empty
// olbTxnIds and fewer than two non-empty lines are rejected. entityType is
// vestigial: the feed row carries the created txn type.
func PlanSplit(accountID string, olbTxnIDs []string, entityType string, lines []SplitLine) (*RequestPlan, error) {
	ids := normalizeExcludeIDs(olbTxnIDs)
	if len(ids) == 0 {
		return nil, ErrEmptySplitIDs
	}
	clean := make([]SplitLine, 0, len(lines))
	for _, l := range lines {
		if strings.TrimSpace(l.CategoryRef.Value) == "" {
			continue
		}
		clean = append(clean, l)
	}
	if len(clean) < 2 {
		return nil, ErrEmptySplitLines
	}
	if accountID == "" {
		accountID = DefaultAccountID
	}
	details := splitDetailList(clean)
	olbTxns := make([]any, 0, len(ids))
	for _, id := range ids {
		olbTxns = append(olbTxns, map[string]any{
			"olbTxnId":    id,
			"acceptType":  "ADD",
			"addAsQboTxn": map[string]any{"details": details},
		})
	}
	body, err := buildAcceptBody(accountID, olbTxns)
	if err != nil {
		return nil, fmt.Errorf("encoding split request: %w", err)
	}
	return newPOSTPlan(batchAcceptPath+batchAcceptQuery, body, "batchAcceptTransactions", "captured POST /olb/ng/batchAcceptTransactions?acceptOnly=true; not sent"), nil
}

// PlanTransfer builds the batchAcceptTransactions?acceptOnly=true POST for
// a transfer accept without opening a socket. The live contract posts each
// pending row with acceptType TRANSFER, transfer:true, and
// addAsQboTxn{txnTypeId:"26", details:[{categoryId:toAccountID}]}; the plan
// body shows stub olbTxns entries — live feed rows hydrate at send time.
// Validation matches ReplayTransfer: empty ids or destination account are
// rejected.
func PlanTransfer(accountID string, olbTxnIDs []string, toAccountID string) (*RequestPlan, error) {
	ids := normalizeExcludeIDs(olbTxnIDs)
	if len(ids) == 0 {
		return nil, ErrEmptyTransferIDs
	}
	if strings.TrimSpace(toAccountID) == "" {
		return nil, ErrEmptyTransferAccount
	}
	if accountID == "" {
		accountID = DefaultAccountID
	}
	olbTxns := make([]any, 0, len(ids))
	for _, id := range ids {
		olbTxns = append(olbTxns, map[string]any{
			"olbTxnId":   id,
			"acceptType": "TRANSFER",
			"transfer":   true,
			"addAsQboTxn": map[string]any{
				"txnTypeId": "26",
				"details": []any{map[string]any{
					"categoryId":      toAccountID,
					"billable":        false,
					"taxApplicableOn": "SALES",
				}},
			},
		})
	}
	body, err := buildAcceptBody(accountID, olbTxns)
	if err != nil {
		return nil, fmt.Errorf("encoding transfer request: %w", err)
	}
	return newPOSTPlan(batchAcceptPath+batchAcceptQuery, body, "batchAcceptTransactions", "captured POST /olb/ng/batchAcceptTransactions?acceptOnly=true (TRANSFER); not sent"), nil
}

// PlanImportCSV builds the processCsvFile POST without opening a socket.
// Live OAuth accounts 204/93 are refused. Date defaults to today AU.
func PlanImportCSV(accountID, date, description, amount string) (*RequestPlan, ImportResult, error) {
	accountID = strings.TrimSpace(accountID)
	description = strings.TrimSpace(description)
	amount = strings.TrimSpace(amount)
	date = strings.TrimSpace(date)
	var meta ImportResult
	if accountID == "" {
		return nil, meta, ErrEmptyImportAccount
	}
	if isLiveBankAccount(accountID) {
		return nil, meta, ErrImportBlockedAccount
	}
	if description == "" {
		return nil, meta, ErrEmptyImportDescription
	}
	if _, err := parseImportAmount(amount); err != nil {
		return nil, meta, ErrEmptyImportAmount
	}
	if date == "" {
		date = defaultImportDate()
	}
	var req csvImportRequest
	req.Data = []struct {
		Date          string `json:"date"`
		Description   string `json:"description"`
		Amount        string `json:"amount"`
		MarkForImport bool   `json:"markForImport"`
	}{{
		Date:          date,
		Description:   description,
		Amount:        amount,
		MarkForImport: true,
	}}
	req.DateFormat = "dd/MM/yyyy"
	req.AccountID = accountID
	req.ColumnMappings = map[string]string{
		"amount":      "2",
		"date":        "0",
		"description": "1",
		"checkNum":    "",
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, meta, fmt.Errorf("encoding processCsvFile request: %w", err)
	}
	meta = ImportResult{
		AccountID:   accountID,
		Imported:    1,
		Date:        date,
		Description: description,
		Amount:      amount,
	}
	return newPOSTPlan(importPath, body, "processCsvFile", "captured POST; not sent"), meta, nil
}

func parseImportAmount(amount string) (float64, error) {
	return strconv.ParseFloat(amount, 64)
}

// PlannedAccountsURL is the captured getInitialData GET.
func PlannedAccountsURL() string {
	return atsBankingTmpl + accountsPath
}

// PlannedFeedURL is the captured getTransactions GET.
func PlannedFeedURL(accountID, reviewState string) string {
	if accountID == "" {
		accountID = DefaultAccountID
	}
	if reviewState == "" {
		reviewState = "PENDING"
	}
	return neoCompanyTmpl + "/olb/ng" + transactionsPath + "?sort=-txnDate&reviewState=" + strings.ToUpper(reviewState) + "&ignoreMatching=false&accountId=" + accountID
}

// PlannedRegisterURL is the captured register transactions GET.
func PlannedRegisterURL(accountID string) string {
	if accountID == "" {
		accountID = DefaultAccountID
	}
	return neoCompanyTmpl + registerPath + "?accountId=" + accountID
}

// PlannedRulesURL is the captured bank-rules GET.
func PlannedRulesURL() string {
	return neoCompanyTmpl + rulesPath
}

// PlannedAuditURL is the captured Audit Log events POST.
func PlannedAuditURL() string {
	return auditLogsURL
}

// postPlanned substitutes the live realm into plan.URL and POSTs. Used only
// by Replay* mutation functions — never by dry-run.
func postPlanned(ctx context.Context, plan *RequestPlan) (int, error) {
	if plan == nil {
		return 0, fmt.Errorf("nil request plan")
	}
	ac, err := newAPIClient()
	if err != nil {
		return 0, err
	}
	if plan.op == "excludeTransactions" || plan.op == "undoTransactions" {
		return replayVerifiedStateChange(ctx, ac, plan)
	}
	return postResolved(ctx, ac, plan, []byte(plan.Body))
}

// postResolved POSTs an explicitly resolved body to plan.URL on an existing
// authenticated client. Replay functions that hydrate the request from live
// reads build their body after loading the session and share the same
// apiClient for the POST.
func postResolved(ctx context.Context, ac *apiClient, plan *RequestPlan, body []byte) (int, error) {
	if plan == nil {
		return 0, fmt.Errorf("nil request plan")
	}
	url := strings.ReplaceAll(plan.URL, realmToken, ac.realm)
	submittingMutation(ctx)
	resp, err := ac.post(ctx, url, body)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", plan.op, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("reading %s: %w", plan.op, err)
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	if plan.op == "batchAcceptTransactions" {
		if err := validateAcceptResponse(raw, body); err != nil {
			return resp.StatusCode, err
		}
		if err := verifyAcceptReadback(ctx, ac, body, raw); err != nil {
			return resp.StatusCode, err
		}
	}
	if plan.op == "processCsvFile" {
		var request struct {
			Data []json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(body, &request); err != nil || len(request.Data) == 0 {
			return resp.StatusCode, fmt.Errorf("CSV import response cannot be checked against request")
		}
		if err := validateCSVImportResponse(raw, len(request.Data)); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

// PlannedReconcileURL advertises the Integration_Reconciliation node read on
// the v4 gateway for a bank account.
func PlannedReconcileURL(accountID string) string {
	return "https://qbo.intuit.com/api/v4/graphql (Integration_Reconciliation node for account " + accountID + ")"
}
