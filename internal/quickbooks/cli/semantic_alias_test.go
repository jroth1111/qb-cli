package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
)

// falseReportAliases are the five catalog primitives that previously
// dispatched the v3 ProfitAndLoss report despite naming unrelated surfaces
// (BAS/IAS/GST-amendments/forecast/performance). No verified native endpoint
// exists for any of them, so they must stay blocked stubs.
var falseReportAliases = []struct {
	id      string
	command string
}{
	{"QBO.TAX.BAS_READ", "tax bas get"},
	{"QBO.TAX.IAS_READ", "tax ias get"},
	{"QBO.TAX.GST_AMENDMENTS_READ", "tax gst-amendments get"},
	{"QBO.REPORTS.FORECAST_READ", "reports forecast get"},
	{"QBO.REPORTS.PERFORMANCE_READ", "reports performance get"},
}

// falseFixedAssetAliases are the five catalog primitives that dispatched the
// v3 Account entity despite naming the QuickBooks Fixed Assets app, which has
// no captured native contract. The live Test Company 2 receipt showed the
// create command returning entity=Account type=Expense, a silent wrong-entity
// write. Keep all five blocked until a native fixed-asset contract exists.
var falseFixedAssetAliases = []struct {
	id      string
	command string
}{
	{"QBO.ACCOUNTING.FIXED_ASSET_CREATE", "accounting fixed-asset create"},
	{"QBO.ACCOUNTING.FIXED_ASSET_DELETE", "accounting fixed-asset delete"},
	{"QBO.ACCOUNTING.FIXED_ASSET_EDIT", "accounting fixed-asset update"},
	{"QBO.ACCOUNTING.FIXED_ASSET_READ", "accounting fixed-asset get"},
	{"QBO.ACCOUNTING.FIXED_ASSET_SEARCH", "accounting fixed-asset search"},
}

// falseStandInAliases are the catalog primitives wired-alias-audit-08
// classified as false or unsupported stand-ins (classes A, B and C). The
// mutating set dispatched a different v3 entity than the named surface —
// tag CRUD rode Class (new-tag creation is retired; tags migrate to custom
// fields), mileage CRUD rode TimeActivity (hours/rate, not
// trips/vehicle/distance), project create/update rode Customer with no
// Job/ParentRef wiring, a generic "txn update" voided only invoices, and
// credit-card-credit create posted a cash Purchase. The read set returned
// Class/Customer/CompanyInfo/Account/Employee/TimeActivity rows or a
// company-wide BalanceSheet for surfaces with no captured native contract
// (appointments, opportunities, tasks, marketing, my-accountant, reconcile
// report, company search, users, payroll categories/super/teams/pay-runs,
// mileage trips, feed rec report, and the expenses/sales overviews and
// expense-claims single-entity stand-ins). Keep them all blocked until a
// native contract exists.
//
// Five rows left this list 2026-09-17 when they were rewired to their real
// contracts: PROJECT_CREATE/EDIT post the v3 Project entity (CustomerRef,
// not a Customer write); TXN_VOID resolves --entity into any voidable v3
// transaction's ?operation=void; CREDIT_CARD_CREDIT_CREATE posts the v3
// CreditCardCredit entity; TASKS_READ runs the captured TaskManagementTasks
// query instead of an Employee stand-in.
var falseStandInAliases = []struct {
	id      string
	command string
}{
	{"QBO.COMPANY.TAG_CREATE", "company tag create"},
	{"QBO.COMPANY.TAG_DELETE", "company tag delete"},
	{"QBO.COMPANY.TAG_EDIT", "company tag update"},
	{"QBO.EXPENSES.MILEAGE_CREATE", "expenses mileage create"},
	{"QBO.EXPENSES.MILEAGE_DELETE", "expenses mileage delete"},
	{"QBO.EXPENSES.MILEAGE_EDIT", "expenses mileage update"},
	{"QBO.COMPANY.TAG_READ", "company tag get"},
	{"QBO.CUSTOMERS.APPOINTMENT_READ", "customers appointment get"},
	{"QBO.CUSTOMERS.APPOINTMENT_SEARCH", "customers appointment search"},
	{"QBO.CUSTOMERS.OPPORTUNITY_READ", "customers opportunity get"},
	{"QBO.CUSTOMERS.OPPORTUNITY_SEARCH", "customers opportunity search"},
	{"QBO.COMPANY.MARKETING_READ", "company marketing get"},
	{"QBO.ACCOUNTING.MY_ACCOUNTANT_READ", "accounting my-accountant get"},
	{"QBO.ACCOUNTING.RECONCILE_READ", "accounting reconcile get"},
	{"QBO.COMPANY.SEARCH", "company search run"},
	{"QBO.COMPANY.USER_READ", "company user get"},
	{"QBO.PAYROLL.DEDUCTION_READ", "payroll deduction get"},
	{"QBO.PAYROLL.LEAVE_CATEGORY_READ", "payroll leave-category get"},
	{"QBO.PAYROLL.PAY_CATEGORY_READ", "payroll pay-category get"},
	{"QBO.PAYROLL.SUPER_READ", "payroll super get"},
	{"QBO.PAYROLL.TEAM_READ", "payroll team get"},
	{"QBO.PAYROLL.TEAM_SEARCH", "payroll team search"},
	{"QBO.PAYROLL.OVERVIEW_SEARCH", "payroll overview search"},
	{"QBO.PAYROLL.PAY_RUN_READ", "payroll pay-run get"},
	{"QBO.EXPENSES.MILEAGE_READ", "expenses mileage get"},
	{"QBO.EXPENSES.MILEAGE_SEARCH", "expenses mileage search"},
	{"QBO.FEED.REC_REPORT", "feed rec get"},
	{"QBO.EXPENSES.OVERVIEW_READ", "expenses overview get"},
	{"QBO.SALES.OVERVIEW_READ", "sales overview get"},
	{"QBO.ADVANCED.EXPENSE_CLAIMS_READ", "advanced expense-claims get"},
}

