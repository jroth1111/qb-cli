package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func chooseRecoveryCompany(ctx context.Context, cmd *cobra.Command, flags *rootFlags, companies []string) (string, error) {
	if flags.noInput || flags.asJSON {
		return "", &auth.CompanySelectionRequired{Companies: companies}
	}
	if runtime.GOOS == "darwin" {
		script := `on run companyNames
set picked to choose from list companyNames with title "QuickBooks setup" with prompt "Choose the default company for this account. Automatic recovery will stay pinned to it." without multiple selections allowed
if picked is false then error number -128
return item 1 of picked
end run`
		args := append([]string{"-e", script, "--"}, companies...)
		out, err := exec.CommandContext(ctx, "/usr/bin/osascript", args...).Output()
		if err != nil {
			return "", errors.New("company selection cancelled or unavailable; use --company")
		}
		return strings.TrimSuffix(string(out), "\n"), nil
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return "", &auth.CompanySelectionRequired{Companies: companies}
	}
	for index, company := range companies {
		fmt.Fprintf(cmd.ErrOrStderr(), "%d. %s\n", index+1, company)
	}
	fmt.Fprint(cmd.ErrOrStderr(), "Choose the default company number: ")
	type choice struct {
		index int
		err   error
	}
	done := make(chan choice, 1)
	go func() {
		var index int
		_, err := fmt.Fscan(cmd.InOrStdin(), &index)
		done <- choice{index, err}
	}()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case result := <-done:
		if result.err != nil || result.index < 1 || result.index > len(companies) {
			return "", errors.New("company selection cancelled or invalid")
		}
		return companies[result.index-1], nil
	}
}
