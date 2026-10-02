package auth

import (
	"context"
	"errors"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

type RecoveryVerification struct {
	OK                           bool     `json:"ok"`
	BaselineVerified             bool     `json:"baseline_verified"`
	WarmSessionPreserved         bool     `json:"warm_session_preserved"`
	LocalAuthLoss                bool     `json:"local_auth_loss"`
	LogicalSessionPreserved      bool     `json:"logical_session_preserved"`
	IdentityPreserved            bool     `json:"identity_preserved"`
	CredentialGenerationAdvanced bool     `json:"credential_generation_advanced"`
	IndependentReadback          bool     `json:"independent_readback"`
	CredentialPurposes           []string `json:"credential_purposes,omitempty"`
	NaturalExpiryTested          bool     `json:"natural_expiry_tested"`
}

func VerifyLoginRecovery(ctx context.Context, localLoss bool) (*RecoveryVerification, error) {
	if IsHarness() {
		return nil, ErrRecoveryDisabled
	}
	r, err := loadRecovery()
	if err != nil {
		return nil, err
	}
	before, err := Load()
	if err != nil || !r.Enabled || !recoveryBound(r, before) {
		return nil, ErrSessionChanged
	}
	if localLoss && !r.TestProfile {
		return nil, errors.New("local auth loss is allowed only on a bootstrapped --test-profile")
	}
	ctx = withManagedHeaded(ctx, r.Headed)
	unlock, err := LockProfile(ctx, HomeDir(), "renewal")
	if err != nil {
		return nil, err
	}
	cap, err := recoveryCapture(ctx, before, nil)
	if err != nil {
		unlock()
		return nil, err
	}
	if err = SaveRenewedCapture(before, cap, "managed-profile"); err != nil {
		unlock()
		return nil, err
	}
	baseline, err := Load()
	if err != nil {
		unlock()
		return nil, err
	}
	result := &RecoveryVerification{BaselineVerified: true, WarmSessionPreserved: SameSession(before, baseline) && baseline.CredentialGeneration == before.CredentialGeneration}
	if !localLoss {
		unlock()
		result.OK = result.BaselineVerified && result.WarmSessionPreserved
		return result, nil
	}
	err = withManagedBrowser(ctx, func(page context.Context) error {
		if err := waitManagedIdentity(page, baseline); err != nil {
			return err
		}
		return chromedp.Run(page, chromedp.Navigate("about:blank"), chromedp.ActionFunc(func(ctx context.Context) error {
			if err := network.ClearBrowserCookies().Do(ctx); err != nil {
				return err
			}
			for _, origin := range []string{"https://accounts.intuit.com", "https://qbo.intuit.com"} {
				if err := storage.ClearDataForOrigin(origin, "all").Do(ctx); err != nil {
					return err
				}
			}
			return nil
		}))
	})
	if err != nil {
		unlock()
		return nil, err
	}
	lock, err := credentialLock(HomeDir())
	if err != nil {
		unlock()
		return nil, err
	}
	current, err := Load()
	record, rerr := loadRecovery()
	if err != nil || rerr != nil || record.ID != r.ID || !record.Enabled || !SameSession(baseline, current) {
		lock()
		unlock()
		return nil, ErrSessionChanged
	}
	current.Cookies = nil
	err = saveAt(HomeDir(), current)
	lock()
	unlock()
	if err != nil {
		return nil, err
	}
	result.LocalAuthLoss = true
	if err = RemintATS(ctx); err != nil {
		return result, err
	}
	after, err := Load()
	if err != nil {
		return result, err
	}
	result.LogicalSessionPreserved = SameSession(baseline, after)
	result.IdentityPreserved = baseline.RealmID == after.RealmID && baseline.Email == after.Email
	result.CredentialGenerationAdvanced = after.CredentialGeneration == baseline.CredentialGeneration+1
	r, err = loadRecovery()
	if err != nil {
		return result, err
	}
	if r.LastEvidence != nil {
		result.CredentialPurposes = r.LastEvidence.CredentialPurposes
	}
	readback, err := recoveryCapture(ctx, after, nil)
	if err != nil {
		return result, err
	}
	result.IndependentReadback = readback.Evidence != nil && readback.Evidence.APIProof && readback.Evidence.IdentityVerified && len(readback.Evidence.CredentialPurposes) == 0
	result.OK = result.BaselineVerified && result.WarmSessionPreserved && result.LogicalSessionPreserved && result.IdentityPreserved && result.CredentialGenerationAdvanced && result.IndependentReadback
	if !result.OK {
		return result, errors.New("recovery verification did not satisfy every acceptance check")
	}
	return result, nil
}
