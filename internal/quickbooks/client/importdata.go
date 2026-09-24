package client

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// CUSTOMERS_IMPORT and SUPPLIERS_IMPORT ride the same-origin neo BFF —
// captured 2026-09-18 on TC2 from /app/importdata/{Customers,Vendors}
// (upload → map → review → Done/Import):
//
//	POST https://qbo.intuit.com/api/neo/v1/company/<realm>/importdata/import?entity=Customers|Vendors
//	{"entityInfo":"<json: {custlist:[{…fields…}],invalidRows,numRows,totalRows}>",
//	 "userMappings":"<json: {numMappings, mappingInfo:{QBOField:[{colNum|colNum:'none',…}]}}>"}
//
// The CSV is parsed client-side by the real wizard too — the wire carries
// mapped records, not the file. The two wizards share the 18-field
// mappingInfo verbatim; only the entity name, the custlist tax key
// (customerTaxIdNumber vs vendorTaxIdNumber), and the Referer differ.
const importDataHost = "qbo.intuit.com"

func plannedImportDataURL(realm, entity string) string {
	return "https://qbo.intuit.com/api/neo/v1/company/" + realm + "/importdata/import?entity=" + entity
}

// importDataField is one QBO wizard field in canonical sortIndex order. The
// unmapped descriptor mirrors the captured wire entry verbatim.
type importDataField struct {
	qboName   string // mappingInfo key
	colHeader string // label shown in the wizard
	importKey string // custlist key
	sortIndex int
	extra     map[string]any // regex/maxLength for unmapped entries
}

// importDataFields is the shared Customers/Vendors field set, verbatim from
// the captured userMappings block (identical on both wizards).
var importDataFields = []importDataField{
	{qboName: "Name", colHeader: "Name", importKey: "name", sortIndex: 0},
	{qboName: "Company", colHeader: "Company", importKey: "company", sortIndex: 1, extra: map[string]any{"regex": map[string]any{}, "maxLength": 50}},
	{qboName: "Email", colHeader: "Email", importKey: "email", sortIndex: 2},
	{qboName: "Phone", colHeader: "Phone", importKey: "phone", sortIndex: 3, extra: map[string]any{"regex": map[string]any{}, "maxLength": 21}},
	{qboName: "Mobile", colHeader: "Mobile", importKey: "mobile", sortIndex: 4, extra: map[string]any{"regex": map[string]any{}, "maxLength": 21}},
	{qboName: "Fax", colHeader: "Fax", importKey: "fax", sortIndex: 5, extra: map[string]any{"regex": map[string]any{}, "maxLength": 21}},
	{qboName: "Website", colHeader: "Website", importKey: "website", sortIndex: 6, extra: map[string]any{"regex": nil, "maxLength": 1000}},
	{qboName: "BillingAddressStreet", colHeader: "Street", importKey: "street", sortIndex: 7, extra: map[string]any{"regex": map[string]any{}, "maxLength": 2000}},
	{qboName: "BillingAddressCity", colHeader: "City", importKey: "city", sortIndex: 8, extra: map[string]any{"regex": map[string]any{}, "maxLength": 255}},
	{qboName: "BillingAddressState", colHeader: "Province/Region/State", importKey: "state", sortIndex: 9, extra: map[string]any{"regex": map[string]any{}, "maxLength": 255}},
	{qboName: "BillingAddressZip", colHeader: "Postcode", importKey: "zip", sortIndex: 10, extra: map[string]any{"regex": map[string]any{}, "maxLength": 30}},
	{qboName: "BillingAddressCountry", colHeader: "Country", importKey: "country", sortIndex: 11, extra: map[string]any{"regex": map[string]any{}, "maxLength": 255}},
	{qboName: "OpeningBalance", colHeader: "Opening Balance", importKey: "openingBalance", sortIndex: 12, extra: map[string]any{"regex": `^(?:(?:(?:\$[\s ]*)?((?:(?:(?:(?:0|[1-9]\d{0,2}(?:[,]\d{3})*)|(?:\d+))(?:\.\d+)?)|(?:(?:\.\d+)?))))|(?:(?:\$[\s ]*)?\-((?:(?:(?:(?:0|[1-9]\d{0,2}(?:[,]\d{3})*)|(?:\d+))(?:\.\d+)?)|(?:(?:\.\d+)?)))))$`, "maxLength": 17}},
	{qboName: "OpeningBalanceDate", colHeader: "Opening Balance Date", importKey: "openingBalanceDate", sortIndex: 13, extra: map[string]any{"regex": map[string]any{}, "maxLength": 11}},
	{qboName: "CustomerTaxIdNumber", colHeader: "Tax Reg. No.", importKey: "customerTaxIdNumber", sortIndex: 14, extra: map[string]any{"regex": map[string]any{}, "maxLength": 20}},
	{qboName: "CustomerTaxResaleNumber", colHeader: "Tax Resale No.", importKey: "customerTaxResaleNumber", sortIndex: 15, extra: map[string]any{"regex": map[string]any{}, "maxLength": 16}},
	{qboName: "VendorTaxIdNumber", colHeader: "Tax Id No.", importKey: "vendorTaxIdNumber", sortIndex: 16, extra: map[string]any{"regex": map[string]any{}, "maxLength": 20}},
	{qboName: "Currency", colHeader: "Currency", importKey: "currency", sortIndex: 17, extra: map[string]any{"regex": nil}},
}

