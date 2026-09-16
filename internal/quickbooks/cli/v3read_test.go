package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// reportReadCmd walks the command tree to the wired `reports report read`
// leaf (newStubCmd swaps in newV3ReportCmd via maybeV3Cmd for
// QBO.REPORTS.REPORT_READ). Pure construction — no RunE, no network.
func reportReadCmd(t *testing.T) *cobra.Command {
	t.Helper()
	root := NewRootCommand()
	reports := findSub(root, "reports")
	if reports == nil {
		t.Fatal("root missing reports subcommand")
	}
	reportEnt := findSub(reports, "report")
	if reportEnt == nil {
		t.Fatal("reports missing report subcommand")
	}
	read := findSub(reportEnt, "get")
	if read == nil {
		t.Fatal("reports report missing get subcommand")
	}
	return read
}

// TestV3ReportCmdAcceptsCatalogFlags is the contract's positive case for
// `qb reports report read`: the catalog (auParamAll) advertises --report,
// --date-range, --accounting-method, --columns, --id, and --limit, so cobra
// MUST accept --limit and --accounting-method rather than failing with
// "unknown flag". Reports are not list endpoints, so --limit is not forwarded
// to the v3 reports API, but it must still parse.
//
// This rejects the regression where newV3ReportCmd registered only --report
// and --date-range: under that regression --limit errors as unknown and this
// test fails.
func TestV3ReportCmdAcceptsCatalogFlags(t *testing.T) {
	read := reportReadCmd(t)

	// --limit and --accounting-method are catalog-advertised and MUST parse.
	if err := read.ParseFlags([]string{"--limit", "1", "--accounting-method", "cash"}); err != nil {
		t.Fatalf("--limit/--accounting-method should parse, got %v", err)
	}
	if got, err := read.Flags().GetInt("limit"); err != nil || got != 1 {
		t.Fatalf("limit=%d err=%v, want 1", got, err)
	}
	if got, err := read.Flags().GetString("accounting-method"); err != nil || got != "cash" {
		t.Fatalf("accounting-method=%q err=%v, want \"cash\"", got, err)
	}

	// The full live-style invocation must parse end-to-end at cobra level
	// (RunE is not invoked here, so no network).
	if err := read.ParseFlags([]string{
		"--report", "ProfitAndLoss",
		"--date-range", "2026-07-01,2026-08-18",
		"--limit", "1",
		"--accounting-method", "accrual",
		"--columns", "monthly",
		"--id", "42",
	}); err != nil {
		t.Fatalf("catalog-advertised flag set should parse, got %v", err)
	}
}

// TestV3ReportCmdRejectsGenuinelyUnknownFlag is the negative case. It proves
// the positive test is meaningful: a flag that is NOT in the catalog MUST
// still error as unknown. We deliberately do NOT assert that --limit is an
// unknown-flag error (it is now valid); instead we use a fabricated flag so
// the test distinguishes "registered" from "anything-goes". Under a blanket
// "accept all flags" bug this test would fail.
func TestV3ReportCmdRejectsGenuinelyUnknownFlag(t *testing.T) {
	read := reportReadCmd(t)

	err := read.ParseFlags([]string{"--definitely-not-a-catalog-flag", "x"})
	if err == nil {
		t.Fatal("expected unknown-flag error for --definitely-not-a-catalog-flag, got nil")
	}
}

func TestTPARReadMapsToTaxablePayments(t *testing.T) {
	spec, ok := v3ByID["QBO.TAX.TPAR_READ"]
	if !ok {
		t.Fatal("missing QBO.TAX.TPAR_READ")
	}
	if spec.Report != "TAXABLE_PAYMENTS" {
		t.Fatalf("QBO.TAX.TPAR_READ report=%q entity=%q, want TAXABLE_PAYMENTS (do not revert to ProfitAndLoss)", spec.Report, spec.Entity)
	}
	if spec.Entity != "" {
		t.Fatalf("QBO.TAX.TPAR_READ entity=%q, want empty (report path; instances via ReplayReport)", spec.Entity)
	}
}

