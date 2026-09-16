package cli

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql"
)

func TestGqlResponseJSONDoesNotHideFailure(t *testing.T) {
	for _, status := range []int{200, 403} {
		var out bytes.Buffer
		response := &gql.Response{Status: status, Body: json.RawMessage(`{"errors":[{"message":"denied"}]}`), Errors: []gql.GraphQLError{{Message: "denied"}}}
		if err := writeGqlResponse(&out, true, "https://example.test/graphql", response); err == nil {
			t.Fatal("GraphQL error returned success")
		}
		var decoded gql.Response
		if err := json.Unmarshal(out.Bytes(), &decoded); err != nil || decoded.Status != status || len(decoded.Errors) != 1 {
			t.Fatalf("error response lost: %s (%v)", out.String(), err)
		}
	}
}

func TestGqlResponseRejectsNonGraphQLPayload(t *testing.T) {
	for _, body := range []string{"<html>login</html>", "{}", "[]", "null"} {
		var out bytes.Buffer
		if err := writeGqlResponse(&out, true, "https://example.test/graphql", &gql.Response{Status: 200, Body: json.RawMessage(body)}); err == nil {
			t.Fatalf("accepted %s", body)
		}
	}
	var out bytes.Buffer
	if err := writeGqlResponse(&out, true, "https://example.test/graphql", &gql.Response{Status: 200, Body: json.RawMessage(`{"data":{"count":1}}`)}); err != nil {
		t.Fatal(err)
	}
}
