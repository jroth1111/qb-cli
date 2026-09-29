package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
)

func TestMemorizedRunBoundsRemoteIndent(t *testing.T) {
	for _, tc := range []struct {
		value  string
		spaces int
	}{
		{"-1", 0}, {"bogus", 0}, {"2", 4}, {"999999999", 128},
	} {
		t.Run(tc.value, func(t *testing.T) {
			res := &client.ReportResult{Report: "R", Rows: json.RawMessage(fmt.Sprintf(`[{"cells":[{"value":"row","attributes":[{"name":"indent","value":%q}]}]}]`, tc.value))}
			var out bytes.Buffer
			printMemorizedRun(&out, &rootFlags{}, res)
			if got, want := out.String(), "R\n"+strings.Repeat(" ", tc.spaces)+"row\n"; got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}
