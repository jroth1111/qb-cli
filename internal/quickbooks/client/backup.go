package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// BACKUP_CREATE rides backuprestore.api.intuit.com — the Online Backup and
// Restore service behind /app/bnr-backup. Captured 2026-09-18 on TC2 from
// "Run manual backup" → Incremental → Back up:
//
//	POST https://backuprestore.api.intuit.com/v1/backup
//	{"data":{"backupType":"incremental","companyRealmId":"<realm>"}}
//
// The dialog offers incremental|full|complete; the host authenticates with
// the harvested Intuit_APIKey Authorization plus the app's access-token /
// token-type / uid / expiry / client header set (carried verbatim from the
// stored host map). A successful POST queues a Manual row on the Backups
// tab — non-destructive; it writes a restore point, not company data.
const backupHost = "backuprestore.api.intuit.com"

var backupTypes = map[string]bool{"incremental": true, "full": true, "complete": true}

func PlannedBackupURL() string { return "https://backuprestore.api.intuit.com/v1/backup" }

// ReplayBackupCreate queues a manual backup. backupType is one of
// incremental (default), full, complete — matching the UI's radio options.
func ReplayBackupCreate(ctx context.Context, backupType string) (*MutateResult, error) {
	backupType = strings.ToLower(strings.TrimSpace(backupType))
	if backupType == "" {
		backupType = "incremental"
	}
	if !backupTypes[backupType] {
		return nil, fmt.Errorf("backup --type %q: want incremental | full | complete", backupType)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if ac.realm == "" {
		return nil, fmt.Errorf("backup create: no realm id in credentials")
	}
	body, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"backupType":     backupType,
			"companyRealmId": ac.realm,
		},
	})
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, PlannedBackupURL(), backupHost, body)
	if err != nil {
		return nil, fmt.Errorf("backup create: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading backup create: %w", err)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	name := backupType + " backup"
	var out struct {
		Data struct {
			ID         string `json:"id"`
			BackupID   string `json:"backupId"`
			BackupType string `json:"backupType"`
			Status     string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err == nil {
		if id := out.Data.ID; id != "" {
			name = id
		} else if out.Data.BackupID != "" {
			name = out.Data.BackupID
		}
		if out.Data.Status != "" {
			name += " (" + out.Data.Status + ")"
		}
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "backup create",
		Entity: "Backup",
		Item:   QueryItem{Type: "Backup", ID: ac.realm, Name: name},
		Note:   "backuprestore POST /v1/backup type=" + backupType,
	}, nil
}