var falseSemanticAliases = append(append(falseReportAliases, falseFixedAssetAliases...), falseStandInAliases...)

// deadMutateMapIDs are the stale v3MutateByID entries wired-alias-audit-08
// finding S2 flagged on already-blocked catalog rows: company settings
// update, delayed-charge/delayed-credit create, and time-invoice create.
// The mode check in maybeV3MutateCmd made each mapping unreachable, but a
// leftover entry would silently re-arm a live mutation if a row ever
// flipped back to wired. The entries were removed; this list pins both the
// absence and the blocked mode so a flip fails here first. Item-receipt
// create/edit dropped off this list 2026-09-16 when they were wired to the
// captured warehouse-management-svc CreateItemReceipt/UpdateItemReceipt
// mutations (not a resurrected v3 mapping).
var deadMutateMapIDs = []string{
	"QBO.COMPANY.SETTINGS_EDIT",
	"QBO.SALES.DELAYED_CHARGE_CREATE",
	"QBO.SALES.DELAYED_CREDIT_CREATE",
	"QBO.SALES.TIME_INVOICE_CREATE",
}

// TestSemanticAliasesAreBlockedRows pins the catalog contract: each false
// alias is still listed (agents must find the row) but with mode=blocked and
// its command string unchanged. Rejects silently dropping the rows or
// re-enabling them as read/wired.
func TestSemanticAliasesAreBlockedRows(t *testing.T) {
	for _, tc := range falseSemanticAliases {
		e, ok := catalogByID(tc.id)
		if !ok {
			t.Errorf("%s missing from catalog", tc.id)
			continue
		}
		if e.Mode != modeBlocked {
			t.Errorf("%s mode=%q, want blocked", tc.id, e.Mode)
		}
		if e.Command != tc.command {
			t.Errorf("%s command=%q, want %q", tc.id, e.Command, tc.command)
		}
	}
}

