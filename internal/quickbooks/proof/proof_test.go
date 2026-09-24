package proof

import (
	"context"
	"testing"
)

func TestAnotherWriteInvalidatesEarlierProof(t *testing.T) {
	ctx := WithScope(context.Background())
	Confirm(ctx)
	if !Confirmed(ctx) {
		t.Fatal("proof missing")
	}
	Submitted(ctx)
	if Confirmed(ctx) || !WasSubmitted(ctx) {
		t.Fatal("later write retained earlier proof")
	}
	Confirm(ctx)
	if !Confirmed(ctx) {
		t.Fatal("new proof missing")
	}
}
