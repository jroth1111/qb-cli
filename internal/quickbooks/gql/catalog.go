// Package gql provides the qb GraphQL gateway: an embedded catalog of QBO
// operations captured from the production SPA bundles, a transport that
// executes them inside the authenticated browser session over CDP, and a
// cursor/offset pagination walker.
//
// Design constraint (proven in the capture corpus): offline HTTP replay of
// saved headers against smallbusiness.api.intuit.com/graphql and the other
// GraphQL hosts returns 401 — those endpoints are session-bound. Every
// request is therefore executed inside a qbo.intuit.com page via
// Runtime.evaluate(fetch(...)) on the OMP relay, so cookies, CSRF tokens and
// the SPA's own auth context all apply.
package gql

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/gql/routes"
)

// Endpoint constants retain the public GraphQL host names. Verified
// per-operation routing and its evidence are shared with the generator
// through the routes package.
const (
	// EndpointDefault is the qbo webapp Apollo link.
	EndpointDefault = routes.EndpointDefault
	// EndpointWarehouse serves Commerce* inventory ops (order-management-ui
	// chunk 3275, inventory-addon-ui chunk 1156).
	EndpointWarehouse = routes.EndpointWarehouse
	// EndpointCommerceControl serves order-management / P&S list mutations
	// (commercecontrol.api.intuit.com).
	EndpointCommerceControl = routes.EndpointCommerceControl
	// EndpointSpendLists serves spend/tasks/app-rev ops observed on the
	// smallbusiness host (tasks-ui, app-revx-ui, integrations-apptransactions).
	EndpointSpendLists = routes.EndpointSpendLists
)

// Operation kinds.
const (
	KindQuery    = "query"
	KindMutation = "mutation"
)

// Op is one catalog entry.
type Op struct {
	Name string // operation name as declared in the document
	Kind string // query | mutation
	// Endpoint receives the POST. Per-op where the capture corpus proves a
	// dedicated host; EndpointDefault otherwise.
	Endpoint string
	// Module is the UI package the document was extracted from
	// (e.g. "spend-lists-xp"), useful for locating live traffic.
	Module string
	// VarTypes lists the declared variable type names in declaration order,
	// e.g. ["Int!", "String"].
	VarTypes []string
	// InputHint is the primary input type of a mutation when pass3 captured
	// it (e.g. "Banking_DisconnectOlbInput").
	InputHint string
	// BrokenTemplate marks documents whose webpack extraction left a JS
	// interpolation hole (.concat with a variable). They are listed but
	// refuse execution.
	BrokenTemplate bool
	Document       string // raw GraphQL document text
}

// ErrNotFound reports an unknown operation name.
type ErrNotFound struct{ Name string }

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("unknown GraphQL operation %q (see `qb gql ops`)", e.Name)
}

// catalog is built once from the generated table, sorted by name.
var catalog = buildCatalog()