// TestSemanticAliasesHaveNoV3Dispatch is the load-bearing guard: none of the
// false aliases may appear in v3ByID or v3MutateByID, and maybeV3Cmd must
// return nil so newStubCmd cannot swap in a live report or Account command.
// A restored mapping fails here before it can mislead a live run.
func TestSemanticAliasesHaveNoV3Dispatch(t *testing.T) {
	for _, tc := range falseSemanticAliases {
		if spec, ok := v3ByID[tc.id]; ok {
			t.Errorf("%s still in v3ByID entity=%q report=%q (must not alias a v3 read)", tc.id, spec.Entity, spec.Report)
		}
		if spec, ok := v3MutateByID[tc.id]; ok {
			t.Errorf("%s still in v3MutateByID entity=%q op=%q (must not alias a v3 mutation)", tc.id, spec.Entity, spec.Op)
		}
		e, _ := catalogByID(tc.id)
		if cmd := maybeV3Cmd(&rootFlags{}, e, tc.command); cmd != nil {
			t.Errorf("%s maybeV3Cmd returned %q; want nil (stub path)", tc.id, cmd.Short)
		}
	}
	// Sanity guard against over-deletion: real report reads keep their maps.
	if spec := v3ByID["QBO.REPORTS.PROFIT_LOSS_READ"]; spec.Report != "ProfitAndLoss" {
		t.Errorf("PROFIT_LOSS_READ report=%q, want ProfitAndLoss (only the false aliases were removed)", spec.Report)
	}
	if spec := v3ByID["QBO.TAX.TPAR_READ"]; spec.Report != "TAXABLE_PAYMENTS" {
		t.Errorf("TPAR_READ report=%q, want TAXABLE_PAYMENTS", spec.Report)
	}
	if spec := v3ByID["QBO.ACCOUNTING.COA_READ"]; spec.Entity != "Account" {
		t.Errorf("COA_READ entity=%q, want Account", spec.Entity)
	}
	if spec := v3MutateByID["QBO.ACCOUNTING.COA_CREATE"]; spec.Entity != "Account" || spec.Op != "create" {
		t.Errorf("COA_CREATE = %+v, want {Account create}", spec)
	}
	// Accepted safe mappings that must survive the stand-in removals:
	// projects read as job customers and locations read as departments.
	if spec := v3ByID["QBO.ACCOUNTING.PROJECT_READ"]; spec.Entity != "Customer" {
		t.Errorf("PROJECT_READ entity=%q, want Customer (projects are job customers)", spec.Entity)
	}
	if spec := v3ByID["QBO.ACCOUNTING.LOCATION_READ"]; spec.Entity != "Department" {
		t.Errorf("LOCATION_READ entity=%q, want Department", spec.Entity)
	}
}

// TestDeadMutateMappingsStayRemoved is the deadmap-cleanup-10 regression
// invariant: each blocked row in deadMutateMapIDs must stay out of
// v3MutateByID and remain mode=blocked, while the accepted mappings —
// chart of accounts, locations-as-departments, sales orders on the
// dedicated SalesOrder path, and the other verified entities — keep their
// dispatch. Re-adding a dead mapping or deleting a live one fails here.
func TestDeadMutateMappingsStayRemoved(t *testing.T) {
	for _, id := range deadMutateMapIDs {
		if spec, ok := v3MutateByID[id]; ok {
			t.Errorf("%s still in v3MutateByID entity=%q op=%q (dead mapping re-armed on a blocked row)", id, spec.Entity, spec.Op)
		}
		e, ok := catalogByID(id)
		if !ok {
			t.Errorf("%s missing from catalog", id)
			continue
		}
		if e.Mode != modeBlocked {
			t.Errorf("%s mode=%q, want blocked (rewiring needs a reviewed mapping, not a resurrected one)", id, e.Mode)
		}
	}
	// Guard against over-deletion: accepted mutate mappings keep their
	// entities, including the neighbors of the removed rows.
	accepted := map[string]v3MutateSpec{
		"QBO.ACCOUNTING.COA_CREATE":         {Entity: "Account", Op: "create"},
		"QBO.ACCOUNTING.COA_DEACTIVATE":     {Entity: "Account", Op: "deactivate"},
		"QBO.ACCOUNTING.LOCATION_CREATE":    {Entity: "Department", Op: "create"},
		"QBO.COMPANY.COMPANY_INFO_EDIT":     {Entity: "CompanyInfo", Op: "update"},
		"QBO.CUSTOMERS.CUSTOMER_CREATE":     {Entity: "Customer", Op: "create"},
		"QBO.EXPENSES.CHEQUE_CREATE":        {Entity: "Cheque", Op: "create"},
		"QBO.EXPENSES.SUPPLIER_CREATE":      {Entity: "Vendor", Op: "create"},
		"QBO.EXPENSES.TIME_ACTIVITY_CREATE": {Entity: "TimeActivity", Op: "create"},
		"QBO.INVENTORY.ITEM_CREATE":         {Entity: "Item", Op: "create"},
		"QBO.SALES.INVOICE_CREATE":          {Entity: "Invoice", Op: "create"},
		"QBO.SALES.SALES_ORDER_CREATE":      {Entity: "SalesOrder", Op: "create"},
		"QBO.SALES.SALES_ORDER_DELETE":      {Entity: "SalesOrder", Op: "delete"},
		"QBO.SALES.SALES_ORDER_EDIT":        {Entity: "SalesOrder", Op: "update"},
	}
	for id, want := range accepted {
		spec, ok := v3MutateByID[id]
		if !ok {
			t.Errorf("%s missing from v3MutateByID, want %+v (only dead rows were removed)", id, want)
			continue
		}
		if spec != want {
			t.Errorf("v3MutateByID[%s] = %+v, want %+v", id, spec, want)
		}
	}
}