// importDataHeaderAliases maps CSV column headers (case-insensitive) to QBO
// field names — the wizard's own auto-match vocabulary plus common labels.
var importDataHeaderAliases = map[string]string{
	"name": "Name", "customer name": "Name", "customer": "Name", "display name": "Name",
	"vendor name": "Name", "supplier name": "Name", "supplier": "Name", "vendor": "Name",
	"company": "Company", "company name": "Company",
	"email": "Email", "email address": "Email", "e-mail": "Email",
	"phone": "Phone", "phone number": "Phone", "telephone": "Phone",
	"mobile": "Mobile", "mobile number": "Mobile",
	"fax": "Fax", "website": "Website", "web site": "Website",
	"street": "BillingAddressStreet", "address": "BillingAddressStreet", "billing street": "BillingAddressStreet", "billing address": "BillingAddressStreet",
	"city": "BillingAddressCity", "billing city": "BillingAddressCity",
	"state": "BillingAddressState", "province": "BillingAddressState", "region": "BillingAddressState", "province/region/state": "BillingAddressState", "billing state": "BillingAddressState",
	"zip": "BillingAddressZip", "postcode": "BillingAddressZip", "postal code": "BillingAddressZip", "post code": "BillingAddressZip",
	"country": "BillingAddressCountry", "billing country": "BillingAddressCountry",
	"opening balance": "OpeningBalance", "openingbalance": "OpeningBalance",
	"opening balance date": "OpeningBalanceDate",
	"tax reg. no.":         "CustomerTaxIdNumber", "tax reg no": "CustomerTaxIdNumber", "tax id": "CustomerTaxIdNumber",
	"tax resale no.": "CustomerTaxResaleNumber", "tax resale no": "CustomerTaxResaleNumber",
	"tax id no.": "VendorTaxIdNumber", "tax id no": "VendorTaxIdNumber",
	"currency": "Currency",
}

// coaImportFields is the Chart of Accounts field set, verbatim from the
// captured /app/importdata/coa userMappings block. sortIndex 5 is absent on
// the wire (no field occupies it in this wizard variant).
var coaImportFields = []importDataField{
	{qboName: "Number", colHeader: "Account Number", importKey: "number", sortIndex: 0},
	{qboName: "Name", colHeader: "Account Name", importKey: "name", sortIndex: 1},
	{qboName: "Type", colHeader: "Type", importKey: "type", sortIndex: 2},
	{qboName: "Detail Type", colHeader: "Detail Type", importKey: "detailType", sortIndex: 3},
	{qboName: "Currency", colHeader: "Currency", importKey: "currency", sortIndex: 4, extra: map[string]any{"regex": nil}},
	{qboName: "OpeningBalance", colHeader: "Opening Balance", importKey: "openingBalance", sortIndex: 6, extra: map[string]any{"regex": `^(?:(?:(?:\$[\s ]*)?((?:(?:(?:(?:0|[1-9]\d{0,2}(?:[,]\d{3})*)|(?:\d+))(?:\.\d+)?)|(?:(?:\.\d+)?))))|(?:(?:\$[\s ]*)?\-((?:(?:(?:(?:0|[1-9]\d{0,2}(?:[,]\d{3})*)|(?:\d+))(?:\.\d+)?)|(?:(?:\.\d+)?)))))$`, "maxLength": 17}},
	{qboName: "OpeningBalanceDate", colHeader: "Opening Balance Date", importKey: "openingBalanceDate", sortIndex: 7, extra: map[string]any{"regex": map[string]any{}, "maxLength": 11}},
}