func TestAdjustReadMapsToInventoryAdjustment(t *testing.T) {
	for _, id := range []string{"QBO.INVENTORY.ADJUST_READ", "QBO.INVENTORY.ADJUST_SEARCH"} {
		spec, ok := v3ByID[id]
		if !ok {
			t.Fatalf("missing %s", id)
		}
		if spec.Entity != "InventoryAdjustment" {
			t.Fatalf("%s entity=%q, want InventoryAdjustment", id, spec.Entity)
		}
	}
	root := NewRootCommand()
	inv := findSub(root, "inventory")
	adj := findSub(inv, "adjust")
	if adj == nil {
		t.Fatal("inventory adjust missing")
	}
	if findSub(adj, "get") == nil {
		t.Fatal("inventory adjust read missing")
	}
	if findSub(adj, "search") == nil {
		t.Fatal("inventory adjust search missing")
	}
	if findSub(adj, "create") == nil {
		t.Fatal("inventory adjust create missing")
	}
}

func TestItemReceiptReadMapsToGetItemReceipts(t *testing.T) {
	for _, id := range []string{"QBO.INVENTORY.ITEM_RECEIPT_READ", "QBO.INVENTORY.ITEM_RECEIPT_SEARCH"} {
		spec, ok := v3ByID[id]
		if !ok {
			t.Fatalf("missing %s", id)
		}
		if spec.Entity != "ItemReceipt" {
			t.Fatalf("%s entity=%q, want ItemReceipt (warehouse-management-svc GetItemReceipts)", id, spec.Entity)
		}
	}
	root := NewRootCommand()
	inv := findSub(root, "inventory")
	ir := findSub(inv, "item-receipt")
	if ir == nil {
		t.Fatal("inventory item-receipt missing")
	}
	if findSub(ir, "get") == nil {
		t.Fatal("inventory item-receipt read missing")
	}
	if findSub(ir, "search") == nil {
		t.Fatal("inventory item-receipt search missing")
	}
}
func TestSalesOrderReadMapsToGetSalesOrders(t *testing.T) {
	for _, id := range []string{"QBO.SALES.SALES_ORDER_READ", "QBO.SALES.SALES_ORDER_SEARCH"} {
		spec, ok := v3ByID[id]
		if !ok {
			t.Fatalf("missing %s", id)
		}
		if spec.Entity != "SalesOrder" {
			t.Fatalf("%s entity=%q, want SalesOrder (commercecontrol GetSalesOrders; not Invoice 115/87)", id, spec.Entity)
		}
		if spec.Report != "" {
			t.Fatalf("%s must stay a query, not a report", id)
		}
	}
	root := NewRootCommand()
	sales := findSub(root, "sales")
	so := findSub(sales, "sales-order")
	if so == nil {
		t.Fatal("sales sales-order missing")
	}
	read := findSub(so, "get")
	if read == nil {
		t.Fatal("sales sales-order read missing")
	}
	if !strings.Contains(read.Short, "commercecontrol") {
		t.Fatalf("read Short=%q, want commercecontrol GetSalesOrders", read.Short)
	}
	if findSub(so, "search") == nil {
		t.Fatal("sales sales-order search missing")
	}
}

func TestRevenueRecognitionReadMapsToDeferred(t *testing.T) {
	spec, ok := v3ByID["QBO.ACCOUNTING.REVENUE_RECOGNITION_READ"]
	if !ok {
		t.Fatal("missing QBO.ACCOUNTING.REVENUE_RECOGNITION_READ")
	}
	if spec.Entity != "RevenueRecognition" {
		t.Fatalf("entity=%q, want RevenueRecognition (sbseggraphqlorch deferred lines; not Invoice 115/87)", spec.Entity)
	}
	if spec.Report != "" {
		t.Fatalf("must stay a query, not a report")
	}
	root := NewRootCommand()
	acc := findSub(root, "accounting")
	rr := findSub(acc, "revenue-recognition")
	if rr == nil {
		t.Fatal("accounting revenue-recognition missing")
	}
	read := findSub(rr, "get")
	if read == nil {
		t.Fatal("accounting revenue-recognition read missing")
	}
	if !strings.Contains(read.Short, "sbseggraphqlorch") {
		t.Fatalf("read Short=%q, want sbseggraphqlorch deferred lines", read.Short)
	}
}

