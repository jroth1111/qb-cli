package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

type ManagedResponse struct {
	Status     int     `json:"status"`
	Body       string  `json:"body"`
	Failed     bool    `json:"failed"`
	BodyBase64 *string `json:"bodyBase64,omitempty"`
}

var ErrBrowserRequestUncertain = errors.New("browser request interrupted; verify state before retrying a write")

// FetchManaged executes in the same owned profile that minted this session.
// It never signs in or retries a request; callers classify safe read recovery.
func FetchManaged(ctx context.Context, expected *TokenSet, endpoint, method string, headers map[string]string, body []byte) (*ManagedResponse, error) {
	if err := ValidateBrowserDestination(endpoint, expected, headers); err != nil {
		return nil, err
	}
	current, err := Load()
	if err != nil || !SameBrowserSession(expected, current) || current.Source != "managed-profile" {
		return nil, ErrSessionChanged
	}
	var result ManagedResponse
	err = withManagedBrowser(ctx, func(page context.Context) error {
		if err := authenticateManaged(page, managedLoginDriver{ctx: page, company: current.CompanyName}, nil); err != nil {
			return err
		}
		if err := waitManagedIdentity(page, current); err != nil {
			return err
		}
		latest, err := Load()
		if err != nil || !SameBrowserSession(current, latest) || latest.CredentialGeneration != current.CredentialGeneration {
			return ErrSessionChanged
		}
		if headers == nil {
			headers = map[string]string{}
		}
		args, _ := json.Marshal(map[string]any{"endpoint": endpoint, "method": method, "headers": headers, "bodyBase64": base64.StdEncoding.EncodeToString(body), "timeoutMs": 30000})
		script := `(async()=>{try {if(location.origin!=='https://qbo.intuit.com')return {failed:true};
 return await ` + BrowserByteFetchJS + `(` + string(args) + `);
 }catch(_){return {failed:true};}})()`
		if chromedp.Run(page, chromedp.Evaluate(script, &result, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })) != nil || result.Failed {
			return ErrBrowserRequestUncertain
		}
		if err := waitManagedIdentity(page, current); err != nil {
			return err
		}
		latest, err = Load()
		if err != nil || !SameBrowserSession(current, latest) || latest.CredentialGeneration != current.CredentialGeneration {
			return ErrSessionChanged
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result.Status < 100 || result.Status > 599 {
		return nil, ErrBrowserRequestUncertain
	}
	if result.BodyBase64 != nil {
		decoded, err := base64.StdEncoding.DecodeString(*result.BodyBase64)
		if err != nil {
			return nil, ErrBrowserRequestUncertain
		}
		result.Body = string(decoded)
	}
	return &result, nil
}
