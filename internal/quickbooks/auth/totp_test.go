package auth

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

func TestTOTPRFC6238Vectors(t *testing.T) {
	for _, v := range []struct{ algorithm, seed, want string }{
		{"SHA1", "12345678901234567890", "94287082"},
		{"SHA256", "12345678901234567890123456789012", "46119246"},
		{"SHA512", "1234567890123456789012345678901234567890123456789012345678901234", "90693936"},
	} {
		t.Run(v.algorithm, func(t *testing.T) {
			seed := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(v.seed))
			secret, err := parseTOTP("otpauth://totp/fixture?secret=" + seed + "&algorithm=" + v.algorithm + "&digits=8&period=30")
			if err != nil {
				t.Fatal(err)
			}
			got, err := secret.code(time.Unix(59, 0))
			if err != nil || got != v.want {
				t.Fatal("RFC vector mismatch")
			}
		})
	}
}

func TestTOTPRejectsInvalidConfiguration(t *testing.T) {
	for _, input := range []string{"123456", "invalid!", "otpauth://hotp/test?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP", "otpauth://totp/test?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&digits=7", "otpauth://totp/test?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&period=0", "otpauth://totp/test?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&algorithm=MD5"} {
		if _, err := parseTOTP(input); err == nil {
			t.Fatal("invalid seed/configuration accepted")
		}
	}
	secret, err := parseTOTP(strings.ToLower("JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := secret.code(time.Unix(29, 0))
	b, _ := secret.code(time.Unix(30, 0))
	if a == b {
		t.Fatal("code did not rotate at the time-step boundary")
	}
	if _, err = secret.code(time.Unix(-1, 0)); err == nil {
		t.Fatal("negative clock accepted")
	}
}