func TestMileageCreateAcceptsEmployeeFlag(t *testing.T) {
	root := NewRootCommand()
	exp := findSub(root, "expenses")
	mil := findSub(exp, "mileage")
	create := findSub(mil, "create")
	if create == nil {
		t.Fatal("expenses mileage create missing")
	}
	// wired-alias-block-09: mileage create is a blocked stand-in (the
	// TimeActivity dispatch was wrong-entity), so the not-wired stub exposes
	// only the catalog params; the alias-only --customer/--hours flags are
	// gone with the dispatch.
	if err := create.ParseFlags([]string{"--employee", "11", "--vehicle", "9", "--date", "01/09/2026", "--distance-km", "5"}); err != nil {
		t.Fatalf("mileage create catalog params should parse: %v", err)
	}
	if err := create.ParseFlags([]string{"--customer", "1"}); err == nil {
		t.Fatal("mileage create --customer should be unknown after the stand-in block")
	}
	ts := findSub(findSub(findSub(root, "payroll"), "timesheet"), "update")
	if ts == nil {
		t.Fatal("payroll timesheet edit missing")
	}
	if err := ts.ParseFlags([]string{"--id", "1", "--hours", "3"}); err != nil {
		t.Fatalf("timesheet edit --hours should parse: %v", err)
	}
}

func TestEmployeeReadAcceptsActiveFlag(t *testing.T) {
	root := NewRootCommand()
	pay := findSub(root, "payroll")
	emp := findSub(pay, "employee")
	read := findSub(emp, "get")
	if read == nil {
		t.Fatal("payroll employee read missing")
	}
	if err := read.ParseFlags([]string{"--id", "37", "--active", "false"}); err != nil {
		t.Fatalf("--active=false should parse, got %v", err)
	}
	if got, err := read.Flags().GetString("active"); err != nil || got != "false" {
		t.Fatalf("active=%q err=%v, want false", got, err)
	}
}

func TestIntegrationTxnReadOffPurchase(t *testing.T) {
	if spec, ok := v3ByID["QBO.ACCOUNTING.INTEGRATION_TXN_READ"]; ok {
		t.Fatalf("INTEGRATION_TXN_READ still in v3ByID entity=%q (must not be Purchase leftovers 121/92)", spec.Entity)
	}
	root := NewRootCommand()
	acc := findSub(root, "accounting")
	ent := findSub(acc, "integration-txn")
	if ent == nil {
		t.Fatal("accounting integration-txn missing")
	}
	read := findSub(ent, "get")
	if read == nil {
		t.Fatal("accounting integration-txn read missing")
	}
	if strings.Contains(read.Short, "v3 query") || strings.Contains(read.Short, "Purchase") {
		t.Fatalf("short still Purchase/v3: %s", read.Short)
	}
	if !strings.Contains(read.Short, "connections") {
		t.Fatalf("short=%q, want v4 connections GraphQL", read.Short)
	}
	if err := read.ParseFlags([]string{"--limit", "2", "--id", "x"}); err != nil {
		t.Fatalf("--limit/--id should parse: %v", err)
	}
}

