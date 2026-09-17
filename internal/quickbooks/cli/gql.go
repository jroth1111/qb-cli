package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql"
	"github.com/spf13/cobra"
)

// contextWithTimeout bounds a gql command by the persistent --timeout.
func contextWithTimeout(cmd *cobra.Command, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(cmd.Context(), d)
}

// newGqlCmd builds the `qb gql` tree: catalog browsing plus relay-backed
// execution of captured GraphQL operations. All network paths run inside
// the authenticated browser session (offline header replay 401s).
func newGqlCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gql",
		Short: "Captured GraphQL operations (list, query, mutate, walk)",
		Long: "qb gql runs GraphQL operations captured from the QBO webapp.\n\n" +
			"Execution happens inside your authenticated qbo.intuit.com browser tab\n" +
			"via ego-browser; these GraphQL hosts require the browser session.\n" +
			"Browse locally with `qb gql ops <filter>`; run with `qb gql query|mutate|walk`.",
		SilenceUsage: true,
	}
	cmd.AddCommand(newGqlOpsCmd(flags))
	cmd.AddCommand(newGqlRunCmd(flags, gql.KindQuery))
	mutate := newGqlMutateCmd(flags)
	for _, sub := range newGqlMutationFlagCmds(flags) {
		mutate.AddCommand(sub)
	}
	cmd.AddCommand(mutate)
	cmd.AddCommand(newGqlWalkCmd(flags))
	return cmd
}

// newGqlOpsCmd implements `qb gql ops [substr]`: pure local listing.
func newGqlOpsCmd(flags *rootFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "ops [substr]",
		Short: "List captured operations matching substr (no network)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			substr := ""
			if len(args) > 0 {
				substr = args[0]
			}
			ops := gql.Ops(substr)
			if flags.asJSON {
				type opRow struct {
					Name     string   `json:"name"`
					Kind     string   `json:"kind"`
					Endpoint string   `json:"endpoint"`
					Module   string   `json:"module"`
					VarTypes []string `json:"varTypes,omitempty"`
				}
				rows := make([]opRow, len(ops))
				for i, op := range ops {
					rows[i] = opRow{op.Name, op.Kind, op.Endpoint, op.Module, op.VarTypes}
				}
				return json.NewEncoder(cmd.OutOrStdout()).Encode(rows)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%d operations\n", len(ops))
			for _, op := range ops {
				fmt.Fprintf(out, "%-50s %-8s %s\n", op.Name, op.Kind, endpointHost(op.Endpoint))
				if len(op.VarTypes) > 0 {
					fmt.Fprintf(out, "  vars: %s\n", strings.Join(op.VarTypes, ", "))
				}
			}
			return nil
		},
	}
}

// newGqlRunCmd implements both `qb gql query <Op>` and
// `qb gql mutate <Op>` — same path, different catalog kind filter.
func newGqlRunCmd(flags *rootFlags, kind string) *cobra.Command {
	var rawVars []string
	use := "query"
	short := "Execute a captured query"
	if kind == "mutate" {
		use = "mutate"
		short = "Execute a captured mutation (requires --yes or --dry-run)"
	}
	if kind == gql.KindMutation {
		use = "mutate"
		short = "Execute a captured mutation"
	}
	cmd := &cobra.Command{
		Use:   use + " <OpName> [--vars k=v ...]",
		Short: short + " via the ego-browser tab",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			op, err := gql.Lookup(args[0])
			if err != nil {
				return err
			}
			if kind == gql.KindMutation && op.Kind != gql.KindMutation {
				return fmt.Errorf("%s is a %s; use `qb gql query %s`", op.Name, op.Kind, op.Name)
			}
			if kind == gql.KindQuery && op.Kind != gql.KindQuery {
				return fmt.Errorf("%s is a %s; use `qb gql mutate %s`", op.Name, op.Kind, op.Name)
			}
			vars, err := parseVars(rawVars)
			if err != nil {
				return err
			}
			if err := op.ValidateVars(vars); err != nil {
				return exitInput(err)
			}
			// Mutations are writes: require explicit confirmation.
			if op.Kind == "mutation" && !flags.yes && !flags.dryRun {
				return &ExitError{Code: ExitInputError, Err: fmt.Errorf(
					"mutation requires --yes to execute or --dry-run to preview")}
			}
			if flags.dryRun {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"dry_run": true, "dial": false,
					"operation": op.Name, "variables": vars,
				})
			}
			ctx, cancel := contextWithTimeout(cmd, flags.timeout)
			defer cancel()
			resp, err := gql.Execute(ctx, gql.Request{Op: op, Variables: vars})
			if err != nil {
				return exitRelay(err)
			}
			return writeGqlResponse(cmd.OutOrStdout(), flags.asJSON, op.Endpoint, resp)
		},
	}
	cmd.Flags().StringSliceVar(&rawVars, "vars", nil, "variables as k=v pairs (k must match the op's declared $k)")
	return cmd
}