// coaImportHeaderAliases maps COA CSV column headers to QBO field names.
var coaImportHeaderAliases = map[string]string{
	"account name": "Name", "name": "Name", "account": "Name",
	"account number": "Number", "number": "Number", "acct number": "Number", "acc number": "Number",
	"type": "Type", "account type": "Type",
	"detail type": "Detail Type", "detailtype": "Detail Type", "detail": "Detail Type",
	"currency":        "Currency",
	"opening balance": "OpeningBalance", "openingbalance": "OpeningBalance", "balance": "OpeningBalance",
	"opening balance date": "OpeningBalanceDate",
}

// importDataSpec is one entity's import contract — the neo wizard shares
// the endpoint and envelope; fields, aliases, custlist keys, required
// columns, entity name, and Referer differ per wizard.
type importDataSpec struct {
	entity   string
	noun     string   // for error messages
	listKeys []string // custlist record keys in captured order
	referer  string
	fields   []importDataField
	aliases  map[string]string
	required []string // qboName fields that must map to a CSV column
}

// custlistBaseKeys is the shared key order before the tax/currency tail.
var custlistBaseKeys = []string{
	"email", "name", "company", "phone", "mobile", "fax", "website",
	"street", "city", "state", "zip", "country",
	"openingBalance", "openingBalanceDate",
}

var customerImportSpec = importDataSpec{
	entity: "Customers",
	noun:   "customers",
	listKeys: append(append([]string{}, custlistBaseKeys...),
		"customerTaxIdNumber", "currency"),
	referer:  "https://qbo.intuit.com/app/importdata/Customers",
	fields:   importDataFields,
	aliases:  importDataHeaderAliases,
	required: []string{"Name"},
}

var vendorImportSpec = importDataSpec{
	entity: "Vendors",
	noun:   "suppliers",
	listKeys: append(append([]string{}, custlistBaseKeys...),
		"vendorTaxIdNumber", "currency"),
	referer:  "https://qbo.intuit.com/app/importdata/Vendors",
	fields:   importDataFields,
	aliases:  importDataHeaderAliases,
	required: []string{"Name"},
}

var coaImportSpec = importDataSpec{
	entity:   "coa",
	noun:     "chart of accounts",
	listKeys: []string{"type", "number", "detailType", "name", "currency", "openingBalance", "openingBalanceDate"},
	referer:  "https://qbo.intuit.com/app/importdata/coa",
	fields:   coaImportFields,
	aliases:  coaImportHeaderAliases,
	required: []string{"Name", "Type"},
}

// PlannedImportDataURL reports the mutation URL for dry-run plans.
func PlannedImportDataURL(entity string) string {
	return "https://qbo.intuit.com/api/neo/v1/company/<realm>/importdata/import?entity=" + entity
}

// ReplayCustomersImport imports a CSV through the Customers wizard contract.
func ReplayCustomersImport(ctx context.Context, path string) (*MutateResult, error) {
	return replayImportData(ctx, customerImportSpec, path)
}

// ReplaySuppliersImport imports a CSV through the Vendors wizard contract.
func ReplaySuppliersImport(ctx context.Context, path string) (*MutateResult, error) {
	return replayImportData(ctx, vendorImportSpec, path)
}

// ReplayCoaImport imports a CSV through the Chart of Accounts wizard
// contract (/app/importdata/coa, entity=coa).
func ReplayCoaImport(ctx context.Context, path string) (*MutateResult, error) {
	return replayImportData(ctx, coaImportSpec, path)
}

// fieldColHeader returns the wizard's column label for a QBO field name.
func fieldColHeader(spec importDataSpec, qboName string) string {
	for i := range spec.fields {
		if spec.fields[i].qboName == qboName {
			return spec.fields[i].colHeader
		}
	}
	return qboName
}

// requiredHint joins required column labels for error messages.
func requiredHint(spec importDataSpec) string {
	labels := make([]string, 0, len(spec.required))
	for _, req := range spec.required {
		labels = append(labels, fieldColHeader(spec, req))
	}
	return strings.Join(labels, ", ")
}

