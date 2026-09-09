package auth

import (
	"strings"
	"time"
)

// Identity is the signed-in user/company mined from a hydrated SPA DOM at
// login time. Values are informational (display only), never credentials.
type Identity struct {
	Company string `json:"company,omitempty"`
	Email   string `json:"email,omitempty"`
	Realm   string `json:"realm,omitempty"`
}

// IdentityJS is evaluated in a hydrated, authenticated QBO tab (ego capture
// script and relay Runtime.evaluate share it verbatim). It returns
// {company, email, realm} mined from the SPA bootstrap, or empty strings
// when the markers are absent. Shapes verified against the live TC2 homepage
// 2026-09-08: "companyName":"Test Company 2", "email":"...", "realmId":"...".
// Server-side HTML fetches must NOT use these patterns: the unhydrated shell
// carries decoys ("companyName":"qbo") and no email at all.
const IdentityJS = `(() => {
  try {
    const html = document.documentElement.innerHTML;
    const pick = (re) => {
      const m = html.match(re);
      return m ? m[1].slice(0, 160) : "";
    };
    return {
      company: pick(/"companyName":"([^"\\]{1,120})"/),
      email: pick(/"email":"([^"\\]{1,160})"/),
      realm: pick(/"realmId":"([0-9]{1,32})"/),
    };
  } catch (_) {
    return { company: "", email: "", realm: "" };
  }
})()`

// fetchTimeout bounds `status --live` (PingV3) so a hung endpoint never
// stalls the CLI past its own deadline.
const fetchTimeout = 20 * time.Second

// sessionCookieHeader joins the jar into one Cookie header value. Empty
// names are skipped; expiry is irrelevant — the server decides.
func sessionCookieHeader(cookies []Cookie) string {
	var b strings.Builder
	for _, c := range cookies {
		if c.Name == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString(c.Name)
		b.WriteString("=")
		b.WriteString(c.Value)
	}
	return b.String()
}
