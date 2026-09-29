package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Captured 2026-08-19 from POST https://qbo.intuit.com/app/revenue-recognition
// (leftover-xhr-bodies.json). Wrap ATS/cookie proved HTTP 200 empty
// (edges=[]) via rr_ats_probe_test.go. Honest AU deferred-line list —
// not the Invoice 115/87 stand-in.
const revenueRecognitionGraphQLURL = "https://sbseggraphqlorch.api.intuit.com/graphql"

const accountingDeferredTransactionLineDetailsQuery = `query accountingDeferredTransactionLineDetailsOp($first: Int, $after: String, $last: Int, $before: String, $orderBy: Accounting_DeferredRecognitionTransactionLinesOrderBy, $filter: Accounting_DeferredRecognitionTransactionLinesFilter) {
  accountingDeferredRecognitionTransactionLineDetails(first: $first, after: $after, last: $last, before: $before, orderBy: $orderBy, filter: $filter) {
    pageInfo {
      hasNextPage
      hasPreviousPage
      startCursor
      endCursor
      __typename
    }
    edges {
      node {
        id
        docNum
        postingStatus
        customer {
          id
          __typename
        }
        sourceDetail {
          deferredAmount {
            value
            __typename
          }
          transaction {
            id
            type
            __typename
          }
          transactionLine {
            sequence
            __typename
          }
          __typename
        }
        remainingAmount {
          value
          currency
          __typename
        }
        serviceStartDate
        item {
          id
          __typename
        }
        scheduleLines {
          sourceEntityDetails {
            txId
            sequence
            __typename
          }
          recognitionDate
          recognitionAmount {
            value
            __typename
          }
          sequence
          recognitionPercentage
          name
          __typename
        }
        template {
          name
          __typename
        }
        __typename
      }
      __typename
    }
    __typename
  }
}`

func PlannedRevenueRecognitionURL() string { return revenueRecognitionGraphQLURL }

// replayAccountingDeferredTransactionLineDetails POSTs the captured
// accountingDeferredTransactionLineDetailsOp GraphQL. id/query are
// accepted for cobra flag parity but not forwarded — the captured body
// has filter={} only.
func replayAccountingDeferredTransactionLineDetails(ctx context.Context, _, _ string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 99999 // unbounded ask; service applies its own ceiling
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": "accountingDeferredTransactionLineDetailsOp",
		"variables": map[string]any{
			"first":  limit,
			"filter": map[string]any{},
		},
		"query": accountingDeferredTransactionLineDetailsQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("revenue-recognition deferred lines: %w", err)
	}
	extra := map[string]string{
		"Referer":      "https://qbo.intuit.com/app/revenue-recognition",
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, revenueRecognitionGraphQLURL, "sbseggraphqlorch.api.intuit.com", payload, extra)
	if err != nil {
		return nil, fmt.Errorf("revenue-recognition deferred lines: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading revenue-recognition deferred lines: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	res := projectDeferredRecognitionLines(body)
	res.Status = resp.StatusCode
	return res, nil
}

func projectDeferredRecognitionLines(body []byte) *QueryResult {
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Lines struct {
				Typename string `json:"__typename"`
				Edges    []struct {
					Node struct {
						ID            string `json:"id"`
						DocNum        string `json:"docNum"`
						PostingStatus string `json:"postingStatus"`
						ServiceStart  string `json:"serviceStartDate"`
						// Common_MoneyAmountV2 serialises value as a
						// string ("120.00"); a float64 field fails the
						// whole decode and reads as an unparsed empty list.
						Remaining struct {
							Value    any    `json:"value"`
							Currency string `json:"currency"`
						} `json:"remainingAmount"`
						Item struct {
							ID string `json:"id"`
						} `json:"item"`
						SourceDetail struct {
							Transaction struct {
								ID   string `json:"id"`
								Type string `json:"type"`
							} `json:"transaction"`
						} `json:"sourceDetail"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"accountingDeferredRecognitionTransactionLineDetails"`
		} `json:"data"`
	}
	note := "sbseggraphqlorch accountingDeferredTransactionLineDetailsOp"
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{
			Entity: "RevenueRecognition",
			Counts: map[string]int{"items": 0},
			Items:  []QueryItem{},
			Note:   note + " (unparsed)",
		}
	}
	items := make([]QueryItem, 0, len(wrap.Data.Lines.Edges))
	for _, e := range wrap.Data.Lines.Edges {
		n := e.Node
		if n.ID == "" && n.DocNum == "" {
			continue
		}
		items = append(items, QueryItem{
			ID:        n.ID,
			Name:      n.PostingStatus,
			Date:      n.ServiceStart,
			DocNumber: n.DocNum,
			Amount:    acctAnyFloat(n.Remaining.Value),
			Type:      n.SourceDetail.Transaction.Type,
			ItemID:    n.Item.ID,
		})
	}
	note = fmt.Sprintf("%s edges=%d", note, len(wrap.Data.Lines.Edges))
	if wrap.Data.Lines.Typename != "" {
		note += " " + wrap.Data.Lines.Typename
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{
		Entity: "RevenueRecognition",
		Counts: map[string]int{"items": len(items), "edges": len(wrap.Data.Lines.Edges)},
		Items:  items,
		Note:   note,
	}
}
