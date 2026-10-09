package cli

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/spf13/cobra"
)

func TestMachineCompanySelectionReturnsDialogChoicesWithoutOpeningUI(t *testing.T) {
	for _, flags := range []*rootFlags{{asJSON: true}, {noInput: true}} {
		_, err := chooseRecoveryCompany(context.Background(), &cobra.Command{}, flags, []string{"First", "Second"})
		var selection *auth.CompanySelectionRequired
		if !errors.As(err, &selection) || len(selection.Companies) != 2 {
			t.Fatal("missing company-selection continuation")
		}
	}
}

func TestEnrollmentMachineModePreservesCompanyChoices(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	t.Setenv("PRINTING_PRESS_VERIFY", "")
	t.Setenv("PRINTING_PRESS_DOGFOOD", "")
	old := enrollRecovery
	defer func() { enrollRecovery = old }()
	enrollRecovery = func(_ context.Context, _ auth.RecoverySecrets, opts auth.RecoveryOptions) error {
		if opts.Company != "Second" || opts.ChooseCompany == nil {
			t.Fatal("company setup options not passed")
		}
		return &auth.CompanySelectionRequired{Companies: []string{"First", "Second"}}
	}
	root := NewRootCommand()
	var output strings.Builder
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetIn(strings.NewReader(`{"username":"fixture-user","password":"fixture-secret"}`))
	root.SetArgs([]string{"auth", "enroll", "--credentials-stdin", "--bootstrap", "--launch", "--enable-recovery", "--company", "Second", "--json", "--no-input"})
	if root.Execute() == nil {
		t.Fatal("missing selection failure")
	}
	var result struct {
		RequiresSelection bool     `json:"requires_company_selection"`
		Companies         []string `json:"companies"`
	}
	if json.Unmarshal([]byte(output.String()), &result) != nil || !result.RequiresSelection || len(result.Companies) != 2 || strings.Contains(output.String(), "fixture-secret") {
		t.Fatal("unsafe or missing structured choices")
	}
}
