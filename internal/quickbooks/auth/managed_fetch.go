package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

type ManagedResponse struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
	Failed bool   `json:"failed"`
}

// FetchManaged executes in the same owned profile that minted this session.
// It never signs in or retries a request; callers classify safe read recovery.
func FetchManaged(ctx context.Context, expected *TokenSet, endpoint, method string, headers map[string]string, body []byte) (*ManagedResponse, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || (u.Hostname() != "qbo.intuit.com" && !strings.HasSuffix(u.Hostname(), ".api.intuit.com")) {
		return nil, errors.New("managed fetch requires an HTTPS QBO API endpoint")
	}
	current, err := Load()
	if err != nil || !SameSession(expected, current) || current.Source != "managed-profile" {
		return nil, ErrSessionChanged
	}
	for key, value := range headers {
		if (strings.EqualFold(key, "intuit-company-id") || strings.EqualFold(key, "intuit-realm-id")) && value != "" && value != current.RealmID {
			return nil, ErrSessionChanged
		}
	}
	if u.Hostname() == "qbo.intuit.com" {
		for _, prefix := range []string{"/api/v3/company/", "/api/neo/v1/company/", "/ats/v1/company/"} {
			if rest, ok := strings.CutPrefix(u.Path, prefix); ok {
				realm, _, _ := strings.Cut(rest, "/")
				if realm != current.RealmID {
					return nil, ErrSessionChanged
				}
			}
		}
	}
	var result ManagedResponse
	err = withManagedBrowser(ctx, func(page context.Context) error {
		if err := authenticateManaged(page, managedLoginDriver{ctx: page}, nil); err != nil {
			return err
		}
		if err := waitManagedIdentity(page, current); err != nil {
			return err
		}
		latest, err := Load()
		if err != nil || !SameSession(current, latest) || latest.CredentialGeneration != current.CredentialGeneration {
			return ErrSessionChanged
		}
		args, _ := json.Marshal(map[string]any{"endpoint": endpoint, "method": method, "headers": headers, "body": string(body)})
		script := `(async()=>{try {if(location.origin!=='https://qbo.intuit.com')return {failed:true};
 const a=` + string(args) + `;const o={method:a.method,headers:a.headers,credentials:'include',redirect:'error',signal:AbortSignal.timeout(30000)};
 if(a.method!=='GET' && a.method!=='HEAD' && a.body)o.body=a.body;
 const r=await fetch(a.endpoint,o);return {status:r.status,body:await r.text()};
 }catch(_){return {failed:true};}})()`
		if chromedp.Run(page, chromedp.Evaluate(script, &result, func(p *runtime.EvaluateParams) *runtime.EvaluateParams { return p.WithAwaitPromise(true) })) != nil || result.Failed {
			return errors.New("managed browser request outcome unknown; inspect state before retrying a write")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &result, nil
}
