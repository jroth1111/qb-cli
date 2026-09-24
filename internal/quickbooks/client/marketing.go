package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// Captured 2026-09-19 on /app/usermgt (in-product marketing placements, the
// "IPD" offer engine behind QBO's upsell banners).
//
//	POST https://personalization.api.intuit.com/v1/experience/ipd/placement/{placement}
//	  ?intuit_apikey=<app key>&offerData=true&numberOfRecommendations={n}
//	  &locale=en-au&intuit_tid={uuid}
//	body: eligibility flags {qboCompanyId, isMasterAdmin, ipdSkuId, ...}
//	→ 200 {recommendations:{recommendation:[{offerId,score,copyData{...}}]}}
//	→ 204 when the placement has no offer
//
// Auth rides the intuit_apikey URL parameter plus the session cookie set —
// the browser sends no Authorization header to this host. doURIHost's
// leftover-host guard accepts a URL-carried apikey for this auth style.
const marketingHost = "personalization.api.intuit.com"
const marketingPlacementURL = "https://" + marketingHost + "/v1/experience/ipd/placement/"

// The page's own marketing-ipd-tsa-widgets key; same apikey mechanism the
// browser used (in the URL, not a header).
const marketingAPIKey = "prdakyresCDZxKguxqjr77Nr3HNz363NYssMuiS1"

// PlannedMarketingURL is the dry-run surface for the marketing read path.
func PlannedMarketingURL(placement string) string {
	if strings.TrimSpace(placement) == "" {
		placement = "AdvancedShellBanner"
	}
	return marketingPlacementURL + placement + "?intuit_apikey=…&offerData=true&numberOfRecommendations=<limit>"
}

// replayMarketingOffers backs ReplayQuery("MarketingOffer", ...): POST the
// placement endpoint with the captured eligibility flag set; offerData=true
// returns offer rows. placement selects the surface (AdvancedShellBanner,
// UserMgtTop, QBOModalInterrupters, ...); id doubles as the flag value.
func replayMarketingOffers(ctx context.Context, placement, query string, limit int) (*QueryResult, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	placement = strings.TrimSpace(placement)
	if placement == "" {
		placement = "AdvancedShellBanner"
	}
	if limit < 1 {
		limit = -1 // captured: numberOfRecommendations=-1 asks for all
	}
	trace := freshTraceHeaders()
	u := marketingPlacementURL + url.PathEscape(placement) +
		"?intuit_apikey=" + marketingAPIKey +
		"&offerData=true&numberOfRecommendations=" + fmt.Sprintf("%d", limit) +
		"&locale=en-au&intuit_tid=" + url.QueryEscape(trace["intuit_tid"])
	body, err := json.Marshal(map[string]any{
		"isMasterAdmin": true,
		"isAdmin":       true,
		"isAccountant":  false,
		"isEmbedded":    false,
		"ipdSkuId":      "17",
		"HasPayroll":    false,
		"HasPayments":   false,
		"qboCompanyId":  ac.realm,
		"companyName":   "Test Company 2",
		"Locale":        "en-au",
		"isoLocale":     "en-au",
		"numLogin":      52,
	})
	if err != nil {
		return nil, fmt.Errorf("marketing placement: %w", err)
	}
	overrides := map[string]string{
		"Referer":    "https://qbo.intuit.com/",
		"intuit_tid": trace["intuit_tid"],
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, u, marketingHost, body, overrides)
	if err != nil {
		return nil, fmt.Errorf("marketing placement: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading marketing placement: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	res := projectMarketingOffers(raw, query, limit)
	res.Status = resp.StatusCode
	return res, nil
}

func projectMarketingOffers(body []byte, query string, limit int) *QueryResult {
	note := "personalization.api ipd placement"
	var wrap struct {
		Recommendations struct {
			Recommendation []struct {
				OfferID   string  `json:"offerId"`
				Name      string  `json:"name"`
				Status    string  `json:"status"`
				UIPattern string  `json:"uiPattern"`
				Score     float64 `json:"score"`
				CopyData  struct {
					CTAText          string `json:"ctaText"`
					CTAUrl           string `json:"ctaUrl"`
					Body             string `json:"body"`
					SecondaryCTAText string `json:"secondaryCtaText"`
				} `json:"copyData"`
			} `json:"recommendation"`
		} `json:"recommendations"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil || len(body) == 0 {
		return &QueryResult{Entity: "MarketingOffer", Counts: map[string]int{"items": 0}, Items: []QueryItem{}, Note: note + " (no offers)"}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	items := make([]QueryItem, 0, len(wrap.Recommendations.Recommendation))
	for _, r := range wrap.Recommendations.Recommendation {
		it := QueryItem{
			ID:   r.OfferID,
			Name: firstNonEmpty(r.CopyData.CTAText, r.Name),
			Type: firstNonEmpty(r.UIPattern, "offer"),
			Item: firstNonEmpty(r.CopyData.Body, r.CopyData.SecondaryCTAText),
		}
		if q != "" && !strings.Contains(strings.ToLower(it.ID+" "+it.Name+" "+it.Type+" "+it.Item+" "+r.Name), q) {
			continue
		}
		items = append(items, it)
		if limit > 0 && len(items) >= limit {
			break
		}
	}
	note = fmt.Sprintf("%s offers=%d items=%d", note, len(wrap.Recommendations.Recommendation), len(items))
	return &QueryResult{
		Entity: "MarketingOffer",
		Counts: map[string]int{"items": len(items), "totalCount": len(wrap.Recommendations.Recommendation)},
		Items:  items,
		Note:   note,
	}
}