// docOverrides replaces generated documents where the live schema drifted
// after the /tmp/qbo-cap corpus was extracted. Add entries only with live
// evidence (the server's validation errors + a verified corrected doc).
// 2026-09-16: commercecontrol dropped Product.channelIds and
// ProductVariant.channelIds; the rest of the captured GetProduct selection
// validates and returns live rows.
var docOverrides = map[string]string{
	"GetProduct": `query GetProduct($id: ID!) {
    item: getProduct(id: $id) {
      id
      description
      name
      status
      type
      trackStock
      source
      taxable
      systemGenerated
      default
      deferredRevenue
      deferredRevenueAccountId
      revRecDurationValue
      revRecDurationType
      category {
        name
        id
      }
      brand {
        name
      }
      source
      sellable
      purchasable
      mediaCollection {
        medias {
          id
          version
          documentId
          url
          mediumThumbnailUrl
          largeThumbnailUrl
          extraLargeThumbnailUrl
        }
      }
      taxCategoryId
      klassId
      version
      productAccountReferences {
        preferredVendorId
        purchaseDescription
        salesDescription
        purchaseAccountId
        salesAccountId
        salesTaxCodeId
        purchaseTaxCodeId
      }
      purchaseAccountId
      variabilityDimensions {
        name
        values
      }
      variants {
        id
        version
        name
        status
        price
        cost
        trackStock
        salesDescription
        sku
        qboItemId
        status
        purchaseRateIncludesTax
        salesRateIncludesTax
        costGroupId
        productVariantAccountReferences {
          purchaseDescription
          salesDescription
          salesAccountId
          purchaseAccountId
          salesTaxCodeId
          purchaseTaxCodeId
        }
        variabilityValues {
          dimension
          value
        }
        inventoryItemSummary {
          lowOnStock
          outOfStock
        }
        inventoryItem {
          id
          itemVersion
          accountReferences {
            cogsAccountId
            assetAccountId
          }
          inventoryLevels {
            qtyAvailable
            qtyOnHands
            maxReorderPoint
            committedOnSO
            committedOnMO
            committedOnTransfer
            incomingOnPO
            reorderPoint
            lowOnStock
            outOfStock
            quantityOnEstimates
          }
        }
        customExtensions {
          dimensions {
            definition {
              id
            }
            value
          }
          customObjects {
            definition {
              id
            }
            values
          }
        }
      }
    }
  }`,
	// Live captures 2026-09-17: the corpus docs for the reconcile session
	// mutations were truncated heads (undefined ...F7/...Fc fragments →
	// PLT-8000). These are the byte-exact documents the SPA sent; BEGIN and
	// CANCEL both verified end-to-end on TC2 account 45.
	"StartReconcile__integration_banking_reconcile_ui_qbo": `mutation StartReconcile__integration_banking_reconcile_ui_qbo(
  $input_0: UpdateIntegration_ReconciliationInput!
) {
  updateIntegration_Reconciliation(input: $input_0) {
    clientMutationId
    ...Fc
  }
}
fragment F0 on Integration_Reconciliation {
  id
  beginningBalance
  inProgress
  expenseTransactionAmount
  expenseTransactionAccount {
    id
    __typename
  }
  expenseTransactionDate
  expenseTransactionExchangeRate
  incomeTransactionAmount
  incomeTransactionAccount {
    id
    __typename
  }
  incomeTransactionDate
  incomeTransactionExchangeRate
  lastStatementEndingBalance
}
fragment F1 on Integration_Reconciliation {
  id
  beginningBalance
  inProgress
  lastStatementEndingBalance
}
fragment F2 on Integration_Reconciliation {
  id
  statementEndingDate
}
fragment F3 on Integration_Reconciliation {
  id
  beginningBalance
  statementEndingBalance
  statementEndingDate
  incomeTransactionAmount
  incomeTransactionAccount {
    id
    __typename
  }
  incomeTransactionDate
  incomeTransactionExchangeRate
  expenseTransactionAmount
  expenseTransactionAccount {
    id
    __typename
  }
  expenseTransactionDate
  expenseTransactionExchangeRate
}
fragment F4 on Integration_Reconciliation {
  statementEndingDate
  id
}
fragment F5 on Integration_Reconciliation {
  id
  statementEndingDate
  statementEndingBalance
  incomeTransactionDate
  incomeTransactionExchangeRate
  expenseTransactionDate
  expenseTransactionExchangeRate
  ...F4
}
fragment F6 on Integration_Reconciliation {
  id
  beginningBalance
  inProgress
  statementEndingBalance
  statementEndingDate
  ...F3
  ...F5
}
fragment F7 on Integration_Reconciliation {
  id
  beginningBalance
  inProgress
  statementEndingDate
  statementEndingBalance
  ...F6
}
fragment F8 on Integration_Reconciliation {
  id
}
fragment F9 on Integration_Reconciliation {
  id
  beginningBalance
  statementEndingBalance
  statementEndingDate
  incomeTransactionAmount
  incomeTransactionDate
  incomeTransactionExchangeRate
  expenseTransactionAmount
  expenseTransactionDate
  expenseTransactionExchangeRate
}
fragment Fa on Integration_Reconciliation {
  id
  beginningBalance
  inProgress
  statementEndingBalance
  statementEndingDate
  ...F9
  ...F5
}
fragment Fb on Integration_Reconciliation {
  id
  beginningBalance
  inProgress
  statementEndingDate
  statementEndingBalance
  ...Fa
}
fragment Fc on UpdateIntegration_ReconciliationPayload {
  integrationReconciliation {
    id
    ...F0
    id
    id
    ...F1
    id
    id
    ...F2
    id
    ...F7
    id
    id
    id
    id
    id
    id
    id
    id
    id
    ...F8
    id
    id
    id
    id
    ...Fb
    id
    id
  }
}`,
	"CancelReconcile__integration_banking_reconcile_ui_qbo": `mutation CancelReconcile__integration_banking_reconcile_ui_qbo(
  $input_0: UpdateIntegration_ReconciliationInput!
) {
  updateIntegration_Reconciliation(input: $input_0) {
    clientMutationId
    ...F7
  }
}
fragment F0 on Integration_Reconciliation {
  id
  beginningBalance
  inProgress
  expenseTransactionAmount
  expenseTransactionAccount {
    id
    __typename
  }
  expenseTransactionDate
  expenseTransactionExchangeRate
  incomeTransactionAmount
  incomeTransactionAccount {
    id
    __typename
  }
  incomeTransactionDate
  incomeTransactionExchangeRate
  lastStatementEndingBalance
}
fragment F1 on Integration_Reconciliation {
  id
  statementEndingDate
}
fragment F2 on Integration_Reconciliation {
  id
  beginningBalance
  statementEndingBalance
  statementEndingDate
  incomeTransactionAmount
  incomeTransactionAccount {
    id
    __typename
  }
  incomeTransactionDate
  incomeTransactionExchangeRate
  expenseTransactionAmount
  expenseTransactionAccount {
    id
    __typename
  }
  expenseTransactionDate
  expenseTransactionExchangeRate
}
fragment F3 on Integration_Reconciliation {
  statementEndingDate
  id
}
fragment F4 on Integration_Reconciliation {
  id
  statementEndingDate
  statementEndingBalance
  incomeTransactionDate
  incomeTransactionExchangeRate
  expenseTransactionDate
  expenseTransactionExchangeRate
  ...F3
}
fragment F5 on Integration_Reconciliation {
  id
  beginningBalance
  inProgress
  statementEndingBalance
  statementEndingDate
  ...F2
  ...F4
}
fragment F6 on Integration_Reconciliation {
  id
  beginningBalance
  inProgress
  statementEndingDate
  statementEndingBalance
  ...F5
}
fragment F7 on UpdateIntegration_ReconciliationPayload {
  integrationReconciliation {
    id
    ...F0
    id
    id
    ...F1
    id
    id
    ...F6
    id
    id
    id
    id
    id
    id
    id
    id
    id
    id
  }
}`,
	// Minimal verified doc 2026-09-17: the corpus doc was a truncated head
	// (...F7 undefined → PLT-8000). This selection is the envelope every
	// reconcile mutation shares; FINISH verified end-to-end on TC2 account 45
	// (inProgress=false after completion).
	"FinishReconcile__integration_banking_reconcile_ui_qbo": `mutation FinishReconcile__integration_banking_reconcile_ui_qbo(
  $input_0: UpdateIntegration_ReconciliationInput!
) {
  updateIntegration_Reconciliation(input: $input_0) {
    clientMutationId
    integrationReconciliation {
      id
      inProgress
      statementEndingDate
      statementEndingBalance
    }
  }
}`,
	// Live replay 2026-09-17: the captured CustomFieldsQueryCES doc carries a
	// webpack interpolation artifact (`customFieldDefinitions "," {`) that
	// the CES endpoint rejects with InvalidSyntax. This is the verified
	// working document (same shape the customfields UI sends).
	"CustomFieldsQueryCES": `query CustomFieldsQueryCES {
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
          }
          uiValidations {
            mandatory
            defaultValue
          }
          metadataProperties
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
          }
        }
        colorCode
      }
    }
  }
}`,
	// Live capture + replay 2026-09-17: the corpus docs for the custom-field
	// mutations ride the smallbusiness spend-lists schema
	// (*Common_CustomFieldDefinition inputs) which 400/401s on TC2. The
	// customfields UI actually posts create/updateCustomFieldDefinition to
	// customextensions with a CustomFieldDefinitionMutationInput payload
	// (verified end-to-end: create→update→delete→restore round-trip).
	"CustomFieldDefinitionCreateMutation": `mutation CustomFieldDefinitionCreateMutation(
  $input_0: CustomFieldDefinitionMutationInput!
) {
  createCustomFieldDefinition(input: $input_0) {
    id
    name
    deleted
    schema {
      title
      type
    }
  }
}`,
	"UpdateFieldDefinitionCreateMutation": `mutation UpdateFieldDefinitionCreateMutation(
  $input_0: CustomFieldDefinitionMutationInput!
) {
  updateCustomFieldDefinition(input: $input_0) {
    id
    name
    deleted
    schema {
      title
      type
    }
  }
}`,
}

