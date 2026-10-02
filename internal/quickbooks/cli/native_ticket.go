package cli

import (
	"context"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/spf13/cobra"
)

func newAuthTicketCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "ticket", Short: "Inspect or extend the current managed browser ticket"}
	cmd.AddCommand(&cobra.Command{Use: "status", Short: "Show native ticket-extension timing", Args: cobra.NoArgs, Annotations: map[string]string{"mcp:read-only": "true"}, RunE: func(cmd *cobra.Command, _ []string) error {
		tok, err := auth.Load()
		if err != nil {
			return recoveryResult(cmd, flags, nil, err)
		}
		return recoveryResult(cmd, flags, map[string]any{"native_ticket": tok.NativeTicket}, nil)
	}})
	var launch, force bool
	extend := &cobra.Command{Use: "extend", Short: "Run the native sessionextender read and independent API proof", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !launch || flags.dryRun {
			return recoveryResult(cmd, flags, map[string]any{"extended": false, "requires": "--launch; optional --force bypasses the due-time check"}, nil)
		}
		tok, err := auth.Load()
		if err != nil {
			return recoveryResult(cmd, flags, nil, err)
		}
		timeout := flags.timeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
		defer cancel()
		var previousAttempt time.Time
		if tok.NativeTicket != nil {
			previousAttempt = tok.NativeTicket.LastAttempt
		}
		if err = auth.ExtendNativeTicket(ctx, tok, force); err != nil {
			return recoveryResult(cmd, flags, nil, err)
		}
		tok, err = auth.Load()
		if err != nil {
			return recoveryResult(cmd, flags, nil, err)
		}
		performed := tok.NativeTicket != nil && tok.NativeTicket.LastAttempt.After(previousAttempt)
		return recoveryResult(cmd, flags, map[string]any{"ok": true, "extended": performed && tok.NativeTicket.State == auth.NativeTicketExtended, "native_ticket": tok.NativeTicket}, nil)
	}}
	extend.Flags().BoolVar(&launch, "launch", false, "allow headless managed browser maintenance")
	extend.Flags().BoolVar(&force, "force", false, "perform a bounded extension even when not due")
	cmd.AddCommand(extend)
	return cmd
}