// writeGqlResponse preserves machine-readable errors and still fails the command.
func writeGqlResponse(out io.Writer, asJSON bool, endpoint string, resp *gql.Response) error {
	if resp == nil || !json.Valid(resp.Body) {
		return fmt.Errorf("GraphQL response is not valid JSON")
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(resp.Body, &envelope) != nil || envelope == nil {
		return fmt.Errorf("GraphQL response must be an object")
	}
	if _, data := envelope["data"]; !data && len(resp.Errors) == 0 {
		return fmt.Errorf("GraphQL response has neither data nor errors")
	}
	if asJSON {
		if err := json.NewEncoder(out).Encode(resp); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "POST %s -> %d\n", endpoint, resp.Status)
		pretty, _ := json.MarshalIndent(envelope, "", "  ")
		fmt.Fprintln(out, string(pretty))
	}
	if len(resp.Errors) > 0 {
		msgs := make([]string, len(resp.Errors))
		for i, e := range resp.Errors {
			msgs[i] = e.Message
		}
		return fmt.Errorf("GraphQL errors: %s", strings.Join(msgs, "; "))
	}
	if resp.Status >= 400 {
		return &ExitError{Code: ExitRelayError, Err: fmt.Errorf("HTTP %d from %s", resp.Status, endpoint)}
	}
	return nil
}

func gqlWalkError(res *gql.WalkResult) error {
	if len(res.Errors) > 0 {
		return fmt.Errorf("GraphQL errors: %s", strings.Join(res.Errors, "; "))
	}
	return nil
}

// newGqlWalkCmd implements `qb gql walk <OpName> --from --to [--page-size N]`.
func newGqlWalkCmd(flags *rootFlags) *cobra.Command {
	var (
		rawVars  []string
		maxPages int
		pageSize int
	)
	cmd := &cobra.Command{
		Use:   "walk <OpName>",
		Short: "Walk all pages of a paginated query and merge nodes",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			op, err := gql.Lookup(args[0])
			if err != nil {
				return err
			}
			if op.Kind != gql.KindQuery {
				return fmt.Errorf("walk applies to queries only; %s is a %s", op.Name, op.Kind)
			}
			vars, err := parseVars(rawVars)
			if err != nil {
				return err
			}
			if err := op.ValidateVars(vars); err != nil {
				return exitInput(err)
			}
			w := gql.Walker{MaxPages: maxPages}
			if pageSize > 0 {
				w.PageSize = pageSize
			}
			ctx, cancel := contextWithTimeout(cmd, flags.timeout)
			defer cancel()
			res, err := gql.Walk(ctx, gql.Request{Op: op, Variables: vars}, w)
			if err != nil {
				if res != nil && res.Pages > 0 && flags.asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					_ = enc.Encode(res)
					return exitRelay(err)
				}
				return exitRelay(err)
			}
			if flags.asJSON {
				if err := json.NewEncoder(cmd.OutOrStdout()).Encode(res); err != nil {
					return err
				}
				return gqlWalkError(res)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s: style=%s pages=%d nodes=%d", res.Op, res.Style, res.Pages, len(res.Nodes))
			if res.Truncated {
				fmt.Fprint(out, " TRUNCATED")
			}
			fmt.Fprintln(out)
			for _, n := range res.Nodes {
				b, err := json.Marshal(n)
				if err == nil {
					fmt.Fprintln(out, string(b))
				}
			}
			return gqlWalkError(res)
		},
	}
	cmd.Flags().IntVar(&maxPages, "max-pages", 10, "hard ceiling on pages fetched (0 = walk to exhaustion)")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "rows bound to $first/$limit per page")
	cmd.Flags().StringSliceVar(&rawVars, "vars", nil, "variables as k=v pairs")
	// First-class walks hang off `walk` itself: qb gql walk bills|tasks|items
	// route to their dedicated commands; any other name still reaches the
	// generic catalog walker above.
	cmd.AddCommand(newGqlWalkBillsCmd(flags))
	cmd.AddCommand(newGqlWalkTasksCmd(flags))
	cmd.AddCommand(newGqlWalkItemsCmd(flags))
	return cmd
}

// gqlDateFlags binds --from/--to date strings in dd/MM/yyyy (ISO also
// accepted).
type gqlDateFlags struct {
	from, to string
}

func (d *gqlDateFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&d.from, "from", "", "window start, dd/MM/yyyy (or yyyy-MM-dd)")
	cmd.Flags().StringVar(&d.to, "to", "", "window end, dd/MM/yyyy (or yyyy-MM-dd)")
}

