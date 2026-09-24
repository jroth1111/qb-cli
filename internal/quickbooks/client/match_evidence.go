package client

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

// Match evidence contains transaction data only, never credentials or headers.
// A durable before-state is required before the mutation is submitted.
func startMatchEvidence(accountID string, rows, registers []map[string]any, targets []MatchTxn, body []byte) (string, error) {
	selected := make([]map[string]any, 0, len(targets))
	for _, t := range targets {
		selected = append(selected, findRegisterRow(registers, t.TxnID))
	}
	return startMutationEvidence("match", map[string]any{"account_id": accountID, "feed_rows": rows, "register_rows": selected}, body)
}

func startMutationEvidence(operation string, snapshot any, body []byte) (string, error) {
	root := filepath.Join(auth.HomeDir(), "mutation-evidence")
	if err := os.MkdirAll(root, 0700); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, operation+"-")
	if err != nil {
		return "", err
	}
	before, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(dir, "before.json"), before, 0600); err != nil {
		return "", err
	}
	if err = os.WriteFile(filepath.Join(dir, "request.json"), body, 0600); err != nil {
		return "", err
	}
	return dir, nil
}
