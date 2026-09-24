package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// Custom-field create/update ride the same CES mutation family as delete on
// customextensions.api.intuit.com — createCustomFieldDefinition /
// updateCustomFieldDefinition with a payload carrying the full schema +
// associatedEntityTypes. The catalog's recorded smallbusiness
// *Common_CustomFieldDefinition ops are stale: they 400/401 live on TC2
// (2026-09-17) while the CES surface round-trips.
const createCustomFieldCESDoc = `mutation createCustomFieldCES($input_0: CustomFieldDefinitionMutationInput!) {
  createCustomFieldDefinition(input: $input_0) {
    id
    name
    deleted
    schema { title type }
  }
}`

// cesEntityAliases maps the CLI's --entities vocabulary onto CES
// associatedEntityTypes entries. Only pairs observed in live CES traffic are
// listed; raw "type:subtype" pairs are also accepted.
var cesEntityAliases = map[string][2]string{
	"customer":    {"/network/Contact", "CUSTOMER"},
	"contact":     {"/network/Contact", "CUSTOMER"},
	"sale":        {"/transactions/Transaction", "SALE"},
	"sales":       {"/transactions/Transaction", "SALE"},
	"transaction": {"/transactions/Transaction", "SALE"},
}

func cesAssociations(spec string) []map[string]any {
	var pairs [][2]string
	for e := range strings.SplitSeq(spec, ",") {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if pair, ok := cesEntityAliases[strings.ToLower(e)]; ok {
			pairs = append(pairs, pair)
			continue
		}
		if typ, sub, ok := strings.Cut(e, ":"); ok && strings.HasPrefix(typ, "/") {
			pairs = append(pairs, [2]string{typ, strings.ToUpper(sub)})
		}
	}
	out := make([]map[string]any, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, map[string]any{
			"type":              p[0],
			"deleted":           false,
			"allowedOperations": []any{},
			"entityConditions": []any{map[string]any{
				"subtype":           p[1],
				"deleted":           false,
				"allowedOperations": []any{},
			}},
		})
	}
	return out
}

// defaultCESAssociations matches what the customfields UI produced for a
// customer-facing text field on TC2.
func defaultCESAssociations() []map[string]any {
	return cesAssociations("customer,sale")
}

func cesSchemaType(t string) string {
	if strings.EqualFold(strings.TrimSpace(t), "text") {
		return "string"
	}
	if s := strings.ToLower(strings.TrimSpace(t)); s != "" {
		return s
	}
	return "string"
}

