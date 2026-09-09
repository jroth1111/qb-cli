package cli

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/doctor"
	"github.com/spf13/cobra"
)

// newDoctorCmd implements `qb doctor`: preflight session health checks
// (credentials, ATS auth, CSRF, realm consistency, live probe, expiry) plus an
// informational coverage check counted from the action catalog.
// Non-zero exit when any session check fails so scripts can gate on it.
func newDoctorCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check session health before running feed operations",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res := appendCoverageCheck(doctor.RunDoctor(ctx))
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			for _, c := range res.Checks {
				mark := "ok  "
				if !c.OK {
					mark = "FAIL"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "[%s] %-18s %s\n", mark, c.Name, c.Detail)
			}
			if !res.OverallOK {
				fmt.Fprintln(cmd.ErrOrStderr(), "doctor: one or more checks failed")
				return &ExitError{Code: ExitInputError, Err: fmt.Errorf("doctor found %d failing checks", countFailed(res)), Silent: flags.asJSON}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nall checks passed\n")
			return nil
		},
	}
	return cmd
}

func countFailed(res doctor.DoctorResult) int {
	n := 0
	for _, c := range res.Checks {
		if !c.OK {
			n++
		}
	}
	return n
}
