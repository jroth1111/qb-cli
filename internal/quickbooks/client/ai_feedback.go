// ai_feedback.go wires the AI/help/feedback + labels/templates + tags +
// threads work area (capture 2026-08, unmapped_by_area.json).
//
// Endpoint evidence (js_extraction.json url_literals + deobfuscated modules):
//
//	GET  https://qbo.intuit.com/api/v1/label-attributes            (products-and-services-core 57061.js)
//	GET  https://qbo.intuit.com/api/v1/label-templates?entityType=X (products-and-services-core 57061.js; SPA default entityType=ITEM)
//	GET  https://qbo.intuit.com/v2/tags                            (banking-reconcile-ui 11005.js; body {"tags":[...]})
//	GET  https://mccrmmessaging.api.intuit.com/v1/survey-settings  (crm-leads-ui 46136.js; LEAD_MANAGEMENT_SERVICE,
//	                                                                  prod base confirmed in 4190.js leadManagementServiceBaseUrl)
//	POST https://mccrmmessaging.api.intuit.com/v1/api/threads/generate-responses (body {threadId, previousMessageId})
//	POST https://mccrmmessaging.api.intuit.com/v1/linked-referred-customer       (body {referralId, referredCustomerId}; expects 201)
//
// Not wired (no verb assigned, kept documentation-only): POST
// triggerservice.api.intuit.com/v1/getSurveyFlag. The area notes list it as
// GET, but every capture and deobfuscated survey-libs copy (e.g. b2b-ui
// 1318) shows POST with an Intuit_APIKey/bearer Authorization and
// credentials:'include'; the bundled apikey is public but the eventMetaData
// block (authId, createdDate, intuitTid, realmId) makes it a
// browser-context call, not a replayable CLI read.
//
// Transport split:
//   - qbo.intuit.com same-origin GETs ride the existing apiClient with the
//     full captured ATS header set (same policy as the /ats/v1 banking and
//     /api/v3 report controls that succeed with ats_headers).
//   - mccrmmessaging.api.intuit.com authenticates with browser-context
//     cookies alone; direct cross-origin replay 403s (cookie_remint probe,
//     feedback_engagement row). Unlike the api/core leads service these
//     /v1 paths have no relay-tab transport wired, so they are documented
//     as BLOCKED_EXTERNAL: plans render under --dry-run, live attempts
//     fail fast with ErrCrmBlockedExternal after the standard session and
//     realm-pin guards, never dialing the host.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

// crmMessagingBaseURL is the prod LEAD_MANAGEMENT_SERVICE base
// (crm-leads-ui pluginConfig, deobfuscated 4190.js:351). The e2e/qal
// variants (-e2e/-qal) are intentionally unreachable from the CLI.
const crmMessagingBaseURL = "https://mccrmmessaging.api.intuit.com"

// Same-origin qbo.intuit.com roots for the label/template/tag reads.
const (
	labelAttributesPath = "/api/v1/label-attributes"
	labelTemplatesPath  = "/api/v1/label-templates"
	tagsPath            = "/v2/tags"

	qboSameOriginBase = "https://qbo.intuit.com"
)

// ErrCrmBlockedExternal reports that an endpoint lives on a host that
// authenticates with browser-context cookies only and has no wired
// transport: offline header replay 403s, and no relay-tab route exists.
// The command surfaces it after the standard credential and realm-pin
// guards so a missing session still exits as an auth failure.
var ErrCrmBlockedExternal = errors.New("blocked external: mccrmmessaging.api.intuit.com requires browser-context cookies and has no wired transport")

// Input-validation errors for the blocked-area arguments. They fire before
// any credential load so malformed invocations fail identically with or
// without a saved session.
var (
	ErrEmptyThreadData     = errors.New("threads generate-responses requires non-empty --data JSON")
	ErrInvalidThreadData   = errors.New("--data must be a JSON object (e.g. {\"threadId\":\"…\",\"previousMessageId\":\"…\"})")
	ErrEmptyReferralID     = errors.New("referred-customer link requires --referral-id")
	ErrEmptyCustomerID     = errors.New("referred-customer link requires --customer-id")
	ErrEmptyTemplateEntity = errors.New("labels list-templates entity type must not be empty")
)