// runNamedWalk is the shared engine behind `qb gql walk bills|tasks|items`:
// it parses the date window, executes the paged query through the relay,
// and prints either the WalkResult JSON (--json) or one merged node per
// line.
func runNamedWalk(cmd *cobra.Command, flags *rootFlags, opName string, spec gql.DateWindowSpec, dates gqlDateFlags, w gql.Walker, extraVars map[string]any) error {
	win, err := spec.ParseWindow(dates.from, dates.to)
	if err != nil {
		return exitInput(err)
	}
	if spec.Field != "" {
		w.Window = win
	}
	op, err := gql.Lookup(opName)
	if err != nil {
		return err
	}
	if op.Kind != gql.KindQuery {
		return fmt.Errorf("walk applies to queries only; %s is a %s", op.Name, op.Kind)
	}
	vars := extraVars
	if vars == nil {
		vars = map[string]any{}
	}
	if opName == "GetBills" {
		request := gql.BillsRequest(win)
		op, vars, w.Window = request.Op, request.Variables, nil
	}
	if opName == "Items" {
		request := gql.ItemsRequest()
		op, vars, w.InputVar = request.Op, request.Variables, ""
		w.NodesPath = []string{"data", "result", "entities"}
	}
	if err := op.ValidateVars(vars); err != nil {
		return exitInput(err)
	}
	ctx, cancel := contextWithTimeout(cmd, flags.timeout)
	defer cancel()
	res, err := gql.Walk(ctx, gql.Request{Op: op, Variables: vars}, w)
	if err != nil {
		if res != nil && res.Pages > 0 && flags.asJSON {
			_ = json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			return exitRelay(err)
		}
		return exitRelay(err)
	}
	out := cmd.OutOrStdout()
	if flags.asJSON {
		if err := json.NewEncoder(out).Encode(res.Nodes); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "%s: style=%s pages=%d nodes=%d", res.Op, res.Style, res.Pages, len(res.Nodes))
		if res.Truncated {
			fmt.Fprint(out, " TRUNCATED")
		}
		fmt.Fprintln(out)
		for _, n := range res.Nodes {
			b, err := json.Marshal(n)
			if err == nil {
				fmt.Fprintln(out, string(b))
			}
		}
	}
	return gqlWalkError(res)
}

// newGqlWalkBillsCmd implements `qb gql walk bills --from --to`: every bill
// whose transaction date falls in the window, merged across all pages.
func newGqlWalkBillsCmd(flags *rootFlags) *cobra.Command {
	var (
		dates    gqlDateFlags
		maxPages int
		pageSize int
	)
	cmd := &cobra.Command{
		Use:   "bills",
		Short: "Walk QBO bills, optionally filtered by transaction date",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := gql.Walker{MaxPages: maxPages}
			if pageSize > 0 {
				w.PageSize = pageSize
			}
			return runNamedWalk(cmd, flags, "GetBills", gql.BillDateWindow, dates, w, nil)
		},
	}
	dates.register(cmd)
	cmd.Flags().IntVar(&maxPages, "max-pages", 10, "hard ceiling on pages fetched (0 = walk to exhaustion)")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "rows per page")
	return cmd
}

// newGqlWalkTasksCmd implements `qb gql walk tasks --from --to`: every
// task whose due date falls in the window, merged across all pages.
func newGqlWalkTasksCmd(flags *rootFlags) *cobra.Command {
	var (
		dates    gqlDateFlags
		maxPages int
		pageSize int
	)
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "Walk all tasks (TaskManagementTasks), optionally filtered by due date",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := gql.Walker{MaxPages: maxPages}
			if pageSize > 0 {
				w.PageSize = pageSize
			}
			return runNamedWalk(cmd, flags, "TaskManagementTasks", gql.TaskDueDateWindow, dates, w, taskPanelVariables())
		},
	}
	dates.register(cmd)
	cmd.Flags().IntVar(&maxPages, "max-pages", 10, "hard ceiling on pages fetched (0 = walk to exhaustion)")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "rows per page")
	return cmd
}

// taskPanelVariables follows the QBO task-panel request captured on 2026-09-12.
// The service rejects an empty filter; the union covers the panel's namespaces.
func taskPanelVariables() map[string]any {
	namespaces := []string{"DTM_TASKS", "ADV_NTTF_SETUP_TASKS", "ADV_UPGRADER_SETUP_TASKS", "BASIC_BUSINESS_INFO_TASK", "APP_ONBOARDING_TASK", "IES_SETUP_TASKS", "BANKING_TASKS", "SPEND_TASKS", "BILL_PAY_TASKS", "CUSTOMER_HUB_TASKS", "PAYMENTS_TASKS", "INVOICING_TASKS", "INDIRECT_TAX", "QBL_TASKS", "QBL_FREE_SETUP_TASK", "MAILCHIMP_TASK"}
	filters := make([]any, 0, len(namespaces))
	for _, name := range namespaces {
		filters = append(filters, map[string]any{"domain": map[string]any{"equals": "QBO"}, "useCase": map[string]any{"equals": name}})
	}
	return map[string]any{"filter": map[string]any{"namespace": map[string]any{"in": filters}}}
}

