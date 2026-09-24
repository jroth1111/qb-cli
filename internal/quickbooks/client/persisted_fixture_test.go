package client

import (
	"bytes"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
)

// withPersistedV3 is an explicit positive mock-server adapter. Writes persist
// the submitted fields independently of their coarse legacy receipt fixtures;
// subsequent GETs read that state. Negative readback tests deliberately do not
// use this adapter. It never intercepts production/network transports.
func withPersistedV3(next http.Handler) http.Handler {
	var mu sync.Mutex
	state := map[string]map[string]any{}
	deleted := map[string]bool{}
	queryID := regexp.MustCompile(`(?i)FROM\s+(\w+)\s+WHERE\s+Id\s*=\s*'([^']+)'`)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		path, id := "", ""
		if len(parts) >= 5 && parts[0] == "api" && parts[1] == "v3" {
			path = parts[4]
			if len(parts) > 5 {
				id = parts[5]
			}
		}
		entity := ""
		for name, p := range v3Path {
			if p == path {
				entity = name
				break
			}
		}
		key := entity + "/" + id
		if r.Method == "GET" && id != "" && state[key] != nil {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{entity: state[key]})
			return
		}
		if r.Method == "GET" && path == "query" {
			m := queryID.FindStringSubmatch(r.URL.Query().Get("query"))
			if len(m) == 3 {
				key = m[1] + "/" + m[2]
				if deleted[key] || state[key] != nil {
					rows := []any{}
					if !deleted[key] {
						rows = append(rows, state[key])
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"QueryResponse": map[string]any{m[1]: rows}})
					return
				}
			}
		}
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		recorder := httptest.NewRecorder()
		next.ServeHTTP(recorder, r)
		var env map[string]any
		_ = json.Unmarshal(recorder.Body.Bytes(), &env)
		if entity != "" && recorder.Code == 200 {
			obj, _ := env[entity].(map[string]any)
			if r.Method == "GET" && obj != nil {
				state[entity+"/"+jsonNumberString(obj["Id"])] = obj
			}
			if r.Method == "POST" && obj != nil {
				var submitted map[string]any
				_ = json.Unmarshal(raw, &submitted)
				id = jsonNumberString(obj["Id"])
				if id == "" {
					id = jsonNumberString(submitted["Id"])
				}
				key = entity + "/" + id
				if id != "" {
					stored := shallowCopyMap(state[key])
					if stored == nil {
						stored = map[string]any{}
					}
					for k, v := range submitted {
						if k != "sparse" {
							stored[k] = v
						}
					}
					stored["Id"] = id
					stored["SyncToken"] = "999"
					if r.URL.Query().Get("operation") == "void" {
						stored["status"] = "Voided"
						stored["TotalAmt"] = float64(0)
						for _, line := range sliceObjects(stored["Line"]) {
							line["Amount"] = float64(0)
						}
					}
					state[key] = stored
					if entity == "Payment" {
						for _, line := range sliceObjects(stored["Line"]) {
							for _, link := range sliceObjects(line["LinkedTxn"]) {
								linked := jsonNumberString(link["TxnType"]) + "/" + jsonNumberString(link["TxnId"])
								if state[linked] != nil {
									state[linked]["Balance"] = float64(0)
								}
							}
						}
					}
					deleted[key] = r.URL.Query().Get("operation") == "delete"
				}
			}
		}
		maps.Copy(w.Header(), recorder.Header())
		w.WriteHeader(recorder.Code)
		_, _ = w.Write(recorder.Body.Bytes())
	})
}
