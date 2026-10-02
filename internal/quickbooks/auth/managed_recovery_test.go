package auth

import (
	"context"
	"errors"
	"os"
	"testing"
)

type fixtureLoginDriver struct {
	steps               []loginSnapshot
	index               int
	submitted           []loginStep
	authenticatorChosen bool
}

func TestManagedFetchRefusesMismatchedRealmBeforeBrowserLaunch(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	t.Setenv("PRINTING_PRESS_VERIFY", "1")
	tok := &TokenSet{Source: "managed-profile", RealmID: "1", Email: "fixture@example.invalid", Authorization: "Intuit_APIKey fixture"}
	if err := Save(tok); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"https://qbo.intuit.com/api/v3/company/2/query", "https://qbo.intuit.com/api/neo/v1/company/2/olb/ng", "https://qbo.intuit.com/ats/v1/company/2/banking"} {
		if _, err := FetchManaged(context.Background(), tok, endpoint, "GET", nil, nil); !errors.Is(err, ErrSessionChanged) {
			t.Fatal("mismatched URL realm was not refused before browser launch")
		}
	}
	if _, err := FetchManaged(context.Background(), tok, "https://fitransactions.api.intuit.com/graphql", "POST", map[string]string{"Intuit-Company-Id": "2"}, nil); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("mismatched header realm was accepted")
	}
	if _, err := os.Stat(ManagedProfileDir()); !os.IsNotExist(err) {
		t.Fatal("realm rejection launched or created a browser profile")
	}
}

func (d *fixtureLoginDriver) snapshot(context.Context) (loginSnapshot, error) {
	if d.index >= len(d.steps) {
		return loginSnapshot{}, ErrRecoveryAttention
	}
	return d.steps[d.index], nil
}
func (d *fixtureLoginDriver) submit(_ context.Context, step loginStep, value string) error {
	if value == "" {
		return ErrRecoveryAttention
	}
	d.submitted = append(d.submitted, step)
	d.index++
	return nil
}
func (d *fixtureLoginDriver) chooseAuthenticator(context.Context) error {
	d.authenticatorChosen = true
	d.index++
	return nil
}

func (d *fixtureLoginDriver) choosePassword(context.Context) error { d.index++; return nil }
func (d *fixtureLoginDriver) chooseCompany(context.Context) error  { d.index++; return nil }

func fixtureSteps(steps ...loginStep) []loginSnapshot {
	var result []loginSnapshot
	for _, step := range steps {
		u := "https://accounts.intuit.com/app/sign-in"
		if step == stepApp {
			u = BankingCaptureURL
		}
		result = append(result, loginSnapshot{URL: u, Step: step})
	}
	return result
}

func TestManagedLoginStateMachine(t *testing.T) {
	for _, scenario := range []struct {
		name  string
		steps []loginStep
		want  int
		ok    bool
	}{
		{"warm", []loginStep{stepApp}, 0, true},
		{"password", []loginStep{stepUsername, stepPassword, stepApp}, 2, true},
		{"choose-password", []loginStep{stepUsername, stepPasswordChoice, stepPassword, stepApp}, 2, true},
		{"choose-company", []loginStep{stepUsername, stepPasswordChoice, stepPassword, stepCompanyChoice, stepApp}, 2, true},
		{"totp", []loginStep{stepUsername, stepPassword, stepTOTP, stepApp}, 3, true},
		{"choose-authenticator", []loginStep{stepUsername, stepPassword, stepAuthenticator, stepTOTP, stepApp}, 3, true},
		{"challenge", []loginStep{stepUsername, stepPassword, stepChallenge}, 2, false},
		{"repeated-username", []loginStep{stepUsername, stepPassword, stepUsername}, 2, false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			d := &fixtureLoginDriver{steps: fixtureSteps(scenario.steps...)}
			err := authenticateManaged(context.Background(), d, &RecoverySecrets{Username: "fixture", Password: "fixture", TOTP: "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"})
			if (err == nil) != scenario.ok || len(d.submitted) != scenario.want {
				t.Fatal("unexpected login sequence or repeated submission")
			}
		})
	}
}

func TestManagedLoginRefusesForeignOriginsAndNonAuthenticatorChallenges(t *testing.T) {
	for _, raw := range []string{"https://evil.invalid/accounts.intuit.com", "https://accounts.intuit.com.evil.invalid/", "http://accounts.intuit.com/", "https://accounts.intuit.com:444/", "https://user:pass@accounts.intuit.com/"} {
		d := &fixtureLoginDriver{steps: []loginSnapshot{{URL: raw, Step: stepPassword}}}
		if err := authenticateManaged(context.Background(), d, &RecoverySecrets{Password: "fixture"}); err == nil || len(d.submitted) != 0 {
			t.Fatal("secret submitted to an untrusted origin")
		}
	}
	d := &fixtureLoginDriver{steps: fixtureSteps(stepTOTP)}
	if err := authenticateManaged(context.Background(), d, &RecoverySecrets{Username: "fixture", Password: "fixture"}); !errors.Is(err, ErrRecoveryAttention) || len(d.submitted) != 0 {
		t.Fatal("TOTP attempted without an enrolled seed")
	}
	d = &fixtureLoginDriver{steps: fixtureSteps(stepUsername)}
	if err := authenticateManaged(context.Background(), d, nil); !errors.Is(err, ErrRemintNeedsLogin) || len(d.submitted) != 0 {
		t.Fatal("warm capture submitted credentials")
	}
}

func TestManagedDriverChecksRevocationBeforeTouchingPage(t *testing.T) {
	driver := managedLoginDriver{authorize: func() error { return ErrSessionChanged }}
	if _, err := driver.snapshot(context.Background()); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("snapshot ignored revoked enrollment")
	}
	if err := driver.submit(context.Background(), stepPassword, "fixture"); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("password submission ignored revocation")
	}
	if err := driver.chooseAuthenticator(context.Background()); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("challenge interaction ignored revocation")
	}
	if err := driver.choosePassword(context.Background()); !errors.Is(err, ErrSessionChanged) {
		t.Fatal("password method selection ignored revocation")
	}
}

func TestManagedWarmCompanySelectionDoesNotUseCredentials(t *testing.T) {
	driver := &fixtureLoginDriver{steps: fixtureSteps(stepCompanyChoice, stepApp)}
	if err := authenticateManaged(context.Background(), driver, nil); err != nil || len(driver.submitted) != 0 {
		t.Fatal("warm company selection did not preserve the credential-free flow")
	}
	driver = &fixtureLoginDriver{steps: fixtureSteps(stepCompanyChoice, stepPassword)}
	if err := authenticateManaged(context.Background(), driver, nil); !errors.Is(err, ErrRemintNeedsLogin) || len(driver.submitted) != 0 {
		t.Fatal("warm selection crossed into password submission")
	}
}

func TestLoginDiagnosticsExcludeArbitraryProviderValues(t *testing.T) {
	for _, key := range []string{"message", "code", "errorCode", "password", "totp", "successCode", "optionType"} {
		if recordedLoginEnum(key, "FIXTURE_SECRET_VALUE") {
			t.Fatalf("arbitrary %s value was permitted in diagnostics", key)
		}
	}
	for _, example := range []struct{ key, value string }{{"__typename", "Identity_BrowserSignInWithPasswordNothingElseRequired"}, {"optionType", "TIME_BASED_ONE_TIME_PASSWORD"}, {"successCode", "SOMETHING_ELSE_REQUIRED"}} {
		if !recordedLoginEnum(example.key, example.value) {
			t.Fatal("known authentication schema metadata was excluded")
		}
	}
}
