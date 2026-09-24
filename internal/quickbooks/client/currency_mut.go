package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Currency exchange-rate edits ride
// businessTransactionUpdateCurrencyExchangeRate on
// sbseggraphqlorch.api.intuit.com — captured live on TC2 2026-09-17 from the
// Currency Centre "Edit currencies exchange" dialog (Market Rate / Your Rate).
// The input takes currencyCode, exchangeRate, asOfDate and entityVersion:0
// (the server assigns the version on write).
const currencyExchangeHost = "sbseggraphqlorch.api.intuit.com"
const currencyExchangeURL = "https://sbseggraphqlorch.api.intuit.com/graphql"

const currencyRateSetDoc = `mutation businessTransactionUpdateCurrencyExchangeRate($input: BusinessTransaction_UpdateCurrencyExchangeRateInput!) {
  businessTransactionUpdateCurrencyExchangeRate(input: $input) {
    currencyExchangeRate {
      exchangeRate
      entityVersion
      fromCurrency { currencyCode }
    }
  }
}`

const currencyRateVersionDoc = `query GetBusinessTransactionExchangeRates($filter: BusinessTransaction_CurrencyExchangeRatesFilter, $first: PositiveInt) {
  businessTransactionCurrencyExchangeRates(filter: $filter, first: $first) {
    edges { node { fromCurrency { currencyCode } entityVersion } }
  }
}`

// Currency delete rides businessTransactionDeletePreferredForeignCurrency on
// the same host — captured live on TC2 2026-09-17 from the Currency Centre
// row Action menu → Delete → "Are you sure" → Yes (NZD added then removed).
const currencyDeleteDoc = `mutation businessTransactionDeletePreferredForeignCurrency($input: BusinessTransaction_PreferredForeignCurrencyInput!) {
  businessTransactionDeletePreferredForeignCurrency(input: $input) {
    currencyCode
  }
}`

// ReplayCurrencyRateSet posts a "Your Rate" exchange-rate override for a
// currency code (e.g. USD) as of a date (dd/MM/yyyy or ISO; default today).
func ReplayCurrencyRateSet(ctx context.Context, code, rate, date string) (*MutateResult, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 3 {
		return nil, fmt.Errorf("currency update requires --id <3-letter currency code, e.g. USD>")
	}
	rate = strings.TrimSpace(rate)
	if rate == "" {
		return nil, fmt.Errorf("currency update requires --rate <exchange rate vs home currency>")
	}
	asOf := normalizeDate(strings.TrimSpace(date))
	if asOf == "" {
		asOf = normalizeDate(defaultImportDate())
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	// The rate row is versioned — a repeat write for the same currency+date
	// needs its current entityVersion (STALE/validation error otherwise).
	ver, err := currencyRateVersion(ctx, ac, code, asOf)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"query": currencyRateSetDoc,
		"variables": map[string]any{
			"input": map[string]any{
				"asOfDate":      asOf,
				"currencyCode":  code,
				"exchangeRate":  rate,
				"entityVersion": ver,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, currencyExchangeURL, currencyExchangeHost, payload)
	if err != nil {
		return nil, fmt.Errorf("currency rate update: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading currency rate update: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data struct {
			U struct {
				Rate *struct {
					ExchangeRate  json.Number `json:"exchangeRate"`
					EntityVersion int         `json:"entityVersion"`
					FromCurrency  struct {
						Code string `json:"currencyCode"`
					} `json:"fromCurrency"`
				} `json:"currencyExchangeRate"`
			} `json:"businessTransactionUpdateCurrencyExchangeRate"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("currency rate update: malformed response: %w", err)
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		return nil, fmt.Errorf("currency rate update: %s", wrap.Errors[0].Message)
	}
	if wrap.Data.U.Rate == nil || wrap.Data.U.Rate.ExchangeRate.String() == "" || wrap.Data.U.Rate.FromCurrency.Code != code {
		return nil, fmt.Errorf("currency rate update outcome unknown: missing or mismatched currencyExchangeRate receipt; inspect before retrying")
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "update",
		Entity: "Currency",
		Item:   QueryItem{Type: "Currency", ID: code, Name: code + " " + wrap.Data.U.Rate.ExchangeRate.String()},
		Note:   "sbseggraphqlorch businessTransactionUpdateCurrencyExchangeRate asOf=" + asOf,
	}, nil
}

// currencyRateVersion returns the live entityVersion for code@asOf, or 0
// when no rate row exists for that pair yet.
func currencyRateVersion(ctx context.Context, ac *apiClient, code, asOf string) (int, error) {
	payload, err := json.Marshal(map[string]any{
		"query":     currencyRateVersionDoc,
		"variables": map[string]any{"filter": map[string]any{"asOfDate": asOf}, "first": 100},
	})
	if err != nil {
		return 0, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, currencyExchangeURL, currencyExchangeHost, payload)
	if err != nil {
		return 0, fmt.Errorf("currency rate version read: %w", err)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return 0, fmt.Errorf("currency rate version read: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data struct {
			R struct {
				Edges []struct {
					Node struct {
						From struct {
							Code string `json:"currencyCode"`
						} `json:"fromCurrency"`
						EntityVersion *int `json:"entityVersion"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"businessTransactionCurrencyExchangeRates"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return 0, fmt.Errorf("currency rate version: malformed response")
	}
	for _, e := range wrap.Data.R.Edges {
		if e.Node.From.Code == code && e.Node.EntityVersion != nil {
			return *e.Node.EntityVersion, nil
		}
	}
	return 0, nil
}

// ReplayCurrencyDelete removes a foreign currency by code
// (businessTransactionDeletePreferredForeignCurrency). QBO refuses when
// transactions are attached; the home currency cannot be deleted.
func ReplayCurrencyDelete(ctx context.Context, code string) (*MutateResult, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 3 {
		return nil, fmt.Errorf("currency delete requires --id <3-letter currency code, e.g. NZD>")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"query":     currencyDeleteDoc,
		"variables": map[string]any{"input": map[string]any{"currencyCode": code}},
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, currencyExchangeURL, currencyExchangeHost, payload)
	if err != nil {
		return nil, fmt.Errorf("currency delete: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading currency delete: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data struct {
			D *struct {
				Code string `json:"currencyCode"`
			} `json:"businessTransactionDeletePreferredForeignCurrency"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("currency delete: malformed response: %w", err)
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		return nil, fmt.Errorf("currency delete: %s", wrap.Errors[0].Message)
	}
	if wrap.Data.D == nil || wrap.Data.D.Code != code {
		return nil, fmt.Errorf("currency delete outcome unknown: missing or mismatched currency receipt; inspect before retrying")
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "delete",
		Entity: "Currency",
		Item:   QueryItem{Type: "Currency", ID: code, Name: code},
		Note:   "sbseggraphqlorch businessTransactionDeletePreferredForeignCurrency",
	}, nil
}

// PlannedCurrencyRateURL reports the mutation host for dry-run plans.
func PlannedCurrencyRateURL() string { return currencyExchangeURL }

// PlannedCurrencyDeleteURL reports the delete host for dry-run plans.
func PlannedCurrencyDeleteURL() string { return currencyExchangeURL }
