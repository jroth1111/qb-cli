package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

var ErrStateVerification = errors.New("feed state change unverified; inspect current state before retrying")
var mutationEntityName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*$`)
var mutationEntityID = regexp.MustCompile(`^[0-9]+$`)

func readMutationEntity(ctx context.Context, ac *apiClient, entity, id string) (map[string]any, error) {
	if !mutationEntityName.MatchString(entity) || !mutationEntityID.MatchString(id) {
		return nil, fmt.Errorf("invalid linked entity identity")
	}
	query := "SELECT * FROM " + entity + " WHERE Id = '" + id + "'"
	switch entity {
	case "Account", "Class", "Department", "Customer", "Vendor", "Employee", "Item", "Term", "PaymentMethod":
		query += " AND Active IN (true, false)" // inactive is not proof of deletion
	}
	resp, err := ac.getJSON(ctx, "https://qbo.intuit.com/api/v3/company/"+ac.realm+"/query?minorversion=73&query="+url.QueryEscape(query), "")
	if err != nil {
		return nil, err
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("entity read returned %d", resp.StatusCode)
	}
	var env struct {
		Query map[string]json.RawMessage `json:"QueryResponse"`
		Fault json.RawMessage            `json:"Fault"`
	}
	if err = json.Unmarshal(raw, &env); err != nil || env.Query == nil || len(env.Fault) > 0 {
		return nil, fmt.Errorf("entity query did not return QueryResponse")
	}
	var rows []map[string]any
	if data := env.Query[entity]; len(data) > 0 {
		if string(data) == "null" {
			return nil, fmt.Errorf("entity query returned null, not proven absence")
		}
		if err = json.Unmarshal(data, &rows); err != nil {
			return nil, err
		}
	}
	for key := range env.Query {
		if key != entity && key != "startPosition" && key != "maxResults" && key != "totalCount" {
			return nil, fmt.Errorf("entity query returned a different resource")
		}
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) != 1 || jsonNumberString(rows[0]["Id"]) != id {
		return nil, fmt.Errorf("entity identity mismatch")
	}
	return rows[0], nil
}

func stateRows(ctx context.Context, ac *apiClient, account, state string, ids []string, requireAll bool) ([]map[string]any, error) {
	found := map[string]map[string]any{}
	seen := map[string]bool{}
	exhausted := false
	for page := range 100 {
		start := page * 300
		q := url.Values{"accountId": {account}, "reviewState": {state}, "sort": {"-txnDate"}, "ignoreMatching": {"false"}, "startIndex": {fmt.Sprint(start)}, "chunkSize": {"300"}}
		resp, err := ac.get(ctx, ac.neoFeedURL()+"/getTransactions?"+q.Encode(), fmt.Sprintf("items=%d-%d", start, start+299))
		if err != nil {
			return nil, err
		}
		raw, err := readBody(resp)
		_ = drainAndClose(resp)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("state read returned %d", resp.StatusCode)
		}
		var envelope map[string]json.RawMessage
		if json.Unmarshal(raw, &envelope) != nil || len(bytes.TrimSpace(envelope["items"])) == 0 || bytes.TrimSpace(envelope["items"])[0] != '[' {
			return nil, fmt.Errorf("state response lacks items")
		}
		var env struct {
			Items []map[string]any `json:"items"`
		}
		if err = json.Unmarshal(raw, &env); err != nil {
			return nil, err
		}
		fresh := 0
		for _, row := range env.Items {
			id := mapStr(row, "id")
			if !seen[id] {
				seen[id] = true
				fresh++
			}
			for _, want := range ids {
				if feedRowMatchesID(row, want) {
					if jsonNumberString(row["qboAccountId"]) != account {
						return nil, fmt.Errorf("feed account mismatch")
					}
					found[want] = row
				}
			}
		}
		if requireAll && len(found) == len(ids) {
			break
		}
		// A short nonempty page is not terminal on the banking endpoint.
		// Absence requires an empty page, not merely fewer than requested.
		if len(env.Items) == 0 {
			exhausted = true
			break
		}
		if fresh == 0 {
			return nil, fmt.Errorf("state pagination repeated before exhaustion")
		}
	}
	if requireAll && len(found) != len(ids) {
		return nil, fmt.Errorf("%w: %s state did not contain requested rows", ErrFeedRowsNotFound, state)
	}
	if !requireAll && !exhausted {
		return nil, fmt.Errorf("state absence not proved before page ceiling")
	}
	out := []map[string]any{}
	for _, id := range ids {
		if r := found[id]; r != nil {
			out = append(out, r)
		}
	}
	return out, nil
}

func comparableEntity(in map[string]any) map[string]any {
	out := shallowCopyMap(in)
	delete(out, "SyncToken")
	if m, ok := out["MetaData"].(map[string]any); ok {
		m = shallowCopyMap(m)
		delete(m, "LastUpdatedTime")
		out["MetaData"] = m
	}
	return out
}

func replayVerifiedStateChange(ctx context.Context, ac *apiClient, plan *RequestPlan) (int, error) {
	var req struct {
		Next struct {
			Account string `json:"accountId"`
			State   string `json:"reviewState"`
		} `json:"nextTxnInfo"`
		IDs struct {
			IDs []string `json:"olbTxnIds"`
		} `json:"txnIdList"`
	}
	if err := json.Unmarshal(plan.Body, &req); err != nil {
		return 0, err
	}
	from, to := req.Next.State, "PENDING"
	if plan.op == "excludeTransactions" {
		to = "EXCLUDED"
	}
	before, err := stateRows(ctx, ac, req.Next.Account, from, req.IDs.IDs, true)
	if err != nil {
		return 0, err
	}
	linked := map[string]struct {
		Entity string
		Record map[string]any
	}{}
	if from == "ACCEPTED" {
		if err := preflightUnpostReconciliation(ctx, ac, req.Next.Account, before); err != nil {
			return 0, err
		}
		for _, row := range before {
			entries, _ := row["matchedQboTxns"].([]any)
			for _, v := range entries {
				q, _ := v.(map[string]any)
				id := jsonNumberString(q["qboTxnId"])
				entity := mapStr(q, "txnFdmName")
				record, err := readMutationEntity(ctx, ac, entity, id)
				if err != nil || record == nil {
					return 0, fmt.Errorf("cannot preserve linked %s %s", entity, id)
				}
				linked[id] = struct {
					Entity string
					Record map[string]any
				}{entity, record}
			}
		}
	}
	dir, err := startMutationEvidence(plan.op, map[string]any{"account_id": req.Next.Account, "source_state": from, "target_state": to, "feed_rows": before, "linked_records": linked}, plan.Body)
	if err != nil {
		return 0, err
	}
	submittingMutation(ctx)
	resp, err := ac.post(ctx, strings.ReplaceAll(plan.URL, realmToken, ac.realm), plan.Body)
	if err != nil {
		return 0, fmt.Errorf("%w: %v (evidence %s)", ErrStateVerification, err, dir)
	}
	raw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return resp.StatusCode, fmt.Errorf("%w: response read", ErrStateVerification)
	}
	if err = os.WriteFile(filepath.Join(dir, "response.json"), raw, 0600); err != nil {
		return resp.StatusCode, fmt.Errorf("%w: response persistence", ErrStateVerification)
	}
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("%w: HTTP %d (evidence %s)", ErrStateVerification, resp.StatusCode, dir)
	}
	var result struct {
		Rows struct {
			Success []struct {
				ID      string `json:"olbTxnId"`
				Deleted []struct {
					ID string `json:"qboTxnId"`
				} `json:"deletedQboTxns"`
			} `json:"success"`
		} `json:"olbtxns"`
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return 200, fmt.Errorf("%w: invalid response", ErrStateVerification)
	}
	deleted := map[string]bool{}
	if plan.op == "undoTransactions" {
		if len(result.Rows.Success) != len(before) {
			return 200, fmt.Errorf("%w: missing Undo success records", ErrStateVerification)
		}
		success := map[string]bool{}
		for _, r := range result.Rows.Success {
			success[r.ID] = true
			for _, d := range r.Deleted {
				if _, ok := linked[d.ID]; !ok {
					return 200, fmt.Errorf("%w: unexpected deleted entity", ErrStateVerification)
				}
				deleted[d.ID] = true
			}
		}
		for _, row := range before {
			if !success[mapStr(row, "olbTxnId")] {
				return 200, fmt.Errorf("%w: wrong Undo success IDs", ErrStateVerification)
			}
		}
	}
	after, err := stateRows(ctx, ac, req.Next.Account, to, req.IDs.IDs, true)
	if err != nil {
		return 200, fmt.Errorf("%w: %v", ErrStateVerification, err)
	}
	for i, row := range before {
		for _, key := range []string{"id", "amount", "olbTxnDate", "origDescription", "qboAccountId"} {
			if !reflect.DeepEqual(row[key], after[i][key]) {
				return 200, fmt.Errorf("%w: %s changed", ErrStateVerification, key)
			}
		}
	}
	remaining, err := stateRows(ctx, ac, req.Next.Account, from, req.IDs.IDs, false)
	if err != nil || len(remaining) != 0 {
		return 200, fmt.Errorf("%w: source-state absence not proved", ErrStateVerification)
	}
	for id, old := range linked {
		now, err := readMutationEntity(ctx, ac, old.Entity, id)
		if err != nil {
			return 200, fmt.Errorf("%w: linked entity read", ErrStateVerification)
		}
		if deleted[id] {
			if now != nil {
				return 200, fmt.Errorf("%w: reported deletion not observed", ErrStateVerification)
			}
		} else if now == nil || !reflect.DeepEqual(comparableEntity(old.Record), comparableEntity(now)) {
			return 200, fmt.Errorf("%w: retained linked entity changed", ErrStateVerification)
		}
	}
	evidence, _ := json.Marshal(map[string]any{"verified": true, "target_rows": after, "source_rows_absent": true, "deleted_ids": deleted, "linked_records_checked": true})
	if err = os.WriteFile(filepath.Join(dir, "verified.json"), evidence, 0600); err != nil {
		return 200, fmt.Errorf("%w: verification persistence", ErrStateVerification)
	}
	confirmMutation(ctx)
	return 200, nil
}
