package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
)

type entityReadCacheKey struct{}

// Browser round trips are expensive. Native ID-filtered queries retain the
// exact target set and full entities without weakening per-record verification.
func readBatchEntities(ctx context.Context, ac *apiClient, targets map[string][]string) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	for _, entity := range []string{"Purchase", "Deposit"} {
		ids := targets[entity]
		if len(ids) == 0 {
			continue
		}
		wanted := map[string]bool{}
		quoted := []string{}
		for _, id := range ids {
			if !mutationEntityID.MatchString(id) || wanted[id] {
				return nil, fmt.Errorf("invalid or duplicate batch query target")
			}
			wanted[id] = true
			quoted = append(quoted, "'"+id+"'")
		}
		sort.Strings(quoted)
		query := "SELECT * FROM " + entity + " WHERE Id IN (" + strings.Join(quoted, ",") + ") MAXRESULTS 1000"
		u := "https://qbo.intuit.com/api/v3/company/" + ac.realm + "/query?query=" + url.QueryEscape(query) + "&minorversion=73"
		resp, err := ac.getJSON(ctx, u, "")
		if err != nil {
			return nil, err
		}
		raw, err := readBody(resp)
		_ = drainAndClose(resp)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("batch entity query HTTP %d", resp.StatusCode)
		}
		var body struct {
			QueryResponse map[string]json.RawMessage `json:"QueryResponse"`
		}
		if err = json.Unmarshal(raw, &body); err != nil {
			return nil, err
		}
		var records []map[string]any
		if err = json.Unmarshal(body.QueryResponse[entity], &records); err != nil {
			return nil, fmt.Errorf("batch entity query missing %s records", entity)
		}
		seen := map[string]bool{}
		for _, record := range records {
			id := jsonNumberString(record["Id"])
			if !wanted[id] || seen[id] {
				return nil, fmt.Errorf("batch entity query identity mismatch")
			}
			seen[id] = true
			out[entity+"/"+id] = record
		}
		if len(seen) != len(wanted) {
			return nil, fmt.Errorf("batch entity query did not return every target")
		}
	}
	return out, nil
}

func browserBatchTargets(items []any, receipts bool) map[string][]string {
	if os.Getenv("QB_HTTP_TRANSPORT") != "ego" {
		return nil
	}
	targets := map[string][]string{}
	count := 0
	for _, item := range items {
		obj, _ := item.(map[string]any)
		if obj["Query"] != nil || receipts && obj["QueryResponse"] != nil {
			continue
		}
		if !receipts && obj["operation"] != "update" {
			return nil
		}
		found := false
		for _, entity := range []string{"Purchase", "Deposit"} {
			if body, ok := obj[entity].(map[string]any); ok {
				id := jsonNumberString(body["Id"])
				if id == "" {
					return nil
				}
				targets[entity] = append(targets[entity], id)
				count++
				found = true
			}
		}
		if !found {
			return nil
		}
	}
	if count < 2 {
		return nil
	}
	return targets
}