func TestListReadMapsToListsPrefs(t *testing.T) {
	spec, ok := v3ByID["QBO.COMPANY.LIST_READ"]
	if !ok {
		t.Fatal("missing QBO.COMPANY.LIST_READ")
	}
	if spec.Entity == "Account" {
		t.Fatal("list must not stay Account stand-in")
	}
	if spec.Entity != "ListsPrefs" {
		t.Fatalf("entity=%q, want ListsPrefs (v4/entities ListsPrefs; not Account)", spec.Entity)
	}
	root := NewRootCommand()
	co := findSub(root, "company")
	lst := findSub(co, "list")
	if lst == nil {
		t.Fatal("company list missing")
	}
	read := findSub(lst, "get")
	if read == nil {
		t.Fatal("company list read missing")
	}
	if !strings.Contains(read.Short, "ListsPrefs") {
		t.Fatalf("read Short=%q, want ListsPrefs", read.Short)
	}
}

func TestCustomersOverviewReadMapsToCustomersOverview(t *testing.T) {
	spec, ok := v3ByID["QBO.CUSTOMERS.OVERVIEW_READ"]
	if !ok {
		t.Fatal("missing QBO.CUSTOMERS.OVERVIEW_READ")
	}
	if spec.Entity == "Customer" {
		t.Fatal("customers overview must not stay Customer stand-in")
	}
	if spec.Entity != "CustomersOverview" {
		t.Fatalf("entity=%q, want CustomersOverview (not Customer)", spec.Entity)
	}
	root := NewRootCommand()
	cu := findSub(root, "customers")
	ov := findSub(cu, "overview")
	if ov == nil {
		t.Fatal("customers overview missing")
	}
	read := findSub(ov, "get")
	if read == nil {
		t.Fatal("customers overview read missing")
	}
	if !strings.Contains(read.Short, "invoiceSummary") {
		t.Fatalf("read Short=%q, want invoiceSummary+getTransactions", read.Short)
	}
}

func TestInventoryOverviewMapsOffItem(t *testing.T) {
	for _, id := range []string{"QBO.INVENTORY.OVERVIEW_READ", "QBO.INVENTORY.OVERVIEW_SEARCH"} {
		spec, ok := v3ByID[id]
		if !ok {
			t.Fatalf("missing %s", id)
		}
		if spec.Entity == "Item" {
			t.Fatalf("%s still Item stand-in", id)
		}
	}
	if v3ByID["QBO.INVENTORY.OVERVIEW_READ"].Entity != "InventoryOverview" {
		t.Fatalf("read entity=%q, want InventoryOverview", v3ByID["QBO.INVENTORY.OVERVIEW_READ"].Entity)
	}
	if v3ByID["QBO.INVENTORY.OVERVIEW_SEARCH"].Entity != "InventoryOverviewSearch" {
		t.Fatalf("search entity=%q, want InventoryOverviewSearch", v3ByID["QBO.INVENTORY.OVERVIEW_SEARCH"].Entity)
	}
	root := NewRootCommand()
	inv := findSub(root, "inventory")
	ov := findSub(inv, "overview")
	if ov == nil {
		t.Fatal("inventory overview missing")
	}
	read := findSub(ov, "get")
	if read == nil {
		t.Fatal("inventory overview read missing")
	}
	if !strings.Contains(read.Short, "GetAllListViewEntities") {
		t.Fatalf("read Short=%q", read.Short)
	}
	search := findSub(ov, "search")
	if search == nil {
		t.Fatal("inventory overview search missing")
	}
	if !strings.Contains(search.Short, "SearchAllListViewEntities") {
		t.Fatalf("search Short=%q", search.Short)
	}
	if strings.Contains(read.Short, "GetIMSPref") || strings.Contains(search.Short, "GetIMSPref") {
		t.Fatal("must not wire GetIMSPref")
	}
	if err := search.ParseFlags([]string{"--query", "QB-CLI", "--limit", "2"}); err != nil {
		t.Fatalf("--query/--limit should parse: %v", err)
	}
}

func TestWorkflowReadMapsToWASAllRules(t *testing.T) {
	spec, ok := v3ByID["QBO.ADVANCED.WORKFLOWS_READ"]
	if !ok || spec.Entity != "Workflow" {
		t.Fatalf("WORKFLOWS_READ entity=%q, want Workflow", spec.Entity)
	}
}

