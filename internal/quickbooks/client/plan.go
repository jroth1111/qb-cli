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
	categorisePath   = "/olb/ng/categoriseTransactions"
	tsplitPath       = "/olb/ng/splitTransactions"
	matchPath        = "/olb/ng/matchTransactions"
	splitPath        = "/olb/ng/splitTransactions"
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
	req.NextTxnInfo.NextTransactionIndex = 0
	req.NextTxnInfo.ReviewState = "PENDING"
	req.NextTxnInfo.Sort = "-txnDate"
	req.TxnIDList.ExternalTxnIDs = []string{}
	req.TxnIDList.OlbTxnIDs = ids
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding exclude request: %w", err)
	}
	return newPOSTPlan(excludePath, body, "excludeTransactions", "captured POST; not sent"), nil
}

// PlanUndo builds the undoTransactions POST without opening a socket.
func PlanUndo(accountID string, olbTxnIDs []string) (*RequestPlan, error) {
	ids := normalizeExcludeIDs(olbTxnIDs)
	if len(ids) == 0 {
		return nil, ErrEmptyUndoIDs
	}
	if accountID == "" {
		accountID = DefaultAccountID
	}
	var req undoRequest
	req.NextTxnInfo.AccountID = accountID
	req.NextTxnInfo.NextTransactionIndex = 0
	req.NextTxnInfo.ReviewState = "EXCLUDED"
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

// PlanCategorise builds the categoriseTransactions POST without opening a
// socket. Validation matches ReplayCategorise: empty olbTxnIds and an empty
// categoryRef are rejected. entityType defaults to Expense.
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
	if detail.EntityType == "" {
		detail.EntityType = "Expense"
	}
	var req categoriseRequest
	req.NextTxnInfo.AccountID = accountID
	req.NextTxnInfo.NextTransactionIndex = 0
	req.NextTxnInfo.ReviewState = "PENDING"
	req.NextTxnInfo.Sort = "-txnDate"
	req.TxnIDList.ExternalTxnIDs = []string{}
	req.TxnIDList.OlbTxnIDs = ids
	req.TransactionDetail = detail
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding categorise request: %w", err)
	}
	return newPOSTPlan(categorisePath, body, "categoriseTransactions", "captured POST; not sent"), nil
}

// PlanMatch builds the matchTransactions POST without opening a socket.
// Validation matches ReplayMatch: empty olbTxnIds and an empty matchTxns
// list are rejected.
func PlanMatch(accountID string, olbTxnIDs []string, matchTxns []MatchTxn) (*RequestPlan, error) {
	ids := normalizeExcludeIDs(olbTxnIDs)
	if len(ids) == 0 {
		return nil, ErrEmptyMatchIDs
	}
	clean := make([]MatchTxn, 0, len(matchTxns))
	for _, m := range matchTxns {
		if strings.TrimSpace(m.TxnID) == "" {
			continue
		}
		if m.TxnType == "" {
			m.TxnType = "Bill"
		}
		clean = append(clean, m)
	}
	if len(clean) == 0 {
		return nil, ErrEmptyMatchTxns
	}
	if accountID == "" {
		accountID = DefaultAccountID
	}
	var req matchRequest
	req.NextTxnInfo.AccountID = accountID
	req.NextTxnInfo.NextTransactionIndex = 0
	req.NextTxnInfo.ReviewState = "PENDING"
	req.NextTxnInfo.Sort = "-txnDate"
	req.TxnIDList.ExternalTxnIDs = []string{}
	req.TxnIDList.OlbTxnIDs = ids
	req.MatchTxns = clean
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding match request: %w", err)
	}
	return newPOSTPlan(matchPath, body, "matchTransactions", "captured POST; not sent"), nil
}

// PlanBatchAccept builds the batchAcceptTransactions POST without opening a
// socket. Validation matches ReplayBatchAccept: empty olbTxnIds are rejected.
func PlanBatchAccept(accountID string, olbTxnIDs []string) (*RequestPlan, error) {
	ids := normalizeExcludeIDs(olbTxnIDs)
	if len(ids) == 0 {
		return nil, ErrEmptyBatchAcceptIDs
	}
	if accountID == "" {
		accountID = DefaultAccountID
	}
	var req batchAcceptRequest
	req.NextTxnInfo.AccountID = accountID
	req.NextTxnInfo.NextTransactionIndex = 0
	req.NextTxnInfo.ReviewState = "PENDING"
	req.NextTxnInfo.Sort = "-txnDate"
	req.TxnIDList.ExternalTxnIDs = []string{}
	req.TxnIDList.OlbTxnIDs = ids
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding batch accept request: %w", err)
	}
	return newPOSTPlan(batchAcceptPath, body, "batchAcceptTransactions", "captured POST; not sent"), nil
}

// PlanSplit builds the splitTransactions POST without opening a socket.
// Validation matches ReplaySplit: empty olbTxnIds and fewer than two lines
// are rejected; every line needs a non-empty category.
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
	if entityType == "" {
		entityType = "Expense"
	}
	var req splitRequest
	req.NextTxnInfo.AccountID = accountID
	req.NextTxnInfo.NextTransactionIndex = 0
	req.NextTxnInfo.ReviewState = "PENDING"
	req.NextTxnInfo.Sort = "-txnDate"
	req.TxnIDList.ExternalTxnIDs = []string{}
	req.TxnIDList.OlbTxnIDs = ids
	req.TransactionDetail.EntityType = entityType
	req.TransactionDetail.Lines = clean
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("encoding split request: %w", err)
	}
	return newPOSTPlan(splitPath, body, "splitTransactions", "captured POST; not sent"), nil
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
	return atsBankingTmpl + transactionsPath + "?accountId=" + accountID + "&reviewState=" + strings.ToUpper(reviewState)
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
	url := strings.ReplaceAll(plan.URL, realmToken, ac.realm)
	resp, err := ac.post(ctx, url, []byte(plan.Body))
	if err != nil {
		return 0, fmt.Errorf("%s: %w", plan.op, err)
	}
	defer drainAndClose(resp)
	raw, err := readBody(resp)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("reading %s: %w", plan.op, err)
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return resp.StatusCode, nil
}
