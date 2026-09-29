package client

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql"
)

// Reconcile sessions mutate through updateIntegration_Reconciliation on the
// qbonline-aws gateway — live capture on TC2 (2026-09-17) shows the webapp
// posts StartReconcile/CancelReconcile there, not to the qbo.intuit.com
// endpoint the webpack catalog recorded. The service key travels in the URL
// query, matching how the SPA calls the host cross-origin.
const qbonlineGraphqlURL = "https://qbonline-aws.api.intuit.com/v4/graphql?intuit_apikey=prdakyresQezKev1gdJPPXKhfflCMm9JjfdZpdlP"

// qbonline-aws is authenticated by the service apikey in the URL plus the
// first-party qbo.intuit.com header set (Intuit_APIKey) — no leftover-host
// capture exists for it. doURIHost's host arg selects the jar entry, so the
// calls key on qbo.intuit.com while dialing the aws gateway URL.
const qbonlineHeaderHost = "qbo.intuit.com"

// reconcileOpForAction maps reconcileAction enum values to their captured
// catalog operations. Only ops with complete live-captured documents are
// reachable; the rest remain BrokenTemplate in the catalog.
var reconcileOpForAction = map[string]string{
	"BEGIN":  "StartReconcile__integration_banking_reconcile_ui_qbo",
	"CANCEL": "CancelReconcile__integration_banking_reconcile_ui_qbo",
	"FINISH": "FinishReconcile__integration_banking_reconcile_ui_qbo",
}

// accountNodeID builds the Relay node id for a v3 Account — same scheme as
// the reconciliation node (v4.1:<realm>:<typeHash>:<id>) with the Account
// type hash captured live on TC2 2026-09-19.
func accountNodeID(realm, accountID string) string {
	raw := "v4.1:" + realm + ":51ced8536c"
	return base64.RawURLEncoding.EncodeToString([]byte(raw)) + ":" + accountID
}

// reconcileFinishNode is the node-read subset a FINISH needs: session state
// and the server's own defaults for the adjusting entry.
type reconcileFinishNode struct {
	InProgress          bool
	StatementEndingDate string
	AdjustAccountNodeID string
}

