package cli

import (
	"fmt"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

func mutationNotWired(flags *rootFlags, verb string) error {
	if err := client.RequireSession(); err != nil {
		return feedErr(flags, err)
	}
	return &ExitError{
		Code:   ExitInputError,
		Err:    fmt.Errorf("%s: %w", verb, client.ErrMutationNotWired),
		Silent: flags.asJSON,
	}
}

func newStubCmd(flags *rootFlags, group, use, short string) *cobra.Command {
	command := group + " " + use
	e, ok := catalogByCommand(command)
	if ok {
		if v3 := maybeV3Cmd(flags, e, command); v3 != nil {
			return v3
		}
	}
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			if flags.dryRun {
				var entry primitiveEntry
				if ok {
					entry = e
				}
				return writePlan(cmd, flags, uncapturedPlan(cmd, command, entry))
			}
			return mutationNotWired(flags, command)
		},
	}
	if ok {
		cmd.Long = longHelp(e)
		cmd.Example = usageFor(e)
		attachParamFlags(cmd, e.ID)
	}
	return cmd
}