// newGqlWalkItemsCmd implements `qb gql walk items`: the product/service
// catalog as a baseline walk (no date filter; native offset pagination).
func newGqlWalkItemsCmd(flags *rootFlags) *cobra.Command {
	var (
		maxPages int
		pageSize int
	)
	cmd := &cobra.Command{
		Use:   "items",
		Short: "Walk the product/service catalog and variants using native pagination",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := gql.Walker{MaxPages: maxPages, InputVar: "getItemsInput"}
			if pageSize > 0 {
				w.PageSize = pageSize
			}
			return runNamedWalk(cmd, flags, "Items", gql.DateWindowSpec{}, gqlDateFlags{}, w, nil)
		},
	}
	cmd.Flags().IntVar(&maxPages, "max-pages", 10, "hard ceiling on pages fetched (0 = walk to exhaustion)")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "rows per page")
	return cmd
}

// parseVars converts k=v CLI strings into a variables map. Values stay
// strings unless they are bare integers/floats or true/false/null, which
// covers most scalar GraphQL inputs. JSON object/list values may be passed
// verbatim when they start with { or [.
func parseVars(raw []string) (map[string]any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(map[string]any, len(raw))
	for _, kv := range raw {
		name, val, ok := strings.Cut(kv, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("--vars expects k=v, got %q", kv)
		}
		name = strings.TrimPrefix(name, "$")
		out[name] = coerceVar(val)
	}
	return out, nil
}

func coerceVar(val string) any {
	switch val {
	case "true":
		return true
	case "false":
		return false
	case "null":
		return nil
	case "":
		return nil
	}
	if strings.HasPrefix(val, "{") || strings.HasPrefix(val, "[") {
		var v any
		if json.Unmarshal([]byte(val), &v) == nil {
			return v
		}
		return val
	}
	if i, err := strconv.Atoi(val); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(val, 64); err == nil {
		return f
	}
	return val
}

// endpointHost trims an endpoint URL to its host for compact listing.
func endpointHost(endpoint string) string {
	s := strings.TrimPrefix(endpoint, "https://")
	s = strings.TrimPrefix(s, "http://")
	return s
}

// exitInput tags user-input failures with ExitInputError.
func exitInput(err error) error { return &ExitError{Code: ExitInputError, Err: err} }

// exitRelay tags transport failures with ExitRelayError after mapping
// ErrNeedsLogin to its actionable message.
func exitRelay(err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: ExitRelayError, Err: err}
}

// gqlMutationFlagCmds declares the top-value mutations as named
// `qb gql mutate <verb>` subcommands with typed flags, so users never touch
// --vars or input JSON shapes. Each spec maps its flags onto the exact
// variable envelope captured from the QBO webapp (verified against the
// deobfuscated capture corpus), then reuses runMutationOp for validation,
// relay transport and output.
type gqlMutationFlagSpec struct {
	use     string // subcommand verb under `qb gql mutate`
	opName  string // catalog operation name
	short   string
	long    string
	example string
	// endpoint overrides the catalog's captured host when the SPA provably
	// routes the operation elsewhere (e.g. fitGql ops → fitransactions).
	// Empty means "use the catalog endpoint".
	endpoint string
	build    func(cmd *cobra.Command) (map[string]any, error)
	// run replaces the GraphQL relay path entirely for ops whose live
	// contract is a first-party REST call (e.g. bank-disconnect rides neo
	// lists/account/save, not the captured fitransactions mutation).
	run func(cmd *cobra.Command, flags *rootFlags) error
}

