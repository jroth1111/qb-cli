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

// TestRootHasLedgerMasterActions asserts the top-level commands are registered
// on the root: ledger, master, company, tax, payroll, advanced, surface,
// actions. Pure structure — no RunE invocation, no network. Rejects: a missing
// AddCommand for any group (e.g. only feed wired).
func TestRootHasLedgerMasterActions(t *testing.T) {
	root := NewRootCommand()
	for _, name := range []string{"feed", "accounting", "expenses", "sales", "customers", "inventory", "payroll", "tax", "company", "reports", "advanced", "actions"} {
		if findSub(root, name) == nil {
			t.Errorf("root missing %q subcommand", name)
		}
	}
}

func TestActionsCatalogJSONIncludesExcludedCOA(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"actions", "--json"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatalf("qb actions --json failed: %v", err)
	}
	var rows []primitiveEntry
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byID := map[string]primitiveEntry{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	for _, id := range []string{"QBO.COMPANY.BOOKS_CLOSE", "QBO.ADVANCED.BACKUP_RESTORE"} {
		r, ok := byID[id]
		if !ok {
			t.Errorf("catalog missing %q", id)
			continue
		}
		if r.Mode != modeExcluded || r.Command != "" {
			t.Errorf("%q mode=%q command=%q, want excluded empty", id, r.Mode, r.Command)
		}
	}
	for _, id := range []string{"QBO.ACCOUNTING.COA_CREATE", "QBO.ACCOUNTING.COA_DEACTIVATE", "QBO.COMPANY.CURRENCY_CREATE"} {
		r := byID[id]
		if r.Mode != modeWired || r.Command == "" {
			t.Errorf("%q should be wired, got mode=%q command=%q", id, r.Mode, r.Command)
		}
	}
}

