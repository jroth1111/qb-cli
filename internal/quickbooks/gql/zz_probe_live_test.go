//go:build live

package gql

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestLiveGQLExec runs a named op with QB_GQL_VARS JSON against the live
// authenticated browser session for one-off mutation verification.
func TestLiveGQLExec(t *testing.T) {
	opName := os.Getenv("QB_GQL_OP")
	if opName == "" {
		t.Skip("set QB_GQL_OP")
	}
	op, err := Lookup(opName)
	if err != nil {
		t.Fatal(err)
	}
	var vars map[string]any
	if raw := os.Getenv("QB_GQL_VARS"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &vars); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	res, err := Execute(ctx, Request{Op: op, Variables: vars})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("status=%d errors=%v body=%s", res.Status, res.Errors, string(res.Body))
}
