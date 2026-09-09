package client

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestVerifyWebhookSignature(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"eventNotifications":[{"realmId":"12345"}]}`)
	mac := hmac.New(sha256.New, []byte("token123"))
	mac.Write(payload)
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if err := VerifyWebhookSignature(payload, sig, "token123"); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := VerifyWebhookSignature(payload, sig, "wrong"); err == nil {
		t.Error("wrong token must fail")
	}
	if err := VerifyWebhookSignature(payload, "!!!not-base64!!!", "token123"); err == nil {
		t.Error("garbage signature must fail")
	}
	if err := VerifyWebhookSignature(nil, sig, "token123"); err == nil {
		t.Error("empty payload must fail")
	}
}
