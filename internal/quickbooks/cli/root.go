package cli

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// CanonicalBinaryName is the command users type.
const CanonicalBinaryName = "qb"

func init() {
	// Cobra stores template functions globally. Register before concurrent
	// command construction or help rendering can access that map.
	cobra.AddTemplateFunc("coverageLine", coverageLine)
}

// Exit codes for structured error reporting. main.go maps an *ExitError to
// its Code and prints the message unless Silent.
const (
	ExitSuccess      = 0
	ExitInputError   = 1
	ExitAuthError    = 2
	ExitRelayError   = 3
	ExitUnknownError = 4
)

// ExitError wraps an error with a specific exit code. When Silent is true the
// message is suppressed (used when structured --json output already carried
// the failure details on stdout).
type ExitError struct {
	Code   int
	Err    error
	Silent bool
}

func (e *ExitError) Error() string {
	if e == nil || e.Err == nil {
		return ""
	}
	return e.Err.Error()
}

func (e *ExitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// rootFlags holds the persistent flags shared by every qb subcommand. It is
// populated by newRootCmd's persistent flag bindings and read by RunE closures.
type rootFlags struct {
	asJSON, dryRun, quiet, noInput, yes bool
	auditDir, configPath, homePath      string
	timeout                             time.Duration
	relayURL                            string
}

// Execute builds the root command and runs it against os.Args, wiring SIGINT
// and SIGTERM to the command context so long-running subcommands (login poll)
// shut down gracefully instead of taking the default kill.
func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return NewRootCommand().ExecuteContext(ctx)
}

// NewRootCommand is the public entry point used by cmd/qb/main.go and tests.
func NewRootCommand() *cobra.Command {
	return newRootCmd(&rootFlags{})
}

// newRootCmd builds the root `qb` command with persistent flags and all
// subcommands. The flags pointer is shared with every child command so a
// single binding source backs both root and subcommand RunE closures.
func newRootCmd(flags *rootFlags) *cobra.Command {
	root := &cobra.Command{
		Use:   CanonicalBinaryName,
		Short: "QuickBooks Online CLI",
		Long: "qb captures and replays a QuickBooks Online browser session.\n\n" +
			"Agent entry point: qb actions --json\n" +
			"Each row has id, mode (read|wired|blocked|excluded), command, usage, params[], notes.\n" +
			"read/wired honour --flags. blocked documents flags then exits not-wired.\n" +
			"excluded has no command (backup-restore, books-close).\n\n" +
			"Groups: feed accounting expenses sales customers inventory payroll tax company reports advanced.\n" +
			"Shape: qb <domain> <entity> <verb>. Verbs: list|get|search|create|update|delete|run|verify|export|import (copy on sales estimate/invoice). Wired: feed account list; feed txn list|get|population|verify|import|update exclude|undo-excluded|categorise|match|split|batch-accept; accounting register get.\n\n" +

			"Mutations require independent live readback; check actions.verification. Unsupported writes are blocked before submission.\n" +
			"Globals: --json --home --timeout --relay-url --dry-run --yes --no-input --quiet --audit-dir\n" +
			"Dates are dd/MM/yyyy (en-AU). Feed mutations take olbTxnIds, never :ofx display ids.\n" +
			"Select the company and account explicitly; numeric IDs are company-scoped. TC2 test account 44 is documented; 209 is inactive there. Import refuses 204 and 93.",
		SilenceUsage: true,

		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Home override: --home wins, else $QB_HOME. Export so the auth
			// package's HomeDir() (which reads $QB_HOME) sees the same root.
			if flags.homePath == "" {
				flags.homePath = os.Getenv("QB_HOME")
			}
			if flags.homePath != "" {
				_ = os.Setenv("QB_HOME", flags.homePath)
			}
			return nil
		},
	}

	// Coverage in every help screen without per-command edits: a template
	// function appends mode counts computed from catalogPrimitives — the same
	// source that feeds `qb actions` — to the root and each domain group.
	root.SetHelpTemplate(`{{with (or .Long .Short)}}{{. | trimTrailingWhitespaces}}{{coverageLine $}}
{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}`)

	pf := root.PersistentFlags()
	pf.BoolVar(&flags.asJSON, "json", false, "emit machine-readable JSON on stdout")
	pf.BoolVar(&flags.dryRun, "dry-run", false, "plan only; make no changes")
	pf.BoolVar(&flags.quiet, "quiet", false, "suppress non-essential output")
	pf.BoolVar(&flags.noInput, "no-input", false, "never prompt for input")
	pf.BoolVar(&flags.yes, "yes", false, "assume yes to prompts")
	pf.StringVar(&flags.auditDir, "audit-dir", "", "audit output directory")
	pf.StringVar(&flags.configPath, "config", "", "config file path")
	pf.StringVar(&flags.homePath, "home", "", "qb config home (defaults to $QB_HOME or ~/.config/qb)")
	pf.DurationVar(&flags.timeout, "timeout", 10*time.Minute, "overall command timeout")
	pf.StringVar(&flags.relayURL, "relay-url", "", "Chrome DevTools relay URL (defaults to $QB_RELAY_URL or http://127.0.0.1:9224)")
	// Tag cobra flag/argument errors so main.go can map them to
	// ExitInputError (1) while reserving ExitUnknownError (4) for genuine
	// unclassified failures.
	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return &ExitError{Code: ExitInputError, Err: err}
	})
	root.AddCommand(newLoginCmd(flags))
	root.AddCommand(newAuthCmd(flags))
	root.AddCommand(newDoctorCmd(flags))
	root.AddCommand(newGqlCmd(flags))
	root.AddCommand(newFeedCmd(flags))
	root.AddCommand(newAccountingCmd(flags))
	root.AddCommand(newExpensesCmd(flags))
	root.AddCommand(newSalesCmd(flags))
	root.AddCommand(newCustomersCmd(flags))
	root.AddCommand(newInventoryCmd(flags))
	root.AddCommand(newPayrollCmd(flags))
	root.AddCommand(newTaxCmd(flags))
	root.AddCommand(newSalesTaxCmd(flags))
	root.AddCommand(newCompanyCmd(flags))
	root.AddCommand(newReportsCmd(flags))
	root.AddCommand(newCrmCmd(flags))
	// Service map (2026-08-24): documentation domains for never-triggered
	// production services. Blocked stubs only.
	root.AddCommand(newAccountantServiceMapCmd(flags))
	root.AddCommand(newIntegrationsServiceMapCmd(flags))
	root.AddCommand(newAdvancedCmd(flags))
	root.AddCommand(newCustomObjectsCmd(flags))
	root.AddCommand(newCostGroupsCmd(flags))
	root.AddCommand(newDocumentsCmd(flags))
	root.AddCommand(newActionsCmd(flags))
	installMutationGates(root, flags)
	return root
}

// ShouldSuppress reports whether an error's message is already carried by
// structured output (ExitError.Silent) and must not be echoed to stderr.
func ShouldSuppress(err error) bool {
	var exitErr *ExitError
	return errors.As(err, &exitErr) && exitErr.Silent
}

// ClassifyExitCode maps an execution error to its exit code. Flag/argument
// errors (tagged by SetFlagErrorFunc as ExitError) carry their own code;
// every other untyped error is unclassified → ExitUnknownError.
func ClassifyExitCode(err error) int {
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return ExitUnknownError
}