// AIListResult is the tolerant, secret-free projection for same-origin
// reads whose item schema varies by bundle version. Items keeps each row
// verbatim; Count reflects what was parsed, not a server total.
type AIListResult struct {
	Status int               `json:"status"`
	Count  int               `json:"count"`
	Items  []json.RawMessage `json:"items"`
	Note   string            `json:"note,omitempty"`
}

// --- plans ------------------------------------------------------------------

// aiPlan stamps a secret-free RequestPlan. None of these URLs embed a
// realm id, so the verbatim host+path is already redacted-safe.
func aiPlan(method, rawURL, op string, body []byte, note string) *RequestPlan {
	return &RequestPlan{
		DryRun: true,
		Dial:   false,
		Method: method,
		URL:    rawURL,
		Body:   json.RawMessage(body),
		Note:   note,
		op:     op,
	}
}

// PlanSurveySettings builds the GET /v1/survey-settings plan
// (BLOCKED_EXTERNAL; rendered for --dry-run only).
func PlanSurveySettings() *RequestPlan {
	return aiPlan(http.MethodGet, crmMessagingBaseURL+"/v1/survey-settings",
		"ai.surveysettings.get", nil, "BLOCKED_EXTERNAL; mccrmmessaging cookie-only host; not sent")
}

// PlanThreadsGenerateResponses builds the POST /v1/api/threads/
// generate-responses plan. data must be a JSON object matching the SPA
// shape {threadId, previousMessageId}; it is stored verbatim.
func PlanThreadsGenerateResponses(data string) (*RequestPlan, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil, ErrEmptyThreadData
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &obj); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidThreadData, err)
	}
	return aiPlan(http.MethodPost, crmMessagingBaseURL+"/v1/api/threads/generate-responses",
		"ai.threads.generate_responses", []byte(data), "BLOCKED_EXTERNAL; mccrmmessaging cookie-only host; not sent"), nil
}

