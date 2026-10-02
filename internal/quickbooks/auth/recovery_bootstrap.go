package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

type managedHeadedKey struct{}

func withManagedHeaded(ctx context.Context, headed bool) context.Context {
	return context.WithValue(ctx, managedHeadedKey{}, headed)
}

type LoginEvidence struct {
	mu                       sync.Mutex
	CredentialPurposes       []string        `json:"credential_purposes"`
	CookieRestore            bool            `json:"cookie_restore"`
	IdentityVerified         bool            `json:"identity_verified"`
	APIProof                 bool            `json:"api_proof"`
	LastSnapshot             *loginSnapshot  `json:"last_snapshot,omitempty"`
	Responses                []loginResponse `json:"responses,omitempty"`
	Exceptions               int             `json:"exceptions,omitempty"`
	PasskeyRequestsCancelled int             `json:"passkey_requests_cancelled,omitempty"`
}

type loginResponse struct {
	Path      string   `json:"path"`
	Status    int      `json:"status"`
	Schema    []string `json:"schema,omitempty"`
	Enums     []string `json:"enums,omitempty"`
	RequestID string   `json:"request_id,omitempty"`
	Operation string   `json:"operation,omitempty"`
}

type LoginFailure struct {
	Cause    error
	Evidence *LoginEvidence
}

func (e *LoginFailure) Error() string { return fmt.Sprintf("%v", e.Cause) }
func (e *LoginFailure) Unwrap() error { return e.Cause }

type loginEvidenceKey struct{}

func loginEvidence(ctx context.Context) *LoginEvidence {
	e, _ := ctx.Value(loginEvidenceKey{}).(*LoginEvidence)
	return e
}

var bootstrapCapture = captureManagedBootstrap

func bootstrapLoginRecovery(ctx context.Context, secrets RecoverySecrets, opts RecoveryOptions) error {
	unlock, err := LockProfile(ctx, HomeDir(), "renewal")
	if err != nil {
		return err
	}
	if _, err = Load(); !errors.Is(err, ErrNoCredentials) {
		unlock()
		return errors.New("--bootstrap requires a fresh QB_HOME without saved credentials")
	}
	if _, err = loadRecovery(); !errors.Is(err, ErrRecoveryDisabled) {
		unlock()
		return errors.New("--bootstrap requires a fresh QB_HOME without enrollment")
	}
	ctx = withManagedHeaded(ctx, opts.Headed)
	cap, err := bootstrapCapture(ctx, secrets)
	if err != nil {
		unlock()
		return err
	}
	if cap == nil || cap.Identity.Realm == "" || cap.Identity.Email == "" || !cap.HasKey() {
		unlock()
		return ErrSessionIdentityUnverified
	}
	if strings.Contains(secrets.Username, "@") && !strings.EqualFold(strings.TrimSpace(secrets.Username), cap.Identity.Email) {
		unlock()
		return ErrSessionChanged
	}
	tok := &TokenSet{Version: CurrentVersion, Source: "managed-profile", CapturedAt: time.Now().UTC(), LoginURL: DefaultLoginURL, FinalURL: BankingCaptureURL}
	if err = tok.ApplyATSCapture(cap); err == nil {
		tok.ClassifyCookies()
		err = Save(tok)
	}
	unlock()
	if err != nil {
		return err
	}
	testProfile := opts.TestProfile
	opts.Bootstrap = false
	opts.TestProfile = false
	if err = EnrollLoginRecovery(ctx, secrets, opts); err != nil {
		return err
	}
	lock, err := credentialLock(HomeDir())
	if err != nil {
		return err
	}
	defer lock()
	r, err := loadRecovery()
	if err != nil || !recoveryBound(r, tok) {
		return ErrSessionChanged
	}
	r.TestProfile = testProfile
	if cap.Evidence != nil {
		r.LastEvidence = cap.Evidence
	}
	return writeRecovery(r)
}

func captureManagedBootstrap(ctx context.Context, secrets RecoverySecrets) (*ATSCapture, error) {
	trace := &LoginEvidence{}
	ctx = context.WithValue(ctx, loginEvidenceKey{}, trace)
	var cap *ATSCapture
	err := withManagedBrowser(ctx, func(page context.Context) error {
		if err := authenticateManaged(page, managedLoginDriver{ctx: page}, &secrets); err != nil {
			return err
		}
		headers, secondary, hosts, err := interceptManagedATS(page, BankingCaptureURL)
		if err != nil {
			return err
		}
		var identity Identity
		if chromedp.Run(page, chromedp.Evaluate(IdentityJS, &identity)) != nil || identity.Realm == "" || identity.Email == "" {
			return ErrSessionIdentityUnverified
		}
		if strings.Contains(secrets.Username, "@") && !strings.EqualFold(strings.TrimSpace(secrets.Username), identity.Email) {
			return ErrSessionChanged
		}
		cookies, err := snapshotManagedCookies(page)
		if err != nil {
			return ErrSessionIdentityUnverified
		}
		cap = &ATSCapture{Headers: headers, SecondaryHeaders: secondary, HostHeaders: hosts, Cookies: cookies, Identity: identity, Evidence: trace}
		expected := &TokenSet{RealmID: identity.Realm, Email: identity.Email}
		if err = verifyRecoveryIdentity(expected, cap); err != nil {
			return err
		}
		trace.IdentityVerified = true
		if err = proveManagedCompany(page, expected, cap); err != nil {
			return err
		}
		trace.APIProof = true
		return nil
	})
	if err != nil {
		return cap, &LoginFailure{Cause: err, Evidence: trace}
	}
	return cap, nil
}
