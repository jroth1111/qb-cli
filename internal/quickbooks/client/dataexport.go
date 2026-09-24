package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// DATA_EXPORT_GET — the Export Data dialog (/app/exportdata) posts one
// exportReport request per selected report/list on the neo gateway. Captured
// verbatim from the qbo-export-data-ui widget on TC2 2026-09-24:
//
//	POST https://c7.qbo.intuit.com/qbo7/neo/v1/company/{realm}/reports/exportReport
//	Authorization: Intuit_APIKey intuit_apikey=prdakyreshQ8k75GB5HDwp4VXWXelKFnuYlsomZC,…
//	intuit-company-id: {realm}
//	{"reportCustomizationAttributes":{"token":"PANDL",…},"reportFormat":"xlsx"}
//
//	→ 200 application/force-download; attachment; filename="<Company>_<TOKEN>.xlsx"
//	→ body is the xlsx file itself (PK zip)
//
// The widget fires eight POSTs (one per enabled switch): BAL_SHEET, GEN_LEDGER,
// JOURNAL, PANDL, TRIAL_BAL reports plus CUST_CONTACT, EMP_CONTACT, VEND_CONTACT
// lists. --token selects one; the request carries the same customization block
// the UI sends, with the token's own column layout.
const dataExportHost = "https://c7.qbo.intuit.com"

// exportTokens lists the report/list tokens the Export Data dialog offers.
var exportTokens = map[string]string{
	"BAL_SHEET":    "Balance Sheet",
	"GEN_LEDGER":   "General Ledger",
	"JOURNAL":      "Journal",
	"PANDL":        "Profit and Loss",
	"TRIAL_BAL":    "Trial Balance",
	"CUST_CONTACT": "Customer contact list",
	"EMP_CONTACT":  "Employee contact list",
	"VEND_CONTACT": "Supplier contact list",
}

// exportColumns carries the per-token column layout the widget posts. Financial
// reports share one layout (GEN_LEDGER adds a running balance column); contact
// lists have their own. Captured verbatim.
var exportColumns = map[string]string{
	"BAL_SHEET":    "~date:TxDate,~txn_type_label:TxTypeID,~doc_num:TxHeader/DocNum,~name_label:TxHeader/NameID,~memo_desc_label:MemoText,~account_id_label:AccountID,!~debit_label:Amount,!~credit_label:Amount,",
	"JOURNAL":      "~date:TxDate,~txn_type_label:TxTypeID,~doc_num:TxHeader/DocNum,~name_label:TxHeader/NameID,~memo_desc_label:MemoText,~account_id_label:AccountID,!~debit_label:Amount,!~credit_label:Amount,",
	"PANDL":        "~date:TxDate,~txn_type_label:TxTypeID,~doc_num:TxHeader/DocNum,~name_label:TxHeader/NameID,~memo_desc_label:MemoText,~account_id_label:AccountID,!~debit_label:Amount,!~credit_label:Amount,",
	"TRIAL_BAL":    "~date:TxDate,~txn_type_label:TxTypeID,~doc_num:TxHeader/DocNum,~name_label:TxHeader/NameID,~memo_desc_label:MemoText,~account_id_label:AccountID,!~debit_label:Amount,!~credit_label:Amount,",
	"GEN_LEDGER":   "~date:TxDate,~txn_type_label:TxTypeID,~doc_num:TxHeader/DocNum,~name_label:TxHeader/NameID,~memo_desc_label:MemoText,~account_id_label:AccountID,!~debit_label:Amount,!~credit_label:Amount,=~balance_label:HomeNaturalAmount,",
	"CUST_CONTACT": "~cs_customer_label:CustomerID,<~phone_numbers,Phone:PrimaryContact/Telephone1,Fax:PrimaryContact/Fax,Mobile:PrimaryContact/Mobile,Pager:PrimaryContact/Pager>,~email:BillingEmail,{~full_name,PrimaryContact/Salutation,PrimaryContact/FirstName,PrimaryContact/MiddleName,PrimaryContact/LastName,PrimaryContact/Suffix},<~billing_address,BillingContact/Address/AddressLines,{CSZ,BillingContact/Address/City,BillingContact/Address/State,BillingContact/Address/PostalCode},BillingContact/Address/Country>,<~shipping_address,ShippingContact/Address/AddressLines,{CSZ,ShippingContact/Address/City,ShippingContact/Address/State,ShippingContact/Address/PostalCode},ShippingContact/Address/Country>,",
	"EMP_CONTACT":  "~employee_label:EmployeeID,<~phone_numbers,Phone:PrimaryContact/Telephone1,Fax:PrimaryContact/Fax,Mobile:PrimaryContact/Mobile>,~email:PrimaryContact/Email,<~address_label,PrimaryContact/Address/AddressLines,{CSZ,PrimaryContact/Address/City,PrimaryContact/Address/State,PrimaryContact/Address/PostalCode},PrimaryContact/Address/Country>,",
	"VEND_CONTACT": "~vendor_name_id:VendorID,<~phone_numbers,Phone:PrimaryContact/Telephone1,Fax:PrimaryContact/Fax,Mobile:PrimaryContact/Mobile,Pager:PrimaryContact/Pager>,~email:PrimaryContact/Email,{~full_name,PrimaryContact/Salutation,PrimaryContact/FirstName,PrimaryContact/MiddleName,PrimaryContact/LastName,PrimaryContact/Suffix},<~address_label,BillingContact/Address/AddressLines,{CSZ,BillingContact/Address/City,BillingContact/Address/State,BillingContact/Address/PostalCode},BillingContact/Address/Country>,~account_number:AccountNumber,",
}

