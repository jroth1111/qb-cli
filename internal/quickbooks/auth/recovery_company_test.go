package auth

import (
	"context"
	"errors"
	"testing"
)

func TestBootstrapCompanySelection(t *testing.T) {
	for _, fixture := range []struct {
		name      string
		companies []string
		expected  string
		want      string
		failure   bool
	}{
		{"single-default", []string{"Only Company"}, "", "Only Company", false},
		{"explicit-default", []string{"First", "Second"}, "Second", "Second", false},
		{"missing-company", []string{"First"}, "Other", "", true},
		{"duplicate-name", []string{"Same", "Same"}, "Same", "", true},
		{"empty-list", nil, "", "", true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			got, err := selectBootstrapCompany(context.Background(), fixture.expected, fixture.companies)
			if (err != nil) != fixture.failure || got != fixture.want {
				t.Fatal("unexpected company selection", got, err)
			}
		})
	}
}

func TestMultipleCompaniesNeverChooseFirstSilently(t *testing.T) {
	companies := []string{"First", "Second"}
	_, err := selectBootstrapCompany(context.Background(), "", companies)
	var selection *CompanySelectionRequired
	if !errors.As(err, &selection) || len(selection.Companies) != 2 {
		t.Fatal("missing structured selection requirement")
	}
	called := false
	ctx := context.WithValue(context.Background(), bootstrapCompanyKey{}, RecoveryOptions{ChooseCompany: func(_ context.Context, choices []string) (string, error) {
		called = true
		if len(choices) != 2 {
			t.Fatal("dialog did not receive all choices")
		}
		return choices[1], nil
	}})
	got, err := selectBootstrapCompany(ctx, "", companies)
	if err != nil || got != "Second" || !called {
		t.Fatal("user choice was not retained")
	}
	called = false
	got, err = selectBootstrapCompany(ctx, "First", companies)
	if err != nil || got != "First" || called {
		t.Fatal("bound recovery prompted or switched company")
	}
}

func TestCompanyDialogCancellationAndForeignChoicesFailClosed(t *testing.T) {
	for _, chosen := range []string{"", "Foreign"} {
		ctx := context.WithValue(context.Background(), bootstrapCompanyKey{}, RecoveryOptions{ChooseCompany: func(context.Context, []string) (string, error) { return chosen, nil }})
		if _, err := selectBootstrapCompany(ctx, "", []string{"First", "Second"}); err == nil {
			t.Fatal("accepted invalid company choice")
		}
	}
	cancelled := errors.New("selection cancelled")
	ctx := context.WithValue(context.Background(), bootstrapCompanyKey{}, RecoveryOptions{ChooseCompany: func(context.Context, []string) (string, error) { return "", cancelled }})
	if _, err := selectBootstrapCompany(ctx, "", []string{"First", "Second"}); !errors.Is(err, cancelled) {
		t.Fatal("cancellation was ignored")
	}
}