func buildCatalog() map[string]*Op {
	m := make(map[string]*Op, len(catalogEntries))
	for i := range catalogEntries {
		op := catalogEntries[i]
		if endpoint, found := routes.Lookup(op.Name); found {
			op.Endpoint = endpoint
		}
		if doc, found := docOverrides[op.Name]; found {
			op.Document = doc
		}
		m[op.Name] = &op
	}
	return m
}

// Lookup returns the named operation or ErrNotFound.
func Lookup(name string) (*Op, error) {
	op, ok := catalog[name]
	if !ok {
		return nil, &ErrNotFound{Name: name}
	}
	return op, nil
}

// Ops returns every catalog operation sorted by name. When substr is
// non-empty only operations containing it (case-insensitive) are returned.
func Ops(substr string) []*Op {
	names := make([]string, 0, len(catalog))
	sub := strings.ToLower(substr)
	for name := range catalog {
		if sub == "" || strings.Contains(strings.ToLower(name), sub) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	out := make([]*Op, len(names))
	for i, n := range names {
		out[i] = catalog[n]
	}
	return out
}

// Count returns the total number of catalog operations.
func Count() int { return len(catalog) }

// ValidateVars checks user variables against the operation's declared
// signature. It returns the first unknown variable name, if any. Declared
// names come from the document itself ($first, $after, ...), so validation
// matches what the server actually accepts.
func (op *Op) ValidateVars(vars map[string]any) error {
	if len(vars) == 0 {
		return nil
	}
	declared := op.DeclaredVars()
	declSet := make(map[string]bool, len(declared))
	for _, d := range declared {
		declSet[d] = true
	}
	for name := range vars {
		if !declSet[name] {
			return fmt.Errorf("operation %s does not declare $%s; declared variables: %s",
				op.Name, name, joinNames(declared))
		}
	}
	return nil
}

// DeclaredVars returns the variable names declared by the document, in
// declaration order (empty for none).
func (op *Op) DeclaredVars() []string {
	doc := op.Document
	open := strings.Index(doc, "(")
	closing := strings.Index(doc, ")")
	selection := strings.Index(doc, "{")
	if open < 0 || closing < open || (selection >= 0 && selection < open) || !strings.HasPrefix(strings.TrimSpace(doc), op.Kind) {
		return nil
	}
	sig := doc[open+1 : closing]
	var out []string
	for _, match := range varDeclNameRe.FindAllStringSubmatch(sig, -1) {
		out = append(out, match[1])
	}
	return out
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "(none)"
	}
	q := make([]string, len(names))
	for i, n := range names {
		q[i] = "$" + n
	}
	return strings.Join(q, ", ")
}

// WalkStyle classifies how an operation pages.
type WalkStyle string

// Pagination styles auto-detected from the document's variables.
const (
	WalkNone   WalkStyle = ""       // no pagination variables at all
	WalkCursor WalkStyle = "cursor" // $after / pageInfo.endCursor
	WalkOffset WalkStyle = "offset" // $offset incremented per page
	WalkSingle WalkStyle = "single" // limit-only: one fetch is the whole set
)

// DetectWalkStyle inspects the document for cursor ($after), offset
// ($offset/$skip) or limit-only pagination.
func (op *Op) DetectWalkStyle() WalkStyle {
	declared := op.DeclaredVars()
	has := func(names ...string) bool {
		for _, v := range declared {
			for _, n := range names {
				if strings.EqualFold(v, n) {
					return true
				}
			}
		}
		return false
	}
	switch {
	case has("after", "afterCursor", "cursor"):
		return WalkCursor
	case has("offset", "skip"):
		return WalkOffset
	default:
		return WalkSingle
	}
}
