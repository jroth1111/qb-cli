package client

import "testing"

func TestIntuitDestinationBoundary(t *testing.T) {
	for _, u := range []string{"https://attacker.invalid/x", "https://qbo.intuit.com.attacker.invalid", "http://qbo.intuit.com", "https://qbo.intuit.com:8443/x", "https://user@qbo.intuit.com/x", ":bad"} {
		if validateIntuitDestination(u) == nil {
			t.Errorf("accepted unsafe destination %q", u)
		}
	}
	for _, u := range []string{"https://qbo.intuit.com/api/v4/graphql", "https://qbonline-aws.api.intuit.com/v4/graphql", "https://c7.qbo.intuit.com/x"} {
		if err := validateIntuitDestination(u); err != nil {
			t.Errorf("valid destination: %v", err)
		}
	}
}
