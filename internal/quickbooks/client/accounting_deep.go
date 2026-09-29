package client

// accounting_deep.go implements the accounting-domain deep CRUD surfaces
// discovered in the JS capture corpus (/tmp/qbo-cap, 2026-08):
//
//   budgeting.api.intuit.com/graphql — budget lists: fetchAllBudgets,
//       captured from the current budgeting UI (2026-09-12). Budget-by-ID
//       reads use the native v3 query path. Writes stay on the proven
//       businessPlanningCreateBudget / v3 /budget paths.
//
//   coa-core.api.intuit.com/graphql    (plugin.customization-ui chunk 6882,
//       uxfabric.accounting-ledger-ui chunk 5898) — dimension reads:
//       GetKlasses and GetDepartments (dataAccess* projections). The
//       customization-ui bundle carries this host literal next to those
//       operations, so the reads route here rather than the v4 gateway.
//
//   sbseggraphqlorch.api.intuit.com/graphql — deferred-recognition/prepaid
//       schedule reads and the captured schedule write: GET_SCHEDULE,
//       GET_SCHEDULE_PREVIEW, UpdateDeferredRecognitionScheduleOp. The
//       deferral-acctng-ui's own backend (deferredrecognition.api.intuit.com)
//       never fires in the TC2 UI; the orchestrator serves the same
//       Accounting_DeferredRecognition* schema family live.
//
//   v3 /journalentry and /batch — journal CRUD plus true bulk create/delete
//       through BatchItemRequest, and the opening-balance trial-balance
//       journal.
//
// Transport: every new GraphQL host is posted through apiClient.postJSONExtra
// (full captured ATS header set from http.go, cookies, csrf, realm pin, 401
// remint). On a 403 only, a single retry is issued through the
// accountingURIFallback seam (default: the leftover-host uri-host transport),
// mirroring the stdlib→surf→uri-host ladder philosophy of http.go. The
// budget reads reuse the dedicated budgeting-ui surf transport from
// budget_graphql.go behind a seam var so tests stay offline.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Endpoints (capture-corpus evidence in the file comment above).
// ---------------------------------------------------------------------------

const (
	coaCoreGraphQLURL     = "https://coa-core.api.intuit.com/graphql"
	deferredRecogGraphQL  = "https://sbseggraphqlorch.api.intuit.com/graphql"
	v3CompanyBatchPathFmt = "https://qbo.intuit.com/api/v3/company/%s/batch?minorversion=73"

	budgetReferer     = "https://qbo.intuit.com/app/budgets"
	dimensionsReferer = "https://qbo.intuit.com/app/classes"
	deferralUIReferer = "https://qbo.intuit.com/app/revenue-recognition/templates"
	openingBalReferer = "https://qbo.intuit.com/app/chartofaccounts"
)

// ---------------------------------------------------------------------------
// Sentinels. Like ErrEmptyExcludeIDs these fire before any dial so callers
// cannot mistake a rejected no-op for success.
// ---------------------------------------------------------------------------

var (
	ErrEmptyBudgetID      = errors.New("budget get requires a non-empty id")
	ErrEmptyDimensionID   = errors.New("dimension delete requires a non-empty id")
	ErrEmptyJournalIDs    = errors.New("journal bulk delete requires at least one journal entry id")
	ErrEmptyJournalItems  = errors.New("journal bulk create requires at least one item")
	ErrEmptyScheduleInput = errors.New("deferred schedule write requires --data JSON input (schema not fully captured; input is caller-supplied)")
	ErrUnbalancedLines    = errors.New("opening balance lines are unbalanced: total debits must equal total credits")
)

// accountingURIFallback is the 403 fallback for the new *.api.intuit.com
// GraphQL hosts. Default: doURIHost with the URL's own host (requires that
// host's captured Intuit_APIKey; errors with guidance when absent). Tests
// replace it.
var accountingURIFallback = func(ac *apiClient, ctx context.Context, method, rawURL string, body []byte) (*http.Response, error) {
	return ac.doURIHost(ctx, method, rawURL, hostOf(rawURL), body)
}

func hostOf(rawURL string) string {
	u := rawURL
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if i := strings.IndexAny(u, "/?"); i >= 0 {
		u = u[:i]
	}
	return u
}

