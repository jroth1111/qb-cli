package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"net/http"
	"strings"
	"time"
)

const budgetGraphQLURL = "https://budgeting.api.intuit.com/graphql"

// budgetCreateMutation is the budgeting-ui 1.535.1 mutation captured
// 2026-08-18 from POST /app/budgets/create Save (HTTP 200).
const budgetCreateMutation = `mutation businessPlanningCreateBudget($budgetInput: BusinessPlanning_BudgetInput!) {
  businessPlanningCreateBudget(budgetInput: $budgetInput) {
    budget {
      budgetId
      budgetName
      startDate
      endDate
      intervalType
      budgetType
      secondaryListType
      dimensionDefId
      syncToken
      viewSettings {
        primaryId
        secondaryId
        axisIsSecondary
        showBlanks
        __typename
      }
      budgetDetails {
        primaryId
        secondaryId
        budgetData {
          amount
          budgetDate
          __typename
        }
        type
        __typename
      }
      __typename
    }
    clientMutationId
    __typename
  }
}
`

// PlannedBudgetCreateURL is the UI create endpoint (not v3 /budget).
func PlannedBudgetCreateURL() string { return budgetGraphQLURL }

func budgetGraphQLInput(flags map[string]string) map[string]any {
	name := firstFlag(flags, "name")
	if name == "" {
		name = "QB-CLI-TEST-BUDGET"
	}
	start := firstFlag(flags, "start-date", "fiscal-year")
	if start == "" {
		start = fmt.Sprintf("%d-07-01", time.Now().Year())
	}
	if len(start) == 4 {
		start = start + "-07-01"
	}
	start = normalizeDate(start)
	end := firstFlag(flags, "end-date")
	if end == "" {
		if t, err := time.Parse("2006-01-02", start); err == nil {
			end = t.AddDate(1, 0, -1).Format("2006-01-02")
		}
	} else {
		end = normalizeDate(end)
	}
	typ := "PROFIT_AND_LOSS"
	if strings.Contains(strings.ToLower(firstFlag(flags, "type")), "balance") {
		typ = "BALANCE_SHEET"
	}
	interval := "YEARLY"
	switch strings.ToLower(firstFlag(flags, "interval", "entry-type")) {
	case "month", "monthly":
		interval = "MONTHLY"
	case "quarter", "quarterly":
		interval = "QUARTERLY"
	}
	return map[string]any{
		"budgetId":          "",
		"budgetDetails":     []any{},
		"budgetName":        name,
		"budgetType":        typ,
		"startDate":         start,
		"endDate":           end,
		"intervalType":      interval,
		"secondaryListType": "NOT_SUBDIVIDED",
		"dimensionDefId":    "",
		"viewSettings": map[string]any{
			"secondaryId":       "",
			"showReferenceData": true,
			"referenceDataId":   "Actuals 2027 (YTD)",
			"referenceDataType": "Actuals",
			"showBlanks":        true,
			"archived":          false,
		},
		"syncToken":        0,
		"clientMutationId": "1",
	}
}

func (c *apiClient) intuitUserID() string {
	if c.tok == nil {
		return ""
	}
	for _, ck := range c.tok.Cookies {
		if strings.HasSuffix(ck.Name, ".userIdentifier") && ck.Value != "" {
			return ck.Value
		}
	}
	return ""
}

// postBudgetGraphQL sends the captured budgeting-ui header set and does
// not copy ATS RequestHeaders (those 403 this host; evidence 2026-08-18).
func (c *apiClient) postBudgetGraphQL(ctx context.Context, body []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, budgetGraphQLURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	authz := ""
	if c.tok != nil {
		authz = strings.TrimSpace(headerGetFold(c.tok.URIHostHeaders["budgeting.api.intuit.com"], "authorization"))
		if authz == "" {
			authz = strings.TrimSpace(c.tok.AuditAuthorization)
		}
		if authz == "" {
			// Sessions captured without an audit-ui request fall back to
			// the main ATS Intuit_APIKey; budgeting.api accepts it.
			authz = strings.TrimSpace(c.tok.Authorization)
		}
	}
	if authz == "" {
		return nil, fmt.Errorf("budget graphql: missing ATS/audit-ui Intuit_APIKey")
	}
	req.Header.Set("Authorization", authz)
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Accept", "application/json;charset=UTF-8")
	req.Header.Set("intuit-plugin-id", "budgeting-ui")
	req.Header.Set("intuit-company-id", c.realm)
	req.Header.Set("Referer", "https://qbo.intuit.com/")
	req.Header.Set("Origin", "https://qbo.intuit.com")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/151.0.0.0 Safari/537.36")
	req.Header.Set("Accept-Language", "en-AU,en;q=0.9")
	uid := headerGetFold(c.tok.URIHostHeaders["budgeting.api.intuit.com"], "intuit-user-id")
	if uid == "" {
		uid = c.intuitUserID()
	}
	if uid != "" {
		req.Header.Set("intuit-user-id", uid)
	}
	if c.cookies != "" {
		req.Header.Set("Cookie", c.cookies)
	}
	auth.BindRequest(req, c.tok, "budgeting.api.intuit.com")
	return impersonatedDo(req)
}

// ReplayBudgetCreate posts businessPlanningCreateBudget as captured from
// /app/budgets/create. The resulting Budget is also visible on v3 query.
func ReplayBudgetCreate(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{
		"operationName": "businessPlanningCreateBudget",
		"variables":     map[string]any{"budgetInput": budgetGraphQLInput(flags)},
		"query":         budgetCreateMutation,
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.postBudgetGraphQL(ctx, body)
	if err != nil {
		return nil, fmt.Errorf("budget graphql: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading budget graphql: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data struct {
			Create struct {
				Budget struct {
					BudgetID   string `json:"budgetId"`
					BudgetName string `json:"budgetName"`
				} `json:"budget"`
			} `json:"businessPlanningCreateBudget"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("decoding budget graphql: %w", err)
	}
	if len(wrap.Errors) > 0 {
		return nil, fmt.Errorf("budget graphql: %s", wrap.Errors[0].Message)
	}
	id := wrap.Data.Create.Budget.BudgetID
	name := wrap.Data.Create.Budget.BudgetName
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "create",
		Entity: "Budget",
		Item:   QueryItem{ID: id, Name: name, Type: "Budget"},
		Note:   "budgeting.api.intuit.com graphql businessPlanningCreateBudget",
	}, nil
}
