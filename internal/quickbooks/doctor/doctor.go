// Package doctor runs preflight session health checks so auth problems
// surface before long operations rather than mid-walk.
package doctor

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
)

// Check is one named diagnostic result.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
}

// DoctorResult aggregates all checks.
type DoctorResult struct {
	OverallOK bool    `json:"overallOk"`
	Checks    []Check `json:"checks"`
}

var probeSession = client.ProbeSession

// RunDoctor executes every check and returns the aggregate result. All
// checks run; the first failure does not short-circuit the rest.
func RunDoctor(ctx context.Context) DoctorResult {
	res := DoctorResult{OverallOK: true}
	add := func(name string, ok bool, detail string) {
		res.Checks = append(res.Checks, Check{Name: name, OK: ok, Detail: detail})
		if !ok {
			res.OverallOK = false
		}
	}

	tok, err := auth.Load()
	if err != nil {
		add("credentials-file", false, "missing or unreadable; run qb login")
		res.OverallOK = false
		return res
	}
	add("credentials-file", true, "loaded")

	hasAuth := tok.HasATSAuthorization()
	add("has-ats-auth", hasAuth,
		pick(hasAuth, "ATS Intuit_APIKey present",
			"cookie-only session cannot call banking APIs"))

	csrf := csrfFromCookies(tok)
	add("csrf-present", csrf != "",
		pick(csrf != "", "qbo.csrftoken found",
			"qbo.csrftoken cookie missing; re-run qb auth remint"))

	hdrCompany := tok.RequestHeaders["intuit-company-id"]
	realmOK := hdrCompany == "" || hdrCompany == tok.RealmID
	add("realm-consistency", realmOK,
		pick(realmOK, "realm "+tok.RealmID,
			"mixed-realm session: stored "+tok.RealmID+" != headers "+hdrCompany+"; re-run qb auth remint"))

	ok, status, detail := probeSession(ctx)
	add("live-probe", ok, detail+" (status "+itoa(status)+")")

	// Zero AccessExpiry means "unknown" (remint does not persist it):
	// reported as a failure so operators know the check is indeterminate,
	// not silently passed.
	if tok.AccessExpiry.IsZero() {
		add("expiry", false, "access expiry unknown (relay remint captures no cookie lifetimes); treat as unverified")
	} else if tok.AccessExpiry.Before(time.Now().UTC()) {
		add("expiry", false, "access token expired; run qb auth remint")
	} else {
		add("expiry", true, "access token not expired")
	}

	return res
}

// CredDir exposes the credential directory for doctor output.
func CredDir() string {
	if v := os.Getenv("QB_HOME"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".config", "qb")
	}
	return filepath.Join(home, ".config", "qb")
}

func pick(cond bool, a, b string) string {
	if cond {
		return a
	}
	return b
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// csrfFromCookies extracts the qbo.csrftoken cookie value.
func csrfFromCookies(tok *auth.TokenSet) string {
	for _, c := range tok.Cookies {
		if c.Name == "qbo.csrftoken" {
			return c.Value
		}
	}
	return ""
}