// fetchReconcileFinishNode reads the Integration_Reconciliation node for
// accountID on the first-party v4 gateway (the read path FINISH defaults
// resolve against).
func fetchReconcileFinishNode(ctx context.Context, ac *apiClient, accountID string) (*reconcileFinishNode, error) {
	doc := `query w($id: ID!) {
  node(id: $id) {
    id
    ... on Integration_Reconciliation {
      inProgress
      statementEndingDate
      adjustingEntryDefaultAccount { id }
    }
  }
}`
	payload, err := json.Marshal(map[string]any{
		"query":     doc,
		"variables": map[string]any{"id": reconciliationNodeID(ac.realm, accountID)},
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, v4GraphQLURL, qbonlineHeaderHost, payload)
	if err != nil {
		return nil, err
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data struct {
			Node struct {
				InProgress          bool   `json:"inProgress"`
				StatementEndingDate string `json:"statementEndingDate"`
				AdjustAccount       struct {
					ID string `json:"id"`
				} `json:"adjustingEntryDefaultAccount"`
			} `json:"node"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("decoding reconciliation node: %w", err)
	}
	return &reconcileFinishNode{
		InProgress:          wrap.Data.Node.InProgress,
		StatementEndingDate: wrap.Data.Node.StatementEndingDate,
		AdjustAccountNodeID: wrap.Data.Node.AdjustAccount.ID,
	}, nil
}

// ReplayReconcileSession runs a reconcile session mutation (BEGIN or CANCEL)
// for a bank account. accountID is the v3 account id (e.g. 45); the Relay
// node id is derived exactly as the reconcile read does. BEGIN requires
// endingBalance (decimal string) and endingDate (yyyy-MM-dd).
func ReplayReconcileSession(ctx context.Context, action, accountID, endingBalance, endingDate string) (*MutateResult, error) {
	action = strings.ToUpper(strings.TrimSpace(action))
	opName, ok := reconcileOpForAction[action]
	if !ok || action == "FINISH" {
		return nil, fmt.Errorf("unsupported reconcileAction %q (proven: BEGIN, CANCEL; FINISH rides the dedicated finish path)", action)
	}
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("reconcile %s requires --id <account-id>", strings.ToLower(action))
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	rec := map[string]any{
		"id":                 reconciliationNodeID(ac.realm, accountID),
		"reconcileAction":    action,
		"closeBooksPassword": nil,
	}
	if action == "BEGIN" {
		if strings.TrimSpace(endingBalance) == "" || strings.TrimSpace(endingDate) == "" {
			return nil, fmt.Errorf("reconcile begin requires --ending-balance and --ending-date (yyyy-MM-dd)")
		}
		rec["statementEndingBalance"] = endingBalance
		rec["statementEndingDate"] = endingDate
	}
	return postReconcileMutation(ctx, ac, action, opName, rec)
}

// postReconcileMutation POSTs updateIntegration_Reconciliation for action
// with the shaped integrationReconciliation record on qbonline-aws.
func postReconcileMutation(ctx context.Context, ac *apiClient, action, opName string, rec map[string]any) (*MutateResult, error) {
	gop, err := gql.Lookup(opName)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": gop.Name,
		"variables": map[string]any{"input_0": map[string]any{
			"pluginInfo": map[string]any{"customHeaders": map[string]any{
				"intuit-query-id": "reconciliationW",
			}},
			"clientMutationId":          "qb-cli",
			"integrationReconciliation": rec,
		}},
		"query": gop.Document,
	})
	if err != nil {
		return nil, fmt.Errorf("reconcile %s: %w", action, err)
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, qbonlineGraphqlURL, qbonlineHeaderHost, payload)
	if err != nil {
		return nil, fmt.Errorf("reconcile %s: %w", action, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading reconcile %s: %w", action, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data struct {
			Update struct {
				Rec *struct {
					ID         string `json:"id"`
					InProgress *bool  `json:"inProgress"`
				} `json:"integrationReconciliation"`
			} `json:"updateIntegration_Reconciliation"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("reconcile %s: malformed response: %w", action, err)
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		return nil, fmt.Errorf("reconcile %s: %s", action, wrap.Errors[0].Message)
	}
	expectedID, _ := rec["id"].(string)
	if wrap.Data.Update.Rec == nil || strings.TrimSpace(wrap.Data.Update.Rec.ID) == "" || wrap.Data.Update.Rec.InProgress == nil {
		return nil, fmt.Errorf("reconcile %s outcome unknown: missing integrationReconciliation receipt; inspect before retrying", action)
	}
	if wrap.Data.Update.Rec.ID != expectedID {
		return nil, fmt.Errorf("reconcile %s outcome unknown: receipt id %q does not match requested id %q; inspect before retrying", action, wrap.Data.Update.Rec.ID, expectedID)
	}
	expectInProgress := action == "BEGIN"
	if *wrap.Data.Update.Rec.InProgress != expectInProgress {
		return nil, fmt.Errorf("reconcile %s outcome unknown: receipt inProgress=%v, want %v; inspect before retrying", action, *wrap.Data.Update.Rec.InProgress, expectInProgress)
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     strings.ToLower(action),
		Entity: "Reconciliation",
		Item:   QueryItem{ID: wrap.Data.Update.Rec.ID, Type: "Reconciliation"},
		Note:   fmt.Sprintf("qbonline-aws %s inProgress=%v", opName, *wrap.Data.Update.Rec.InProgress),
	}, nil
}

// reconcileFinishRec resolves the FINISH record fields against the open
// session's node read — the same defaults the rec UI applies (statement
// ending date, adjustingEntryDefaultAccount). adjAccount accepts a Relay
// node id or a bare v3 account id.
func reconcileFinishRec(realm, accountID, adjDate, adjAccount, exchangeRate, stmtDate string, node *reconcileFinishNode) (map[string]any, error) {
	if strings.TrimSpace(stmtDate) == "" && node != nil {
		stmtDate = node.StatementEndingDate
	}
	if strings.TrimSpace(adjDate) == "" {
		adjDate = stmtDate
	}
	if strings.TrimSpace(stmtDate) == "" {
		return nil, fmt.Errorf("reconcile finish needs the statement ending date — pass --ending-date")
	}
	adjNode := strings.TrimSpace(adjAccount)
	switch {
	case adjNode == "" && node != nil:
		adjNode = node.AdjustAccountNodeID
	case adjNode != "" && !strings.Contains(adjNode, ":"):
		adjNode = accountNodeID(realm, adjNode)
	}
	if adjNode == "" {
		return nil, fmt.Errorf("reconcile finish needs an adjusting account — pass --adjust-account <account-id>")
	}
	rate := 1.0
	if s := strings.TrimSpace(exchangeRate); s != "" {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("reconcile finish: --exchange-rate %q: %w", exchangeRate, err)
		}
		rate = f
	}
	return map[string]any{
		"id":                           reconciliationNodeID(realm, accountID),
		"reconcileAction":              "FINISH",
		"adjustingEntryDate":           adjDate,
		"adjustingEntryAccount":        map[string]any{"id": adjNode},
		"adjustingEntryExchangeRate":   rate,
		"effectiveStatementEndingDate": stmtDate,
		"closeBooksPassword":           nil,
	}, nil
}

// ReplayReconcileFinish runs reconcileAction FINISH on the open session of
// accountID — the "accept the difference, book an adjusting entry" contract
// the rec UI posts when a reconciliation will not balance. Verified live on
// TC2 2026-09-19: the session closes (inProgress=false) and the service
// posts a "Reconcile Adjustment" transaction (Deposit or Purchase by
// direction, DocNumber ADJ) with its line on adjustingEntryAccount.
//
// Empty adjDate/adjAccount/stmtDate resolve against the open session's node
// read (statementEndingDate, adjustingEntryDefaultAccount) exactly as the
// UI preselects them; adjAccount accepts a Relay node id or a bare v3
// account id. exchangeRate defaults to 1 — the home-currency rate.
func ReplayReconcileFinish(ctx context.Context, accountID, adjDate, adjAccount, exchangeRate, stmtDate string) (*MutateResult, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("reconcile finish requires --account-id <account-id>")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	node, err := fetchReconcileFinishNode(ctx, ac, accountID)
	if err != nil {
		return nil, fmt.Errorf("reconcile finish: %w", err)
	}
	if !node.InProgress {
		return nil, fmt.Errorf("reconcile finish: account %s has no open session — begin one first (accounting reconcile create)", accountID)
	}
	rec, err := reconcileFinishRec(ac.realm, accountID, adjDate, adjAccount, exchangeRate, stmtDate, node)
	if err != nil {
		return nil, err
	}
	return postReconcileMutation(ctx, ac, "FINISH", reconcileOpForAction["FINISH"], rec)
}

// PlanReconcileFinish builds the FINISH payload for dry-run without opening
// a socket. Node-resolved defaults (adjustingEntryDefaultAccount, statement
// ending date) surface as flag values or the {realm} placeholder shape.
func PlanReconcileFinish(accountID, adjDate, adjAccount, exchangeRate, stmtDate string) (*RequestPlan, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, fmt.Errorf("reconcile finish requires --account-id <account-id>")
	}
	rec, err := reconcileFinishRec(realmToken, accountID, adjDate, adjAccount, exchangeRate, stmtDate, nil)
	if err != nil {
		return nil, err
	}
	gop, err := gql.Lookup(reconcileOpForAction["FINISH"])
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(map[string]any{
		"operationName": gop.Name,
		"variables": map[string]any{"input_0": map[string]any{
			"pluginInfo": map[string]any{"customHeaders": map[string]any{
				"intuit-query-id": "reconciliationW",
			}},
			"clientMutationId":          "qb-cli",
			"integrationReconciliation": rec,
		}},
		"query": gop.Document,
	})
	if err != nil {
		return nil, fmt.Errorf("reconcile finish: %w", err)
	}
	return &RequestPlan{
		DryRun: true,
		Dial:   false,
		Method: http.MethodPost,
		URL:    PlannedReconcileMutateURL(),
		Body:   json.RawMessage(payload),
		Note:   "qbonline-aws FinishReconcile; open session required; not sent",
		op:     "FinishReconcile",
	}, nil
}

// plannedQbonlineGraphqlURL is qbonlineGraphqlURL with the embedded service
// apikey redacted — request plans are a secret-free description of the HTTP
// call even when the credential is a public web-client key.
const plannedQbonlineGraphqlURL = "https://qbonline-aws.api.intuit.com/v4/graphql?intuit_apikey={embedded}"

// PlannedReconcileMutateURL reports the mutation host for dry-run plans.
func PlannedReconcileMutateURL() string { return plannedQbonlineGraphqlURL }