// TestClassDeleteCarriesClassCatalogID pins the wired-alias-audit-08 S1 fix:
// `accounting class delete` is constructed with catalogID
// QBO.ACCOUNTING.CLASS_DELETE, so its dry-run plan and help document the
// class surface it actually deactivates — not the retired tag row.
func TestClassDeleteCarriesClassCatalogID(t *testing.T) {
	out, err := runCoverageRoot(t, "accounting", "class", "delete", "--id", "61", "--dry-run", "--json")
	if err != nil {
		t.Fatalf("accounting class delete --dry-run: %v", err)
	}
	var plan planEnvelope
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &plan); err != nil {
		t.Fatalf("plan decode: %v (%s)", err, out)
	}
	if plan.ID != "QBO.ACCOUNTING.CLASS_DELETE" {
		t.Errorf("class delete plan id=%q, want QBO.ACCOUNTING.CLASS_DELETE", plan.ID)
	}
	if plan.Mode != modeWired {
		t.Errorf("class delete plan mode=%q, want wired", plan.Mode)
	}
}

// TestSemanticAliasCommandsAreNotWired walks the real command tree to each
// leaf and proves it is the generic not-wired stub: --dry-run emits a plan
// with mode=blocked, dial=false, and no endpoint URL; ordinary invocation
// fails fast with ErrNoCredentials or ErrMutationNotWired — never a live
// ProfitAndLoss read or Account mutation.
func TestSemanticAliasCommandsAreNotWired(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	for _, tc := range falseSemanticAliases {
		parts := strings.Fields(tc.command)

		// Dry-run: plan must announce blocked mode and no request.
		out, err := runCoverageRoot(t, append(parts, "--dry-run", "--json")...)
		if err != nil {
			t.Fatalf("%s --dry-run: %v", tc.command, err)
		}
		var plan planEnvelope
		if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &plan); err != nil {
			t.Fatalf("%s plan decode: %v (%s)", tc.command, err, out)
		}
		if plan.Mode != modeBlocked || plan.Dial || !plan.DryRun {
			t.Errorf("%s plan = mode=%q dial=%v dry_run=%v, want blocked/dial=false/dry_run=true", tc.command, plan.Mode, plan.Dial, plan.DryRun)
		}
		if strings.Contains(plan.URL, "ProfitAndLoss") || strings.Contains(plan.URL, "/reports/") || strings.Contains(plan.URL, "account") || strings.Contains(plan.URL, "http") {
			t.Errorf("%s plan URL=%q must not name a live endpoint", tc.command, plan.URL)
		}
		if !strings.Contains(plan.Note, "not sent") {
			t.Errorf("%s plan note=%q must say nothing is sent", tc.command, plan.Note)
		}

		// Ordinary run: not-wired/auth failure, never a report result.
		root := NewRootCommand()
		root.SetArgs(parts)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		var buf bytes.Buffer
		root.SetOut(&buf)
		root.SetErr(&buf)
		err = root.ExecuteContext(ctx)
		cancel()
		if err == nil {
			t.Fatalf("%s unexpectedly succeeded: %s", tc.command, buf.String())
		}
		if !errors.Is(err, client.ErrNoCredentials) && !errors.Is(err, client.ErrMutationNotWired) {
			t.Fatalf("%s error = %v, want ErrNoCredentials or ErrMutationNotWired", tc.command, err)
		}
	}
}

// TestSemanticAliasHelpSaysNotWired asserts the leaf help tells the truth:
// the blocked note explains that no endpoint is wired and --dry-run does not
// POST, rather than advertising a report read or Account mutation.
func TestSemanticAliasHelpSaysNotWired(t *testing.T) {
	root := NewRootCommand()
	for _, tc := range falseSemanticAliases {
		target, _, err := root.Find(strings.Fields(tc.command))
		if err != nil || target == nil {
			t.Fatalf("%s command %q does not resolve: %v", tc.id, tc.command, err)
		}
		if !strings.Contains(target.Long, "Not wired") {
			t.Errorf("%s help missing \"Not wired\" note: %q", tc.id, target.Long)
		}
		if strings.Contains(target.Long, "ProfitAndLoss") || strings.Contains(target.Short, "(v3 ") {
			t.Errorf("%s help still advertises a v3 dispatch: short=%q", tc.id, target.Short)
		}
	}
}
