package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/proof"
	"github.com/spf13/cobra"
)

const readbackAnnotation = "qb:independent-readback"

// Legacy aliases absent from the catalog are explicitly classified, not
// inferred from an HTTP verb (several legitimate reads use POST).
var legacyReadAliases = map[string]bool{
	"accounting deferred-recognition get": true, "crm association list": true,
	"crm lead get": true, "crm lead list": true, "crm lead search": true, "crm opportunity list": true,
	"customobjects get": true, "customobjects list": true,
	"documents email check": true, "documents email get": true, "documents email history": true,
	"documents list": true, "documents pdf": true,
	"inventory adjust get": true, "inventory adjust search": true, "inventory count get": true, "inventory count list": true,
	"salestx invoice pdf": true, "salestx gst list": true, "salestx bas list": true, "salestx taxcode list": true,
	"salestx taxrate get": true, "salestx taxrate list": true, "salestx tpar list": true,
}

// Only reviewed adapters may submit a mutation. Unknown catalog additions
// default to blocked before RunE, not to an optimistic success path.
func hasReadbackAdapter(cmd *cobra.Command, e primitiveEntry) bool {
	if cmd.Annotations[readbackAnnotation] == "true" {
		return true
	}
	switch e.ID {
	case "QBO.ADVANCED.BATCH_RUN", "QBO.EXPENSES.EXPENSE_RECATEGORISE",
		"QBO.FEED.RULE_CREATE", "QBO.FEED.RULE_EDIT",
		"QBO.COMPANY.ATTACHABLE_CREATE", "QBO.FEED.TXN_ATTACH",
		"QBO.ACCOUNTING.CLASS_CREATE", "QBO.ACCOUNTING.CLASS_EDIT", "QBO.ACCOUNTING.CLASS_DELETE",
		"QBO.ACCOUNTING.LOCATION_CREATE", "QBO.ACCOUNTING.LOCATION_EDIT", "QBO.ACCOUNTING.LOCATION_DELETE",
		"QBO.FEED.TXN_EXCLUDE", "QBO.FEED.TXN_UNDO_EXCLUDED", "QBO.FEED.TXN_UNPOST", "QBO.FEED.TXN_MATCH",
		"QBO.FEED.TXN_CATEGORISE", "QBO.FEED.TXN_SPLIT", "QBO.FEED.TXN_BATCH_ACCEPT", "QBO.FEED.RULE_DELETE":
		return true
	}
	return false
}

func installMutationGates(root *cobra.Command, flags *rootFlags) {
	var visit func(*cobra.Command)
	visit = func(cmd *cobra.Command) {
		if cmd.RunE != nil {
			path := strings.TrimPrefix(cmd.CommandPath(), root.Name()+" ")
			e, known := catalogByCommand(path)
			if !known {
				first := strings.Fields(path)[0]
				if cmd.Annotations["qb:read-only"] == "true" || legacyReadAliases[path] {
					if cmd.Annotations == nil {
						cmd.Annotations = map[string]string{}
					}
					cmd.Annotations["qb:read-only"] = "true"
				} else if first != "qb" && first != "auth" && first != "login" && first != "doctor" && first != "actions" && first != "gql" {
					e = primitiveEntry{ID: "UNCLASSIFIED." + path, Mode: modeWired, Risk: "R4", Command: path}
					known = true
				}
			}
			if known && e.Mode == modeWired && e.Risk != "R0" {
				if cmd.Annotations == nil {
					cmd.Annotations = map[string]string{}
				}
				cmd.Annotations["qb:verification-gated"] = "true"
				run := cmd.RunE
				available := hasReadbackAdapter(cmd, e)
				if available {
					cmd.Long += "\n\nLive success requires independent readback; an unverified outcome is a nonzero exit and must not be blindly retried."
				} else {
					cmd.Long += "\n\nLive execution is blocked before submission: no reviewed independent readback adapter. --dry-run remains available."
				}
				cmd.RunE = func(c *cobra.Command, args []string) error {
					if flags.dryRun {
						return run(c, args)
					}
					if e.ID == "QBO.SALES.INVOICE_REMIND" {
						send, _ := c.Flags().GetBool("send")
						if !send {
							return run(c, args)
						}
					}
					if !available {
						err := fmt.Errorf("%w: %s; --dry-run remains available", client.ErrReadbackUnavailable, e.ID)
						if flags.asJSON {
							_ = json.NewEncoder(c.OutOrStdout()).Encode(map[string]any{"ok": false, "verified": false, "outcome": "not_submitted", "readback_required": false, "error": err.Error()})
						}
						return &ExitError{Code: ExitInputError, Err: err, Silent: flags.asJSON}
					}
					ctx := proof.WithScope(c.Context())
					c.SetContext(ctx)
					out := c.OutOrStdout()
					var captured bytes.Buffer
					c.SetOut(&captured)
					defer c.SetOut(out)
					err := run(c, args)
					if err == nil && !proof.Confirmed(ctx) {
						err = client.ErrMutationUnverified
					}
					if err != nil {
						if flags.asJSON {
							var receipt any
							if json.Valid(captured.Bytes()) {
								_ = json.Unmarshal(captured.Bytes(), &receipt)
							}
							// Once a handler ran, absence of a submission marker is
							// not proof of no write (a future adapter may omit it).
							_ = json.NewEncoder(out).Encode(map[string]any{"ok": false, "verified": false, "outcome": "unverified", "readback_required": true, "receipt": receipt, "error": err.Error()})
						}
						return err
					}
					if flags.asJSON {
						var obj map[string]any
						if json.Unmarshal(captured.Bytes(), &obj) == nil && obj != nil {
							obj["verified"] = true
							return json.NewEncoder(out).Encode(obj)
						}
					}
					_, err = out.Write(captured.Bytes())
					return err
				}
			}
		}
		for _, child := range cmd.Commands() {
			visit(child)
		}
	}
	visit(root)
}
