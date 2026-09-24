package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// orDate returns d when non-empty, else fallback.
func orDate(d, fallback string) string {
	if strings.TrimSpace(d) != "" {
		return strings.TrimSpace(d)
	}
	return fallback
}

// quarterStart/quarterEnd bracket the current calendar quarter — the default
// BAS liability window the tax home shows.
func quarterStart() string {
	n := time.Now()
	m := ((int(n.Month()) - 1) / 3) * 3
	return fmt.Sprintf("%04d-%02d-01", n.Year(), m+1)
}

func quarterEnd() string {
	n := time.Now()
	m := ((int(n.Month())-1)/3)*3 + 3
	last := time.Date(n.Year(), time.Month(m+1), 0, 0, 0, 0, 0, time.UTC)
	return last.Format("2006-01-02")
}

// Indirect-tax (AU GST/BAS/IAS) reads captured from the Test Company 2 tax
// centre (2026-09-17). All ride qbo.intuit.com/api/v4/graphql:
//   - TaxAuthorities__indirect_tax_ui_qbo  → agency list
//   - node__indirect_tax_ui_qbo          → agency node → taxReturns / taxLiability

const v4GraphQLURL = "https://qbo.intuit.com/api/v4/graphql"

const taxAgencyListDoc = `query TaxAuthorities__indirect_tax_ui_qbo($limit: Int!) {
  company {
    taxAgencies(first: $limit) {
      edges { node { id code displayName __typename } __typename }
      __typename
    }
    __typename
  }
}`

const taxReturnsDoc = `query node__indirect_tax_ui_qbo($id: ID!, $limit: Int, $taxReturnTypeFilterExpression: String) {
  node(id: $id) {
    id
    ...TaxReturnsTable_TaxAgency
    __typename
  }
}

fragment TaxReturnsTable_TaxAgency on Indirecttaxes_TaxAgency {
  id
  name
  displayName
  code
  basisType
  filingFrequency
  taxReturns(first: $limit, filterBy: $taxReturnTypeFilterExpression) {
    edges {
      node {
        id
        startDate
        endDate
        fileDate
        upcomingFiling
        netTaxAmountDue
        taxReturnType
        totalPaymentMade
        eFilingStatus
        eFilingStatusCode
        taxAgency { id name code __typename }
        __typename
      }
      __typename
    }
    __typename
  }
  __typename
}`