// gqlMutationFlagSpecTable is the single source of truth for the named
// mutation subcommands; tests iterate it to stay drift-free with what
// ships in the command tree.
func gqlMutationFlagSpecTable() []gqlMutationFlagSpec {
	return []gqlMutationFlagSpec{
		{
			use:    "create-account",
			opName: "CreateAccount",
			short:  "Create a ledger account",
			long: "Create a ledger account via CreateAccount on the verified\n" +
				"coa-core.api.intuit.com host — the document and route the live\n" +
				"chart-of-accounts SPA uses — without hand-building the\n" +
				"Accounting_CreateAccountInput envelope. --name and --subtype are\n" +
				"required; --type defaults to BANK. Enum values are normalized to\n" +
				"UPPER_SNAKE before sending (e.g. --subtype 'cash and cash\n" +
				"equivalents' → CASH_AND_CASH_EQUIVALENTS).",
			example: "  qb gql mutate create-account --name 'Payroll Clearing' --subtype Checking\n" +
				"  qb gql mutate create-account --name Petty\\ Cash --subtype 'cash and cash equivalents' --type BANK --number 1010",
			build: func(cmd *cobra.Command) (map[string]any, error) {
				name, _ := cmd.Flags().GetString("name")
				subtype, _ := cmd.Flags().GetString("subtype")
				if name == "" || subtype == "" {
					return nil, fmt.Errorf("--name and --subtype are required")
				}
				input := map[string]any{
					"name":        name,
					"accountType": gqlEnumUpper(mustString(cmd, "type", "Bank")),
					"detailType":  gqlEnumUpper(subtype),
					"description": mustString(cmd, "description", ""),
					"number":      mustString(cmd, "number", ""),
				}
				for flag, key := range map[string]string{
					"parent-id": "parentAccountId",
					"currency":  "currency",
				} {
					if v := mustString(cmd, flag, ""); v != "" {
						input[key] = v
					}
				}
				return map[string]any{"input": input}, nil
			},
		},
		{
			use:    "bank-disconnect",
			opName: "BankingDisconnectOlbAccounts",
			short:  "Disconnect bank feed on GL accounts (--id)",
			long: "Disconnect the bank feed on one or more GL accounts via the\n" +
				"first-party neo save the banking UI uses: GET\n" +
				"/api/neo/v1/company/{realm}/lists/account/{id} then POST\n" +
				"lists/account/save with disconnectAccount:true. Proven live on\n" +
				"Test Company 2 (2026-09-16).\n\n" +
				"The catalogued BankingDisconnectOlbAccounts (fitransactions)\n" +
				"403s in current AU builds — this command wires the observed\n" +
				"truth instead. DESTRUCTIVE: pending feed rows are dropped and\n" +
				"reconnecting needs bank credentials.",
			example: "  qb gql mutate bank-disconnect --id 44 --yes\n" +
				"  qb gql mutate bank-disconnect --id 44 --id 45 --yes",
			build: func(cmd *cobra.Command) (map[string]any, error) {
				return nil, fmt.Errorf("bank-disconnect rides neo REST, not the GraphQL op")
			},
			run: func(cmd *cobra.Command, flags *rootFlags) error {
				raw, _ := cmd.Flags().GetStringSlice("id")
				if len(raw) == 0 {
					return exitInput(fmt.Errorf("--id is required (GL account id, repeatable)"))
				}
				fm := collectFlags(cmd)
				if flags.dryRun {
					return writePlan(cmd, flags, planEnvelope{
						Command: "gql mutate bank-disconnect",
						ID:      "QBO.GQL.MUTATE_BANK_DISCONNECT",
						Mode:    modeWired,
						Method:  "POST",
						URL:     client.PlannedBankDisconnectURL(),
						Flags:   fm,
						Note:    "neo lists/account/save disconnectAccount:true; not sent",
					})
				}
				if !flags.yes {
					return &ExitError{Code: ExitInputError, Err: fmt.Errorf(
						"disconnect is destructive: pending feed rows are dropped; pass --yes to execute"), Silent: flags.asJSON}
				}
				ctx, cancel := contextWithTimeout(cmd, flags.timeout)
				defer cancel()
				res, err := client.ReplayBankDisconnect(ctx, fm)
				if err != nil {
					return feedErr(flags, err)
				}
				if flags.asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(res)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.ID)
				return nil
			},
		},
		{
			use:    "batch-update-products",
			opName: "BatchUpdateProducts",
			short:  "Bulk product updates from JSON rows (--products-json)",
			long: "Run Commerce batchUpdateProducts with rows read from --products-json\n" +
				"(inline JSON array of Commerce_BatchUpdateProductInput objects, or an\n" +
				"@file reference). The SPA builds these rows programmatically; rather\n" +
				"than guess its per-field flags, this exposes the array verbatim.",
			example: "  qb gql mutate batch-update-products --products-json @rows.json\n" +
				"  qb gql mutate batch-update-products --products-json '[{\"correlationId\":\"3\",\"product\":{\"id\":\"3\",\"version\":0,\"type\":\"PHYSICAL\",\"name\":\"CR-Test-Item\",\"variants\":[{\"id\":\"3\",\"version\":0,\"name\":\"CR-Test-Item\",\"price\":\"0\",\"cost\":\"0\",\"sku\":\"P28SKU\",\"variabilityValues\":[],\"inventoryItems\":[]}]}}]'",
			build: func(cmd *cobra.Command) (map[string]any, error) {
				raw, _ := cmd.Flags().GetString("products-json")
				if raw == "" {
					return nil, fmt.Errorf("--products-json is required (@file or inline JSON array)")
				}
				b, err := gqlFlagJSONArg(raw)
				if err != nil {
					return nil, err
				}
				var rows []any
				if err := json.Unmarshal(b, &rows); err != nil {
					return nil, fmt.Errorf("--products-json must decode to a JSON array: %w", err)
				}
				if len(rows) == 0 {
					return nil, fmt.Errorf("--products-json must contain at least one row")
				}
				return map[string]any{"products": rows}, nil
			},
		},
		{
			use:    "assign-dimensions",
			opName: "AssignDimensionsForProducts",
			short:  "Assign custom dimensions to products",
			long: "Assign product dimension values via AssignDimensionsForProducts.\n" +
				"The SPA forwards its task payload straight through as $input, so this\n" +
				"command accepts the AssignProductDimensionsInput object verbatim via\n" +
				"--input-json (@file allowed) while still validating required-ness.\n" +
				"Live contract (2026-09-16): input.products[] needs {id, version};\n" +
				"input.dimensions[] needs {id, value} — definition and value ids come\n" +
				"from GetCustomDimensionDefinitions/GetCustomDimensionValues.",
			example: "  qb gql mutate assign-dimensions --input-json '{\"products\":[{\"id\":\"3\",\"version\":2}],\"dimensions\":[{\"id\":\"<defId>\",\"value\":\"<valueId>\"}]}'",
			build: func(cmd *cobra.Command) (map[string]any, error) {
				input, err := gqlFlagInputObject(cmd)
				if err != nil {
					return nil, err
				}
				return map[string]any{"input": input}, nil
			},
		},
		{
			use:    "cancel-reconcile",
			opName: "CancelReconcile__integration_banking_reconcile_ui_qbo",
			short:  "Cancel an in-flight reconcile session",
			long: "Cancel a banking reconciliation session via\n" +
				"CancelReconcile__integration_banking_reconcile_ui_qbo. The webapp wraps\n" +
				"every reconciliation call in the same pluginInfo/clientMutationId\n" +
				"envelope; only --reconciliation-id varies between sessions.",
			example: "  qb gql mutate cancel-reconcile --reconciliation-id 12345:integration/Reconciliation:55",
			build: func(cmd *cobra.Command) (map[string]any, error) {
				id, _ := cmd.Flags().GetString("reconciliation-id")
				action, _ := cmd.Flags().GetString("action")
				if id == "" {
					return nil, fmt.Errorf("--reconciliation-id is required")
				}
				return map[string]any{"input_0": map[string]any{
					"pluginInfo":       map[string]any{"customHeaders": map[string]any{}},
					"clientMutationId": "qb-cli",
					"integrationReconciliation": map[string]any{
						"id":              id,
						"reconcileAction": action,
					},
				}}, nil
			},
		},
		{
			use:    "receive-inventory",
			opName: "CommerceReceiveInventory",
			short:  "Record inbound inventory movement (warehouse svc)",
			long: "Record inbound stock against a source transaction via\n" +
				"CommerceReceiveInventory on warehouse-management-svc. The source\n" +
				"transaction identity comes from --txn-id/--txn-type/--source-version\n" +
				"(+ optional date, vendor, customer); line items come from\n" +
				"--lines-json (@file allowed) and must carry the line identity fields\n" +
				"the service requires (sourceTransactionLineId, lineOrder).\n" +
				"--txn-date takes an RFC3339 full date (yyyy-MM-dd).",
			example: "  qb gql mutate receive-inventory --txn-id 198 --txn-type Bill --source-version 1 \\\n" +
				"    --lines-json '[{\"itemId\":\"72\",\"quantity\":3,\"inventoryLocationId\":\"L1\",\"sourceTransactionLineId\":\"1\",\"lineOrder\":1}]'",
			build: func(cmd *cobra.Command) (map[string]any, error) {
				input, err := gqlCommerceInventoryInput(cmd, "lines-json")
				if err != nil {
					return nil, err
				}
				return map[string]any{"input": input}, nil
			},
		},
		{
			use:    "consume-inventory",
			opName: "CommerceConsumeInventory",
			short:  "Record outbound inventory movement (warehouse svc)",
			long: "Record outbound stock against a source transaction via\n" +
				"CommerceConsumeInventory on warehouse-management-svc. Same flags as\n" +
				"receive-inventory — including required --source-version\n" +
				"(sourceTransaction.version, NonNull Int!); only the mutation differs.",
			example: "  qb gql mutate consume-inventory --txn-id 201 --txn-type Invoice --source-version 1 \\\n" +
				"    --lines-json '[{\"itemId\":\"72\",\"quantity\":1,\"sourceTransactionLineId\":\"2\",\"lineOrder\":1}]'",
			build: func(cmd *cobra.Command) (map[string]any, error) {
				input, err := gqlCommerceInventoryInput(cmd, "lines-json")
				if err != nil {
					return nil, err
				}
				return map[string]any{"input": input}, nil
			},
		},
		{
			use:    "activate-products",
			opName: "UpdateItemStatus",
			short:  "Reactivate inactivated products/services",
			long: "Reactivate products/services via UpdateItemStatus on the\n" +
				"commerce-control supergraph. Entities are built from repeatable flags;\n" +
				"each --entity-id gets entityType\n" +
				"(--entity-type, default INVENTORY) and version (--entity-version).\n" +
				"For mixed batches pass full JSON instead via --entities-json.",
			example: "  qb gql mutate activate-products --entity-id 72 --entity-id 73\n" +
				"  qb gql mutate activate-products --entities-json '[{\"id\":\"72\",\"version\":0,\"type\":\"NON_INVENTORY\"}]'",
			build: func(cmd *cobra.Command) (map[string]any, error) {
				rawEntities, _ := cmd.Flags().GetString("entities-json")
				ids, _ := cmd.Flags().GetStringSlice("entity-id")
				typ, _ := cmd.Flags().GetString("entity-type")
				version, _ := cmd.Flags().GetInt("entity-version")
				switch {
				case rawEntities != "":
					b, err := gqlFlagJSONArg(rawEntities)
					if err != nil {
						return nil, err
					}
					var entities []any
					if err := json.Unmarshal(b, &entities); err != nil {
						return nil, fmt.Errorf("--entities-json must decode to a JSON array: %w", err)
					}
					return map[string]any{"entities": entities}, nil
				case len(ids) > 0:
					entities := make([]any, 0, len(ids))
					for _, id := range ids {
						if id == "" {
							return nil, fmt.Errorf("--entity-id values must be non-empty")
						}
						entities = append(entities, map[string]any{"id": id, "version": version, "entityType": typ, "actionType": "ACTIVATE"})
					}
					return map[string]any{"entities": entities}, nil
				default:
					return nil, fmt.Errorf("one of --entity-id or --entities-json is required")
				}
			},
		},
	}
}