// postAccountingGraphQL posts payload to rawURL with the full ATS header
// set. On 403 it retries once through accountingURIFallback. The response
// body is the caller's to close.
func (c *apiClient) postAccountingGraphQL(ctx context.Context, rawURL, referer string, payload []byte) (*http.Response, error) {
	resp, err := c.postJSONExtra(ctx, rawURL, payload, map[string]string{
		"Referer": referer,
		"Origin":  "https://qbo.intuit.com",
	})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusForbidden {
		return resp, nil
	}
	_ = drainAndClose(resp)
	return accountingURIFallback(c, ctx, http.MethodPost, rawURL, payload)
}

// decodeGQL unwraps a GraphQL envelope into data and the first error
// message ("" when none).
func decodeGQL(body []byte) (json.RawMessage, string, error) {
	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, "", fmt.Errorf("decoding graphql envelope: %w", err)
	}
	msg := ""
	if len(env.Errors) > 0 {
		msg = env.Errors[0].Message
	}
	return env.Data, msg, nil
}

// ---------------------------------------------------------------------------
// Budget: current budgeting UI list query + native v3 get.
// ---------------------------------------------------------------------------

// budgetListQuery is the current budgeting UI document captured 2026-09-12.
// The older DataAccess_BudgetFilter is not in this host's schema.
const budgetListQuery = `query fetchAllBudgets($first: Int, $after: String, $last: Int, $before: String, $sortBy: BusinessPlanning_SortByColumn, $sortDirection: BusinessPlanning_SortDirection, $budgetFilters: BusinessPlanning_BudgetFilters) {
  businessPlanningBudgets(first: $first, after: $after, last: $last, before: $before, sortBy: $sortBy, sortDirection: $sortDirection, budgetFilters: $budgetFilters) {
    pageInfo {
      hasPreviousPage
      hasNextPage
      startCursor
      endCursor
      __typename
    }
    edges {
      cursor
      node {
        budgetId
        budgetName
        budgetType
        startDate
        endDate
        linkedEntityId
        intervalType
        syncToken
        secondaryListType
        dimensionDefId
        budgetMetaData {
          createdBy
          createdAt
          lastUpdatedBy
          updatedAt
          __typename
        }
        viewSettings {
          archived
          __typename
        }
        approvalDetails {
          approvalStatus
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
}
`

// PlannedBudgetListURL is the budgeting.api graphql endpoint for lists.
func PlannedBudgetListURL() string { return budgetGraphQLURL }

// PlannedBatchURL is the v3 batch endpoint template.
func PlannedBatchURL() string {
	return "https://qbo.intuit.com/api/v3/company/{realm}/batch?minorversion=73"
}

// PlannedDeferredRecognitionURL is the deferredrecognition graphql endpoint.
func PlannedDeferredRecognitionURL() string { return deferredRecogGraphQL }

// postBudgetGraphQLFn is the transport seam for budgeting.api graphql.
// Production default: the proven budgeting-ui surf transport.
var postBudgetGraphQLFn = func(ac *apiClient, ctx context.Context, body []byte) (*http.Response, error) {
	return ac.postBudgetGraphQL(ctx, body)
}

type budgetListNode struct {
	ID           json.RawMessage `json:"budgetId"`
	Name         string          `json:"budgetName"`
	Type         string          `json:"budgetType"`
	StartDate    string          `json:"startDate"`
	ViewSettings *struct {
		Archived *bool `json:"archived"`
	} `json:"viewSettings"`
}

func budgetNodeToItem(node *budgetListNode) (QueryItem, error) {
	if node == nil {
		return QueryItem{}, errors.New("missing budget node")
	}
	var id string
	if err := json.Unmarshal(node.ID, &id); err != nil {
		// Accept numeric IDs without rounding through float64.
		var number json.Number
		if err := json.Unmarshal(node.ID, &number); err != nil {
			return QueryItem{}, errors.New("invalid budgetId")
		}
		id = number.String()
	}
	if strings.TrimSpace(id) == "" {
		return QueryItem{}, errors.New("missing budgetId")
	}
	item := QueryItem{ID: id, Name: node.Name, Type: node.Type, Date: node.StartDate}
	if node.ViewSettings != nil && node.ViewSettings.Archived != nil {
		active := !*node.ViewSettings.Archived
		item.Active = &active
	}
	return item, nil
}

