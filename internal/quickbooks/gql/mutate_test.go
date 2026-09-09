package gql

import (
	"strings"
	"testing"
)

func mustSig(t *testing.T, name string) string {
	t.Helper()
	sig, err := LookupMutation(name)
	if err != nil {
		t.Fatalf("LookupMutation(%q): %v", name, err)
	}
	return sig
}

// TestLookupMutationKnown covers exact-name lookups across the catalog's
// signature shapes: single input object, list-of-inputs, scalar-only,
// multi-variable, and the nullable-variable mutations.
func TestLookupMutationKnown(t *testing.T) {
	cases := map[string]string{
		"CommerceConsumeInventory":                 "$input: Commerce_ConsumeInventoryInput!",
		"BatchUpdateProducts":                      "$products: [Commerce_BatchUpdateProductInput!]!",
		"IMMEDIATE_RECOGNIZE_SCHEDULE":             "$sequence: Int!",
		"BulkUpdateCustomExtensionRecommendations": "$sourceEntityType: AppFoundations_CustomExtensionRecommendationSourceEntity!, $input: [AppFoundations_CustomExtensionRecommendationUpdateInput!]!",
		"ActivateProductEntities":                  "$entities: [InactivateEntity!]",
	}
	for name, want := range cases {
		if got := mustSig(t, name); got != want {
			t.Errorf("LookupMutation(%q) = %q, want %q", name, got, want)
		}
	}
}

// TestListMutationsComplete pins the catalog size and its sort order —
// pass 3 captured exactly 84 mutation signatures.
func TestListMutationsComplete(t *testing.T) {
	got := ListMutations()
	if len(got) != 84 {
		t.Fatalf("ListMutations() = %d names, want 84", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("ListMutations() not sorted: %q >= %q", got[i-1], got[i])
		}
	}
}

func TestCountMutationParity(t *testing.T) {
	if len(ListMutations()) != len(mutationInputs) {
		t.Fatal("ListMutations and mutationInputs disagree on size")
	}
}

// TestLookupMutationUnknown: unknown names error with an actionable hint.
func TestLookupMutationUnknown(t *testing.T) {
	sig, err := LookupMutation("CreateAccountTypo")
	if err == nil {
		t.Fatal("unknown name returned nil error")
	}
	if sig != "" {
		t.Errorf("error path returned non-empty signature %q", sig)
	}
	if !strings.Contains(err.Error(), `unknown GraphQL mutation "CreateAccountTypo"`) {
		t.Errorf("error lacks canonical prefix: %v", err)
	}
	if !strings.Contains(err.Error(), "CreateAccount") { // prefix hit
	}
}

func TestLookupMutationNoHit(t *testing.T) {
	_, err := LookupMutation("ZZZNoSubstringMatch")
	if err == nil || !strings.Contains(err.Error(), "see `qb gql mutate`") {
		t.Errorf("no-hit error lacks full-list pointer: %v", err)
	}
}

// TestMutationVarTypesExtraction proves per-variable type extraction from
// raw signature strings, including multi-space webpack artifacts.
func TestMutationVarTypesExtraction(t *testing.T) {
	tests := []struct {
		name string
		sig  string
		want map[string]string
	}{
		{"single", "$input: Commerce_ConsumeInventoryInput!", map[string]string{"input": "Commerce_ConsumeInventoryInput!"}},
		{"list", "$products: [Commerce_BatchUpdateProductInput!]!", map[string]string{"products": "[Commerce_BatchUpdateProductInput!]!"}},
		{
			"multi",
			"$sourceEntityType: AppFoundations_CustomExtensionRecommendationSourceEntity!     $input: [AppFoundations_CustomExtensionRecommendationUpdateInput!]!",
			map[string]string{
				"sourceEntityType": "AppFoundations_CustomExtensionRecommendationSourceEntity!",
				"input":            "[AppFoundations_CustomExtensionRecommendationUpdateInput!]!",
			},
		},
		{"numbered", "$input_0: UpdateIntegration_ReconciliationInput!", map[string]string{"input_0": "UpdateIntegration_ReconciliationInput!"}},
		{"nullable list", "$entities: [InactivateEntity!]", map[string]string{"entities": "[InactivateEntity!]"}},
	}
	for _, tc := range tests {
		got := MutationVarTypes(tc.sig)
		if len(got) != len(tc.want) {
			t.Errorf("%s: got %d vars %v, want %d", tc.name, len(got), got, len(tc.want))
			continue
		}
		for k, w := range tc.want {
			if got[k] != w {
				t.Errorf("%s: var $%s = %q, want %q", tc.name, k, got[k], w)
			}
		}
	}
}

func TestRequiredMutationVars(t *testing.T) {
	req, err := RequiredMutationVars("CommerceConsumeInventory")
	if err != nil || len(req) != 1 || req[0] != "input" {
		t.Errorf("CommerceConsumeInventory required = %v, %v; want [input], nil", req, err)
	}
	// Nullable variable => nothing required.
	req2, err := RequiredMutationVars("ActivateProductEntities")
	if err != nil || req2 != nil {
		t.Errorf("ActivateProductEntities required = %v, %v; want nil, nil", req2, err)
	}
	if _, err := RequiredMutationVars("NoSuchMutation"); err == nil {
		t.Error("unknown mutation returned nil error")
	}
}

// TestValidateMutationVars enforces the pre-network contract: missing
// non-null variables are refused locally; extra/undeclared variables too.
func TestValidateMutationVars(t *testing.T) {
	if err := ValidateMutationVars("CommerceConsumeInventory", nil); err == nil ||
		!strings.Contains(err.Error(), "requires -$input Commerce_ConsumeInventoryInput!") {
		t.Errorf("missing required: %v", err)
	}
	if err := ValidateMutationVars("CommerceConsumeInventory",
		map[string]any{"input": map[string]any{"id": "x"}}); err != nil {
		t.Errorf("complete vars rejected: %v", err)
	}
	// Nullable-only mutation accepts zero variables...
	if err := ValidateMutationVars("ActivateProductEntities", nil); err != nil {
		t.Errorf("nullable-only mutation with no vars: %v", err)
	}
	// ...and null still satisfies presence for a nullable variable.
	if err := ValidateMutationVars("ActivateProductEntities", map[string]any{"entities": nil}); err != nil {
		t.Errorf("null for nullable var: %v", err)
	}
	if err := ValidateMutationVars("CommerceConsumeInventory", map[string]any{"bogus": 1}); err == nil {
		t.Error("undeclared variable accepted")
	}
	if err := ValidateMutationVars("NoSuchMutation", nil); err == nil {
		t.Error("unknown mutation accepted")
	}
}

// TestValidateMutationVarsMultiVar covers the two-required case end to end.
func TestValidateMutationVarsMultiVar(t *testing.T) {
	err := ValidateMutationVars("BulkUpdateCustomExtensionRecommendations",
		map[string]any{"sourceEntityType": "X"})
	if err == nil || !strings.Contains(err.Error(), "$input") {
		t.Errorf("second missing required not reported: %v", err)
	}
	err = ValidateMutationVars("BulkUpdateCustomExtensionRecommendations",
		map[string]any{"sourceEntityType": "X", "input": []any{}})
	if err != nil {
		t.Errorf("both supplied but rejected: %v", err)
	}
}
