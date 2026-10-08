package gql

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
)

var transactionLocalID = regexp.MustCompile(`^[1-9][0-9]*$`)

// Select only fields supplied by the current purchase forms. The broader
// discovery-catalog transaction fragment includes unrelated form contracts.
const nativePurchaseInspectionQuery = `query TxnQuery_qbo($id_0: ID!, $with: String) {
 node(id: $id_0, with: $with) {
  ... on Transactions_Transaction {
   id type entityVersion
   header {
    amount txnDate cleared privateMemo
    account { id }
    currencyInfo { code exchangeRate }
   }
   qboAppData { hasPurchaseTax txnTypeId }
   traits {
    payment { paymentType }
    tax { taxType totalTaxAmount totalTaxableAmount taxReclaimable taxGroup { id configType } }
   }
   lines { accountLines { edges { node {
    id sequence amount description
    account { id fullName }
    class { id fullyQualifiedName }
    traits {
     tax { taxable totalTaxAmount totalTaxableAmount taxInclusiveAmount taxGroup { id configType } }
     billable { billable salesAmt billableTo { id } }
    }
   } } } }
   stageEntity { edges { node {
    id entityVersion matchMode
    qboAppData {
     ruleId matchModeId matchModeDisplayName olbTxnType olbTxnTypeId
     olbPayee olbAmount sequence olbDisallowUnmatch externalMatch
    }
    transactionTrait { transaction { ... on Transactions_Transaction {
     id header { amount txnDate privateMemo currencyInfo { code } }
    } } }
   } } }
  }
 }
}`

// NativeTransactionInspection retains null and absent native tax values instead
// of converting them to false, zero or a clearing-state assertion.
type NativeTransactionInspection struct {
	Status                        int                               `json:"status"`
	LocalID                       string                            `json:"localId"`
	NodeID                        string                            `json:"nodeId"`
	EntityVersion                 string                            `json:"entityVersion"`
	Header                        json.RawMessage                   `json:"header"`
	Tax                           json.RawMessage                   `json:"tax"`
	QBOAppData                    json.RawMessage                   `json:"qboAppData"`
	AccountLines                  []json.RawMessage                 `json:"accountLines"`
	BankLinks                     []json.RawMessage                 `json:"bankLinks"`
	MatchControls                 []NativeMatchControl              `json:"matchControls"`
	RecordIdentityVerified        bool                              `json:"recordIdentityVerified"`
	ClearingStateRequiresRegister bool                              `json:"clearingStateRequiresRegister"`
	MetadataEffectsCertified      bool                              `json:"metadataEffectsCertified"`
	ObservationPresence           map[string]NativeObservationState `json:"observationPresence"`
}

type NativeMatchControl struct {
	MatchMode   string            `json:"matchMode"`
	UnmatchPath NativeUnmatchPath `json:"unmatchPath"`
}

type NativeUnmatchPath string
type NativeObservationState string

const (
	unmatchImmediate   NativeUnmatchPath      = "immediate_backend_unmatch"
	unmatchStageUpdate NativeUnmatchPath      = "stage_update_path"
	unmatchUnknown     NativeUnmatchPath      = "not_proven"
	observationAbsent  NativeObservationState = "absent"
	observationNull    NativeObservationState = "null"
	observationValue   NativeObservationState = "value"
)

func TransactionInspectionRequest(realm, localID string, credit bool) (Request, error) {
	if !transactionLocalID.MatchString(realm) || !transactionLocalID.MatchString(localID) {
		return Request{}, fmt.Errorf("transaction inspection requires numeric company and local transaction IDs")
	}
	op, err := Lookup("TxnQuery_qbo")
	if err != nil {
		return Request{}, err
	}
	if op.Kind != KindQuery {
		return Request{}, fmt.Errorf("transaction inspection operation is not read-only")
	}
	selected := *op
	selected.Document = nativePurchaseInspectionQuery
	filter := "txnType='PURCHASE'"
	if credit {
		filter = "type='PURCHASE' && paymentType='CREDIT_CARD_CREDIT' && txnType='PURCHASE'"
	}
	id := base64.RawURLEncoding.EncodeToString([]byte("v4.1:"+realm+":80271edd8a")) + ":" + localID
	return Request{Op: &selected, Variables: map[string]any{"id_0": id, "with": filter}}, nil
}

func InspectTransaction(ctx context.Context, realm, localID string, credit bool) (*NativeTransactionInspection, error) {
	req, err := TransactionInspectionRequest(realm, localID, credit)
	if err != nil {
		return nil, err
	}
	response, err := Execute(ctx, req)
	if err != nil {
		return nil, err
	}
	return ParseTransactionInspection(response, realm, localID, credit)
}

