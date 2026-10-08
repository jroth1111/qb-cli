package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/proof"
)

var ErrMutationUnverified = errors.New("mutation outcome unverified; do not replay the write, use read-only recovery")
var ErrReadbackUnavailable = errors.New("mutation blocked before submission: independent readback contract unavailable")

// normalizedJSON makes typed map/slice builders comparable to decoded JSON.
func normalizedJSON(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// expectedFields checks the submitted fields, not the echoed write response.
// Server additions are allowed, but explicit lines/links cannot disappear or
// expand. Existing line IDs drive matching when present, never array position.
func expectedFields(want, got any, path string) error {
	switch w := want.(type) {
	case map[string]any:
		g, ok := got.(map[string]any)
		if !ok {
			return fmt.Errorf("%s shape differs", path)
		}
		if strings.Contains(path, ".Line") && w["ClassRef"] == nil && g["ClassRef"] != nil {
			return fmt.Errorf("%s retained an unintended ClassRef", path)
		}
		keys := make([]string, 0, len(w))
		for k := range w {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "SyncToken" || k == "MetaData" || k == "sparse" || k == "domain" {
				continue
			}
			// Ref display labels may be hydrated or renamed; identity is value.
			if k == "name" && w["value"] != nil {
				continue
			}
			actual, exists := g[k]
			if !exists {
				if w[k] == nil || w[k] == "" {
					continue
				}
				// The native service may omit an empty extension collection when
				// converting a payment type. Nonempty extensions remain mandatory.
				if extensions, ok := w[k].([]any); k == "CustomExtensions" && ok && len(extensions) == 0 {
					continue
				}
				return fmt.Errorf("%s.%s missing", path, k)
			}
			if err := expectedFields(w[k], actual, path+"."+k); err != nil {
				return err
			}
		}
	case []any:
		g, ok := got.([]any)
		if !ok || len(w) != len(g) {
			return fmt.Errorf("%s row count differs", path)
		}
		used := map[int]bool{}
		for i, v := range w {
			index := i
			if m, ok := v.(map[string]any); ok && m["Id"] != nil {
				index = -1
				for j, a := range g {
					if am, ok := a.(map[string]any); ok && am["Id"] == m["Id"] {
						if index != -1 {
							return fmt.Errorf("%s duplicate line identity", path)
						}
						index = j
					}
				}
			} else if m, ok := v.(map[string]any); ok && strings.HasSuffix(path, ".AttachableRef") {
				ref, ok := m["EntityRef"].(map[string]any)
				if !ok || ref["type"] == nil || ref["value"] == nil {
					return fmt.Errorf("%s incomplete attachment link identity", path)
				}
				index = -1
				for j, a := range g {
					am, _ := a.(map[string]any)
					actual, _ := am["EntityRef"].(map[string]any)
					if actual["type"] == ref["type"] && actual["value"] == ref["value"] {
						if index != -1 {
							return fmt.Errorf("%s duplicate attachment link identity", path)
						}
						index = j
					}
				}
			}
			if index < 0 || used[index] {
				return fmt.Errorf("%s line identity missing", path)
			}
			used[index] = true
			if err := expectedFields(v, g[index], fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	default:
		if !reflect.DeepEqual(want, got) {
			return fmt.Errorf("%s differs", path)
		}
	}
	return nil
}

func preservedMapFields(before, submitted, after map[string]any, path string) error {
	if oldRef, ok := before["value"]; ok && submitted["value"] != nil && oldRef != submitted["value"] {
		return nil
	}
	for key, old := range before {
		if key == "SyncToken" || key == "MetaData" || key == "domain" || key == "sparse" || (key == "name" && before["value"] != nil) {
			continue
		}
		if want, changed := submitted[key]; changed {
			oldMap, okOld := old.(map[string]any)
			wantMap, okWant := want.(map[string]any)
			afterMap, okAfter := after[key].(map[string]any)
			if okOld && okWant && okAfter {
				if err := preservedMapFields(oldMap, wantMap, afterMap, path+"."+key); err != nil {
					return err
				}
			}
			continue
		}
		if err := expectedFields(old, after[key], path+"."+key); err != nil {
			return err
		}
	}
	return nil
}

func verifyV3Readback(ctx context.Context, ac *apiClient, entity, op, id string, before, submitted map[string]any, evidence string) error {
	fail := func(err error) error {
		return fmt.Errorf("%w: %s/%s: %v (evidence %s)", ErrMutationUnverified, entity, id, err, evidence)
	}
	if id == "" {
		return fail(fmt.Errorf("receipt lacks target ID"))
	}
	if op == "delete" {
		after, err := readMutationEntity(ctx, ac, entity, id)
		if err != nil {
			return fail(err)
		}
		if after != nil {
			return fail(fmt.Errorf("deleted entity is still present"))
		}
		return persistReadback(evidence, map[string]any{"entity": entity, "id": id, "absent": true})
	}
	path := v3Path[entity]
	if path == "" {
		return fail(ErrReadbackUnavailable)
	}
	var after map[string]any
	var err error
	if bulk, ok := ctx.Value(entityReadCacheKey{}).(map[string]map[string]any); ok {
		after = bulk[entity+"/"+id]
		if after == nil {
			return fail(fmt.Errorf("independent batch entity query omitted target"))
		}
	} else {
		after, err = fetchV3(ctx, ac, path, id)
	}
	if err != nil {
		return fail(err)
	}
	if jsonNumberString(after["Id"]) != id {
		return fail(fmt.Errorf("independent entity identity mismatch"))
	}
	if err := persistReadback(evidence, after); err != nil {
		return fail(err)
	}
	want := normalizedJSON(submitted).(map[string]any)
	if op == "void" {
		// A token/id receipt alone cannot prove a void. Require an explicit
		// native marker, or all original monetary lines and total at zero.
		zero, ok := after["TotalAmt"].(float64)
		voided := strings.EqualFold(fmt.Sprint(after["status"]), "Voided") || strings.EqualFold(fmt.Sprint(after["TxnStatus"]), "Voided")
		if !voided && (!ok || zero != 0 || before == nil || before["SyncToken"] == after["SyncToken"]) {
			return fail(fmt.Errorf("void not independently observed"))
		}
		if !voided {
			for _, v := range sliceObjects(after["Line"]) {
				if amount, ok := v["Amount"].(float64); ok && amount != 0 {
					return fail(fmt.Errorf("void retained a nonzero line"))
				}
			}
		}
	} else {
		if err := expectedFields(want, after, entity); err != nil {
			return fail(err)
		}
	}
	// Preserve fields not explicitly changed; ignore server-maintained counters
	// and derived amounts only when the request changed monetary lines.
	if op != "create" && before != nil {
		old := normalizedJSON(before).(map[string]any)
		for k, v := range old {
			// This is a newly signed download capability, not persisted file
			// identity. Document ID, filename, size, type, note and links below
			// still require exact preservation.
			if entity == "Attachable" && k == "TempDownloadUri" {
				continue
			}
			if _, changed := want[k]; changed {
				oldMap, oldOK := v.(map[string]any)
				wantMap, wantOK := want[k].(map[string]any)
				afterMap, afterOK := after[k].(map[string]any)
				if oldOK && wantOK && afterOK {
					if err := preservedMapFields(oldMap, wantMap, afterMap, entity+".preserved."+k); err != nil {
						return fail(err)
					}
				}
				continue
			}
			if k == "SyncToken" || k == "MetaData" || k == "domain" || k == "sparse" {
				continue
			}
			if k == "FullyQualifiedName" && (want["Name"] != nil || want["ParentRef"] != nil) {
				continue
			}
			if op == "void" && (k == "Line" || k == "TotalAmt" || k == "Balance" || k == "TxnStatus" || k == "status" || k == "PrivateNote") {
				continue
			}
			if want["Line"] != nil && (k == "TotalAmt" || k == "Balance" || k == "HomeTotalAmt" || k == "TxnTaxDetail") {
				continue
			}
			if err := expectedFields(v, after[k], entity+".preserved."+k); err != nil {
				return fail(err)
			}
		}
	}
	return nil
}

func sliceObjects(v any) []map[string]any {
	var out []map[string]any
	rows, _ := normalizedJSON(v).([]any)
	for _, row := range rows {
		if m, ok := row.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func persistReadback(dir string, value any) error {
	if dir == "" {
		return nil
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "readback.json"), b, 0600)
}

func confirmMutation(ctx context.Context)    { proof.Confirm(ctx) }
func submittingMutation(ctx context.Context) { proof.Submitted(ctx) }