// ReplayCustomFieldMutate executes createCustomFieldDefinition (op "create")
// or updateCustomFieldDefinition (op "update") on the CES host. --name maps
// to schema.title (the server assigns the ucf_/udcf_ system name itself);
// --type maps to schema.type; --entities scopes associatedEntityTypes.
// Update fetches the live node and merges edits so untouched facets survive.
func ReplayCustomFieldMutate(ctx context.Context, op string, flags map[string]string) (*MutateResult, error) {
	name := strings.TrimSpace(flags["name"])
	var payload map[string]any
	var doc, gqlField string
	switch op {
	case "create":
		if name == "" {
			return nil, fmt.Errorf("custom-field create requires --name")
		}
		assoc := cesAssociations(flags["entities"])
		if len(assoc) == 0 {
			assoc = defaultCESAssociations()
		}
		payload = map[string]any{
			"name":                  name,
			"deleted":               false,
			"schema":                map[string]any{"type": cesSchemaType(flags["type"]), "title": name, "allowedOperations": []any{}, "allowedValues": []any{}, "metadataProperties": []any{}},
			"associatedEntityTypes": assoc,
		}
		doc, gqlField = createCustomFieldCESDoc, "createCustomFieldDefinition"
	case "update":
		id := strings.TrimSpace(flags["id"])
		if id == "" {
			return nil, fmt.Errorf("custom-field update requires --id")
		}
		node, err := fetchCustomFieldNode(ctx, id)
		if err != nil {
			return nil, err
		}
		schema := stripTypename(node["schema"]).(map[string]any)
		if name != "" {
			schema["title"] = name
		}
		if t := strings.TrimSpace(flags["type"]); t != "" {
			schema["type"] = cesSchemaType(t)
		}
		assoc := stripTypename(node["associatedEntityTypes"])
		if built := cesAssociations(flags["entities"]); len(built) > 0 {
			assoc = built
		}
		payload = map[string]any{
			"id":                    node["id"],
			"name":                  node["name"],
			"deleted":               node["deleted"],
			"schema":                schema,
			"associatedEntityTypes": assoc,
		}
		doc, gqlField = updateCustomFieldCESDoc, "updateCustomFieldDefinition"
	default:
		return nil, fmt.Errorf("custom-field: unknown op %q", op)
	}
	body, err := json.Marshal(map[string]any{
		"operationName": strings.TrimSuffix(gqlField, "Definition") + "CES",
		"variables": map[string]any{
			"input_0": map[string]any{
				"payload":          payload,
				"clientMutationId": "qb-cli",
			},
		},
		"query": doc,
	})
	if err != nil {
		return nil, fmt.Errorf("custom-field %s: %w", op, err)
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, customFieldGraphQLURL, customFieldHost, body)
	if err != nil {
		return nil, fmt.Errorf("custom-field %s: %w", op, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading custom-field %s: %w", op, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data map[string]*struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("custom-field %s: malformed response: %w", op, err)
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		return nil, fmt.Errorf("custom-field %s: %s", op, wrap.Errors[0].Message)
	}
	receipt, ok := wrap.Data[gqlField]
	if !ok || receipt == nil || strings.TrimSpace(receipt.ID) == "" {
		return nil, fmt.Errorf("custom-field %s outcome unknown: missing %s receipt; inspect before retrying", op, gqlField)
	}
	if op == "update" && receipt.ID != strings.TrimSpace(fmt.Sprint(payload["id"])) {
		return nil, fmt.Errorf("custom-field update outcome unknown: receipt id %q does not match requested id %q; inspect before retrying", receipt.ID, payload["id"])
	}
	item := QueryItem{Type: "CustomField", Name: name}
	item.ID, item.Name = receipt.ID, receipt.Name
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     op,
		Entity: "CustomField",
		Item:   item,
		Note:   "customextensions " + gqlField,
	}, nil
}

// PlannedCustomFieldMutateURL reports the GraphQL host for dry-run plans.
func PlannedCustomFieldMutateURL() string { return customFieldGraphQLURL }

// Custom-field delete is QBO's "Make inactive": the customfields UI posts
// updateCustomFieldDefinition on customextensions with payload.deleted=true
// plus the field's full schema/associatedEntityTypes payload — captured live
// on TC2 2026-09-17 (row dropdown → Make inactive → Yes).
const updateCustomFieldCESDoc = `fragment schemaFields on CustomFieldDefinition_Schema {
  type
  title
  format
  allowedOperations
  allowedValues {
    id
    deleted
    order
    value
    __typename
  }
  metadataProperties
  __typename
}

fragment associatedEntityTypesFields on CustomFieldDefinition_AssociatedEntityType {
  type
  deleted
  allowedOperations
  entityConditions {
    subtype
    deleted
    allowedOperations
    __typename
  }
  __typename
}

mutation updateCustomFieldCES($input_0: CustomFieldDefinitionMutationInput!) {
  updateCustomFieldDefinition(input: $input_0) {
    id
    schema {
      ...schemaFields
      __typename
    }
    name
    deleted
    associatedEntityTypes {
      ...associatedEntityTypesFields
      __typename
    }
    colorCode
    __typename
  }
}`

