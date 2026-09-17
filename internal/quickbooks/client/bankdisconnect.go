package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Bank feed disconnect rides the first-party neo account save, not the
// catalogued fitransactions BankingDisconnectOlbAccounts (which 403s in
// current AU builds). The UI posts the account's edit-form model plus
// disconnectAccount:true; the minimal proven body is the identity fields the
// neo GET returns — enum names resolve server-side from typeId/detailTypeId.
// Contract proven live on Test Company 2 (2026-09-16): GET lists/account/{id}
// then POST lists/account/save both return 200.

const neoListsAccount = "lists/account/"

// neoAccountSave is the fields the save endpoint needs for a disconnect;
// everything comes from GET lists/account/{id}.
type neoAccountSave struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	EditSequence string `json:"editSequence"`
	TypeID       string `json:"typeId"`
	DetailTypeID string `json:"detailTypeId"`
	CurrencyType string `json:"currencyType"`
	Disconnect   bool   `json:"disconnectAccount"`
}

// ReplayBankDisconnect disconnects the bank feed on each --id GL account by
// loading its neo account model and re-saving it with disconnectAccount:true.
func ReplayBankDisconnect(ctx context.Context, flags map[string]string) (*MutateResult, error) {
	rawIDs := strings.TrimSpace(firstFlag(flags, "id", "account-id"))
	ids := parseIDList(rawIDs)
	if len(ids) == 0 {
		return nil, fmt.Errorf("bank-disconnect requires --id (GL account id, repeatable or comma-separated)")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	base := "https://qbo.intuit.com/api/neo/v1/company/" + ac.realm + "/"
	var lastID string
	for _, id := range ids {
		if err := neoDisconnectOne(ctx, ac, base, id); err != nil {
			return nil, err
		}
		lastID = id
	}
	return &MutateResult{
		Status: http.StatusOK,
		Op:     "disconnect",
		Entity: "Account",
		Note:   fmt.Sprintf("neo lists/account/save disconnectAccount (%d account(s))", len(ids)),
		Item:   QueryItem{ID: lastID, Type: "Bank"},
	}, nil
}

// neoDisconnectOne loads the account's save model and reposts it with the
// disconnect flag. A 400/500 with a server message is surfaced verbatim.
func neoDisconnectOne(ctx context.Context, ac *apiClient, base, id string) error {
	getResp, err := ac.doURIHost(ctx, http.MethodGet, base+neoListsAccount+id, "qbo.intuit.com", nil)
	if err != nil {
		return fmt.Errorf("account %s neo fetch: %w", id, err)
	}
	raw, err := readBody(getResp)
	_ = drainAndClose(getResp)
	if err != nil {
		return fmt.Errorf("account %s neo fetch: %w", id, err)
	}
	if getResp.StatusCode != http.StatusOK {
		return &ReplayError{Status: getResp.StatusCode, Message: errorMessage(raw)}
	}
	var model struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		EditSequence string `json:"editSequence"`
		TypeID       string `json:"typeId"`
		DetailTypeID string `json:"detailTypeId"`
		CurrencyType struct {
			ISOCode string `json:"isoCode"`
		} `json:"currencyType"`
	}
	if err := json.Unmarshal(raw, &model); err != nil {
		return fmt.Errorf("account %s neo parse: %w", id, err)
	}
	if model.ID == "" || model.Name == "" {
		return fmt.Errorf("account %s neo model missing id/name", id)
	}
	body, err := json.Marshal(neoAccountSave{
		ID:           model.ID,
		Name:         model.Name,
		EditSequence: model.EditSequence,
		TypeID:       model.TypeID,
		DetailTypeID: model.DetailTypeID,
		CurrencyType: model.CurrencyType.ISOCode,
		Disconnect:   true,
	})
	if err != nil {
		return err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, base+neoListsAccount+"save", "qbo.intuit.com", body)
	if err != nil {
		return fmt.Errorf("account %s disconnect: %w", id, err)
	}
	sraw, err := readBody(resp)
	_ = drainAndClose(resp)
	if err != nil {
		return fmt.Errorf("account %s disconnect: %w", id, err)
	}
	if resp.StatusCode != http.StatusOK {
		return &ReplayError{Status: resp.StatusCode, Message: errorMessage(sraw)}
	}
	return nil
}

// PlannedBankDisconnectURL is the dry-run target surfaced by --dry-run plans.
func PlannedBankDisconnectURL() string {
	return "https://qbo.intuit.com/api/neo/v1/company/{realm}/lists/account/save"
}
