package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
)

func stubCrmWrites(t *testing.T, failure error) {
	t.Helper()
	old := crmWriteReplay
	t.Cleanup(func() { crmWriteReplay = old })
	crmWriteReplay.leadCreate = func(context.Context, client.CrmLeadInput) (*client.CrmLead, error) {
		if failure != nil {
			return nil, failure
		}
		return &client.CrmLead{ID: "L1", DisplayName: "Lead", Email: "lead@example.invalid"}, nil
	}
	crmWriteReplay.leadUpdate = func(context.Context, string, client.CrmLeadInput) (*client.CrmLead, error) {
		if failure != nil {
			return nil, failure
		}
		return &client.CrmLead{ID: "L1", DisplayName: "Updated", Phone: "555"}, nil
	}
	crmWriteReplay.leadDelete = func(context.Context, string) (int, error) {
		if failure != nil {
			return 500, failure
		}
		return 204, nil
	}
	crmWriteReplay.leadConvert = func(context.Context, string) (*client.CrmConvertResult, error) {
		if failure != nil {
			return nil, failure
		}
		return &client.CrmConvertResult{Status: 200, State: "CONVERTED", CustomerID: "C1"}, nil
	}
	crmWriteReplay.opportunityCreate = func(context.Context, client.CrmOpportunityInput) (*client.CrmOpportunity, error) {
		if failure != nil {
			return nil, failure
		}
		return &client.CrmOpportunity{ID: "O1", Name: "Opportunity", Amount: 12.5}, nil
	}
}

func crmWriteCases() []struct {
	args  []string
	key   string
	value any
} {
	return []struct {
		args  []string
		key   string
		value any
	}{
		{[]string{"lead", "create", "--name", "Lead"}, "email", "lead@example.invalid"},
		{[]string{"lead", "update", "--id", "L1", "--name", "Updated"}, "phone", "555"},
		{[]string{"lead", "delete", "--id", "L1"}, "ok", true},
		{[]string{"lead", "convert", "--id", "L1"}, "customerId", "C1"},
		{[]string{"opportunity", "create", "--name", "Opportunity"}, "amount", 12.5},
	}
}

func TestCrmResponseWriteCommandsJSON(t *testing.T) {
	stubCrmWrites(t, nil)
	for _, tc := range crmWriteCases() {
		t.Run(strings.Join(tc.args[:2], "-"), func(t *testing.T) {
			// Test the response formatter in isolation. The public root gate is
			// separately required to block these unverified mutation adapters.
			cmd := newCrmCmd(&rootFlags{asJSON: true})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(tc.args)
			err := cmd.Execute()
			stdout := out.String()
			if err != nil {
				t.Fatalf("command: %v; %s", err, stdout)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("non-JSON response %q: %v", stdout, err)
			}
			if got[tc.key] != tc.value {
				t.Fatalf("got %v, want %s=%v", got, tc.key, tc.value)
			}
		})
	}
}

func TestCrmPublicWritesBlockedWithoutReadback(t *testing.T) {
	stubCrmWrites(t, nil)
	for _, tc := range crmWriteCases() {
		args := append([]string{"crm"}, tc.args...)
		_, _, err := runQB(t, args...)
		if !errors.Is(err, client.ErrReadbackUnavailable) {
			t.Fatalf("unverified CRM command escaped gate: %v", err)
		}
	}
}

func TestCrmResponseWriteCommandsPropagateFailures(t *testing.T) {
	want := &client.ReplayError{Status: 500, Message: "fake CRM failure"}
	stubCrmWrites(t, want)
	for _, tc := range crmWriteCases() {
		for _, asJSON := range []bool{false, true} {
			name := strings.Join(tc.args[:2], "-")
			if asJSON {
				name += "-json"
			}
			t.Run(name, func(t *testing.T) {
				flags := &rootFlags{asJSON: asJSON}
				cmd := newCrmCmd(flags)
				var out bytes.Buffer
				cmd.SetOut(&out)
				cmd.SetErr(io.Discard)
				cmd.SilenceErrors = true
				cmd.SilenceUsage = true
				cmd.SetArgs(tc.args)
				err := cmd.Execute()
				var exit *ExitError
				if !errors.As(err, &exit) || exit.Code == 0 || !errors.Is(err, want) {
					t.Fatalf("error not propagated: %v", err)
				}
				if out.Len() != 0 {
					t.Fatalf("failure printed success output: %q", out.String())
				}
			})
		}
	}
}

type crmFailWriter struct{ err error }

func (w crmFailWriter) Write([]byte) (int, error) { return 0, w.err }

func TestCrmResponseWriteCommandsPropagateOutputErrors(t *testing.T) {
	stubCrmWrites(t, nil)
	want := errors.New("output closed")
	for _, tc := range crmWriteCases() {
		t.Run(strings.Join(tc.args[:2], "-"), func(t *testing.T) {
			cmd := newCrmCmd(&rootFlags{asJSON: true})
			cmd.SetOut(crmFailWriter{want})
			cmd.SetErr(io.Discard)
			cmd.SilenceErrors = true
			cmd.SilenceUsage = true
			cmd.SetArgs(tc.args)
			if err := cmd.Execute(); !errors.Is(err, want) {
				t.Fatalf("got %v, want output failure", err)
			}
		})
	}
}

func TestCrmResponseConversionDoesNotInventState(t *testing.T) {
	stubCrmWrites(t, nil)
	crmWriteReplay.leadConvert = func(context.Context, string) (*client.CrmConvertResult, error) {
		return &client.CrmConvertResult{Status: 200, CustomerID: "C1"}, nil
	}
	cmd := newCrmCmd(&rootFlags{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"lead", "convert", "--id", "L1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToUpper(out.String()), "CONVERTED") || !strings.Contains(out.String(), "C1") {
		t.Fatalf("invented state or lost acknowledgement: %q", out.String())
	}
}