// PlanLinkedReferralCustomer builds the POST /v1/linked-referred-customer
// plan with the captured body shape {referralId, referredCustomerId}.
func PlanLinkedReferralCustomer(referralID, customerID string) (*RequestPlan, error) {
	referralID = sanitizeToken(referralID)
	customerID = sanitizeToken(customerID)
	if referralID == "" {
		return nil, ErrEmptyReferralID
	}
	if customerID == "" {
		return nil, ErrEmptyCustomerID
	}
	body, err := json.Marshal(map[string]string{
		"referralId":         referralID,
		"referredCustomerId": customerID,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding linked-referred-customer body: %w", err)
	}
	return aiPlan(http.MethodPost, crmMessagingBaseURL+"/v1/linked-referred-customer",
		"ai.referred_customer.link", body, "BLOCKED_EXTERNAL; mccrmmessaging cookie-only host; not sent"), nil
}

// --- same-origin reads ------------------------------------------------------

// aiSameOriginGet loads credentials, enforces the realm pin, then issues a
// same-origin GET on qbo.intuit.com with the full captured ATS header set.
func aiSameOriginGet(ctx context.Context, rawURL string) (*http.Response, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	return ac.getJSON(ctx, rawURL, "")
}

// ReplayLabelAttributes GETs qbo.intuit.com/api/v1/label-attributes.
// Read-only.
func ReplayLabelAttributes(ctx context.Context) (*AIListResult, error) {
	return aiListRead(ctx, qboSameOriginBase+labelAttributesPath, "label-attributes")
}

// ReplayLabelTemplates GETs qbo.intuit.com/api/v1/label-templates with the
// SPA's entityType filter (default ITEM, per products-and-services-core
// 57061.js). Read-only.
func ReplayLabelTemplates(ctx context.Context, entityType string) (*AIListResult, error) {
	entityType = strings.TrimSpace(entityType)
	if entityType == "" {
		return nil, ErrEmptyTemplateEntity
	}
	v := url.Values{}
	v.Set("entityType", entityType)
	return aiListRead(ctx, qboSameOriginBase+labelTemplatesPath+"?"+v.Encode(), "label-templates")
}

// labelQueryValues starts an empty query builder; kept separate so both
// template callers stay symmetric if filters accumulate.

// ReplayTags GETs qbo.intuit.com/v2/tags (document-service tag list;
// response body {"tags":[...]}). Read-only.
func ReplayTags(ctx context.Context) (*AIListResult, error) {
	return aiListRead(ctx, qboSameOriginBase+tagsPath, "tags")
}

// aiListRead performs one same-origin GET and projects the body into an
// AIListResult. Non-200 answers surface as ReplayError with the server
// message; parsing stays tolerant (Count 0 + note) rather than failing.
func aiListRead(ctx context.Context, rawURL, label string) (*AIListResult, error) {
	resp, err := aiSameOriginGet(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", label, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	res := &AIListResult{Status: resp.StatusCode, Items: []json.RawMessage{}}
	items, ok := aiRawItems(body)
	if ok {
		res.Items = items
		res.Count = len(items)
	} else {
		res.Note = label + ": unrecognized body shape; raw parse skipped"
	}
	return res, nil
}

// aiRawItems extracts the item array from a tolerant set of shapes: a bare
// top-level array, a preferred named key ({"tags": [...]}), or any single
// array-valued field. ok is false when none match.
func aiRawItems(body []byte) ([]json.RawMessage, bool) {
	var arr []json.RawMessage
	if err := json.Unmarshal(body, &arr); err == nil {
		return arr, true
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, false
	}
	for _, prefer := range []string{"tags", "labelAttributes", "labelTemplates", "items", "rows"} {
		if raw, ok := obj[prefer]; ok {
			var inner []json.RawMessage
			if err := json.Unmarshal(raw, &inner); err == nil {
				return inner, true
			}
		}
	}
	for _, raw := range obj {
		var inner []json.RawMessage
		if err := json.Unmarshal(raw, &inner); err == nil {
			return inner, true
		}
	}
	return nil, false
}

// --- blocked-external replays ----------------------------------------------

// aiBlockedExternal runs the shared guard ladder for mccrmmessaging
// endpoints: load credentials (fast auth exit), enforce the realm pin,
// then refuse with ErrCrmBlockedExternal. It never opens a socket.
func aiBlockedExternal() error {
	if _, err := newAPIClient(); err != nil {
		return err
	}
	return ErrCrmBlockedExternal
}

// ReplaySurveySettings documents GET mccrmmessaging/v1/survey-settings as
// BLOCKED_EXTERNAL. Returns ErrNoCredentials without a usable session,
// otherwise ErrCrmBlockedExternal.
func ReplaySurveySettings(ctx context.Context) error {
	_ = ctx // reserved for a future relay-tab transport; no dial today
	return aiBlockedExternal()
}

// ReplayThreadsGenerateResponses documents POST …/v1/api/threads/
// generate-responses as BLOCKED_EXTERNAL. data is validated exactly as in
// the plan so bad payloads fail before the credential check.
func ReplayThreadsGenerateResponses(ctx context.Context, data string) error {
	if _, err := PlanThreadsGenerateResponses(data); err != nil {
		return err
	}
	return aiBlockedExternal()
}

// ReplayLinkedReferralCustomer documents POST …/v1/linked-referred-customer
// as BLOCKED_EXTERNAL. IDs are validated exactly as in the plan.
func ReplayLinkedReferralCustomer(ctx context.Context, referralID, customerID string) error {
	if _, err := PlanLinkedReferralCustomer(referralID, customerID); err != nil {
		return err
	}
	return aiBlockedExternal()
}
