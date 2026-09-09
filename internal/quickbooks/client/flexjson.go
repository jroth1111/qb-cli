package client

import (
	"bytes"
	"encoding/json"
	"strings"
)

// flexibleString unmarshals a JSON string or number into a string.
// QBO neo payloads use numeric ids (txnId, rule id).
type flexibleString string

func (s *flexibleString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = ""
		return nil
	}
	if b[0] == '"' {
		var v string
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		*s = flexibleString(v)
		return nil
	}
	*s = flexibleString(strings.Trim(string(b), `"`))
	return nil
}

func (s flexibleString) String() string { return string(s) }
