package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// CURRENCY_REVALUE — "company currency update revalue". Captured on TC2
// 2026-09-19: Settings > Currencies > home-currency row > Action menu >
// "Revalue currency" is a two-call neo flow:
//
//	GET  /api/neo/v1/company/{realm}/multicurrency/calculateHomeCurrencyAdjustment
//	     ?currencyCode={home}&exchangeRate={rate}&revaluedDate={dd/MM/yyyy}
//	     → bare array of entity rows (verbatim selectedEntityInfoList)
//	POST /api/neo/v1/company/{realm}/multicurrency/doHomeCurrencyAdjustment
//	     {adjustmentDate: ISO, currency, exchangeRate, memo,
//	      selectedEntityInfoList}
//
// Creates a JournalEntry debiting/crediting each entity against the Exchange
// Gain or Loss account (live-proven: JE 199, deleted). Requires multicurrency
// (Preferences.CurrencyPrefs.MultiCurrencyEnabled) — the action errors early
// when the company has it off. --rate maps to the dialog's "Custom rate"
// radio; the default is the market rate (1 for the home currency).
func ReplayCurrencyRevalue(ctx context.Context, date, rateS string) (*MutateResult, error) {
	iso, err := billDateISO(strings.TrimSpace(date))
	if err != nil {
		return nil, fmt.Errorf("--date: %w", err)
	}
	parsed, _ := time.Parse("2006-01-02", iso)
	slashDate := parsed.Format("02/01/2006")
	rate := "1"
	if s := strings.TrimSpace(rateS); s != "" {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("--rate must be a positive exchange rate, got %q", rateS)
		}
		rate = s
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	home, multi, err := revalueHomeCurrency(ctx, ac)
	if err != nil {
		return nil, err
	}
	if !multi {
		return nil, fmt.Errorf("multicurrency is disabled for this company — revalue needs Settings > Advanced > Currency on")
	}
	calcURL := fmt.Sprintf(
		"https://qbo.intuit.com/api/neo/v1/company/%s/multicurrency/calculateHomeCurrencyAdjustment?currencyCode=%s&exchangeRate=%s&revaluedDate=%s",
		ac.realm, url.QueryEscape(home), url.QueryEscape(rate), url.QueryEscape(slashDate))
	resp, err := ac.get(ctx, calcURL, "")
	if err != nil {
		return nil, fmt.Errorf("calculateHomeCurrencyAdjustment: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	got, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(got)}
	}
	var entities []json.RawMessage
	if err := json.Unmarshal(got, &entities); err != nil {
		return nil, fmt.Errorf("calculateHomeCurrencyAdjustment: malformed response %.200s", got)
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("no revalue-eligible balances at %s — the open-balances table is empty", slashDate)
	}
	body, err := json.Marshal(map[string]any{
		"adjustmentDate":         iso,
		"currency":               home,
		"exchangeRate":           rate,
		"memo":                   "",
		"selectedEntityInfoList": entities,
	})
	if err != nil {
		return nil, err
	}
	postURL := fmt.Sprintf("https://qbo.intuit.com/api/neo/v1/company/%s/multicurrency/doHomeCurrencyAdjustment", ac.realm)
	// The multicurrency plugin routes on its OWN apikey — the captured
	// qbo.intuit.com header set carries a different plugin's key and
	// doHomeCurrencyAdjustment 500s without these overrides (verified live:
	// bare postJSON → 500 "System is unable to process your request").
	extra := map[string]string{
		"Authorization":    "Intuit_APIKey intuit_apikey=prdakyresvCJTFaNkTwO8eBcn3DDj0n3ZvOND03h, intuit_apikey_version=1.0",
		"Referer":          "https://qbo.intuit.com/app/currencycenter/revalue?currency=" + url.QueryEscape(home),
		"intuit-plugin-id": "qbo-multicurrency-ui-v2",
		// applyHeaders copies the stale captured CsrfToken and returns before
		// the fresh cookie-derived c.csrf is applied; this endpoint validates
		// csrftoken against the live qbo.csrftoken cookie (verified: verbatim
		// UI headers → 200, jar headers → 500).
		"csrftoken": ac.csrf,
		// postJSONExtra forces Accept: application/json, which this endpoint
		// rejects with a bare 500 — the captured UI request sends */*
		// (verified: bisected live, Accept:application/json is the trigger).
		"Accept": "*/*",
	}
	presp, err := ac.postJSONExtra(ctx, postURL, body, extra)
	if err != nil {
		return nil, fmt.Errorf("doHomeCurrencyAdjustment: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(presp)
	pgot, err := readBody(presp)
	if err != nil {
		return nil, err
	}
	if presp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: presp.StatusCode, Message: errorMessage(pgot)}
	}
	note := fmt.Sprintf("%d entities, rate %s", len(entities), rate)
	// doHomeCurrencyAdjustment returns the created JournalEntry id as a bare
	// JSON number (live: body "201" → JE 201).
	jeID := strings.Trim(string(pgot), ` "`)
	if _, err := strconv.ParseInt(jeID, 10, 64); err != nil {
		jeID = ""
		var ack map[string]any
		if json.Unmarshal(pgot, &ack) == nil {
			for _, k := range []string{"journalEntryId", "journalEntryID", "id", "Id", "txnId"} {
				if v, ok := ack[k]; ok && fmt.Sprint(v) != "" {
					jeID = fmt.Sprint(v)
					break
				}
			}
		}
	}
	if n, err := strconv.ParseInt(jeID, 10, 64); err != nil || n <= 0 {
		return nil, fmt.Errorf("currency adjustment returned no JournalEntry identity; inspect before retrying")
	}
	note = "JE " + jeID + " — " + note
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "revalue",
		Entity: "HomeCurrencyAdjustment",
		Item:   QueryItem{Type: "JournalEntry", ID: jeID, Name: iso},
		Note:   note,
	}, nil
}

// revalueHomeCurrency reads Preferences for the home currency code and the
// multicurrency flag.
func revalueHomeCurrency(ctx context.Context, ac *apiClient) (string, bool, error) {
	rows, err := queryV3Rows(ctx, ac, "Preferences", "select * from Preferences")
	if err != nil {
		return "", false, fmt.Errorf("preferences: %w", err)
	}
	if len(rows) == 0 {
		return "", false, fmt.Errorf("preferences: empty response")
	}
	cp, _ := rows[0]["CurrencyPrefs"].(map[string]any)
	hc, _ := cp["HomeCurrency"].(map[string]any)
	home, _ := hc["value"].(string)
	if home == "" {
		home = "AUD"
	}
	multi, _ := cp["MultiCurrencyEnabled"].(bool)
	return home, multi, nil
}