func ParseTransactionInspection(response *Response, realm, localID string, credit bool) (*NativeTransactionInspection, error) {
	req, err := TransactionInspectionRequest(realm, localID, credit)
	if err != nil {
		return nil, err
	}
	if response == nil || response.Status != 200 || len(response.Errors) != 0 {
		return nil, fmt.Errorf("native transaction read did not return a clean successful response")
	}
	var envelope struct {
		Data struct {
			Node struct {
				ID         string          `json:"id"`
				Type       string          `json:"type"`
				Version    string          `json:"entityVersion"`
				Header     json.RawMessage `json:"header"`
				QBOAppData json.RawMessage `json:"qboAppData"`
				Traits     struct {
					Tax     json.RawMessage `json:"tax"`
					Payment struct {
						Type string `json:"paymentType"`
					} `json:"payment"`
				} `json:"traits"`
				Lines struct {
					AccountLines struct {
						Edges []struct {
							Node json.RawMessage `json:"node"`
						} `json:"edges"`
					} `json:"accountLines"`
				} `json:"lines"`
				Stage struct {
					Edges []struct {
						Node json.RawMessage `json:"node"`
					} `json:"edges"`
				} `json:"stageEntity"`
			} `json:"node"`
		} `json:"data"`
		Errors []json.RawMessage `json:"errors"`
	}
	if err = json.Unmarshal(response.Body, &envelope); err != nil {
		return nil, fmt.Errorf("invalid native transaction response: %w", err)
	}
	node := envelope.Data.Node
	if len(envelope.Errors) != 0 || node.ID != req.Variables["id_0"] || node.Type != "PURCHASE" || node.Version == "" {
		return nil, fmt.Errorf("native transaction identity, type or version mismatch")
	}
	if credit && node.Traits.Payment.Type != "CREDIT_CARD_CREDIT" {
		return nil, fmt.Errorf("native transaction is not a credit-card credit")
	}
	if len(node.Header) == 0 || bytes.Equal(node.Header, []byte("null")) {
		return nil, fmt.Errorf("native transaction header missing")
	}
	var raw struct {
		Data struct {
			Node map[string]json.RawMessage `json:"node"`
		} `json:"data"`
	}
	if err = json.Unmarshal(response.Body, &raw); err != nil {
		return nil, fmt.Errorf("invalid native field envelope")
	}
	for _, key := range []string{"header", "traits", "qboAppData", "lines", "stageEntity"} {
		if _, present := raw.Data.Node[key]; !present {
			return nil, fmt.Errorf("native observation omitted requested field %s", key)
		}
	}
	var header map[string]json.RawMessage
	if json.Unmarshal(node.Header, &header) != nil || header == nil {
		return nil, fmt.Errorf("native transaction header is not an object")
	}
	for _, key := range []string{"amount", "txnDate", "cleared"} {
		if _, present := header[key]; !present {
			return nil, fmt.Errorf("native header omitted requested field %s", key)
		}
	}
	out := &NativeTransactionInspection{Status: 200, LocalID: localID, NodeID: node.ID, EntityVersion: node.Version,
		Header: node.Header, Tax: node.Traits.Tax, QBOAppData: node.QBOAppData,
		AccountLines: []json.RawMessage{}, BankLinks: []json.RawMessage{}, MatchControls: []NativeMatchControl{},
		RecordIdentityVerified: true, ClearingStateRequiresRegister: true}
	out.ObservationPresence = map[string]NativeObservationState{
		"headerCleared": nativeFieldPresence(header["cleared"]),
		"tax":           nativeFieldPresence(node.Traits.Tax),
		"stageEntity":   nativeFieldPresence(raw.Data.Node["stageEntity"]),
	}
	if out.ObservationPresence["stageEntity"] != "value" {
		out.BankLinks = nil
	}
	for _, edge := range node.Lines.AccountLines.Edges {
		if len(edge.Node) == 0 || bytes.Equal(edge.Node, []byte("null")) {
			return nil, fmt.Errorf("native transaction line is null")
		}
		out.AccountLines = append(out.AccountLines, edge.Node)
	}
	for _, edge := range node.Stage.Edges {
		if len(edge.Node) == 0 || bytes.Equal(edge.Node, []byte("null")) {
			return nil, fmt.Errorf("native bank link is null")
		}
		out.BankLinks = append(out.BankLinks, edge.Node)
		var link struct {
			Mode string `json:"matchMode"`
		}
		if err = json.Unmarshal(edge.Node, &link); err != nil {
			return nil, fmt.Errorf("invalid bank-match metadata")
		}
		control := NativeMatchControl{MatchMode: link.Mode, UnmatchPath: unmatchUnknown}
		switch link.Mode {
		case "MANUAL_MATCH", "AUTO_MATCH":
			control.UnmatchPath = unmatchImmediate
		case "MANUAL_ADD_AND_MATCH", "AUTO_ADD_AND_MATCH", "AUTO_ADD_AND_MATCH_RULE", "MANUAL_ADD_AND_MATCH_RULE":
			control.UnmatchPath = unmatchStageUpdate
		}
		out.MatchControls = append(out.MatchControls, control)
	}
	return out, nil
}

func nativeFieldPresence(value json.RawMessage) NativeObservationState {
	if len(value) == 0 {
		return observationAbsent
	}
	if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return observationNull
	}
	return observationValue
}
