package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type batchIntent struct {
	entity, op, id string
	before, body   map[string]any
}

func prepareBatchReadback(ctx context.Context, ac *apiClient, items []any) (map[string]batchIntent, error) {
	intents := map[string]batchIntent{}
	targets := map[string]bool{}
	for _, item := range items {
		obj := item.(map[string]any)
		bid := obj["bId"].(string)
		if q, ok := obj["Query"].(string); ok {
			if obj["operation"] != nil || !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(q)), "SELECT ") {
				return nil, fmt.Errorf("batch Query must be an explicit SELECT without operation")
			}
			continue
		}
		op, _ := obj["operation"].(string)
		if op != "create" && op != "update" && op != "delete" {
			return nil, fmt.Errorf("%w: batch operation %q", ErrReadbackUnavailable, op)
		}
		in := batchIntent{op: op}
		for entity := range v3Path {
			if body, ok := obj[entity].(map[string]any); ok {
				if in.entity != "" {
					return nil, fmt.Errorf("ambiguous batch entity")
				}
				in.entity = entity
				in.body = body
			}
		}
		if in.entity == "" || in.entity == "SalesOrder" || in.entity == "RecurringTransaction" || in.entity == "Budget" {
			return nil, fmt.Errorf("%w: batch entity", ErrReadbackUnavailable)
		}
		if op != "create" {
			in.id = jsonNumberString(in.body["Id"])
			if in.id == "" {
				return nil, fmt.Errorf("batch mutation requires target Id")
			}
			key := in.entity + "/" + in.id
			if targets[key] {
				return nil, fmt.Errorf("batch repeats a mutation target; serialize dependent writes")
			}
			targets[key] = true
			var err error
			in.before, err = fetchV3(ctx, ac, v3Path[in.entity], in.id)
			if err != nil {
				return nil, err
			}
			if jsonNumberString(in.before["Id"]) != in.id || in.body["SyncToken"] == nil || in.body["SyncToken"] != in.before["SyncToken"] {
				return nil, fmt.Errorf("batch preflight identity/version changed; no writes submitted")
			}
			if memo, ok := in.body["PrivateNote"].(string); ok && in.entity == "Purchase" {
				previous, _ := in.before["PrivateNote"].(string)
				merged, err := preserveManagedMemo(previous, memo)
				if err != nil {
					return nil, err
				}
				in.body["PrivateNote"] = merged
			}
		}
		intents[bid] = in
	}
	return intents, nil
}

func verifyBatchReadback(ctx context.Context, ac *apiClient, items []any, intents map[string]batchIntent, requested map[string]bool, evidence string) error {
	var failures []error
	if len(items) != len(requested) {
		failures = append(failures, fmt.Errorf("batch receipt count mismatch"))
	}
	seen := map[string]bool{}
	targets := map[string]bool{}
	for index, item := range items {
		obj, _ := item.(map[string]any)
		bid, _ := obj["bId"].(string)
		if !requested[bid] || seen[bid] {
			failures = append(failures, fmt.Errorf("batch receipt identity mismatch"))
			continue
		}
		seen[bid] = true
		if fault, ok := obj["Fault"]; ok && fault != nil {
			failures = append(failures, fmt.Errorf("batch item %s returned a Fault", bid))
			continue
		}
		in, mutation := intents[bid]
		if !mutation {
			if _, ok := obj["QueryResponse"].(map[string]any); !ok {
				failures = append(failures, fmt.Errorf("batch query item %s lacks QueryResponse", bid))
			}
			continue
		}
		receipt, _ := obj[in.entity].(map[string]any)
		id := jsonNumberString(receipt["Id"])
		if id == "" || in.id != "" && in.id != id {
			failures = append(failures, fmt.Errorf("batch item %s has wrong entity identity", bid))
			continue
		}
		key := in.entity + "/" + id
		if targets[key] {
			failures = append(failures, fmt.Errorf("batch items share one entity identity"))
			continue
		}
		targets[key] = true
		dir := filepath.Join(evidence, strconv.Itoa(index))
		if err := os.MkdirAll(dir, 0700); err != nil {
			failures = append(failures, err)
			continue
		}
		if err := verifyV3Readback(ctx, ac, in.entity, in.op, id, in.before, in.body, dir); err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%w: %v (evidence %s)", ErrMutationUnverified, errors.Join(failures...), evidence)
	}
	return nil
}

func batchEvidence(items []any) (string, error) {
	raw, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return startMutationEvidence("v3-batch", map[string]any{"items": len(items)}, raw)
}
