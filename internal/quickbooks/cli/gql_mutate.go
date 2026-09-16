package cli

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql"
	"github.com/spf13/cobra"
)

// newGqlMutateCmd implements `qb gql mutate` against the mutation catalog:
// with no arguments it lists captured mutations locally; with <OpName> it
// validates required variables and executes via the relay tab. Execution
// happens inside the authenticated qbo.intuit.com browser session, exactly
// like `qb gql query`.
func newGqlMutateCmd(flags *rootFlags) *cobra.Command {
	var rawVars []string
	cmd := &cobra.Command{
		Use:   "mutate [OpName|named] [--vars k=v ...]",
		Short: "Execute a captured mutation (no name: list all)",
		Long: "Execute a captured GraphQL mutation inside the authenticated browser tab.\n\n" +
			"Run without a name to list every captured mutation with its variable\n" +
			"signature. Pass --vars k=v for each declared $k; variables the signature\n" +
			"declares non-null are validated locally before any network attempt.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return runMutationList(cmd, flags.asJSON)
			}
			name := args[0]
			if sub, _, err := cmd.Find(args); err == nil && sub != cmd && !isCatalogMutationName(name) {
				return sub.RunE(sub, nil)
			}
			op, err := gql.Lookup(name)
			if err != nil {
				return err
			}
			if op.Kind != gql.KindMutation {
				return fmt.Errorf("%s is a %s; use `qb gql query %s`", op.Name, op.Kind, op.Name)
			}
			vars, ok, err := gqlPrepareVars(cmd, rawVars, name)
			if !ok {
				return err
			}
			return runPreparedMutation(cmd, flags, gql.Request{Op: op, Variables: vars})
		},
	}
	cmd.Flags().StringSliceVar(&rawVars, "vars", nil, "variables as k=v pairs (k must match the mutation's declared $k)")
	return cmd
}

// runMutationList prints the mutation catalog: one row per captured
// mutation plus its declared variable signature; --json emits objects.
func runMutationList(cmd *cobra.Command, asJSON bool) error {
	names := gql.ListMutations()
	out := cmd.OutOrStdout()
	type row struct {
		Name     string   `json:"name"`
		Input    string   `json:"input"`
		Required []string `json:"required,omitempty"`
	}
	rows := make([]row, 0, len(names))
	for _, n := range names {
		sig, _ := gql.LookupMutation(n)
		req, _ := gql.RequiredMutationVars(n)
		rows = append(rows, row{Name: n, Input: sig, Required: req})
	}
	if asJSON {
		return json.NewEncoder(out).Encode(rows)
	}
	fmt.Fprintf(out, "%d mutations captured\n", len(rows))
	for _, r := range rows {
		fmt.Fprintf(out, "%-56s %s\n", r.Name, r.Input)
		if len(r.Required) > 0 {
			q := make([]string, len(r.Required))
			for i, v := range r.Required {
				q[i] = "$" + v
			}
			fmt.Fprintf(out, "  required: %s\n", strings.Join(q, ", "))
		}
	}
	return nil
}

// runMutationOp validates a prepared variable set against the catalog
// signature and executes via the relay tab — the shared tail of every
// named flag subcommand. endpoint optionally overrides the catalog host.
func runMutationOp(cmd *cobra.Command, flags *rootFlags, name string, vars map[string]any, endpoint string) error {
	op, err := gql.Lookup(name)
	if err != nil {
		return err
	}
	req := gql.Request{Op: op, Variables: vars}
	if endpoint != "" {
		req.Endpoint = endpoint
	}
	return runPreparedMutation(cmd, flags, req)
}

// isCatalogMutationName reports whether s names a captured catalog
// mutation; named flag subcommands shadow only when this is false.
func isCatalogMutationName(s string) bool {
	return slices.Contains(gql.ListMutations(), s)
}

// gqlPrepareVars parses --vars k=v pairs for the generic path, then applies
// both validation layers. ok=false carries an already-tagged error.
func gqlPrepareVars(cmd *cobra.Command, raw []string, name string) (map[string]any, bool, error) {
	vars, err := parseVars(raw)
	if err != nil {
		return nil, false, exitInput(err)
	}
	op, err := gql.Lookup(name)
	if err != nil {
		return nil, false, err
	}
	if err := op.ValidateVars(vars); err != nil {
		return nil, false, exitInput(err)
	}
	if err := gql.ValidateMutationVars(name, vars); err != nil {
		return nil, false, exitInput(err)
	}
	return vars, true, nil
}

// runPreparedMutation executes an already-built request via the relay tab
// and renders the response as JSON or formatted text.
func runPreparedMutation(cmd *cobra.Command, flags *rootFlags, req gql.Request) error {
	// Mutations are writes: require --yes or --dry-run.
	if !flags.yes && !flags.dryRun {
		return &ExitError{Code: ExitInputError, Err: fmt.Errorf(
			"mutation requires --yes to execute or --dry-run to preview"), Silent: flags.asJSON}
	}
	if flags.dryRun {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
			"dry_run": true, "dial": false,
			"operation": req.Op.Name, "variables": req.Variables,
		})
	}
	endpoint := effectiveMutationEndpoint(req)
	ctx, cancel := contextWithTimeout(cmd, flags.timeout)
	defer cancel()
	resp, err := gql.Execute(ctx, req)
	if err != nil {
		return exitRelay(err)
	}
	if flags.asJSON {
		return json.NewEncoder(cmd.OutOrStdout()).Encode(resp)
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "POST %s -> %d\n", endpoint, resp.Status)
	var pretty map[string]any
	if json.Unmarshal(resp.Body, &pretty) == nil {
		b, _ := json.MarshalIndent(pretty, "", "  ")
		fmt.Fprintln(out, string(b))
	} else {
		fmt.Fprintln(out, string(resp.Body))
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

func effectiveMutationEndpoint(req gql.Request) string {
	if req.Endpoint != "" {
		return req.Endpoint
	}
	if req.Op != nil {
		return req.Op.Endpoint
	}
	return ""
}
