package auth

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type totpSecret struct {
	Seed      string `json:"seed"`
	Algorithm string `json:"algorithm"`
	Digits    int    `json:"digits"`
	Period    int    `json:"period"`
}

var errInvalidTOTP = errors.New("invalid authenticator seed or otpauth TOTP URI")

func parseTOTP(input string) (*totpSecret, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil, nil
	}
	t := &totpSecret{Seed: input, Algorithm: "SHA1", Digits: 6, Period: 30}
	if strings.HasPrefix(input, "otpauth:") {
		u, err := url.Parse(input)
		if err != nil || u.Scheme != "otpauth" || u.Host != "totp" || u.User != nil {
			return nil, errInvalidTOTP
		}
		q := u.Query()
		t.Seed = q.Get("secret")
		if q.Get("algorithm") != "" {
			t.Algorithm = strings.ToUpper(q.Get("algorithm"))
		}
		for key, dst := range map[string]*int{"digits": &t.Digits, "period": &t.Period} {
			if q.Get(key) != "" {
				n, err := strconv.Atoi(q.Get(key))
				if err != nil {
					return nil, errInvalidTOTP
				}
				*dst = n
			}
		}
	}
	t.Seed = strings.ToUpper(strings.TrimRight(strings.Join(strings.Fields(t.Seed), ""), "="))
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(t.Seed)
	if err != nil || len(key) < 16 || len(key) > 128 || (t.Digits != 6 && t.Digits != 8) || t.Period < 15 || t.Period > 120 {
		return nil, errInvalidTOTP
	}
	if t.Algorithm != "SHA1" && t.Algorithm != "SHA256" && t.Algorithm != "SHA512" {
		return nil, errInvalidTOTP
	}
	return t, nil
}

func (t *totpSecret) code(now time.Time) (string, error) {
	if t == nil || now.Unix() < 0 {
		return "", errInvalidTOTP
	}
	validated, err := parseTOTP("otpauth://totp/local?secret=" + url.QueryEscape(t.Seed) + "&algorithm=" + t.Algorithm + "&digits=" + strconv.Itoa(t.Digits) + "&period=" + strconv.Itoa(t.Period))
	if err != nil || validated == nil {
		return "", errInvalidTOTP
	}
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(validated.Seed)
	makeHash := sha1.New
	switch t.Algorithm {
	case "SHA256":
		makeHash = sha256.New
	case "SHA512":
		makeHash = sha512.New
	}
	h := hmac.New(makeHash, key)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(now.Unix()/int64(t.Period)))
	_, _ = h.Write(counter[:])
	digest := h.Sum(nil)
	offset := digest[len(digest)-1] & 15
	n := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
	mod := uint32(1000000)
	if t.Digits == 8 {
		mod = 100000000
	}
	return fmt.Sprintf("%0*d", t.Digits, n%mod), nil
}
