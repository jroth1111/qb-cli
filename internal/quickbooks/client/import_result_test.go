package client

import "testing"

func TestCSVImportRequiresDomainSuccess(t *testing.T) {
	for _, tc := range []struct {
		body string
		bad  bool
	}{
		{`{"duplicates":0,"noOfSuccess":1,"otherErrors":{"rows":[]},"noOfOverwriteSuccess":0}`, false},
		{`{"duplicates":1,"noOfSuccess":0,"otherErrors":{"rows":[]},"noOfOverwriteSuccess":0}`, true},
		{`{"noOfSuccess":1,"noOfOverwriteSuccess":1}`, true},
		{`{"noOfSuccess":1,"otherErrors":{"rows":[{}]}}`, true},
		{`{"ok":true}`, true},
	} {
		if err := validateCSVImportResponse([]byte(tc.body), 1); (err != nil) != tc.bad {
			t.Fatalf("body=%s err=%v", tc.body, err)
		}
	}
}