func newGqlMutationFlagCmds(flags *rootFlags) []*cobra.Command {
	specs := gqlMutationFlagSpecTable()
	cmds := make([]*cobra.Command, 0, len(specs))
	for i := range specs {
		cmds = append(cmds, newGqlMutationFlagCmd(flags, specs[i]))
	}
	return cmds
}

// newGqlMutationFlagCmd materializes one named mutation subcommand: typed
// flags declared by the spec's register hook, variables built by the spec,
// then the shared relay execution path.
func newGqlMutationFlagCmd(flags *rootFlags, spec gqlMutationFlagSpec) *cobra.Command {
	cmd := &cobra.Command{
		Use:   spec.use,
		Short: spec.short + " (" + spec.opName + ")",
		Long:  spec.long + "\n\nRuns " + spec.opName + " inside the authenticated browser tab.\nExample:\n" + spec.example,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if spec.run != nil {
				return spec.run(cmd, flags)
			}
			vars, err := spec.build(cmd)
			if err != nil {
				return exitInput(err)
			}
			return runMutationOp(cmd, flags, spec.opName, vars, spec.endpoint)
		},
	}
	gqlDeclareMutationFlags(cmd, spec.opName)
	return cmd
}

// gqlDeclareMutationFlags registers the typed flags each named mutation
// subcommand understands. Kept next to the specs so a new subcommand needs
// exactly two edits: a spec above and a case here.
func gqlDeclareMutationFlags(cmd *cobra.Command, opName string) {
	fs := cmd.Flags()
	switch opName {
	case "CreateAccount":
		fs.String("name", "", "account name (required)")
		fs.String("subtype", "", "detailType enum, e.g. Checking, Savings, 'cash and cash equivalents' (required)")
		fs.String("type", "Bank", "accountType enum (default BANK)")
		fs.String("description", "", "description")
		fs.String("number", "", "account number")
		fs.String("parent-id", "", "parentAccountId for sub-accounts")
		fs.String("currency", "", "currency code (default company currency)")
	case "BankingDisconnectOlbAccounts":
		fs.StringSlice("id", nil, "GL account id to disconnect, repeatable (required)")
	case "BatchUpdateProducts":
		fs.String("products-json", "", "JSON array of Commerce_BatchUpdateProductInput rows, @file allowed (required)")
	case "AssignDimensionsForProducts":
		fs.String("input-json", "", "AssignProductDimensionsInput object as JSON, @file allowed (required)")
	case "CancelReconcile__integration_banking_reconcile_ui_qbo":
		fs.String("reconciliation-id", "", "Integration_Reconciliation global id (required)")
		fs.String("action", "CANCELLED", "reconcileAction enum value (default CANCELLED)")
	case "CommerceReceiveInventory", "CommerceConsumeInventory":
		fs.String("txn-id", "", "source transaction id (required)")
		fs.String("txn-type", "", "source transaction type, e.g. Bill, Invoice (required)")
		fs.Int("source-version", 0, "sourceTransaction.version, NonNull Int! (required, must be > 0)")
		fs.String("txn-date", "", "source transaction date, ISO 8601 yyyy-MM-dd (optional)")
		fs.String("vendor-id", "", "vendor id (Bill flows)")
		fs.String("customer-id", "", "customer id (Invoice flows)")
		fs.String("lines-json", "", "movement lines as JSON array, @file allowed (required)")
	case "UpdateItemStatus":
		fs.StringSlice("entity-id", nil, "product/service id to activate, repeatable")
		fs.String("entity-type", "INVENTORY", "entity type enum (default INVENTORY)")
		fs.Int("entity-version", 0, "entity version for optimistic locking")
		fs.String("entities-json", "", "full [InactivateEntity!] JSON array overriding --entity-id, @file allowed")
	}
}

