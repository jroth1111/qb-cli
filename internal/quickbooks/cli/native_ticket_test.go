package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/auth"
	"testing"
	"time"
)

func TestNativeTicketDefaultPrintsWithoutLaunch(t *testing.T) {
	t.Setenv("QB_HOME", t.TempDir())
	flags := &rootFlags{asJSON: true}
	cmd := newAuthTicketCmd(flags)
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetArgs([]string{"extend", "--force"})
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(out.Bytes(), &value); err != nil || value["extended"] != false {
		t.Fatal("default ticket command did not print an opt-in continuation")
	}
}

func TestNativeKeeperWakesForTicketBeforeLongHealthInterval(t *testing.T) {
	now := time.Now()
	tok := &auth.TokenSet{Source: "managed-profile", NativeTicket: &auth.NativeTicketStatus{State: "extended", NextDue: now.Add(7 * time.Minute)}}
	if got := nativeKeeperDelay(15*time.Minute, tok, now); got != 6*time.Minute {
		t.Fatalf("native deadline wake=%s", got)
	}
	if got := nativeKeeperDelay(5*time.Minute, tok, now); got != 5*time.Minute {
		t.Fatal("native scheduling changed a shorter healthy probe interval")
	}
	tok.NativeTicket.State = "extension_failed"
	if got := nativeKeeperDelay(15*time.Minute, tok, now); got != 15*time.Minute {
		t.Fatal("failed extension caused a tight retry loop")
	}
}
