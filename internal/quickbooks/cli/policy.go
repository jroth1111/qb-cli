package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/policy"
	"github.com/spf13/cobra"
)

// qb policy serves embedded company accounting policy as curated static
// reference data (pp:novel-static-reference): the Mega Style Apartments
// profile recovered from the quickbooks-ui skill. Nothing here dials QBO,
// reads the session, or gates commands by company — the CLI stays
// company-agnostic; these commands only expose the documented rules and
// reference docs so agents stop re-deriving them from prose.
//
// pp:novel-static-reference — data is embedded curated reference content,
// not an API response.

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// resolveCompany returns --company, or the sole embedded profile when
// unambiguous.
func resolveCompany(explicit string) (string, error) {
	if explicit != "" {
		if !policy.CompanyExists(explicit) {
			companies, _ := policy.Companies()
			return "", fmt.Errorf("unknown company profile %q (embedded: %s)", explicit, strings.Join(companies, ", "))
		}
		return explicit, nil
	}
	companies, err := policy.Companies()
	if err != nil {
		return "", err
	}
	if len(companies) == 1 {
		return companies[0], nil
	}
	return "", fmt.Errorf("multiple company profiles embedded; pass --company (have: %s)", strings.Join(companies, ", "))
}

func newPolicyCmd(flags *rootFlags) *cobra.Command {
	var company string
	cmd := &cobra.Command{
		Use:   "policy",
		Short: "Embedded company accounting policy (MSA profile)",
		Long: "Read-only company accounting policy recovered from the quickbooks-ui skill's\n" +
			"companies/ profile: fee rules, paired-journal templates, chart-of-accounts and\n" +
			"classification references. This is evidence-reviewed treatment data, not\n" +
			"permission to post — mutations still pass the CLI's own readback gates.",
	}
	cmd.PersistentFlags().StringVar(&company, "company", "", "company profile name (default: sole embedded profile)")

	docs := &cobra.Command{
		Use:         "docs",
		Short:       "List embedded reference documents",
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			co, err := resolveCompany(company)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			docs, err := policy.Docs(co)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(docs)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-40s %-7s %s\n", "ID", "BYTES", "TITLE")
			for _, d := range docs {
				fmt.Fprintf(cmd.OutOrStdout(), "%-40s %-7d %s\n", d.ID, d.Bytes, d.Title)
			}
			return nil
		},
	}

	doc := &cobra.Command{
		Use:         "doc <id>",
		Short:       "Print one embedded reference document",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			co, err := resolveCompany(company)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			content, err := policy.Doc(co, args[0])
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]string{"company": co, "id": args[0], "content": content})
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), content)
			return err
		},
	}

	rules := &cobra.Command{
		Use:         "rules",
		Short:       "List machine-readable policy rules (policies.yaml)",
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			co, err := resolveCompany(company)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			rules, err := policy.Rules(co)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(rules)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-34s %-16s %s\n", "RULE_ID", "AUTHORITY", "SCOPE")
			for _, r := range rules {
				fmt.Fprintf(cmd.OutOrStdout(), "%-34s %-16s %s\n", r.RuleID, r.Authority, r.Scope)
			}
			return nil
		},
	}

	rule := &cobra.Command{
		Use:         "rule <rule-id>",
		Short:       "Show one policy rule in full",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			co, err := resolveCompany(company)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			r, err := policy.RuleByID(co, args[0])
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(r)
		},
	}

	var revenue, amount float64
	var class string
	fee := &cobra.Command{
		Use:   "management-fee",
		Short: "Compute the 20.9% landlord management fee for a revenue base",
		Long: "Deterministic check for MSA.LANDLORD_MANAGEMENT_FEE: fee = revenue x 0.209,\n" +
			"rounded to cents. --amount compares a prepared journal line against the\n" +
			"computed fee and reports match/difference without posting anything.\n" +
			"Always verify the revenue base (Client Property Revenue for the period,\n" +
			"same property class) and the prior approved journal before trusting it.",
		Example: "  qb policy management-fee --revenue 3100.00 --class \"10 Toorak Melody\"\n" +
			"  qb policy management-fee --revenue 3100.00 --amount 647.90 --json",
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			co, err := resolveCompany(company)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			calc, err := policy.ManagementFee(co, revenue, class)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			if cmd.Flags().Changed("amount") {
				diff := amount - calc.Fee
				match := math.Abs(diff) < 0.005
				if !match {
					calc.ClassWarning = strings.TrimSpace(calc.ClassWarning + " declared amount differs from computed fee")
				}
				if flags.asJSON {
					enc := json.NewEncoder(cmd.OutOrStdout())
					enc.SetIndent("", "  ")
					return enc.Encode(map[string]any{
						"calculation":     calc,
						"declared_amount": amount,
						"difference":      math.Round(diff*100) / 100,
						"match":           match,
					})
				}
				fmt.Fprintf(cmd.OutOrStdout(), "computed fee: %.2f\ndeclared:     %.2f\ndifference:   %+.2f\n", calc.Fee, amount, diff)
				if match {
					fmt.Fprintln(cmd.OutOrStdout(), "match: yes")
				} else {
					fmt.Fprintln(cmd.OutOrStdout(), "match: NO — verify revenue base, rate and rounding against the prior approved journal")
				}
				return nil
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(calc)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "fee:            %.2f (revenue %.2f x %.3f)\n", calc.Fee, calc.Revenue, calc.Rate)
			fmt.Fprintf(cmd.OutOrStdout(), "debit account:  %s\n", calc.DebitAccount)
			fmt.Fprintf(cmd.OutOrStdout(), "credit account: %s\n", calc.CreditAccount)
			if calc.Class != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "class:          %s\n", calc.Class)
			}
			if calc.ClassWarning != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "warning:        %s\n", calc.ClassWarning)
			}
			return nil
		},
	}
	fee.Flags().Float64Var(&revenue, "revenue", 0, "property Client Property Revenue for the period (required)")
	fee.Flags().Float64Var(&amount, "amount", 0, "declared journal amount to compare against the computed fee")
	fee.Flags().StringVar(&class, "class", "", "QBO class of the journal (advisory; warns on unknown landlord classes)")
	_ = fee.MarkFlagRequired("revenue")

	templates := &cobra.Command{
		Use:         "journal-templates",
		Short:       "List paired-journal account/class templates",
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			tpls := policy.JournalTemplates()
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(tpls)
			}
			for _, t := range tpls {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n  %s\n  Dr %s\n  Cr %s\n", t.Kind, t.Description, t.DebitAccount, t.CreditAccount)
			}
			return nil
		},
	}

	template := &cobra.Command{
		Use:         "journal-template <kind>",
		Short:       "Show one paired-journal template in full",
		Args:        cobra.ExactArgs(1),
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := policy.JournalTemplateFor(args[0])
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(t)
		},
	}

	var itemsJSON, checkKind string
	var checkRevenue, checkExpected, checkSupplier float64
	check := &cobra.Command{
		Use:   "check-journal [file|-]",
		Short: "Validate a journal payload against company policy (read-only)",
		Long: "Check a prepared journal against the documented MSA treatment rules before\n" +
			"posting: balance, paired accounts, same property class on both lines, the\n" +
			"20.9% fee recompute, cleaning carry-forward and recharge margin evidence.\n\n" +
			"Accepts either a v3 JournalEntry (as `accounting journal get` returns, via\n" +
			"file, - or piped stdin) or the same --items-json array that\n" +
			"`accounting journal create` consumes. Item objects may also carry\n" +
			"from-account-name/to-account-name, class-name, kind, revenue,\n" +
			"expected-amount and supplier-cost for checking.\n\n" +
			"Verdicts: pass = every rule verified; unverified = required evidence not\n" +
			"supplied (revenue, prior cleaning amount, supplier cost); fail = rule\n" +
			"violated. Exit is nonzero unless every journal passes.",
		Example: "  qb accounting journal get 123 --json | qb policy check-journal --revenue 3100\n" +
			"  qb policy check-journal --items-json '[{\"amount\":647.90,\"from-account\":\"89\",\"to-account\":\"1\",\"from-account-name\":\"Client Property Expenses:Short-Term Business Operating Expenses:Management Fee\",\"to-account-name\":\"Management Income\",\"class-name\":\"10 Toorak Melody\",\"revenue\":3100}]'",
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw []byte
			var err error
			switch {
			case itemsJSON != "":
				raw = []byte(itemsJSON)
			case len(args) == 1 && args[0] != "-":
				raw, err = os.ReadFile(args[0])
				if err != nil {
					return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
				}
			default:
				in := cmd.InOrStdin()
				if f, ok := in.(*os.File); ok {
					stat, _ := f.Stat()
					if len(args) == 0 && (stat.Mode()&os.ModeCharDevice) != 0 {
						return &ExitError{Code: ExitInputError, Err: fmt.Errorf("no journal payload: pass a file, -, piped stdin or --items-json"), Silent: flags.asJSON}
					}
				}
				raw, err = io.ReadAll(in)
				if err != nil {
					return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
				}
			}
			inputs, err := policy.ParseCheckInputs(raw)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			opts := policy.CheckOpts{Kind: checkKind}
			if cmd.Flags().Changed("revenue") {
				opts.Revenue = &checkRevenue
			}
			if cmd.Flags().Changed("expected-amount") {
				opts.ExpectedAmount = &checkExpected
			}
			if cmd.Flags().Changed("supplier-cost") {
				opts.SupplierCost = &checkSupplier
			}
			results := make([]policy.CheckResult, 0, len(inputs))
			for _, in := range inputs {
				results = append(results, policy.CheckJournal(in, opts))
			}
			allPass := true
			for _, r := range results {
				if !r.OK {
					allPass = false
				}
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				var encErr error
				if len(results) == 1 {
					encErr = enc.Encode(results[0])
				} else {
					encErr = enc.Encode(results)
				}
				if encErr != nil {
					return encErr
				}
				if !allPass {
					return &ExitError{Code: ExitInputError, Err: fmt.Errorf("policy check: not all journals pass"), Silent: true}
				}
				return nil
			}
			for i, r := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "journal %d/%d — verdict: %s (kind: %s, format: %s, balanced: %v)\n", i+1, len(results), r.Verdict, orDash(r.Kind), r.Format, r.Balanced)
				for _, f := range r.Findings {
					fmt.Fprintf(cmd.OutOrStdout(), "  [%-10s] %s: %s\n", f.Severity, f.Check, f.Detail)
				}
			}
			if !allPass {
				return &ExitError{Code: ExitInputError, Err: fmt.Errorf("policy check: not all journals pass")}
			}
			return nil
		},
	}
	check.Flags().StringVar(&itemsJSON, "items-json", "", "items array in the accounting journal create shape (plus optional check metadata)")
	check.Flags().StringVar(&checkKind, "kind", "", "declared journal kind: management-fee|cleaning|owner-distribution|cost-recharge (default: infer from accounts)")
	check.Flags().Float64Var(&checkRevenue, "revenue", 0, "Client Property Revenue base to recompute the 20.9% management fee")
	check.Flags().Float64Var(&checkExpected, "expected-amount", 0, "prior month's approved cleaning fee (MSA.CLEANING_FEE_CARRY_FORWARD)")
	check.Flags().Float64Var(&checkSupplier, "supplier-cost", 0, "underlying supplier cost to verify a recharge margin (MSA.COST_MARGIN_EVIDENCE)")

	var acctQuery string
	accounts := &cobra.Command{
		Use:         "accounts",
		Short:       "Look up chart-of-accounts entries (name substring or exact id)",
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			co, err := resolveCompany(company)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			matches, err := policy.QueryAccounts(co, acctQuery)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(matches)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-92s %-24s %s\n", "ID", "NAME", "TYPE", "SUBTYPE")
			for _, a := range matches {
				fmt.Fprintf(cmd.OutOrStdout(), "%-8s %-92s %-24s %s\n", a.ID, a.Name, a.Type, a.Subtype)
			}
			return nil
		},
	}
	accounts.Flags().StringVar(&acctQuery, "query", "", "name substring or exact account id")

	var classQuery string
	classes := &cobra.Command{
		Use:         "classes",
		Short:       "Look up QBO classes (name substring or exact id)",
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			co, err := resolveCompany(company)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			matches, err := policy.QueryClasses(co, classQuery)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(matches)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-22s %s\n", "ID", "NAME")
			for _, c := range matches {
				fmt.Fprintf(cmd.OutOrStdout(), "%-22s %s\n", c.ID, c.Name)
			}
			return nil
		},
	}
	classes.Flags().StringVar(&classQuery, "query", "", "name substring or exact class id")

	var onRevenue, onFee, onCleaning, onInternet, onDistribution float64
	var onCosts []float64
	var onClass, onJournals, onPnL string
	ownerNet := &cobra.Command{
		Use:   "owner-net",
		Short: "Compute the owner-statement equation for a period",
		Long: "owner net = Client Property Revenue − management fee − cleaning − internet\n" +
			"− direct or recharged property costs.\n\n" +
			"The fee defaults to the computed 20.9% of revenue; pass --management-fee\n" +
			"to check a declared amount against the computed value. Cleaning is\n" +
			"unresolved unless --cleaning carries the prior month's approved amount.\n" +
			"Owner distribution (--distribution) is reported separately, never a\n" +
			"deduction.\n\n" +
			"--journals derives the deduction side from posted journal entries for\n" +
			"--class: `journal get --id … --json` output (file, -, or piped stream),\n" +
			"deduplicated by Id and decomposed into pairs.\n\n" +
			"--pnl supplies the revenue side from a class-filtered ProfitAndLoss\n" +
			"report (`reports profit-loss get --klass <id> --json`, file or -):\n" +
			"Client Property Revenue is the base, Management Income the charged fee,\n" +
			"Cleaning Amenities Income the recharged cleaning.\n\n" +
			"Precedence: explicit flags > journal-derived > P&L-derived. Where two\n" +
			"evidence sources disagree the difference is reported, never silently\n" +
			"folded. One of --revenue or --pnl is required.",
		Example: "  qb policy owner-net --revenue 3100 --cleaning 210 --internet 99 --cost 300 --cost 220 --distribution 2000 --class \"14 Agatha\" --json\n" +
			"  for id in 29633 29634; do qb accounting journal get --id $id --json; done | qb policy owner-net --revenue 2410.72 --class \"14 Agatha\" --journals -\n" +
			"  qb reports profit-loss get --date-range 2026-06-01,2026-06-30 --klass 3700000000000906241 --json | qb policy owner-net --class \"14 Agatha\" --pnl -",
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			in := policy.OwnerNetInput{PropertyCosts: onCosts, Class: onClass}
			if cmd.Flags().Changed("revenue") {
				in.Revenue = onRevenue
			}
			var journalNotes []string
			if onJournals != "" {
				if onClass == "" {
					return &ExitError{Code: ExitInputError, Err: fmt.Errorf("--journals requires --class to scope entries to one property"), Silent: flags.asJSON}
				}
				var jr io.Reader
				switch {
				case onJournals == "-":
					jr = cmd.InOrStdin()
				default:
					f, err := os.Open(onJournals)
					if err != nil {
						return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
					}
					defer func() { _ = f.Close() }()
					jr = f
				}
				co, err := resolveCompany(company)
				if err != nil {
					return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
				}
				derived, notes, err := policy.DeriveOwnerNet(co, onClass, jr)
				if err != nil {
					return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
				}
				journalNotes = notes
				in.ManagementFee = derived.ManagementFee
				in.Cleaning = derived.Cleaning
				in.Distribution = derived.Distribution
				in.PropertyCosts = append(in.PropertyCosts, derived.PropertyCosts...)
			}
			var pnlNotes []string
			if onPnL != "" {
				var pr io.Reader
				switch {
				case onPnL == "-":
					pr = cmd.InOrStdin()
				default:
					f, err := os.Open(onPnL)
					if err != nil {
						return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
					}
					defer func() { _ = f.Close() }()
					pr = f
				}
				data, err := io.ReadAll(pr)
				if err != nil {
					return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
				}
				rep, err := policy.ParsePnLReport(data)
				if err != nil {
					return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
				}
				if onClass == "" {
					pnlNotes = append(pnlNotes, "no --class given — confirm the P&L was fetched with --klass for the intended property")
				}
				var mn []string
				in, mn = policy.MergePnLEvidence(in, rep)
				pnlNotes = append(pnlNotes, mn...)
			}
			if in.Revenue == 0 && !cmd.Flags().Changed("revenue") {
				return &ExitError{Code: ExitInputError, Err: fmt.Errorf("--revenue or --pnl is required (revenue is not a journal quantity)"), Silent: flags.asJSON}
			}
			if cmd.Flags().Changed("management-fee") {
				in.ManagementFee = &onFee
			}
			if cmd.Flags().Changed("cleaning") {
				in.Cleaning = &onCleaning
			}
			if cmd.Flags().Changed("internet") {
				in.Internet = &onInternet
			}
			if cmd.Flags().Changed("distribution") {
				in.Distribution = &onDistribution
			}
			res, err := policy.OwnerNet(in)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			res.JournalNotes = journalNotes
			res.PnLNotes = pnlNotes
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "revenue:      %12.2f\n", res.Revenue)
			for _, d := range res.Deductions {
				fmt.Fprintf(cmd.OutOrStdout(), "  - %-34s %12.2f (%s)\n", d.Label, d.Amount, d.Source)
				if d.Detail != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "      %s\n", d.Detail)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "owner net:    %12.2f\n", res.OwnerNet)
			if res.Distribution != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "distribution: %12.2f (separate — reconcile independently)\n", *res.Distribution)
			}
			for _, n := range res.JournalNotes {
				fmt.Fprintf(cmd.OutOrStdout(), "journal:    %s\n", n)
			}
			for _, n := range res.PnLNotes {
				fmt.Fprintf(cmd.OutOrStdout(), "pnl:        %s\n", n)
			}
			for _, u := range res.Unresolved {
				fmt.Fprintf(cmd.OutOrStdout(), "unresolved: %s\n", u)
			}
			if res.ClassWarning != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", res.ClassWarning)
			}
			return nil
		},
	}
	categorise := &cobra.Command{
		Use:   "categorise <transaction text>",
		Short: "Suggest account/class treatment from posted-pattern evidence",
		Long: "Match transaction text (party + memo + description) against the embedded\n" +
			"categorisation-signals registry — repeated posted mappings from the\n" +
			"two-year audit, with per-signal evidence counts. Weak signals (<98%) and\n" +
			"known conflicts (channel parties, spanning suppliers) never auto-suggest:\n" +
			"they surface their discriminators and alternatives instead.\n\n" +
			"Verdicts: suggested | conflict | unresolved. Exit is nonzero unless\n" +
			"verdict is suggested. Advisory only — it does not post or classify.",
		Example: "  qb policy categorise \"TANGERINE telecom\"\n" +
			"  qb policy categorise \"KEYNEST PTY LTD base charge 2403\" --json\n" +
			"  qb policy categorise \"TPG monthly\" --json",
		Args:        cobra.MinimumNArgs(1),
		Annotations: map[string]string{"qb:read-only": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			co, err := resolveCompany(company)
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			res, err := policy.Categorise(co, strings.Join(args, " "))
			if err != nil {
				return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				if err := enc.Encode(res); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "input:   %s\nverdict: %s\n", res.Input, res.Verdict)
				for _, a := range res.AccountCandidates {
					fmt.Fprintf(cmd.OutOrStdout(), "  account: %s [%s, matched %q]\n      evidence: %s\n", a.Account, a.Confidence, a.Matched, a.Evidence)
					if a.Discriminator != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "      discriminator: %s\n", a.Discriminator)
					}
					if a.Note != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "      note: %s\n", a.Note)
					}
				}
				for _, c := range res.Conflicts {
					fmt.Fprintf(cmd.OutOrStdout(), "  conflict: matched %q — %s\n      alternatives: %s\n", c.Matched, c.Reason, strings.Join(c.Alternatives, ", "))
				}
				for _, cl := range res.ClassCandidates {
					fmt.Fprintf(cmd.OutOrStdout(), "  class: %s (%s)\n", cl.Class, cl.Basis)
				}
				for _, n := range res.Notes {
					fmt.Fprintf(cmd.OutOrStdout(), "  note: %s\n", n)
				}
			}
			if res.Verdict != "suggested" {
				return &ExitError{Code: ExitInputError, Err: fmt.Errorf("categorise: %s", res.Verdict), Silent: flags.asJSON}
			}
			return nil
		},
	}

	ownerNet.Flags().Float64Var(&onRevenue, "revenue", 0, "Client Property Revenue for the period, same property class (required)")
	ownerNet.Flags().Float64Var(&onFee, "management-fee", 0, "declared management fee (default: computed at 20.9%)")
	ownerNet.Flags().Float64Var(&onCleaning, "cleaning", 0, "prior month's approved cleaning fee")
	ownerNet.Flags().Float64Var(&onInternet, "internet", 0, "internet cost for the period")
	ownerNet.Flags().Float64SliceVar(&onCosts, "cost", nil, "a direct or recharged property cost (repeatable)")
	ownerNet.Flags().Float64Var(&onDistribution, "distribution", 0, "owner distribution for the period (separate; never a deduction)")
	ownerNet.Flags().StringVar(&onClass, "class", "", "property class name or id (advisory; required with --journals)")
	ownerNet.Flags().StringVar(&onJournals, "journals", "", "derive deductions from journal entries for --class: file path or - for a piped `journal get --id` stream")
	ownerNet.Flags().StringVar(&onPnL, "pnl", "", "derive revenue + charged-side evidence from a class-filtered `reports profit-loss get --klass <id> --json` payload: file path or -")

	for _, sub := range []*cobra.Command{docs, doc, rules, rule, fee, templates, template, check, accounts, classes, categorise, ownerNet} {
		cmd.AddCommand(sub)
	}
	return cmd
}