func TestBusinessFeedReadMapsToItems(t *testing.T) {
	spec, ok := v3ByID["QBO.COMPANY.BUSINESS_FEED_READ"]
	if !ok || spec.Entity != "BusinessFeed" {
		t.Fatalf("BUSINESS_FEED_READ entity=%q, want BusinessFeed", spec.Entity)
	}
}

func TestProposalReadMapsToCRMProposals(t *testing.T) {
	for _, id := range []string{"QBO.CUSTOMERS.PROPOSAL_READ", "QBO.CUSTOMERS.PROPOSAL_SEARCH"} {
		spec, ok := v3ByID[id]
		if !ok || spec.Entity != "Proposal" {
			t.Fatalf("%s entity=%q, want Proposal (not Estimate)", id, spec.Entity)
		}
	}
}

func TestReviewReadMapsToFeedbackEngagement(t *testing.T) {
	for _, id := range []string{"QBO.CUSTOMERS.REVIEW_READ", "QBO.CUSTOMERS.REVIEW_SEARCH"} {
		spec, ok := v3ByID[id]
		if !ok || spec.Entity != "Review" {
			t.Fatalf("%s entity=%q, want Review (not Customer)", id, spec.Entity)
		}
	}
}

func TestCustomReportsReadMapsToFolio(t *testing.T) {
	spec, ok := v3ByID["QBO.REPORTS.CUSTOM_READ"]
	if !ok {
		t.Fatal("missing QBO.REPORTS.CUSTOM_READ")
	}
	if spec.Entity != "Folio" {
		t.Fatalf("CUSTOM_READ entity=%q report=%q, want Folio (not ProfitAndLoss)", spec.Entity, spec.Report)
	}
	if spec.Report != "" {
		t.Fatalf("CUSTOM_READ must stay a query, not a v3 report (got report=%q)", spec.Report)
	}
}

func TestCustomRolesReadMapsToAccountRoles(t *testing.T) {
	spec, ok := v3ByID["QBO.ADVANCED.CUSTOM_ROLES_READ"]
	if !ok || spec.Entity != "Role" {
		t.Fatalf("CUSTOM_ROLES_READ entity=%q, want Role (roles.api accountRoles; not Employee)", spec.Entity)
	}
	spec, ok = v3ByID["QBO.COMPANY.ROLE_READ"]
	if !ok || spec.Entity != "Role" {
		t.Fatalf("ROLE_READ entity=%q, want Role (roles.api accountRoles; not Employee)", spec.Entity)
	}
}

func TestFormStyleReadMapsToCustomizations(t *testing.T) {
	spec, ok := v3ByID["QBO.COMPANY.FORM_STYLE_READ"]
	if !ok || spec.Entity != "FormStyle" {
		t.Fatalf("FORM_STYLE_READ entity=%q, want FormStyle (txnsrendering customizations; not Preferences)", spec.Entity)
	}
}

func TestSalesHubReadMapsToTransactions(t *testing.T) {
	for _, id := range []string{"QBO.SALES.HUB_READ", "QBO.SALES.HUB_SEARCH"} {
		spec, ok := v3ByID[id]
		if !ok || spec.Entity != "SalesHub" {
			t.Fatalf("%s entity=%q, want SalesHub (v4 graphql transactions; not Invoice)", id, spec.Entity)
		}
	}
}

func TestManagementReportsReadMapsToManagementFolio(t *testing.T) {
	spec, ok := v3ByID["QBO.REPORTS.MANAGEMENT_READ"]
	if !ok || spec.Entity != "ManagementFolio" {
		t.Fatalf("MANAGEMENT_READ entity=%q report=%q, want ManagementFolio (folio no subType; not BalanceSheet)", spec.Entity, spec.Report)
	}
	if spec.Report != "" {
		t.Fatalf("MANAGEMENT_READ must stay a query, not a v3 report (got report=%q)", spec.Report)
	}
}
