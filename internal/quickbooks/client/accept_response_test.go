package client

import "testing"

func TestAcceptRejectsHTTP200WithoutTargetReceipt(t *testing.T) {
	req := []byte(`{"nextTxnInfo":{"accountId":"204"},"txnList":{"olbTxns":[{"olbTxnId":"26747"}]}}`)
	for _, raw := range []string{
		`{"ok":true}`,
		`{"errorDetails":{"26747:ofx":{"code":-1}}}`,
		`{"acceptedTxns":[]}`,
		`{"acceptedTxns":[{"olbTxnId":"wrong","qboAccount":{"accountId":"204"},"addedQboTxns":[{"qboTxnId":"36313"}]}]}`,
		`{"acceptedTxns":[{"olbTxnId":"26747","qboAccount":{"accountId":"209"},"addedQboTxns":[{"qboTxnId":"36313"}]}]}`,
		`{"acceptedTxns":[{"olbTxnId":"26747","qboAccount":{"accountId":"204"},"addedQboTxns":[]}]}`,
	} {
		if validateAcceptResponse([]byte(raw), req) == nil {
			t.Fatalf("accepted false success %s", raw)
		}
	}
	good := []byte(`{"acceptedTxns":[{"olbTxnId":"26747","qboAccount":{"accountId":"204"},"addedQboTxns":[{"qboTxnId":"36313"}]}]}`)
	if err := validateAcceptResponse(good, req); err != nil {
		t.Fatal(err)
	}
}