// replayImportData parses a CSV, maps its columns onto the wizard field
// set, and posts the captured contract the wizard sends on the last step.
func replayImportData(ctx context.Context, spec importDataSpec, path string) (*MutateResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("%s import requires --file <csv>", spec.noun)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%s import: %w", spec.noun, err)
	}
	defer func() { _ = f.Close() }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s import: reading CSV: %w", spec.noun, err)
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("%s import: %s needs a header row plus at least one data row", spec.noun, path)
	}
	header := rows[0]
	data := rows[1:]

	colField := make([]*importDataField, len(header))
	fieldByName := map[string]*importDataField{}
	for i := range spec.fields {
		fieldByName[spec.fields[i].qboName] = &spec.fields[i]
	}
	for ci, h := range header {
		if qf, ok := spec.aliases[strings.ToLower(strings.TrimSpace(h))]; ok {
			colField[ci] = fieldByName[qf]
		}
	}
	mapped := 0
	mappedNames := map[string]bool{}
	for _, f := range colField {
		if f != nil {
			mapped++
			mappedNames[f.qboName] = true
		}
	}
	if mapped == 0 {
		return nil, fmt.Errorf("%s import: no CSV headers match QBO fields (want e.g. %s)", spec.noun, requiredHint(spec))
	}
	for _, req := range spec.required {
		if !mappedNames[req] {
			return nil, fmt.Errorf("%s import: a %s column is required", spec.noun, fieldColHeader(spec, req))
		}
	}

	custlist := make([]map[string]string, 0, len(data))
	for _, rec := range data {
		row := map[string]string{}
		for _, k := range spec.listKeys {
			row[k] = ""
		}
		for ci, f := range colField {
			if f != nil && ci < len(rec) {
				if _, ok := row[f.importKey]; ok {
					row[f.importKey] = strings.TrimSpace(rec[ci])
				}
			}
		}
		row["tag"] = "1"
		custlist = append(custlist, row)
	}
	entityInfo, err := json.Marshal(map[string]any{
		"custlist":    custlist,
		"invalidRows": 0,
		"numRows":     len(custlist),
		"totalRows":   len(custlist),
	})
	if err != nil {
		return nil, err
	}

	mappingInfo := map[string]any{}
	for ci := range header {
		f := colField[ci]
		if f == nil {
			continue
		}
		mappingInfo[f.qboName] = []any{map[string]any{
			"colNum":    ci,
			"colHeader": strings.TrimSpace(header[ci]),
			"selected":  true,
			"sortIndex": f.sortIndex,
		}}
	}
	for i := range spec.fields {
		f := &spec.fields[i]
		if _, ok := mappingInfo[f.qboName]; ok {
			continue
		}
		entry := map[string]any{
			"colHeader": f.colHeader,
			"sortIndex": f.sortIndex,
			"importKey": f.importKey,
			"colNum":    "none",
		}
		maps.Copy(entry, f.extra)
		mappingInfo[f.qboName] = []any{entry}
	}
	userMappings, err := json.Marshal(map[string]any{
		"numMappings": mapped,
		"mappingInfo": mappingInfo,
	})
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(map[string]any{
		"entityInfo":   string(entityInfo),
		"userMappings": string(userMappings),
	})
	if err != nil {
		return nil, err
	}

	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	if ac.realm == "" {
		return nil, fmt.Errorf("%s import: no realm id in credentials", spec.noun)
	}
	overrides := map[string]string{
		"Referer":      spec.referer,
		"Content-Type": "application/json",
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, plannedImportDataURL(ac.realm, spec.entity), importDataHost, body, overrides)
	if err != nil {
		return nil, fmt.Errorf("%s import: %w", spec.noun, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading %s import: %w", spec.noun, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var out struct {
		Message  string `json:"message"`
		Status   string `json:"status"`
		Imported *int   `json:"imported"`
		Total    *int   `json:"total"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("%s import outcome unknown; inspect before retrying: unreadable response: %w", spec.noun, err)
	}
	if !strings.EqualFold(out.Status, "OK") {
		return nil, fmt.Errorf("%s import outcome unknown; inspect before retrying: expected status OK, got %q", spec.noun, out.Status)
	}
	if out.Imported == nil || out.Total == nil {
		return nil, fmt.Errorf("%s import outcome unknown; inspect before retrying: response omitted imported/total row counters", spec.noun)
	}
	if *out.Imported < 0 || *out.Total < 0 {
		return nil, fmt.Errorf("%s import outcome unknown; inspect before retrying: invalid imported/total row counters %d/%d", spec.noun, *out.Imported, *out.Total)
	}
	if *out.Imported < *out.Total {
		return nil, fmt.Errorf("%s import partially completed: %d/%d rows reported imported; inspect before retrying", spec.noun, *out.Imported, *out.Total)
	}
	if *out.Imported != *out.Total || *out.Total != len(custlist) {
		return nil, fmt.Errorf("%s import outcome unknown; inspect before retrying: imported/total row counters %d/%d do not confirm %d submitted rows", spec.noun, *out.Imported, *out.Total, len(custlist))
	}
	note := fmt.Sprintf("neo importdata/import?entity=%s rows=%d", spec.entity, len(custlist))
	if out.Message != "" {
		note += " — " + out.Message
	}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     spec.noun + " import",
		Entity: spec.entity,
		Item:   QueryItem{Type: spec.entity, ID: "", Name: strconv.Itoa(len(custlist)) + " rows"},
		Note:   note,
	}, nil
}
