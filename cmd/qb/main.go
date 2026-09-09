package main

import (
	"fmt"
	"os"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		if !cli.ShouldSuppress(err) {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		os.Exit(cli.ClassifyExitCode(err))
	}
}
