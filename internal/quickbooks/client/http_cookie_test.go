package client

import (
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
)

func TestCookieHeaderDropsVSTicketsUnderCap(t *testing.T) {
	var jar []auth.Cookie
	for range 142 {
		jar = append(jar, auth.Cookie{Name: "VSremint", Value: "x"})
	}
	// 142 identical VS names still count as 142 header pairs if not dropped.
	// Use distinct names so a naive send would exceed the cap.
	jar = jar[:0]
	for i := range 142 {
		jar = append(jar, auth.Cookie{Name: "VS" + strings.Repeat("a", 1) + itoa(i), Value: "x"})
	}
	for _, n := range []string{"qbo.ticket", "qbn.ticket", "ius_session", "other.sid"} {
		jar = append(jar, auth.Cookie{Name: n, Value: "x"})
	}
	// pad other non-VS to 87 keepers like the live prune (16+14+1+56)
	for i := range 83 {
		jar = append(jar, auth.Cookie{Name: "keep" + itoa(i), Value: "x"})
	}
	h := cookieHeader(jar)
	if h == "" {
		t.Fatal("empty cookie header")
	}
	parts := strings.Split(h, "; ")
	if len(parts) > qboQueryCookieCap {
		t.Fatalf("cookie pairs %d > cap %d", len(parts), qboQueryCookieCap)
	}
	for _, p := range parts {
		name, _, _ := strings.Cut(p, "=")
		if strings.HasPrefix(name, "VS") {
			t.Fatalf("VS ticket leaked into Cookie header: %s", name)
		}
	}
	if !strings.Contains(h, "qbo.ticket=") || !strings.Contains(h, "ius_session=") {
		t.Fatal("kept session cookies missing")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