// DataExportResult reports where a downloaded export was written.
type DataExportResult struct {
	Status int    `json:"status"`
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	Token  string `json:"token"`
	Note   string `json:"note,omitempty"`
}

// PlannedDataExportURL is the dry-run URL for the export POST (realm-free).
func PlannedDataExportURL() string {
	return dataExportHost + "/qbo7/neo/v1/company/{realm}/reports/exportReport"
}

// ReplayDataExport POSTs one exportReport request for --token and writes the
// xlsx attachment to outPath. Read-semantics: the service renders a file and
// changes no company state.
func ReplayDataExport(ctx context.Context, token, outPath string) (*DataExportResult, error) {
	token = strings.ToUpper(strings.TrimSpace(token))
	if _, ok := exportTokens[token]; !ok {
		names := make([]string, 0, len(exportTokens))
		for k := range exportTokens {
			names = append(names, k)
		}
		return nil, fmt.Errorf("data export requires --token one of %s (got %q)", strings.Join(names, ", "), token)
	}
	if strings.TrimSpace(outPath) == "" {
		return nil, fmt.Errorf("data export requires --output path")
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	attrs := map[string]string{
		"token":                      token,
		"show_logo":                  "false",
		"date_macro":                 "all",
		"columns":                    exportColumns[token],
		"divideby1000":               "false",
		"hidecents":                  "false",
		"negativenums":               "1",
		"negativered":                "false",
		"show_header_title":          "true",
		"show_header_range":          "true",
		"show_footer_custom_message": "true",
		"show_footer_date":           "true",
		"show_footer_time":           "true",
		"header_alignment":           "Center",
		"footer_alignment":           "Center",
		"show_header_company":        "true",
		"footer_custom_message":      "",
	}
	body, err := json.Marshal(map[string]any{
		"reportCustomizationAttributes": attrs,
		"reportFormat":                  "xlsx",
	})
	if err != nil {
		return nil, err
	}
	u := fmt.Sprintf("%s/qbo7/neo/v1/company/%s/reports/exportReport", dataExportHost, ac.realm)
	// Fresh trace ids per request — the neo gateway treats replayed
	// intuit_tid/x-b3 ids as consumed nonces (proven on txnsrendering).
	resp, err := ac.doURIHost(ctx, http.MethodPost, u, "c7.qbo.intuit.com", body, freshTraceHeaders())
	if err != nil {
		return nil, fmt.Errorf("data export: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	if resp.StatusCode != http.StatusOK {
		raw, err := readBody(resp)
		if err != nil {
			return nil, err
		}
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	n, err := writeDownload(resp.Body, outPath, maxAttachmentBytes, false)
	if err != nil {
		return nil, fmt.Errorf("data export: %w", err)
	}
	return &DataExportResult{
		Status: resp.StatusCode,
		Path:   outPath,
		Bytes:  n,
		Token:  token,
		Note:   "neo POST reports/exportReport (" + exportTokens[token] + ")",
	}, nil
}
