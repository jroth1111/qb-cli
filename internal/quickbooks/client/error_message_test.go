package client

import (
	"strings"
	"testing"
)

func TestErrorMessageExtractsV3FaultDetail(t *testing.T) {
	body := []byte(`{"Fault":{"Error":[{"Message":"A business validation error has occurred while processing your request","Detail":"Budget feature is not enabled","code":"6000"}],"type":"ValidationFault"}}`)
	got := errorMessage(body)
	if !strings.Contains(got, "6000") || !strings.Contains(got, "Budget") {
		t.Fatalf("got %q", got)
	}
}

func TestErrorMessageUnknownJSON(t *testing.T) {
	if errorMessage([]byte(`{"nope":true}`)) != "non-200 response" {
		t.Fatal("unknown JSON must stay generic")
	}
}