// gqlFlagJSONArg resolves a --*-json flag value: "@path" reads the file,
// anything else is used inline. The result need not be valid JSON yet;
// callers unmarshal into their target shape.
func gqlFlagJSONArg(raw string) ([]byte, error) {
	if after, ok := strings.CutPrefix(raw, "@"); ok {
		b, err := os.ReadFile(after)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", raw, err)
		}
		return b, nil
	}
	return []byte(raw), nil
}

// gqlFlagInputObject decodes a single JSON object argument into a generic
// map, rejecting arrays/scalars so the GraphQL input type is respected.
func gqlFlagInputObject(cmd *cobra.Command) (map[string]any, error) {
	raw, _ := cmd.Flags().GetString("input-json")
	if raw == "" {
		return nil, fmt.Errorf("--input-json is required (@file or inline JSON object)")
	}
	b, err := gqlFlagJSONArg(raw)
	if err != nil {
		return nil, err
	}
	var obj map[string]any
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, fmt.Errorf("--input-json must decode to a JSON object: %w", err)
	}
	if len(obj) == 0 {
		return nil, fmt.Errorf("--input-json must not be empty")
	}
	return obj, nil
}

// gqlCommerceInventoryInput assembles the Commerce_Receive/ConsumeInventoryInput
// shape observed in the order-management-ui capture: a sourceTransaction
// identity plus movement lines. Required-ness of txnId/txnType mirrors the
// SPA's own guards ("txnId is required to fetch inventory movement"), and the
// warehouse service declares sourceTransaction.version NonNull Int! — the live
// service rejected a version-less input, so --source-version must be > 0.
func gqlCommerceInventoryInput(cmd *cobra.Command, linesFlag string) (map[string]any, error) {
	txnID, _ := cmd.Flags().GetString("txn-id")
	txnType, _ := cmd.Flags().GetString("txn-type")
	if txnID == "" || txnType == "" {
		return nil, fmt.Errorf("--txn-id and --txn-type are required")
	}
	version, _ := cmd.Flags().GetInt("source-version")
	if version <= 0 {
		return nil, fmt.Errorf("--source-version is required and must be a positive integer (sourceTransaction.version is NonNull Int!)")
	}
	src := map[string]any{"txnId": txnID, "txnType": txnType, "version": version}
	for flag, key := range map[string]string{
		"txn-date":    "txnDate",
		"vendor-id":   "vendorId",
		"customer-id": "customerId",
	} {
		if v := mustString(cmd, flag, ""); v != "" {
			src[key] = v
		}
	}
	rawLines, _ := cmd.Flags().GetString(linesFlag)
	if rawLines == "" {
		return nil, fmt.Errorf("--%s is required (@file or inline JSON array)", linesFlag)
	}
	b, err := gqlFlagJSONArg(rawLines)
	if err != nil {
		return nil, err
	}
	var lines []any
	if err := json.Unmarshal(b, &lines); err != nil {
		return nil, fmt.Errorf("--%s must decode to a JSON array: %w", linesFlag, err)
	}
	if len(lines) == 0 {
		return nil, fmt.Errorf("--%s must contain at least one movement line", linesFlag)
	}
	return map[string]any{"sourceTransaction": src, "lines": lines}, nil
}

// mustString reads a string flag, tolerating missing declarations so specs
// can probe optional flags defensively.
func mustString(cmd *cobra.Command, name, fallback string) string {
	v, err := cmd.Flags().GetString(name)
	if err != nil || v == "" {
		return fallback
	}
	return v
}

// gqlEnumUpper normalizes a user-facing enum flag to the UPPER_SNAKE wire
// form the GraphQL services declare (the SPA sends "BANK",
// "CASH_AND_CASH_EQUIVALENTS"): spaces fold to underscores and letters
// uppercase, so both "Bank" and "cash and cash equivalents" are accepted.
func gqlEnumUpper(v string) string {
	return strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(v)), " ", "_")
}
