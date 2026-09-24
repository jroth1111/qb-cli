package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The customers-import contract posts stringified entityInfo+userMappings
// to the neo importdata BFF. Tests pin the wire shape: custlist records
// carry every field (mapped or empty) plus tag:"1", and userMappings
// mirrors the wizard's full field set with colNum or "none".

func importDataServer(t *testing.T) *domServer {
	t.Helper()
	saveUsableURIHost(t)
	s := &domServer{t: t}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b []byte
		if r.Body != nil {
			b, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		s.mu.Lock()
		s.calls++
		s.lastMeth = r.Method
		s.lastPath = r.URL.Path + "?" + r.URL.RawQuery
		s.lastBody = b
		s.mu.Unlock()
		var outer map[string]string
		_ = json.Unmarshal(b, &outer)
		var entity struct {
			Custlist []any `json:"custlist"`
		}
		_ = json.Unmarshal([]byte(outer["entityInfo"]), &entity)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"imported":%d,"total":%d,"status":"OK"}`, len(entity.Custlist), len(entity.Custlist))
	}))
	t.Cleanup(s.Close)
	interceptHTTP(t, s.URL)
	return s
}

func writeCSV(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cust.csv")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCustomersImportWireShape(t *testing.T) {
	srv := importDataServer(t)
	path := writeCSV(t, "Name,Email\nAcme Pty,acme@example.com\nSecond Co,\n")
	res, err := ReplayCustomersImport(context.Background(), path)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Op != "customers import" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost {
		t.Fatalf("method = %s", meth)
	}
	if !strings.Contains(pathq, "importdata/import") || !strings.Contains(pathq, "entity=Customers") {
		t.Fatalf("path = %s", pathq)
	}
	var outer map[string]string
	if err := json.Unmarshal(body, &outer); err != nil {
		t.Fatalf("outer: %v", err)
	}
	var ei struct {
		Custlist    []map[string]string `json:"custlist"`
		InvalidRows int                 `json:"invalidRows"`
		NumRows     int                 `json:"numRows"`
		TotalRows   int                 `json:"totalRows"`
	}
	if err := json.Unmarshal([]byte(outer["entityInfo"]), &ei); err != nil {
		t.Fatalf("entityInfo: %v", err)
	}
	if len(ei.Custlist) != 2 || ei.NumRows != 2 || ei.TotalRows != 2 || ei.InvalidRows != 0 {
		t.Fatalf("entityInfo = %+v", ei)
	}
	first := ei.Custlist[0]
	if first["name"] != "Acme Pty" || first["email"] != "acme@example.com" || first["tag"] != "1" {
		t.Fatalf("row0 = %v", first)
	}
	for _, k := range customerImportSpec.listKeys {
		if _, ok := first[k]; !ok {
			t.Fatalf("row0 missing key %q: %v", k, first)
		}
	}
	if ei.Custlist[1]["name"] != "Second Co" || ei.Custlist[1]["email"] != "" {
		t.Fatalf("row1 = %v", ei.Custlist[1])
	}
	var um struct {
		NumMappings int                         `json:"numMappings"`
		MappingInfo map[string][]map[string]any `json:"mappingInfo"`
	}
	if err := json.Unmarshal([]byte(outer["userMappings"]), &um); err != nil {
		t.Fatalf("userMappings: %v", err)
	}
	if um.NumMappings != 2 {
		t.Fatalf("numMappings = %d, want 2", um.NumMappings)
	}
	name := um.MappingInfo["Name"][0]
	if name["colNum"] != float64(0) || name["colHeader"] != "Name" || name["selected"] != true || name["sortIndex"] != float64(0) {
		t.Fatalf("Name mapping = %v", name)
	}
	email := um.MappingInfo["Email"][0]
	if email["colNum"] != float64(1) || email["selected"] != true {
		t.Fatalf("Email mapping = %v", email)
	}
	// Unmapped fields keep the captured descriptor with colNum:"none".
	company := um.MappingInfo["Company"][0]
	if company["colNum"] != "none" || company["importKey"] != "company" || company["colHeader"] != "Company" {
		t.Fatalf("Company unmapped = %v", company)
	}
	if len(um.MappingInfo) != len(importDataFields) {
		t.Fatalf("mappingInfo fields = %d, want %d", len(um.MappingInfo), len(importDataFields))
	}
}

func TestCustomersImportHeaderAliases(t *testing.T) {
	importDataServer(t)
	path := writeCSV(t, "Customer Name,Postcode\nAlias Co,2000\n")
	res, err := ReplayCustomersImport(context.Background(), path)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Item.Name != "1 rows" {
		t.Fatalf("result = %+v", res)
	}
}

func TestCustomersImportRequiresNameColumn(t *testing.T) {
	importDataServer(t)
	path := writeCSV(t, "Email,Phone\na@b.c,123\n")
	if _, err := ReplayCustomersImport(context.Background(), path); err == nil ||
		!strings.Contains(err.Error(), "Name column is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestCustomersImportRejectsUnmappedHeaders(t *testing.T) {
	importDataServer(t)
	path := writeCSV(t, "Foo,Bar\nx,y\n")
	if _, err := ReplayCustomersImport(context.Background(), path); err == nil ||
		!strings.Contains(err.Error(), "no CSV headers match") {
		t.Fatalf("err = %v", err)
	}
}

// The suppliers contract is identical except entity=Vendors and the custlist
// tax key: vendors carry vendorTaxIdNumber, never customerTaxIdNumber.
func TestSuppliersImportWireShape(t *testing.T) {
	srv := importDataServer(t)
	path := writeCSV(t, "Name,Email,Tax Id No.\nSupp Pty,s@b.c,TAX123\n")
	res, err := ReplaySuppliersImport(context.Background(), path)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Op != "suppliers import" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost {
		t.Fatalf("method = %s", meth)
	}
	if !strings.Contains(pathq, "importdata/import") || !strings.Contains(pathq, "entity=Vendors") {
		t.Fatalf("path = %s", pathq)
	}
	var outer map[string]string
	if err := json.Unmarshal(body, &outer); err != nil {
		t.Fatalf("outer: %v", err)
	}
	var ei struct {
		Custlist []map[string]string `json:"custlist"`
		NumRows  int                 `json:"numRows"`
	}
	if err := json.Unmarshal([]byte(outer["entityInfo"]), &ei); err != nil {
		t.Fatalf("entityInfo: %v", err)
	}
	if ei.NumRows != 1 || len(ei.Custlist) != 1 {
		t.Fatalf("entityInfo = %+v", ei)
	}
	row := ei.Custlist[0]
	if row["name"] != "Supp Pty" || row["vendorTaxIdNumber"] != "TAX123" || row["tag"] != "1" {
		t.Fatalf("row = %v", row)
	}
	if _, ok := row["customerTaxIdNumber"]; ok {
		t.Fatalf("vendor row must not carry customerTaxIdNumber: %v", row)
	}
	for _, k := range vendorImportSpec.listKeys {
		if _, ok := row[k]; !ok {
			t.Fatalf("row missing key %q: %v", k, row)
		}
	}
	var um struct {
		NumMappings int                         `json:"numMappings"`
		MappingInfo map[string][]map[string]any `json:"mappingInfo"`
	}
	if err := json.Unmarshal([]byte(outer["userMappings"]), &um); err != nil {
		t.Fatalf("userMappings: %v", err)
	}
	if um.NumMappings != 3 {
		t.Fatalf("numMappings = %d, want 3", um.NumMappings)
	}
	vtn := um.MappingInfo["VendorTaxIdNumber"][0]
	if vtn["colNum"] != float64(2) || vtn["selected"] != true {
		t.Fatalf("VendorTaxIdNumber mapping = %v", vtn)
	}
}

// The COA contract uses entity=coa with a 7-field wizard set: custlist
// records carry type/number/detailType/name/currency/openingBalance/
// openingBalanceDate plus tag:"1"; Name and Type columns are required.
func TestCoaImportWireShape(t *testing.T) {
	srv := importDataServer(t)
	path := writeCSV(t, "Account Number,Account Name,Type,Detail Type\n9003,Test Acct,Expenses,Office/General Administrative Expenses\n")
	res, err := ReplayCoaImport(context.Background(), path)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.Op != "chart of accounts import" {
		t.Fatalf("result = %+v", res)
	}
	_, meth, pathq, body := srv.snap()
	if meth != http.MethodPost {
		t.Fatalf("method = %s", meth)
	}
	if !strings.Contains(pathq, "importdata/import") || !strings.Contains(pathq, "entity=coa") {
		t.Fatalf("path = %s", pathq)
	}
	var outer map[string]string
	if err := json.Unmarshal(body, &outer); err != nil {
		t.Fatalf("outer: %v", err)
	}
	var ei struct {
		Custlist []map[string]string `json:"custlist"`
		NumRows  int                 `json:"numRows"`
	}
	if err := json.Unmarshal([]byte(outer["entityInfo"]), &ei); err != nil {
		t.Fatalf("entityInfo: %v", err)
	}
	if ei.NumRows != 1 || len(ei.Custlist) != 1 {
		t.Fatalf("entityInfo = %+v", ei)
	}
	row := ei.Custlist[0]
	if row["type"] != "Expenses" || row["number"] != "9003" || row["name"] != "Test Acct" ||
		row["detailType"] != "Office/General Administrative Expenses" || row["tag"] != "1" {
		t.Fatalf("row = %v", row)
	}
	for _, k := range coaImportSpec.listKeys {
		if _, ok := row[k]; !ok {
			t.Fatalf("row missing key %q: %v", k, row)
		}
	}
	var um struct {
		NumMappings int                         `json:"numMappings"`
		MappingInfo map[string][]map[string]any `json:"mappingInfo"`
	}
	if err := json.Unmarshal([]byte(outer["userMappings"]), &um); err != nil {
		t.Fatalf("userMappings: %v", err)
	}
	if um.NumMappings != 4 {
		t.Fatalf("numMappings = %d, want 4", um.NumMappings)
	}
	num := um.MappingInfo["Number"][0]
	if num["colNum"] != float64(0) || num["colHeader"] != "Account Number" || num["sortIndex"] != float64(0) {
		t.Fatalf("Number mapping = %v", num)
	}
	typ := um.MappingInfo["Type"][0]
	if typ["colNum"] != float64(2) || typ["sortIndex"] != float64(2) {
		t.Fatalf("Type mapping = %v", typ)
	}
	ob := um.MappingInfo["OpeningBalance"][0]
	if ob["colNum"] != "none" || ob["importKey"] != "openingBalance" || ob["maxLength"] != float64(17) {
		t.Fatalf("OpeningBalance unmapped = %v", ob)
	}
	if len(um.MappingInfo) != len(coaImportFields) {
		t.Fatalf("mappingInfo fields = %d, want %d", len(um.MappingInfo), len(coaImportFields))
	}
}

func TestCoaImportRequiresTypeColumn(t *testing.T) {
	importDataServer(t)
	path := writeCSV(t, "Account Name\nSolo Name\n")
	if _, err := ReplayCoaImport(context.Background(), path); err == nil ||
		!strings.Contains(err.Error(), "Type column is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestCustomersImportFileValidation(t *testing.T) {
	if _, err := ReplayCustomersImport(context.Background(), ""); err == nil {
		t.Fatal("expected error for missing --file")
	}
	if _, err := ReplayCustomersImport(context.Background(), "/nonexistent.csv"); err == nil {
		t.Fatal("expected error for missing file")
	}
	path := writeCSV(t, "Name,Email\n")
	if _, err := ReplayCustomersImport(context.Background(), path); err == nil ||
		!strings.Contains(err.Error(), "header row plus at least one data row") {
		t.Fatalf("err = %v", err)
	}
}

func TestCustomersImportRequiresPositiveReceipt(t *testing.T) {
	saveUsableURIHost(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	path := writeCSV(t, "Name\nNo Receipt Pty\n")
	if _, err := ReplayCustomersImport(context.Background(), path); err == nil || !strings.Contains(err.Error(), "outcome unknown") {
		t.Fatalf("err = %v", err)
	}
}

func TestCustomersImportReportsPartialReceiptBeforeRetry(t *testing.T) {
	saveUsableURIHost(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"imported":1,"total":2,"status":"OK"}`))
	}))
	t.Cleanup(srv.Close)
	interceptHTTP(t, srv.URL)
	path := writeCSV(t, "Name\nFirst Pty\nSecond Pty\n")
	if _, err := ReplayCustomersImport(context.Background(), path); err == nil || !strings.Contains(err.Error(), "1/2") || !strings.Contains(err.Error(), "inspect before retrying") {
		t.Fatalf("err = %v", err)
	}
}

func TestCustomersImportRejectsUnprovenRowCounters(t *testing.T) {
	for name, receipt := range map[string]string{
		"missing counterpart": `{"imported":0,"status":"OK"}`,
		"negative counter":    `{"imported":-1,"total":1,"status":"OK"}`,
		"count mismatch":      `{"imported":3,"total":3,"status":"OK"}`,
	} {
		t.Run(name, func(t *testing.T) {
			saveUsableURIHost(t)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(receipt))
			}))
			t.Cleanup(srv.Close)
			interceptHTTP(t, srv.URL)
			path := writeCSV(t, "Name\nFirst Pty\nSecond Pty\n")
			if _, err := ReplayCustomersImport(context.Background(), path); err == nil || !strings.Contains(err.Error(), "inspect before retrying") {
				t.Fatalf("err = %v", err)
			}
		})
	}
}