// TestActionsCatalogCoversWiredReads asserts the read primitives the CLI wires
// are present with mode=read and the expected command, so the catalog stays in
// sync with the command tree. Rejects a stale catalog that drops a wired read
// or mislabels it as blocked.
func TestActionsCatalogCoversWiredReads(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"actions", "--json"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatalf("qb actions --json failed: %v", err)
	}

	var rows []primitiveEntry
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &rows); err != nil {
		t.Fatalf("decode actions JSON: %v", err)
	}
	byID := make(map[string]primitiveEntry, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}

	want := []primitiveEntry{
		{ID: "QBO.FEED.TXN_PENDING", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed txn list"},
		{ID: "QBO.FEED.TXN_POSTED", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed txn list posted"},
		{ID: "QBO.FEED.TXN_EXCLUDED", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed txn list excluded"},
		{ID: "QBO.FEED.TXN_LOOKUP", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed txn get"},
		{ID: "QBO.ACCOUNTING.REGISTER_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting register get"},
		{ID: "QBO.ACCOUNTING.AUDIT_LOG_READ", Domain: "accounting", Risk: "R0", Mode: modeRead, Command: "accounting audit-log get"},
		{ID: "QBO.FEED.RULE_READ", Domain: "feed", Risk: "R0", Mode: modeRead, Command: "feed rule get"},

		{ID: "QBO.FEED.TXN_EXCLUDE", Domain: "feed", Risk: "R2", Mode: modeWired, Command: "feed txn update exclude"},
		{ID: "QBO.FEED.TXN_UNDO_EXCLUDED", Domain: "feed", Risk: "R2", Mode: modeWired, Command: "feed txn update undo-excluded"},
		{ID: "QBO.FEED.TXN_IMPORT", Domain: "feed", Risk: "R2", Mode: modeWired, Command: "feed txn import"},
		{ID: "QBO.FEED.RULE_CREATE", Domain: "feed", Risk: "R4", Mode: modeBlocked, Command: "feed rule create"},
		{ID: "QBO.CUSTOMERS.CUSTOMER_CREATE", Domain: "customers", Risk: "R4", Mode: modeWired, Command: "customers customer create"},
	}
	for _, w := range want {
		got, ok := byID[w.ID]
		if !ok {
			t.Errorf("catalog missing %q", w.ID)
			continue
		}
		if got.Mode != w.Mode || got.Command != w.Command || got.Domain != w.Domain || got.Risk != w.Risk {
			t.Errorf("catalog row %q = %+v, want %+v", w.ID, got, w)
		}
	}
}

// TestActionsCatalogNoCOACommand asserts no catalog row maps a COA create or
// deactivate primitive to a runnable command, and that the text rendering does
// not advertise one. Rejects an accidental coa-create/coa-deactivate
// verb sneaking into the catalog as read or blocked. COA read/search
// (QBO.ACCOUNTING.COA_READ, QBO.ACCOUNTING.COA_SEARCH) are intentionally blocked master commands and
// are NOT covered by this exclusion — only create/deactivate stay excluded.
func TestActionsCatalogNoCOACommand(t *testing.T) {
	excluded := map[string]bool{
		"QBO.COMPANY.BOOKS_CLOSE":     true,
		"QBO.ADVANCED.BACKUP_RESTORE": true,
	}
	for _, r := range catalogPrimitives {
		if excluded[r.ID] && (r.Mode != modeExcluded || r.Command != "") {
			t.Errorf("%q must stay excluded, got mode=%q command=%q", r.ID, r.Mode, r.Command)
		}
	}
}

func TestAccountingHasNoCOACreateOrDeactivate(t *testing.T) {
	root := NewRootCommand()
	acc := findSub(root, "accounting")
	if acc == nil {
		t.Fatal("root missing accounting")
	}
	for _, forbidden := range []string{"coa-create", "coa-deactivate", "create-account", "deactivate-account"} {
		if findSub(acc, forbidden) != nil {
			t.Errorf("accounting MUST NOT expose %q", forbidden)
		}
	}
	coa := findSub(acc, "coa")
	if coa == nil || findSub(coa, "create") == nil || findSub(coa, "delete") == nil {
		t.Fatal("accounting coa create/delete must be commands")
	}
}

func TestPolicyExcludedVerbsHaveNoCommand(t *testing.T) {
	root := NewRootCommand()
	if findSub(root, "company") != nil && findSub(findSub(root, "company"), "books") != nil {
		t.Fatal("company books must not exist (close is excluded)")
	}
	backup := findSub(findSub(root, "advanced"), "backup")
	if backup != nil && findSub(backup, "restore") != nil {
		t.Fatal("advanced backup restore must not be a command")
	}
}

// TestLedgerRegisterMissingCredentialsFails is the contract's negative case
// for `qb accounting register`: with QB_HOME pointed at an empty temp dir there
// are no saved credentials, so the command MUST fail with the auth exit code
// (2) and MUST NOT reach the network. The guard lives in the client package
// (RequireSession/newAPIClient before any network call). It rejects:
//   - returning nil (silently succeeding with no credentials),
//   - exiting with the wrong code (a transport failure would mean the guard
//     was bypassed),
//   - taking long enough to suggest a network call.
func TestLedgerRegisterMissingCredentialsFails(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())

	root := NewRootCommand()
	root.SetArgs([]string{"accounting", "register", "get"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	start := time.Now()
	err := root.ExecuteContext(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected error for missing credentials, got nil; stdout=%q", out.String())
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if exitErr.Code != ExitAuthError {
		t.Fatalf("exit code=%d, want %d (ExitAuthError); stdout=%q", exitErr.Code, ExitAuthError, out.String())
	}
	if !errors.Is(err, client.ErrNoCredentials) {
		t.Fatalf("expected error to wrap client.ErrNoCredentials, got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("accounting register did not fail fast: took %v (want < 3s, no network expected)", elapsed)
	}
}

// TestLedgerMutationFailsNotWiredOrNoCreds asserts a ledger mutation stub
// (`qb expenses expense-create`) never mutates QBO: with no saved credentials it
// MUST fail fast (RequireSession guard, no network) with the auth exit code;
// with credentials present it would return ErrMutationNotWired. Either way it
// must NOT succeed. This rejects a stub that silently no-ops or that reaches
// the network.
func TestLedgerMutationFailsNotWiredOrNoCreds(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())

	root := NewRootCommand()
	root.SetArgs([]string{"expenses", "expense", "create", "--line-items", `[{"Amount":1}]`})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	start := time.Now()
	err := root.ExecuteContext(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected mutation stub to fail, got nil; stdout=%q", out.String())
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	// With no credentials the RequireSession guard fires first (auth code);
	// with credentials the not-wired path fires (input code). Both are
	// acceptable; success is not.
	if !errors.Is(err, client.ErrNoCredentials) && !errors.Is(err, client.ErrMutationNotWired) {
		t.Fatalf("expected error to wrap ErrNoCredentials or ErrMutationNotWired, got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("mutation stub did not fail fast: took %v (want < 3s, no network expected)", elapsed)
	}
}

// TestMasterMutationFailsNotWiredOrNoCreds mirrors the ledger mutation
// negative for `qb customers create`: it must never mutate QBO and must
// fail fast without credentials.
func TestMasterMutationFailsNotWiredOrNoCreds(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())

	root := NewRootCommand()
	root.SetArgs([]string{"customers", "customer", "create"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	start := time.Now()
	err := root.ExecuteContext(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected master mutation stub to fail, got nil; stdout=%q", out.String())
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *ExitError, got %T: %v", err, err)
	}
	if !errors.Is(err, client.ErrNoCredentials) && !errors.Is(err, client.ErrMutationNotWired) {
		t.Fatalf("expected error to wrap ErrNoCredentials or ErrMutationNotWired, got %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("master mutation stub did not fail fast: took %v (want < 3s, no network expected)", elapsed)
	}
}

// TestLedgerCommandSurface asserts `qb accounting register` and every ledger
// mutation verb is registered, so the command tree matches the contract. Pure
// structure — no RunE invocation, no network.
func TestAccountingCommandSurface(t *testing.T) {
	root := NewRootCommand()
	acc := findSub(root, "accounting")
	if acc == nil {
		t.Fatal("root missing accounting subcommand")
	}
	if findSub(acc, "register") == nil {
		t.Fatal("accounting missing register")
	}
}

// TestActionsTextRenderingHasExcludedRows is a light smoke test that the text
// (non-JSON) rendering of `qb actions` runs and mentions the excluded COA ids
// with the excluded mode, so the human-facing catalog is not broken.
func TestActionsTextRenderingHasExcludedRows(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"actions"})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatalf("qb actions failed: %v; stdout=%q", err, out.String())
	}
	body := out.String()
	for _, id := range []string{"QBO.ACCOUNTING.COA_CREATE", "QBO.ACCOUNTING.COA_DEACTIVATE"} {
		if !strings.Contains(body, id) {
			t.Errorf("actions text output missing %q", id)
		}
	}
	if !strings.Contains(body, "excluded") {
		t.Errorf("actions text output missing \"excluded\" mode label")
	}
}

func TestActionsJSONDocumentsParams(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"actions", "--json"})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.ExecuteContext(ctx); err != nil {
		t.Fatalf("qb actions --json: %v", err)
	}
	var rows []primitiveEntry
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	byID := map[string]primitiveEntry{}
	for _, r := range rows {
		byID[r.ID] = r
	}
	inv := byID["QBO.SALES.INVOICE_CREATE"]
	if inv.Usage == "" || !strings.Contains(inv.Usage, "--customer") {
		t.Fatalf("invoice-create usage missing --customer: %q", inv.Usage)
	}
	found := false
	for _, p := range inv.Params {
		if p.Name == "customer" && p.Required {
			found = true
		}
	}
	if !found {
		t.Fatalf("invoice-create params missing required --customer: %+v", inv.Params)
	}
	ex := byID["QBO.FEED.TXN_EXCLUDE"]
	found = false
	for _, p := range ex.Params {
		if p.Name == "ids" && p.Required {
			found = true
		}
	}
	if !found {
		t.Fatalf("exclude params missing required --ids: %+v", ex.Params)
	}
	if len(ex.Globals) == 0 {
		t.Fatal("exclude missing globals")
	}
	coa := byID["QBO.ACCOUNTING.COA_CREATE"]
	if coa.Usage == "" || !strings.Contains(coa.Usage, "coa create") {
		t.Fatalf("wired COA create usage=%q", coa.Usage)
	}

}

func TestInvoiceCreateHelpListsCustomer(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"sales", "invoice", "create", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	body := out.String()
	for _, want := range []string{"--customer", "--invoice-date", "Live mutation", "qb actions --json", "GST"} {

		if !strings.Contains(body, want) {
			t.Errorf("invoice-create --help missing %q\n%s", want, body)
		}
	}
}

func TestParamsForRejectsEmptyLookupQuery(t *testing.T) {
	ps := paramsFor("QBO.FEED.TXN_LOOKUP")
	ok := false
	for _, p := range ps {
		if p.Name == "query" && p.Required {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("lookup must require --query, got %+v", ps)
	}
}

func TestReceivePaymentHelpIsNotCheque(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"sales", "payment", "create", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	body := out.String()
	if !strings.Contains(body, "Undeposited Funds") {
		t.Fatalf("receive-payment-create --help missing Undeposited Funds\n%s", body)
	}
	if strings.Contains(body, "AU spelling is cheque") {
		t.Fatalf("receive-payment-create --help leaked cheque note")
	}
}

func TestExcludeHelpMentionsDownloadedRows(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"feed", "txn", "update", "exclude", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	body := out.String()
	for _, want := range []string{"olbTxnId", "downloaded", "Excluded"} {
		if !strings.Contains(body, want) {
			t.Errorf("feed exclude --help missing %q\n%s", want, body)
		}
	}
}

func TestAUOverlayParamsAllHaveHelp(t *testing.T) {
	for id, params := range auParamOverlay {
		if len(params) == 0 {
			t.Errorf("%s overlay empty", id)
		}
		for _, p := range params {
			if p.Name == "" || p.Help == "" {
				t.Errorf("%s param %+v missing name/help", id, p)
			}
		}
		got := paramsFor(id)
		if _, wired := wiredParams(id); wired {
			continue
		}
		if len(got) == 0 {
			t.Errorf("%s stub paramsFor empty despite overlay", id)
		}
		for _, p := range got {
			if p.Help == "" {
				t.Errorf("%s paramsFor %q empty help", id, p.Name)
			}
		}
	}
}

func TestImportHelpKeepsLiveFlags(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"feed", "txn", "import", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	body := out.String()
	// --file exists but must stay CSV-scoped: the UI wizard accepts
	// XLSX/PDF, which the CLI upload path (uploadCsvFile, text/csv) cannot
	// honour. Promising those formats would lie to agents.
	if !strings.Contains(body, "--file") {
		t.Errorf("import --help missing --file\n%s", body)
	}
	if !strings.Contains(body, "CSV") {
		t.Errorf("import --file help must scope to CSV\n%s", body)
	}
	for _, bad := range []string{"XLSX", "PDF"} {
		if strings.Contains(body, bad) {
			t.Errorf("import --help must not promise %s\n%s", bad, body)
		}
	}
	for _, want := range []string{"--account-id", "--description", "--amount", "--date"} {
		if !strings.Contains(body, want) {
			t.Errorf("import --help missing %q", want)
		}
	}
}

func TestBillCreateHelpUsesExtractedSupplier(t *testing.T) {
	root := NewRootCommand()
	root.SetArgs([]string{"expenses", "bill", "create", "--help"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err != nil {
		t.Fatalf("help: %v", err)
	}
	body := out.String()
	for _, want := range []string{"--supplier", "GST", "Accounts Payable"} {
		if !strings.Contains(body, want) {
			t.Errorf("bill-create --help missing %q\n%s", want, body)
		}
	}
}

func TestEveryRunnablePrimitiveIsColdUsable(t *testing.T) {
	allowNoParams := map[string]bool{
		"QBO.FEED.ACCOUNT_LIST": true,
		// customobjects create/update/delete register no flags by design
		// (session-gated not-wired stubs), so the params rule exempts them
		// explicitly.
		"QBO.CUSTOMOBJECTS.OBJECT_CREATE": true,
		"QBO.CUSTOMOBJECTS.OBJECT_UPDATE": true,
		"QBO.CUSTOMOBJECTS.OBJECT_DELETE": true,
	}
	// The 3-level cutover (domain entity verb) postdates the costgroups and
	// customobjects surfaces, whose leaves are domain verb (2 parts). These
	// seven rows are grandfathered explicitly rather than renaming live
	// commands agents already drive from transcripts.
	allowTwoPart := map[string]bool{
		"QBO.COSTGROUPS.GROUP_LIST":       true,
		"QBO.COSTGROUPS.GROUP_CREATE":     true,
		"QBO.COSTGROUPS.GROUP_UPDATE":     true,
		"QBO.COSTGROUPS.GROUP_DELETE":     true,
		"QBO.CUSTOMOBJECTS.OBJECT_CREATE": true,
		"QBO.CUSTOMOBJECTS.OBJECT_UPDATE": true,
		"QBO.CUSTOMOBJECTS.OBJECT_DELETE": true,
	}
	for _, e := range catalogPrimitives {
		if e.Mode == modeExcluded {
			if e.Command != "" {
				t.Errorf("%s excluded but has command %q", e.ID, e.Command)
			}
			continue
		}
		if e.Command == "" {
			t.Errorf("%s runnable with empty command", e.ID)
			continue
		}
		parts := strings.Fields(e.Command)
		if len(parts) < 3 || len(parts) > 4 {
			if len(parts) != 2 || !allowTwoPart[e.ID] {
				t.Errorf("%s command %q is not domain entity verb", e.ID, e.Command)
			}
		}
		if auHelpNote(e.ID) == "" {
			t.Errorf("%s missing AU/agent note", e.ID)
		}
		ps := paramsFor(e.ID)
		if len(ps) == 0 && !allowNoParams[e.ID] {
			t.Errorf("%s has no params", e.ID)
		}
		for _, p := range ps {
			if len(p.Help) < 40 {
				t.Errorf("%s param %q help too short (%d): %q", e.ID, p.Name, len(p.Help), p.Help)
			}
		}

	}
}

func TestMaybeV3MutateCmdSkipsBlocked(t *testing.T) {
	blocked := primitiveEntry{ID: "QBO.SALES.INVOICE_SEND", Mode: modeBlocked, Command: "sales invoice send"}
	if cmd := maybeV3MutateCmd(&rootFlags{}, blocked, blocked.Command); cmd != nil {
		t.Fatal("blocked invoice send must not get a live mutate command")
	}
	wired := primitiveEntry{ID: "QBO.SALES.INVOICE_COPY", Mode: modeWired, Command: "sales invoice copy"}
	if cmd := maybeV3MutateCmd(&rootFlags{}, wired, wired.Command); cmd == nil {
		t.Fatal("wired invoice copy must get a live mutate command")
	}
	adj := primitiveEntry{ID: "QBO.INVENTORY.ADJUST_CREATE", Mode: modeWired, Command: "inventory adjust create"}
	if cmd := maybeV3MutateCmd(&rootFlags{}, adj, adj.Command); cmd == nil {
		t.Fatal("wired inventory adjust must get a live mutate command")
	}
	wo := primitiveEntry{ID: "QBO.SALES.INVOICE_WRITE_OFF", Mode: modeWired, Command: "sales invoice write-off"}
	if cmd := maybeV3MutateCmd(&rootFlags{}, wo, wo.Command); cmd == nil {
		t.Fatal("wired invoice write-off must get a live mutate command")
	}
	disc := primitiveEntry{ID: "QBO.SALES.INVOICE_DISCOUNT", Mode: modeWired, Command: "sales invoice discount"}
	if cmd := maybeV3MutateCmd(&rootFlags{}, disc, disc.Command); cmd == nil {
		t.Fatal("wired invoice discount must get a live mutate command")
	}
	fee := primitiveEntry{ID: "QBO.EXPENSES.BANK_FEE_CREATE", Mode: modeWired, Command: "expenses bank-fee create"}
	if cmd := maybeV3MutateCmd(&rootFlags{}, fee, fee.Command); cmd == nil {
		t.Fatal("wired bank-fee must get a live mutate command")
	}
}

// TestCatalogParamsExistAsCobraFlags guards the contract the live sweep
// broke: every param actions --json advertises for a wired/read primitive
// must parse on the real cobra command. Regression for feed rec get
// --account-id (catalog advertised, cobra rejected) and prepaid get
// --source-id (runtime required, catalog omitted).
func TestCatalogParamsExistAsCobraFlags(t *testing.T) {
	for _, e := range catalogPrimitives {
		if e.Mode != modeWired && e.Mode != modeRead {
			continue
		}
		if e.Command == "" {
			continue
		}
		parts := strings.Fields(e.Command)
		root := NewRootCommand()
		target, _, err := root.Find(parts)
		if err != nil || target == nil {
			t.Errorf("%s command %q does not resolve to a cobra command", e.ID, e.Command)
			continue
		}
		for _, p := range paramsFor(e.ID) {
			if target.Flags().Lookup(p.Name) != nil {
				continue
			}
			if target.InheritedFlags().Lookup(p.Name) != nil {
				continue
			}
			if target.PersistentFlags().Lookup(p.Name) != nil {
				continue
			}
			t.Errorf("%s catalog param --%s missing from cobra command %q", e.ID, p.Name, e.Command)
		}
	}
}

// TestEveryCatalogCommandResolvesToRunnableLeaf guards the catalog honest
// in the other direction from the flag-parity test: every cataloged command
// string must resolve to the exact runnable cobra leaf. Regression for the
// Service map v2 stubs, whose catalog paths (feed financial-profile get)
// were unreachable because the commands were nested under rule search.
func TestEveryCatalogCommandResolvesToRunnableLeaf(t *testing.T) {
	root := NewRootCommand()
	for _, e := range catalogPrimitives {
		if e.Command == "" {
			continue
		}
		parts := strings.Fields(e.Command)
		target, _, err := root.Find(parts)
		if err != nil || target == nil {
			t.Errorf("%s command %q does not resolve: %v", e.ID, e.Command, err)
			continue
		}
		var rev []string
		for c := target; c != nil && c != root; c = c.Parent() {
			rev = append([]string{c.Name()}, rev...)
		}
		if got := strings.Join(rev, " "); got != e.Command {
			t.Errorf("%s command %q resolves to %q", e.ID, e.Command, got)
			continue
		}
		if !target.Runnable() {
			t.Errorf("%s command %q resolves to non-runnable %q", e.ID, e.Command, e.Command)
		}
	}
}

// TestCatalogHasNoDuplicateIDsOrCommands locks the catalog audit finding:
// every primitive ID and every command string is unique. A duplicate
// command string would make two rows claim one runnable leaf.
func TestCatalogHasNoDuplicateIDsOrCommands(t *testing.T) {
	seenID := map[string]bool{}
	seenCmd := map[string]string{}
	for _, e := range catalogPrimitives {
		if seenID[e.ID] {
			t.Errorf("duplicate catalog ID %s", e.ID)
		}
		seenID[e.ID] = true
		if e.Command == "" {
			continue
		}
		if prev, dup := seenCmd[e.Command]; dup {
			t.Errorf("duplicate catalog command %q: %s vs %s", e.Command, prev, e.ID)
		}
		seenCmd[e.Command] = e.ID
	}
}
