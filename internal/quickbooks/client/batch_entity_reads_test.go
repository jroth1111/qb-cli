package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBatchEntityQueryRequiresExactIdentitySet(t *testing.T) {
	for _, scenario := range []string{"complete", "missing", "duplicate", "extra"} {
		t.Run(scenario, func(t *testing.T) {
			saveUsable(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" || r.URL.Query().Get("query") != "SELECT * FROM Purchase WHERE Id IN ('1','2') MAXRESULTS 1000" {
					t.Error("query changed")
				}
				rows := []map[string]any{{"Id": "1", "Line": []any{map[string]any{"Description": "retain"}}}, {"Id": "2"}}
				switch scenario {
				case "missing":
					rows = rows[:1]
				case "duplicate":
					rows[1]["Id"] = "1"
				case "extra":
					rows = append(rows, map[string]any{"Id": "3"})
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"QueryResponse": map[string]any{"Purchase": rows}})
			}))
			defer server.Close()
			interceptHTTP(t, server.URL)
			ac, err := newAPIClient()
			if err != nil {
				t.Fatal(err)
			}
			got, err := readBatchEntities(context.Background(), ac, map[string][]string{"Purchase": {"2", "1"}})
			if scenario == "complete" {
				if err != nil || len(got) != 2 || got["Purchase/1"]["Line"] == nil {
					t.Fatalf("lost records: %v", err)
				}
			} else if err == nil {
				t.Fatal("accepted ambiguous or incomplete independent query")
			}
		})
	}
}

func TestIndependentBatchSnapshotStillChecksActualClass(t *testing.T) {
	for _, klass := range []string{"parent", "wrong"} {
		after := map[string]any{"Id": "1", "Line": []any{map[string]any{"Id": "1", "AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "90"}, "ClassRef": map[string]any{"value": klass}}}}}
		want := map[string]any{"Id": "1", "sparse": true, "Line": []any{map[string]any{"Id": "1", "AccountBasedExpenseLineDetail": map[string]any{"AccountRef": map[string]any{"value": "90"}, "ClassRef": map[string]any{"value": "parent"}}}}}
		ctx := context.WithValue(context.Background(), entityReadCacheKey{}, map[string]map[string]any{"Purchase/1": after})
		err := verifyV3Readback(ctx, nil, "Purchase", "update", "1", nil, want, t.TempDir())
		if klass == "parent" && err != nil {
			t.Fatal(err)
		}
		if klass == "wrong" && !errors.Is(err, ErrMutationUnverified) {
			t.Fatal("wrong Class accepted from bulk query")
		}
	}
}

func TestBrowserBatchOptimisationExcludesDeleteAndMixedEntities(t *testing.T) {
	t.Setenv("QB_HTTP_TRANSPORT", "ego")
	for _, op := range []string{"create", "delete"} {
		items := []any{map[string]any{"operation": op, "Purchase": map[string]any{"Id": "1"}}, map[string]any{"operation": op, "Purchase": map[string]any{"Id": "2"}}}
		if browserBatchTargets(items, false) != nil {
			t.Fatal("optimisation accepted unsupported operation")
		}
	}
}
