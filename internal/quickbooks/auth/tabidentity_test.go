package auth

import (
	"encoding/json"
	"testing"
)

func TestParseEvalIdentityNested(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"id":2,"result":{"result":{"type":"object","value":` +
		`{"company":"Test Company 2","email":"a@b.c","realm":"123"}}}}`)
	got := parseEvalIdentity(raw)
	if got.Company != "Test Company 2" || got.Email != "a@b.c" || got.Realm != "123" {
		t.Errorf("nested parse = %+v", got)
	}
}

func TestParseEvalIdentityFlat(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{"id":2,"result":{"value":` +
		`{"company":"C","email":"e","realm":"1"}}}`)
	got := parseEvalIdentity(raw)
	if got.Company != "C" || got.Realm != "1" {
		t.Errorf("flat parse = %+v", got)
	}
}

func TestParseEvalIdentityEmpty(t *testing.T) {
	t.Parallel()
	// A hydrated-but-signed-out tab returns empty strings: callers must see
	// zero identity, never a half-filled struct mistaken for a company.
	got := parseEvalIdentity(json.RawMessage(`{"id":2,"result":{"result":{"value":{}}}}`))
	if got != (Identity{}) {
		t.Errorf("empty value must parse to zero identity, got %+v", got)
	}
	if got := parseEvalIdentity(json.RawMessage(`not json`)); got != (Identity{}) {
		t.Errorf("garbage must parse to zero identity, got %+v", got)
	}
}
