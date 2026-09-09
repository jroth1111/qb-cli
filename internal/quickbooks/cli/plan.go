package cli

import (
	"encoding/json"
	"fmt"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// planEnvelope is the secret-free --dry-run output. Dial is always false.
type planEnvelope struct {
	DryRun  bool              `json:"dry_run"`
	Dial    bool              `json:"dial"`
	Command string            `json:"command"`
	ID      string            `json:"id,omitempty"`
	Mode    primitiveMode     `json:"mode,omitempty"`
	Method  string            `json:"method,omitempty"`
	URL     string            `json:"url,omitempty"`
	Body    json.RawMessage   `json:"body,omitempty"`
	Flags   map[string]string `json:"flags,omitempty"`
	Note    string            `json:"note"`
}

func writePlan(cmd *cobra.Command, flags *rootFlags, env planEnvelope) error {
	env.DryRun = true
	env.Dial = false
	if env.Note == "" {
		env.Note = "not sent"
	}
	if flags.asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "dry-run: not sent\n")
	if env.Command != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "command: %s\n", env.Command)
	}
	if env.Method != "" || env.URL != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", env.Method, env.URL)
	}
	if env.Note != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "note: %s\n", env.Note)
	}
	if len(env.Body) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", string(env.Body))
	}
	return nil
}

func localFlagMap(cmd *cobra.Command) map[string]string {
	out := map[string]string{}
	if cmd == nil {
		return out
	}
	cmd.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f == nil || f.Name == "" {
			return
		}
		out[f.Name] = f.Value.String()
	})

	return out
}

func planFromClient(cmd *cobra.Command, command, id string, mode primitiveMode, p *client.RequestPlan) planEnvelope {
	env := planEnvelope{
		Command: command,
		ID:      id,
		Mode:    mode,
		Flags:   localFlagMap(cmd),
	}
	if p != nil {
		env.Method = p.Method
		env.URL = p.URL
		env.Body = p.Body
		env.Note = p.Note
	}
	return env
}

func uncapturedPlan(cmd *cobra.Command, command string, e primitiveEntry) planEnvelope {
	return planEnvelope{
		Command: command,
		ID:      e.ID,
		Mode:    e.Mode,
		Flags:   localFlagMap(cmd),
		Note:    "no captured API; not sent",
	}
}

func dryRunGET(flags *rootFlags, cmd *cobra.Command, command, id, url, note string) (bool, error) {
	if !flags.dryRun {
		return false, nil
	}
	return true, writePlan(cmd, flags, planEnvelope{
		Command: command,
		ID:      id,
		Mode:    modeRead,
		Method:  "GET",
		URL:     url,
		Flags:   localFlagMap(cmd),
		Note:    note,
	})
}