// ReplayCustomFieldDelete soft-deletes a custom-field definition (deleted=true).
func ReplayCustomFieldDelete(ctx context.Context, id string) (*MutateResult, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("custom-field delete requires --id <definition id>")
	}
	node, err := fetchCustomFieldNode(ctx, id)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"id":                    node["id"],
		"name":                  node["name"],
		"deleted":               true,
		"schema":                stripTypename(node["schema"]),
		"associatedEntityTypes": stripTypename(node["associatedEntityTypes"]),
	}
	body, err := json.Marshal(map[string]any{
		"operationName": "updateCustomFieldCES",
		"variables": map[string]any{
			"input_0": map[string]any{
				"payload":          payload,
				"clientMutationId": "qb-cli",
			},
		},
		"query": updateCustomFieldCESDoc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, customFieldGraphQLURL, customFieldHost, body)
	if err != nil {
		return nil, fmt.Errorf("custom-field delete: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading custom-field delete: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data struct {
			Upd *struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				Deleted bool   `json:"deleted"`
			} `json:"updateCustomFieldDefinition"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("custom-field delete: malformed response: %w", err)
	}
	if len(wrap.Errors) > 0 && wrap.Errors[0].Message != "" {
		return nil, fmt.Errorf("custom-field delete: %s", wrap.Errors[0].Message)
	}
	if wrap.Data.Upd == nil || strings.TrimSpace(wrap.Data.Upd.ID) == "" || !wrap.Data.Upd.Deleted {
		return nil, fmt.Errorf("custom-field delete outcome unknown: missing inactive updateCustomFieldDefinition receipt; inspect before retrying")
	}
	if wrap.Data.Upd.ID != id {
		return nil, fmt.Errorf("custom-field delete outcome unknown: receipt id %q does not match requested id %q; inspect before retrying", wrap.Data.Upd.ID, id)
	}
	item := QueryItem{Type: "CustomField", ID: wrap.Data.Upd.ID, Name: wrap.Data.Upd.Name}
	return &MutateResult{
		Status: resp.StatusCode,
		Op:     "delete",
		Entity: "CustomField",
		Item:   item,
		Note:   "customextensions updateCustomFieldDefinition deleted=true",
	}, nil
}

// stripTypename drops __typename keys from read-side nodes — the mutation
// input types reject them.
func stripTypename(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, x := range t {
			if k == "__typename" {
				continue
			}
			out[k] = stripTypename(x)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, x := range t {
			out[i] = stripTypename(x)
		}
		return out
	default:
		return v
	}
}

// fetchCustomFieldNode returns the raw CES node for a definition id (delete
// needs the full schema + associatedEntityTypes payload the UI echoes back).
func fetchCustomFieldNode(ctx context.Context, id string) (map[string]any, error) {
	payload, err := json.Marshal(map[string]any{
		"operationName": "CustomFieldsQueryCES",
		"variables":     map[string]any{},
		"query":         customFieldsFullDoc,
	})
	if err != nil {
		return nil, err
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	resp, err := ac.doURIHost(ctx, http.MethodPost, customFieldGraphQLURL, customFieldHost, payload)
	if err != nil {
		return nil, fmt.Errorf("custom-field fetch: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading custom-field fetch: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	var wrap struct {
		Data struct {
			Defs struct {
				Edges []struct {
					Node map[string]any `json:"node"`
				} `json:"edges"`
			} `json:"customFieldDefinitions"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		return nil, fmt.Errorf("custom-field fetch: malformed response")
	}
	for _, e := range wrap.Data.Defs.Edges {
		if s, _ := e.Node["id"].(string); s == id {
			return e.Node, nil
		}
	}
	return nil, fmt.Errorf("custom field %s not found", id)
}

const customFieldsFullDoc = `query CustomFieldsQueryCES {
  customFieldDefinitions(limit: 1000) {
    edges {
      node {
        id
        schema {
          type
          title
          format
          allowedOperations
          allowedValues {
            id
            value
            deleted
            order
            __typename
          }
          uiValidations {
            mandatory
            defaultValue
            __typename
          }
          metadataProperties
          __typename
        }
        name
        deleted
        associatedEntityTypes {
          type
          deleted
          allowedOperations
          entityConditions {
            subtype
            deleted
            allowedOperations
            __typename
          }
          __typename
        }
        colorCode
        __typename
      }
      __typename
    }
    __typename
  }
}`

// PlannedCustomFieldDeleteURL reports the delete host for dry-run plans.
func PlannedCustomFieldDeleteURL() string { return customFieldGraphQLURL }