// ReplayBudgetList lists budgets using the current UI's active-budget filter.
func ReplayBudgetList(ctx context.Context, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 99999 // unbounded ask; service applies its own ceiling
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "fetchAllBudgets",
		"variables": map[string]any{
			"first": limit, "last": nil, "after": nil, "before": nil,
			"sortBy": "LAST_MODIFIED", "sortDirection": "DESC",
			"budgetFilters": map[string]any{
				"archived":   false,
				"budgetType": []string{"PROFIT_AND_LOSS", "BALANCE_SHEET"},
			},
		},
		"query": budgetListQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("budget list: %w", err)
	}
	resp, err := postBudgetGraphQLFn(ac, ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("budget list: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading budget list: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var envelope struct {
		Data *struct {
			Budgets *struct {
				Edges *[]struct {
					Node *budgetListNode `json:"node"`
				} `json:"edges"`
			} `json:"businessPlanningBudgets"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("budget list: invalid graphql response: %w", err)
	}
	if len(envelope.Errors) > 0 {
		var first struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(envelope.Errors[0], &first) != nil || strings.TrimSpace(first.Message) == "" {
			first.Message = "response contains GraphQL errors"
		}
		return nil, fmt.Errorf("budget list graphql: %s", first.Message)
	}
	if envelope.Data == nil || envelope.Data.Budgets == nil || envelope.Data.Budgets.Edges == nil {
		return nil, errors.New("budget list: missing businessPlanningBudgets.edges collection")
	}
	items := make([]QueryItem, 0, len(*envelope.Data.Budgets.Edges))
	for i, edge := range *envelope.Data.Budgets.Edges {
		item, err := budgetNodeToItem(edge.Node)
		if err != nil {
			return nil, fmt.Errorf("budget list edge %d: %w", i, err)
		}
		items = append(items, item)
	}
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return &QueryResult{
		Status: resp.StatusCode,
		Entity: "Budget",
		Counts: map[string]int{"items": len(items)},
		Items:  items,
		Note:   "budgeting.api fetchAllBudgets",
	}, nil
}

// ReplayBudgetGet uses the proven native v3 Budget query by ID.
func ReplayBudgetGet(ctx context.Context, id string) (*QueryResult, error) {
	id = sanitizeToken(id)
	if id == "" {
		return nil, ErrEmptyBudgetID
	}
	return ReplayQuery(ctx, "Budget", id, "", 20)
}

// ---------------------------------------------------------------------------
// Class / Location: coa-core.api.intuit.com/graphql dimension reads.
// ---------------------------------------------------------------------------

// klassListQuery is the captured GetKlasses document with the captured
// klassAttributes fields inlined (valid standalone document).
const klassListQuery = `query GetKlasses(
    $first: PositiveInt,
    $offset: Int
  ) {
    dataAccessKlasses(
      first: $first
      offset: $offset
    ) {
      edges {
        node {
          id
          name
          fullName
          active
          sourceEntityVersion
          level
        }
      }
      totalCount
    }
  }`

// departmentListQuery is the captured GetDepartments document with the
// captured departmentAttributes fields inlined.
const departmentListQuery = `query GetDepartments(
    $first: PositiveInt,
    $offset: Int
  ) {
    dataAccessDepartments(
      first: $first
      offset: $offset
    ) {
      edges {
        node {
          id
          name
          fullName
          active
          sourceEntityVersion
          level
        }
      }
      totalCount
    }
  }`

// ReplayDimensionList lists classes (dimension "klass") or locations
// (dimension "department") via the captured coa-core dataAccess queries.
func ReplayDimensionList(ctx context.Context, dimension string, limit int) (*QueryResult, error) {
	dimension = strings.ToLower(strings.TrimSpace(dimension))
	if dimension == "location" {
		dimension = "department"
	}
	if dimension != "class" && dimension != "department" {
		return nil, fmt.Errorf("unknown dimension %q (want class or location)", dimension)
	}
	if limit < 1 {
		limit = 99999 // unbounded ask; service applies its own ceiling
	}
	query := klassListQuery
	op := "GetKlasses"
	entity := "Class"
	if dimension == "department" {
		query = departmentListQuery
		op = "GetDepartments"
		entity = "Location"
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": op,
		"variables":     map[string]any{"first": limit, "offset": 0},
		"query":         query,
	})
	if err != nil {
		return nil, fmt.Errorf("%s list: %w", strings.ToLower(entity), err)
	}
	resp, err := ac.postAccountingGraphQL(ctx, coaCoreGraphQLURL, dimensionsReferer, payload)
	if err != nil {
		return nil, fmt.Errorf("%s list: %w", strings.ToLower(entity), err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading %s list: %w", strings.ToLower(entity), err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	data, gmsg, err := decodeGQL(raw)
	if err != nil {
		return nil, fmt.Errorf("%s list: %w", strings.ToLower(entity), err)
	}
	var conn struct {
		Edges []struct {
			Node struct {
				ID       string `json:"id"`
				Name     string `json:"name"`
				FullName string `json:"fullName"`
				Active   bool   `json:"active"`
				Level    int    `json:"level"`
			} `json:"node"`
		} `json:"edges"`
		TotalCount int `json:"totalCount"`
	}
	rootField := "dataAccessKlasses"
	if dimension == "department" {
		rootField = "dataAccessDepartments"
	}
	var top map[string]json.RawMessage
	_ = json.Unmarshal(data, &top)
	if rawConn, ok := top[rootField]; ok {
		_ = json.Unmarshal(rawConn, &conn)
	}
	items := make([]QueryItem, 0, len(conn.Edges))
	for _, e := range conn.Edges {
		active := e.Node.Active
		items = append(items, QueryItem{
			ID:     e.Node.ID,
			Name:   e.Node.FullName,
			Type:   entity,
			Active: &active,
		})
	}
	note := fmt.Sprintf("coa-core.api %s totalCount=%d", op, conn.TotalCount)
	if gmsg != "" {
		note += "; gql: " + gmsg
	}
	return &QueryResult{
		Status: resp.StatusCode,
		Entity: entity,
		Counts: map[string]int{"items": len(items), "totalCount": conn.TotalCount},
		Items:  items,
		Note:   note,
	}, nil
}

// ---------------------------------------------------------------------------
// Journal bulk: v3 /batch BatchItemRequest.
// ---------------------------------------------------------------------------

// JournalBulkItem is one balanced two-line journal entry in a bulk create.
type JournalBulkItem struct {
	Date        string  `json:"date,omitempty"`
	Amount      float64 `json:"amount"`
	FromAccount string  `json:"from-account"`
	ToAccount   string  `json:"to-account"`
	Memo        string  `json:"memo,omitempty"`
	Class       string  `json:"class,omitempty"` // QBO class id; applied to both lines
}

// ParseJournalBulkItems decodes --items-json. Every item must carry a
// non-zero amount and two distinct accounts; the two-line construction is
// balanced by definition (debit = credit = amount).
func ParseJournalBulkItems(raw string) ([]JournalBulkItem, error) {
	var items []JournalBulkItem
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("--items-json must decode to a JournalBulkItem array: %w", err)
	}
	if len(items) == 0 {
		return nil, ErrEmptyJournalItems
	}
	for i, it := range items {
		if it.Amount == 0 {
			return nil, fmt.Errorf("item %d: amount must be non-zero", i)
		}
		if it.FromAccount == "" || it.ToAccount == "" {
			return nil, fmt.Errorf("item %d: from-account and to-account are required", i)
		}
		if it.FromAccount == it.ToAccount {
			return nil, fmt.Errorf("item %d: from-account and to-account must differ", i)
		}
	}
	return items, nil
}

func journalBatchEntry(bID, op string, obj map[string]any) map[string]any {
	return map[string]any{"bId": bID, "operation": op, "JournalEntry": obj}
}

// journalLine builds one v3 JournalEntry line. A non-empty class id adds a
// ClassRef so paired entries can carry the same property class on both sides.
func journalLine(posting, acct string, amt float64, class string) map[string]any {
	detail := map[string]any{
		"PostingType": posting,
		"AccountRef":  map[string]any{"value": acct},
	}
	if class != "" {
		detail["ClassRef"] = map[string]any{"value": class}
	}
	return map[string]any{
		"Amount":                 amt,
		"DetailType":             "JournalEntryLineDetail",
		"JournalEntryLineDetail": detail,
	}
}

// ReplayJournalBulkCreate posts N balanced journal entries in one v3 /batch
// request. Refuses prod writes and empty item lists.
func ReplayJournalBulkCreate(ctx context.Context, items []JournalBulkItem) (*QueryResult, error) {
	if len(items) == 0 {
		return nil, ErrEmptyJournalItems
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	batch := make([]map[string]any, 0, len(items))
	for i, it := range items {
		date := it.Date
		if date == "" {
			date = time.Now().Format("2006-01-02")
		} else {
			date = normalizeDate(date)
		}
		obj := map[string]any{
			"TxnDate": date,
			"Line": []map[string]any{
				journalLine("Debit", it.FromAccount, it.Amount, it.Class),
				journalLine("Credit", it.ToAccount, it.Amount, it.Class),
			},
		}
		if it.Memo != "" {
			obj["PrivateNote"] = it.Memo
		}
		batch = append(batch, journalBatchEntry(strconv.Itoa(i), "create", obj))
	}
	return postJournalBatch(ctx, ac, batch)
}

// ReplayJournalBulkDelete deletes N journal entries in one v3 /batch
// request. Each id is fetched first for its SyncToken (v3 delete contract).
func ReplayJournalBulkDelete(ctx context.Context, ids []string) (*QueryResult, error) {
	clean := normalizeExcludeIDs(ids)
	if len(clean) == 0 {
		return nil, ErrEmptyJournalIDs
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	batch := make([]map[string]any, 0, len(clean))
	for i, id := range clean {
		existing, err := fetchV3(ctx, ac, "journalentry", id)
		if err != nil {
			return nil, fmt.Errorf("journal bulk delete item %d (%s): %w", i, id, err)
		}
		obj := map[string]any{
			"Id":        existing["Id"],
			"SyncToken": existing["SyncToken"],
		}
		batch = append(batch, journalBatchEntry(strconv.Itoa(i), "delete", obj))
	}
	return postJournalBatch(ctx, ac, batch)
}

// postJournalBatch POSTs the BatchItemRequest envelope and projects the
// per-item responses (including faults) into a secret-free QueryResult.
func postJournalBatch(ctx context.Context, ac *apiClient, batch []map[string]any) (*QueryResult, error) {
	payload, err := json.Marshal(map[string]any{"BatchItemRequest": batch})
	if err != nil {
		return nil, fmt.Errorf("journal batch: %w", err)
	}
	resp, err := ac.postJSON(ctx, fmt.Sprintf(v3CompanyBatchPathFmt, ac.realm), payload)
	if err != nil {
		return nil, fmt.Errorf("journal batch: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading journal batch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		BatchItemResponse []struct {
			BID          string         `json:"bId"`
			JournalEntry map[string]any `json:"JournalEntry"`
			Fault        *struct {
				Type string `json:"type"`
			} `json:"fault"`
		} `json:"BatchItemResponse"`
	}
	_ = json.Unmarshal(raw, &wrap)
	items := make([]QueryItem, 0, len(wrap.BatchItemResponse))
	faults := 0
	for _, r := range wrap.BatchItemResponse {
		item := QueryItem{ID: r.BID, Type: "JournalEntry"}
		if r.Fault != nil {
			faults++
			item.Name = "fault: " + r.Fault.Type
		} else if je := r.JournalEntry; je != nil {
			item.ID = anyString(je["Id"])
			item.Date = anyString(je["TxnDate"])
		}
		items = append(items, item)
	}
	note := fmt.Sprintf("v3 batch: %d ok, %d faulted", len(items)-faults, faults)
	return &QueryResult{
		Status: resp.StatusCode,
		Entity: "JournalEntry",
		Counts: map[string]int{"items": len(items), "faults": faults},
		Items:  items,
		Note:   note,
	}, nil
}

// ---------------------------------------------------------------------------
// Opening balance: trial-balance journal via v3 /journalentry.
// ---------------------------------------------------------------------------

// TrialBalanceLine is one opening-balance line: an account with either a
// debit or a credit amount (per the trial balance).
type TrialBalanceLine struct {
	Account string  `json:"account"`
	Debit   float64 `json:"debit,omitempty"`
	Credit  float64 `json:"credit,omitempty"`
}

// ParseOpeningBalanceLines decodes --lines JSON and enforces the trial-
// balance invariant: total debits == total credits (> 0) and each line
// carries exactly one side.
func ParseOpeningBalanceLines(raw string) ([]TrialBalanceLine, error) {
	var lines []TrialBalanceLine
	if err := json.Unmarshal([]byte(raw), &lines); err != nil {
		return nil, fmt.Errorf("--lines must decode to a TrialBalanceLine array: %w", err)
	}
	if len(lines) == 0 {
		return nil, errors.New("opening balance requires at least one line")
	}
	var debits, credits float64
	for i, l := range lines {
		if l.Account == "" {
			return nil, fmt.Errorf("line %d: account is required", i)
		}
		if l.Debit != 0 && l.Credit != 0 {
			return nil, fmt.Errorf("line %d: set debit or credit, not both", i)
		}
		debits += l.Debit
		credits += l.Credit
	}
	if debits == 0 && credits == 0 {
		return nil, ErrUnbalancedLines
	}
	if debits != credits {
		return nil, fmt.Errorf("%w (debits %.2f != credits %.2f)", ErrUnbalancedLines, debits, credits)
	}
	return lines, nil
}

// ReplayOpeningBalanceCreate books the trial balance as one v3 journal
// entry dated date (yyyy-MM-dd or dd/MM/yyyy).
func ReplayOpeningBalanceCreate(ctx context.Context, date string, lines []TrialBalanceLine) (*MutateResult, error) {
	if len(lines) == 0 {
		return nil, errors.New("opening balance requires at least one line")
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	} else {
		date = normalizeDate(date)
	}
	out := map[string]any{"TxnDate": date}
	v3lines := make([]map[string]any, 0, len(lines))
	for _, l := range lines {
		if l.Debit != 0 {
			v3lines = append(v3lines, journalLine("Debit", l.Account, l.Debit, ""))
		}
		if l.Credit != 0 {
			v3lines = append(v3lines, journalLine("Credit", l.Account, l.Credit, ""))
		}
	}
	out["Line"] = v3lines
	return postV3Journal(ctx, out)
}

// postV3Journal posts a fully-built JournalEntry create body.
func postV3Journal(ctx context.Context, body map[string]any) (*MutateResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("journal create: %w", err)
	}
	u := fmt.Sprintf("https://qbo.intuit.com/api/v3/company/%s/journalentry?minorversion=73", ac.realm)
	resp, err := ac.postJSON(ctx, u, raw)
	if err != nil {
		return nil, fmt.Errorf("v3 journalentry create: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	got, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(got)}
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "create",
		Entity: "JournalEntry",
		Item:   projectMutateItem("JournalEntry", got),
		Note:   "v3 journalentry",
	}, nil
}

// ---------------------------------------------------------------------------
// Deferred revenue / prepaid schedules on deferredrecognition.api.intuit.com.
// ---------------------------------------------------------------------------

// deferredScheduleGetQuery is the captured GET_SCHEDULE document with the
// captured scheduleDetails fragment appended.
const deferredScheduleGetQuery = `query GET_SCHEDULE($id: ID!) {
    accountingDeferredRecognitionSchedulesBySourceEntity(sourceEntityId: $id) {
      ...scheduleDetails
    }
  }

fragment scheduleDetails on Accounting_DeferredRecognitionSchedule {
    id
    deferralType
    item {
      id
    }
    sourceDetail {
      transaction {
        id
      }
      transactionLine {
        txId
        sequence
      }
      deferredAmount {
        value
      }
      sourceEntityVersion
    }
    serviceStartDate
    recognitionFrequency
    postingStatus
    scheduleDuration {
      durationType
      durationValue
    }
    remainingAmount {
      value
      currency
    }
    scheduleLines {
      name
      sequence
      sourceEntityDetails {
        txId
        sequence
      }
      recognitionStatus
      recognitionDate
      recognitionAmount {
        value
      }
      recognitionHomeAmount {
        value
      }
      recognitionTransaction {
        id
        type
      }
      recognitionPercentage
      errorDetails {
        code
        message
      }
    }
  }`

// deferredScheduleUpdateMutation is the captured
// UpdateDeferredRecognitionScheduleOp document.
const deferredScheduleUpdateMutation = `mutation UpdateDeferredRecognitionScheduleOp($input: Accounting_UpdateDeferredRecognitionScheduleInput!) {
    accountingUpdateDeferredRecognitionSchedule(input: $input) {
      clientMutationId
    }
  }`

// deferredSchedulePreviewQuery is the captured GET_SCHEDULE_PREVIEW document
// composed with the captured scheduleDetails and template fragments (the
// webpack extraction split them into separate literals).
var deferredSchedulePreviewQueryBody = `query GET_SCHEDULE_PREVIEW($previewSchedulesInput: Accounting_DeferredRecognitionPreviewSchedulesInput!) {
    accountingDeferredRecognitionPreviewSchedule(input: $previewSchedulesInput) {
      ...scheduleDetails
      template {
        ...template
      }
    }
  }`

var deferredSchedulePreviewQuery = deferredSchedulePreviewQueryBody + "\n\n" +
	deferredScheduleGetQuery[strings.Index(deferredScheduleGetQuery, "fragment scheduleDetails"):] + `

fragment template on Accounting_DeferredRecognitionTemplate {
    recognitionMethod {
      name
      ... on Accounting_StraightLineEvenPeriod {
        recognitionDuration {
          recognitionFrequency
        }
      }
      ... on Accounting_StraightLineProratedDays {
        recognitionDuration {
          recognitionFrequency
        }
      }
      ... on Accounting_PercentageBasedMethod {
        recognitionDuration {
          recognitionFrequency
        }
      }
    }
  }`

func postDeferredGraphQL(ctx context.Context, payload []byte) (*http.Response, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	return ac.postAccountingGraphQL(ctx, deferredRecogGraphQL, deferralUIReferer, payload)
}

// ReplayDeferredScheduleGet fetches the deferred/prepaid recognition
// schedule attached to a source transaction (captured GET_SCHEDULE).
func ReplayDeferredScheduleGet(ctx context.Context, sourceEntityID string, prepaid bool) (*QueryResult, error) {
	sourceEntityID = sanitizeToken(sourceEntityID)
	if sourceEntityID == "" {
		return nil, errors.New("deferred schedule get requires a non-empty source entity id")
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "GET_SCHEDULE",
		"variables":     map[string]any{"id": sourceEntityID},
		"query":         deferredScheduleGetQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("deferred schedule get: %w", err)
	}
	resp, err := postDeferredGraphQL(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("deferred schedule get: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading deferred schedule get: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	data, gmsg, err := decodeGQL(raw)
	if err != nil {
		return nil, fmt.Errorf("deferred schedule get: %w", err)
	}
	// The field is a list, not an object — unmarshalling [] into *struct
	// allocates a zero-value struct and reads as a phantom populated item.
	var wrap struct {
		Schedules []struct {
			ID                   string           `json:"id"`
			DeferralType         string           `json:"deferralType"`
			PostingStatus        string           `json:"postingStatus"`
			ServiceStartDate     string           `json:"serviceStartDate"`
			RecognitionFrequency string           `json:"recognitionFrequency"`
			RemainingAmount      map[string]any   `json:"remainingAmount"`
			ScheduleLines        []map[string]any `json:"scheduleLines"`
		} `json:"accountingDeferredRecognitionSchedulesBySourceEntity"`
	}
	_ = json.Unmarshal(data, &wrap)
	entity := "DeferredRevenueSchedule"
	if prepaid {
		entity = "PrepaidSchedule"
	}
	res := &QueryResult{
		Status: resp.StatusCode,
		Entity: entity,
		Counts: map[string]int{"items": 0},
		Items:  []QueryItem{},
	}
	if len(wrap.Schedules) == 0 {
		res.Note = "deferredrecognition GET_SCHEDULE: no schedule for source entity"
		if gmsg != "" {
			res.Note += "; gql: " + gmsg
		}
		return res, nil
	}
	totalLines := 0
	for _, sched := range wrap.Schedules {
		amt := 0.0
		cur := ""
		if sched.RemainingAmount != nil {
			amt = acctAnyFloat(sched.RemainingAmount["value"])
			cur = anyString(sched.RemainingAmount["currency"])
		}
		res.Items = append(res.Items, QueryItem{
			ID:      sched.ID,
			Name:    sched.DeferralType,
			Date:    sched.ServiceStartDate,
			Amount:  amt,
			Account: cur,
			Type:    sched.PostingStatus,
		})
		totalLines += len(sched.ScheduleLines)
	}
	res.Counts["items"] = len(res.Items)
	res.Counts["scheduleLines"] = totalLines
	res.Note = "deferredrecognition GET_SCHEDULE"
	if gmsg != "" {
		res.Note += "; gql: " + gmsg
	}
	return res, nil
}

// parseScheduleInput decodes the caller-supplied mutation input. The
// Accounting_Update/Preview input schemas were not fully captured (pass3
// has only the type names), so the shape is caller-supplied rather than
// invented; only presence is enforced here.
func ParseScheduleInput(raw string) (map[string]any, error) {
	var in map[string]any
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil, fmt.Errorf("--data must decode to a JSON object: %w", err)
	}
	if len(in) == 0 {
		return nil, ErrEmptyScheduleInput
	}
	return in, nil
}

// ReplayDeferredSchedulePreview runs the captured GET_SCHEDULE_PREVIEW with
// caller-supplied input and returns the projected draft schedule.
func ReplayDeferredSchedulePreview(ctx context.Context, input map[string]any, prepaid bool) (*QueryResult, error) {
	if len(input) == 0 {
		return nil, ErrEmptyScheduleInput
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "GET_SCHEDULE_PREVIEW",
		"variables":     map[string]any{"previewSchedulesInput": input},
		"query":         deferredSchedulePreviewQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("deferred schedule preview: %w", err)
	}
	resp, err := postDeferredGraphQL(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("deferred schedule preview: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading deferred schedule preview: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	data, gmsg, err := decodeGQL(raw)
	if err != nil {
		return nil, fmt.Errorf("deferred schedule preview: %w", err)
	}
	entity := "DeferredRevenueSchedule"
	if prepaid {
		entity = "PrepaidSchedule"
	}
	var top map[string]json.RawMessage
	_ = json.Unmarshal(data, &top)
	res := &QueryResult{
		Status: resp.StatusCode,
		Entity: entity,
		Counts: map[string]int{"items": 0},
		Items:  []QueryItem{},
		Note:   "deferredrecognition GET_SCHEDULE_PREVIEW",
	}
	if schedRaw, ok := top["accountingDeferredRecognitionPreviewSchedule"]; ok && string(schedRaw) != "null" {
		var sched struct {
			ID           string `json:"id"`
			DeferralType string `json:"deferralType"`
		}
		_ = json.Unmarshal(schedRaw, &sched)
		res.Counts["items"] = 1
		res.Items = []QueryItem{{ID: sched.ID, Name: sched.DeferralType, Type: entity}}
	}
	if gmsg != "" {
		res.Note += "; gql: " + gmsg
	}
	return res, nil
}

// ReplayDeferredScheduleCreate writes a deferred-revenue or prepaid
// schedule via the captured UpdateDeferredRecognitionScheduleOp. The input
// object is caller-supplied (see parseScheduleInput); nothing is invented.
func ReplayDeferredScheduleCreate(ctx context.Context, input map[string]any, prepaid bool) (*MutateResult, error) {
	if len(input) == 0 {
		return nil, ErrEmptyScheduleInput
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "UpdateDeferredRecognitionScheduleOp",
		"variables":     map[string]any{"input": input},
		"query":         deferredScheduleUpdateMutation,
	})
	if err != nil {
		return nil, fmt.Errorf("deferred schedule create: %w", err)
	}
	resp, err := postDeferredGraphQL(ctx, payload)
	if err != nil {
		return nil, fmt.Errorf("deferred schedule create: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading deferred schedule create: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	_, gmsg, err := decodeGQL(raw)
	if err != nil {
		return nil, fmt.Errorf("deferred schedule create: %w", err)
	}
	entity := "DeferredRevenueSchedule"
	if prepaid {
		entity = "PrepaidSchedule"
	}
	if gmsg != "" {
		return nil, fmt.Errorf("deferred schedule create: gql: %s", gmsg)
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "create",
		Entity: entity,
		Item:   QueryItem{Type: entity},
		Note:   "deferredrecognition UpdateDeferredRecognitionScheduleOp",
	}, nil
}

// jsonFloat coerces a decoded JSON number to float64.
func acctAnyFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case string:
		return parseAmount(t)
	case int:
		return float64(t)
	default:
		return 0
	}
}