// firstTaxAgencyID resolves the company's first indirect-tax agency node id.
func firstTaxAgencyID(ctx context.Context) (string, error) {
	ac, err := newAPIClient()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "TaxAuthorities__indirect_tax_ui_qbo",
		"variables":     map[string]any{"limit": 10},
		"query":         taxAgencyListDoc,
	})
	if err != nil {
		return "", err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, v4GraphQLURL, "qbo.intuit.com", payload)
	if err != nil {
		return "", fmt.Errorf("taxAgencies: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return "", fmt.Errorf("taxAgencies: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var doc struct {
		Data struct {
			Company struct {
				TaxAgencies struct {
					Edges []struct {
						Node struct {
							ID   string `json:"id"`
							Code string `json:"code"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"taxAgencies"`
			} `json:"company"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return "", fmt.Errorf("taxAgencies decode: %w", err)
	}
	for _, e := range doc.Data.Company.TaxAgencies.Edges {
		if e.Node.ID != "" {
			return e.Node.ID, nil
		}
	}
	return "", fmt.Errorf("no indirect-tax agency registered (tax not enabled)")
}

// ReplayTaxReturns lists BAS/IAS activity-statement returns for the company's
// first indirect-tax agency. filterExpr scopes the taxReturns connection
// (e.g. taxReturnType); "" returns all.
func ReplayTaxReturns(ctx context.Context, entity, filterExpr, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 50
	}
	agencyID, err := firstTaxAgencyID(ctx)
	if err != nil {
		return nil, err
	}
	vars := map[string]any{
		"id":    agencyID,
		"limit": 99999999,
	}
	if filterExpr != "" {
		vars["taxReturnTypeFilterExpression"] = filterExpr
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "node__indirect_tax_ui_qbo",
		"variables":     vars,
		"query":         taxReturnsDoc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, v4GraphQLURL, "qbo.intuit.com", payload)
	if err != nil {
		return nil, fmt.Errorf("taxReturns: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("taxReturns: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return projectEdgeNodes(entity, "node__indirect_tax_ui_qbo taxReturns", raw,
		[]string{"data", "node", "taxReturns", "edges"}, query, limit, resp.StatusCode), nil
}

// taxLiabilityDoc is the tax-home agency-node read: per-BAS-box liability
// line items (returnLine name/type/expression, total/liability/exception
// amounts). Captured live on TC2 2026-09-17 — returned real Sales Tax
// liability rows for the current period.
const taxLiabilityDoc = `query node__indirect_tax_ui_qbo($id: ID!, $taxLiabilityFilterExpression: String) {
  node(id: $id) {
    id
    ...Loader_TaxAgency
    __typename
  }
}

fragment Loader_TaxAgency on Indirecttaxes_TaxAgency {
  id
  displayName
  configType
  lastFileDate
  periodStartMonth
  paymentFrequency
  filingFrequency
  startDate
  filingEnabled
  taxOnSale
  taxOnPurchase
  taxOnPurchaseReclaimable
  taxLiability(first: 1, filterBy: $taxLiabilityFilterExpression) {
    edges {
      node {
        id
        taxLiabilityDetails {
          returnLine {
            id
            name
            lineType
            description
            expression
            netTaxLine
            __typename
          }
          totalAmount
          liabilityAmount
          exceptionAmount
          basisSwitchAmount
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
  taxAgencyMetaModel {
    paymentFrequency {
      enabled
      __typename
    }
    __typename
  }
  __typename
}
`

// ReplayTaxLiability reads the indirect-tax liability report (per BAS/GST
// return line) for the company's first tax agency — the modernised liability
// surface the tax home renders.
func ReplayTaxLiability(ctx context.Context, startDate, endDate, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 20
	}
	agencyID, err := firstTaxAgencyID(ctx)
	if err != nil {
		return nil, err
	}
	// The indirect-tax gateway validates the full Relay var envelope the tax
	// home sends — omitting the date/return-type vars errors even though the
	// document does not declare them.
	vars := map[string]any{
		"id":        agencyID,
		"startDate": fmt.Sprintf("%sT00:00:00.000Z", orDate(startDate, quarterStart())),
		"endDate":   fmt.Sprintf("%sT23:59:59.999Z", orDate(endDate, quarterEnd())),
	}
	vars["taxReturnTypeFilterExpression"] = nil
	vars["taxLiabilityFilterExpression"] = fmt.Sprintf(
		"startDate >= '%s' and endDate <= '%s'",
		orDate(startDate, quarterStart()), orDate(endDate, quarterEnd()))
	payload, err := json.Marshal(map[string]any{
		"operationName": "node__indirect_tax_ui_qbo",
		"variables":     vars,
		"query":         taxLiabilityDoc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, v4GraphQLURL, "qbo.intuit.com", payload)
	if err != nil {
		return nil, fmt.Errorf("taxLiability: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("taxLiability: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return projectEdgeNodes("TaxLiability", "node__indirect_tax_ui_qbo taxLiability", raw,
		[]string{"data", "node", "taxLiability", "edges"}, query, limit, resp.StatusCode), nil
}

// planningforecasting getAllBusinessForecasts — proven live on TC2 (typed
// empty edge list; forecasts exist only in Advanced SKU).
const businessForecastsDoc = `query getAllBusinessForecasts($first: Int, $after: String, $last: Int, $before: String, $sortBy: [BusinessForecastSortByFields]) {
  getAllBusinessForecasts(first: $first, after: $after, last: $last, before: $before, sortBy: $sortBy) {
    edges {
      cursor
      node {
        forecastId
        version
        name
        forecastMode
        forecastType
        intervalType
      }
    }
    pageInfo { hasNextPage endCursor }
    totalCount
  }
}`

// ReplayBusinessForecasts lists Advanced forecasts via
// planningforecasting.api.intuit.com — proven live on TC2 (typed empty).
func ReplayBusinessForecasts(ctx context.Context, id, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 50
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "getAllBusinessForecasts",
		"variables":     map[string]any{"first": limit, "sortBy": []string{"LAST_MODIFIED_TIME_DESC"}},
		"query":         businessForecastsDoc,
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost,
		"https://planningforecasting.api.intuit.com/graphql",
		"planningforecasting.api.intuit.com", payload)
	if err != nil {
		return nil, fmt.Errorf("getAllBusinessForecasts: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, fmt.Errorf("getAllBusinessForecasts: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectEdgeNodes("BusinessForecast", "planningforecasting getAllBusinessForecasts", raw,
		[]string{"data", "getAllBusinessForecasts", "edges"}, query, limit, resp.StatusCode)
	if id = strings.TrimSpace(id); id != "" {
		var keep []QueryItem
		for _, it := range res.Items {
			if it.ID == id || strings.HasSuffix(it.ID, ":"+id) {
				keep = append(keep, it)
			}
		}
		res.Items = keep
		res.Counts["items"] = len(keep)
	}
	return res, nil
}

// performanceMetrics are the performance-centre metric set captured on the
// insights dashboard (2026-09-17).
var performanceMetrics = []string{
	"Revenue", "Expenses", "NetProfit", "GrossProfit",
	"AccountsReceivable", "AccountsPayable", "CurrentRatio", "QuickRatio",
	"CashInPeriod",
}

// ReplayPerformanceMetrics POSTs the captured metrics contract to
// universalreportinsights /v1/metrics/{name} — proven live on TC2 (real
// dataFrames with company figures).
func ReplayPerformanceMetrics(ctx context.Context, query string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = len(performanceMetrics)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{
		"filters": []map[string]any{
			{"on": "startDate", "operator": ">=", "value": "2026-01-01"},
			{"on": "endDate", "operator": "<=", "value": "2026-09-17"},
			{"on": "cashBasis", "operator": "=", "value": false},
		},
		"groups": []map[string]any{{"on": "time", "by": "month"}},
	})
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(performanceMetrics))
	var lastErr error
	for _, m := range performanceMetrics {
		if q != "" && !strings.Contains(strings.ToLower(m), q) {
			continue
		}
		u := "https://universalreportinsights.api.intuit.com/v1/metrics/" + m
		resp, err := ac.doURIHost(ctx, http.MethodPost, u, "universalreportinsights.api.intuit.com", body)
		if err != nil {
			lastErr = err
			continue
		}
		raw, rerr := readBody(resp)
		_ = drainAndClose(resp)
		if rerr != nil {
			lastErr = rerr
			continue
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
			continue
		}
		var doc struct {
			DataFrames []struct {
				Columns []string `json:"columns"`
				Rows    [][]any  `json:"rows"`
			} `json:"dataFrames"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			lastErr = err
			continue
		}
		var total float64
		for _, df := range doc.DataFrames {
			for _, row := range df.Rows {
				if len(row) > 1 {
					if v, ok := row[len(row)-1].(float64); ok {
						total += v
					}
				}
			}
		}
		items = append(items, QueryItem{ID: m, Name: m, Amount: total})
		if len(items) >= limit {
			break
		}
	}
	if len(items) == 0 && lastErr != nil {
		return nil, fmt.Errorf("universalreportinsights metrics: %w", lastErr)
	}
	return &QueryResult{Entity: "PerformanceMetric", Status: http.StatusOK,
		Counts: map[string]int{"items": len(items), "totalCount": len(performanceMetrics)}, Items: items,
		Note: "universalreportinsights /v1/metrics"}, nil
}
