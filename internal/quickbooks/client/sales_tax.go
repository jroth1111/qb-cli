// sales_tax.go implements the `qb salestx` domain: the v3 company-API
// transaction surface (invoice/estimate/payment/creditmemo/delayedcharge/
// salesreceipt) plus the tax readers (taxcode/taxrate) and the indirect-tax
// GST/BAS/TPAR operations, all sourced from the 2026-08 capture corpus.
//
// Endpoint evidence:
//
//   - v3 transactions: https://qbo.intuit.com/api/v3/company/{realm}/{entity}
//     (normalized_calls.json "/api/v3/company/"; same POST shape as
//     mutate.go ReplayMutate — create/update/delete via operation=delete,
//     void via operation=void, send via /{entity}/{id}/send?sendTo=…).
//
//   - GetTaxCodes_qbo (graphql/GetTaxCodes_qbo.graphql): company-scoped tax
//     groups with saleRates + qboAppData.salesTaxCodeDisplayName. Executed by
//     the SPA over qbo.intuit.com/api/v4/graphql (gql_uris.json apollo_uris),
//     the app-foundations gateway this CLI already drives for SalesHub and
//     customobjects. [INFERENCE] on routing, flagged per customobjects.go
//     precedent.
//
//   - TaxRates__indirect_tax_ui_qbo (graphql/TaxRates__indirect_tax_ui_qbo):
//     company.taxAgencies → taxRates edges {id name rate status configType
//     taxAgency{id name}}; same v4 gateway.
//
//   - gst list   = IndirectTaxConfigQuery_qbo (taxconfig.api.intuit.com/graphql;
//     bundle 20012.js pins PROD taxConfigApiUrl "https://taxconfig.api.intuit.com"
//     with taxConfigApiPath "graphql").
//     gst file    = CreateTaxPayment mutation → indirectTaxCreatePayment
//     (IndirectTax_CreatePaymentInput{clientMutationId, taxPayment}).
//
//   - bas list   = TaxReturns__indirect_tax_ui_qbo (node(id){...on
//     Indirecttaxes_TaxAgency{taxReturns{edges{node{id startDate
//     returnDueStatus taxAgency{code}}}}}}), agency node from TaxSetting.
//     bas lodge   = EfileTaxReturn mutation → indirectTaxUpdateTaxReturn
//     (input.indirecttaxesTaxReturn{id entityVersion eFilingStatus}; bundle
//     deobfuscated.js lines 400-453).
//
//   - tpar list     = replayTPARInstances (tpar.go universalreportinsights).
//     tpar generate = not wired: no captured generate/lodge API beyond the
//     instances read (QBO.TAX.TPAR_EXPORT stays blocked in the catalog).
//
// The indirect-tax service is GraphQL POST scoped by session cookie (no realm
// URL segment); dry-run plans therefore carry the service URL only.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	taxCodesGraphQLURL  = "https://qbo.intuit.com/api/v4/graphql"
	taxRatesGraphQLURL  = "https://qbo.intuit.com/api/v4/graphql"
	gstConfigURL        = "https://taxconfig.api.intuit.com/graphql"
	taxFilingServiceURL = "https://taxfilingsvc.api.intuit.com/graphql/oigql"
)

const taxReferer = "https://qbo.intuit.com/app/tax/home"

func PlannedSalesTaxGatewayURL() string { return taxCodesGraphQLURL }
func PlannedGSTConfigURL() string       { return gstConfigURL }

// PlannedGSTFileURL is the filing-service oigql endpoint CreateTaxPayment runs
// against when the SPA takes the modernized path.
func PlannedGSTFileURL() string { return taxFilingServiceURL }

// Empty-input sentinels fire during planning, before any session load or dial.
var (
	ErrEmptyTaxPaymentID  = errors.New("gst get/file requires a non-empty payment id")
	ErrEmptyTaxReturnID   = errors.New("bas lodge requires a non-empty return id")
	ErrMissingTaxAgencyID = errors.New("bas list requires a non-empty --agency id")
	ErrTPARGenerateNoAPI  = errors.New("tpar generate: no captured API (instances read only)")
)

// --- captured operations -----------------------------------------------------

const getTaxCodesQuery = `query GetTaxCodes_qbo($filterBy: String, $orderBy: String, $limit: Int, $offset: Int) {
        company {
          taxGroups(orderBy: $orderBy, filterBy: $filterBy, limit: $limit, offset: $offset) {
            edges {
              node {
                id
                hidden
                deleted
                name
                saleRates {
                  taxRateOrder
                }
                qboAppData {
                  salesTaxCodeDisplayName
                }
              }
            }
          }
        }
      }`

const taxRatesQuery = `query TaxRates__indirect_tax_ui_qbo {
                company {
                    taxAgencies {
                        edges {
                            node {
                                taxRates {
                                    edges {
                                        node {
                                            status
                                            id
                                            name
                                            rate
                                            configType
                                            taxAgency {
                                                name
                                                id
                                                configType
                                            }
                                        }
                                    }
                                }
                            }
                        }
                    }
                }
            }`

const indirectTaxConfigQuery = `query IndirectTaxConfigQuery_qbo{indirectTaxTaxAgencies{edges{node{globalId legacyDisplayName name legacyTaxAgencyCode taxCategories{edges{node{id filingConfig{taxReportBasis taxScheme}}}}taxRegistrationConfig{taxRegistrationNumbers{code taxRegistrationNumber}}}}}indirectTaxTaxGroups{edges{node{globalId taxGroupComplianceCode name description configType taxRates{edges{node{id globalId taxCategoryId legacyTaxRateCode name configType applicableRates{applicableOn effectiveRates{ratePercentage flatAmount startDate endDate}}}}}}}}}`

const taxReturnsQuery = `query TaxReturns__indirect_tax_ui_qbo($id: ID!) {
                node(id: $id) {
                    id
                    ... on Indirecttaxes_TaxAgency {
                        id
                        taxReturns(first: 9999) {
                            edges {
                                node {
                                    id
                                    startDate
                                    returnDueStatus
                                    taxAgency {
                                        code
                                    }
                                }
                            }
                        }
                    }
                }
            }`

const createTaxPaymentMutation = `mutation CreateTaxPayment($input: IndirectTax_CreatePaymentInput!) {
                indirectTaxCreatePayment(input: $input) {
                    clientMutationId
                }
            }`

const efileTaxReturnMutation = `mutation EfileTaxReturn($input: IndirectTax_UpdateReturnInput!) {
                indirectTaxUpdateTaxReturn(input: $input) {
                    clientMutationId
                    indirecttaxesTaxReturn {
                        id
                        startDate
                        endDate
                        netTaxAmountDue
                        taxReturnType
                        fileDate
                    }
                }
            }`

const getTaxPaymentsQuery = `query GetPayments($filterBy: IndirectTax_TaxPaymentFilter) {
        indirectTaxGetTaxPayments(filterBy: $filterBy) {
            id
            paymentAmount
            paymentDate
            description
        }
    }`

// --- plans -------------------------------------------------------------------

func gqlPlan(url, op string, body []byte, note string) *RequestPlan {
	return &RequestPlan{
		DryRun: true,
		Dial:   false,
		Method: http.MethodPost,
		URL:    url,
		Body:   json.RawMessage(body),
		Note:   note,
		op:     op,
	}
}

// PlanTaxCodeList builds the captured GetTaxCodes_qbo POST.
func PlanTaxCodeList(limit int) *RequestPlan {
	body, err := json.Marshal(map[string]any{
		"operationName": "GetTaxCodes_qbo",
		"variables": map[string]any{
			"filterBy": nil,
			"orderBy":  nil,
			"limit":    clampPage(limit),
			"offset":   0,
		},
		"query": getTaxCodesQuery,
	})
	if err != nil {
		return gqlPlan(taxCodesGraphQLURL, "salestx.taxcode.list", []byte("{}"), "GetTaxCodes_qbo; not sent")
	}
	return gqlPlan(taxCodesGraphQLURL, "salestx.taxcode.list", body, "GetTaxCodes_qbo; not sent")
}

// PlanTaxRateList builds the captured TaxRates__indirect_tax_ui_qbo POST.
func PlanTaxRateList() *RequestPlan {
	body, err := json.Marshal(map[string]any{
		"operationName": "TaxRates__indirect_tax_ui_qbo",
		"variables":     map[string]any{},
		"query":         taxRatesQuery,
	})
	if err != nil {
		body = []byte("{}")
	}
	return gqlPlan(taxRatesGraphQLURL, "salestx.taxrate.list", body, "TaxRates__indirect_tax_ui_qbo; not sent")
}

// PlanGSTList builds the captured IndirectTaxConfigQuery_qbo POST.
func PlanGSTList() *RequestPlan {
	body, err := json.Marshal(map[string]any{
		"operationName": "IndirectTaxConfigQuery_qbo",
		"variables":     map[string]any{},
		"query":         indirectTaxConfigQuery,
	})
	if err != nil {
		body = []byte("{}")
	}
	return gqlPlan(gstConfigURL, "salestx.gst.list", body, "IndirectTaxConfigQuery_qbo (taxconfig graphql); not sent")
}

// PlanBASList builds TaxReturns__indirect_tax_ui_qbo for one agency node.
func PlanBASList(agencyID string) (*RequestPlan, error) {
	agencyID = sanitizeToken(agencyID)
	if agencyID == "" {
		return nil, ErrMissingTaxAgencyID
	}
	body, err := json.Marshal(map[string]any{
		"operationName": "TaxReturns__indirect_tax_ui_qbo",
		"variables":     map[string]any{"id": agencyID},
		"query":         taxReturnsQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding bas list: %w", err)
	}
	return gqlPlan(gstConfigURL, "salestx.bas.list", body, "TaxReturns__indirect_tax_ui_qbo (taxconfig graphql); not sent"), nil
}

// PlanGSTFile builds CreateTaxPayment. amount/date/account/agency mirror the
// SPA payload (paymentAmount/paymentDate/paymentAccount.id/taxAgency.id);
// description is optional.
func PlanGSTFile(amount float64, date, accountID, agencyID, description string) (*RequestPlan, error) {
	if accountID == "" {
		accountID = firstNonEmpty(accountID, "")
	}
	if strings.TrimSpace(accountID) == "" || strings.TrimSpace(agencyID) == "" {
		return nil, ErrEmptyTaxPaymentID
	}
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	payment := map[string]any{
		"id":             "0",
		"refund":         false,
		"deleted":        false,
		"paymentAmount":  amount,
		"paymentDate":    normalizeDate(date),
		"description":    description,
		"printCheck":     false,
		"paymentAccount": map[string]any{"id": strings.TrimSpace(accountID)},
		"taxAgency":      map[string]any{"id": strings.TrimSpace(agencyID)},
	}
	body, err := json.Marshal(map[string]any{
		"operationName": "CreateTaxPayment",
		"variables": map[string]any{
			"input": map[string]any{
				"clientMutationId": "0",
				"taxPayment":       payment,
			},
		},
		"query": createTaxPaymentMutation,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding gst file: %w", err)
	}
	return gqlPlan(taxFilingServiceURL, "salestx.gst.file", body, "CreateTaxPayment (taxfilingsvc oigql); not sent"), nil
}

// PlanBASLodge builds EfileTaxReturn. eFilingStatus follows the captured UI
// flow ("Filed"); entityVersion defaults to 0 when unknown (SPA behaviour).
func PlanBASLodge(returnID, eFilingStatus string, entityVersion int) (*RequestPlan, error) {
	returnID = sanitizeToken(returnID)
	if returnID == "" {
		return nil, ErrEmptyTaxReturnID
	}
	if strings.TrimSpace(eFilingStatus) == "" {
		eFilingStatus = "Filed"
	}
	body, err := json.Marshal(map[string]any{
		"operationName": "EfileTaxReturn",
		"variables": map[string]any{
			"input": map[string]any{
				"indirecttaxesTaxReturn": map[string]any{
					"id":            returnID,
					"entityVersion": entityVersion,
					"eFilingStatus": eFilingStatus,
				},
			},
		},
		"query": efileTaxReturnMutation,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding bas lodge: %w", err)
	}
	return gqlPlan(taxFilingServiceURL, "salestx.bas.lodge", body, "EfileTaxReturn (taxfilingsvc oigql); not sent"), nil
}

// PlanTPARList reuses the embedded TAXABLE_PAYMENTS instances body (tpar.go).
func PlanTPARList(limit int) *RequestPlan {
	var payload map[string]any
	if err := json.Unmarshal(tparInstancesBodyJSON, &payload); err != nil {
		return gqlPlan(tparInstancesURL, "salestx.tpar.list", []byte("{}"), "universalreportinsights TAXABLE_PAYMENTS/instances; not sent")
	}
	if pag, ok := payload["paginationOptions"].(map[string]any); ok {
		pag["pageSize"] = clampPage(limit)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		body = []byte("{}")
	}
	return gqlPlan(tparInstancesURL, "salestx.tpar.list", body, "universalreportinsights TAXABLE_PAYMENTS/instances; not sent")
}

// clampPage normalizes the requested page size; the service enforces its
// own ceiling, so no client-side cap is applied here. A caller limit <= 0
// asks for everything — the service decides how much it returns.
func clampPage(limit int) int {
	if limit < 1 {
		return 99999
	}
	return limit
}

// --- executors ---------------------------------------------------------------

// execTaxGraphQL POSTs a plan with the wrap-ATS header set plus the tax SPA
// Referer. Writes additionally pass the production-realm guard.
func execTaxGraphQL(ctx context.Context, plan *RequestPlan, write bool) ([]byte, int, error) {
	ac, err := newAPIClient()
	if err != nil {
		return nil, 0, err
	}
	extra := map[string]string{
		"Referer":      taxReferer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	resp, err := ac.postJSONExtra(ctx, plan.URL, []byte(plan.Body), extra)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", plan.op, err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	raw, err := readBody(resp)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", plan.op, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, resp.StatusCode, &ReplayError{Status: resp.StatusCode, Message: errorMessage(raw)}
	}
	return raw, resp.StatusCode, nil
}

// --- v3 transaction verbs ----------------------------------------------------

// SalestxV3Entity maps a CLI entity word onto the v3 path token. All six are
// already present in v3Path, so ReplayMutate handles create/update/delete/
// void/send/copy without new plumbing.
func SalestxV3Entity(entity string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(entity)) {
	case "invoice":
		return "Invoice", true
	case "estimate":
		return "Estimate", true
	case "payment":
		return "Payment", true
	case "creditmemo":
		return "CreditMemo", true
	case "delayedcharge":
		return "DelayedCharge", true
	case "salesreceipt":
		return "SalesReceipt", true
	default:
		return "", false
	}
}

// ReplaySalestxMutate runs a v3 write/read-modify op for a salestx entity.
// It is a thin typed wrapper over ReplayMutate so the CLI domain has one
// entry point and tests can pin the entity mapping.
func ReplaySalestxMutate(ctx context.Context, entity, op, id string, flags map[string]string) (*MutateResult, error) {
	v3ent, ok := SalestxV3Entity(entity)
	if !ok {
		return nil, fmt.Errorf("salestx: unknown entity %q", entity)
	}
	return ReplayMutate(ctx, v3ent, op, id, flags)
}

// ReplaySalestxQuery lists or gets a v3 sales transaction by entity word.
func ReplaySalestxQuery(ctx context.Context, entity, id, query string, limit int) (*QueryResult, error) {
	v3ent, ok := SalestxV3Entity(entity)
	if !ok {
		return nil, fmt.Errorf("salestx: unknown entity %q", entity)
	}
	return ReplayQuery(ctx, v3ent, id, query, limit)
}

// PlannedSalestxMutateURL mirrors PlannedMutateURL for dry-run output using
// CLI entity words ("invoice" → Invoice path).
func PlannedSalestxMutateURL(entity, op string) string {
	v3ent, ok := SalestxV3Entity(entity)
	if !ok {
		return ""
	}
	return PlannedMutateURL(v3ent, op)
}

// --- taxcode / taxrate -------------------------------------------------------

// ReplayTaxCodeList runs the captured GetTaxCodes_qbo query. Read-only.
func ReplayTaxCodeList(ctx context.Context, limit int) (*QueryResult, error) {
	plan := PlanTaxCodeList(limit)
	raw, st, err := execTaxGraphQL(ctx, plan, false)
	if err != nil {
		return nil, err
	}
	res := projectTaxGroups(raw)
	res.Status = st
	return res, nil
}

// ReplayTaxRateList runs the captured TaxRates__indirect_tax_ui_qbo query.
// Read-only.
func ReplayTaxRateList(ctx context.Context) (*QueryResult, error) {
	plan := PlanTaxRateList()
	raw, st, err := execTaxGraphQL(ctx, plan, false)
	if err != nil {
		return nil, err
	}
	res := projectTaxRates(raw)
	res.Status = st
	return res, nil
}

func projectTaxGroups(body []byte) *QueryResult {
	note := "GetTaxCodes_qbo"
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Company struct {
				TaxGroups struct {
					Edges []struct {
						Node struct {
							ID         string `json:"id"`
							Hidden     bool   `json:"hidden"`
							Deleted    bool   `json:"deleted"`
							Name       string `json:"name"`
							QboAppData struct {
								SalesTaxCodeDisplayName string `json:"salesTaxCodeDisplayName"`
							} `json:"qboAppData"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"taxGroups"`
			} `json:"company"`
		} `json:"data"`
	}
	items := []QueryItem{}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "TaxCode", Counts: map[string]int{"items": 0}, Items: items, Note: note + " (unparsed)"}
	}
	for _, e := range wrap.Data.Company.TaxGroups.Edges {
		if e.Node.ID == "" && e.Node.Name == "" {
			continue
		}
		name := e.Node.Name
		if e.Node.QboAppData.SalesTaxCodeDisplayName != "" {
			name += " (" + e.Node.QboAppData.SalesTaxCodeDisplayName + ")"
		}
		it := QueryItem{ID: e.Node.ID, Name: name, Type: "TaxCode"}
		if e.Node.Hidden || e.Node.Deleted {
			f := false
			it.Active = &f
		}
		items = append(items, it)
	}
	if len(wrap.Errors) > 0 {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{Entity: "TaxCode", Counts: map[string]int{"items": len(items)}, Items: items, Note: note}
}

func projectTaxRates(body []byte) *QueryResult {
	note := "TaxRates__indirect_tax_ui_qbo"
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Company struct {
				TaxAgencies struct {
					Edges []struct {
						Node struct {
							TaxRates struct {
								Edges []struct {
									Node struct {
										ID        string  `json:"id"`
										Name      string  `json:"name"`
										Rate      float64 `json:"rate"`
										Status    string  `json:"status"`
										TaxAgency struct {
											ID   string `json:"id"`
											Name string `json:"name"`
										} `json:"taxAgency"`
									} `json:"node"`
								} `json:"edges"`
							} `json:"taxRates"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"taxAgencies"`
			} `json:"company"`
		} `json:"data"`
	}
	items := []QueryItem{}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "TaxRate", Counts: map[string]int{"items": 0}, Items: items, Note: note + " (unparsed)"}
	}
	for _, ag := range wrap.Data.Company.TaxAgencies.Edges {
		for _, r := range ag.Node.TaxRates.Edges {
			n := r.Node
			if n.ID == "" && n.Name == "" {
				continue
			}
			items = append(items, QueryItem{
				ID:      n.ID,
				Name:    n.Name,
				Amount:  n.Rate,
				Type:    "TaxRate",
				Account: n.TaxAgency.Name,
			})
		}
	}
	if len(wrap.Errors) > 0 {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{Entity: "TaxRate", Counts: map[string]int{"items": len(items)}, Items: items, Note: note}
}

// --- gst ---------------------------------------------------------------------

// ReplayGSTList returns agencies/groups/categories from taxconfig. Read-only.
func ReplayGSTList(ctx context.Context) (*QueryResult, error) {
	plan := PlanGSTList()
	raw, st, err := execTaxGraphQL(ctx, plan, false)
	if err != nil {
		return nil, err
	}
	res := projectGSTConfig(raw)
	res.Status = st
	return res, nil
}

type gstAgencyRow struct {
	GlobalID            string `json:"globalId"`
	Name                string `json:"name"`
	LegacyDisplayName   string `json:"legacyDisplayName"`
	LegacyTaxAgencyCode string `json:"legacyTaxAgencyCode"`
	TaxCategories       struct {
		Edges []struct {
			Node struct {
				ID string `json:"id"`
			} `json:"node"`
		} `json:"edges"`
	} `json:"taxCategories"`
}

func projectGSTConfig(body []byte) *QueryResult {
	note := "IndirectTaxConfigQuery_qbo (taxconfig)"
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Agencies struct {
				Edges []struct {
					Node gstAgencyRow `json:"node"`
				} `json:"edges"`
			} `json:"indirectTaxTaxAgencies"`
			Groups struct {
				Edges []struct {
					Node struct {
						Name     string `json:"name"`
						GlobalID string `json:"globalId"`
					} `json:"node"`
				} `json:"edges"`
			} `json:"indirectTaxTaxGroups"`
		} `json:"data"`
	}
	items := []QueryItem{}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{Entity: "GST", Counts: map[string]int{"items": 0}, Items: items, Note: note + " (unparsed)"}
	}
	for _, e := range wrap.Data.Agencies.Edges {
		name := e.Node.Name
		if e.Node.LegacyDisplayName != "" && e.Node.LegacyDisplayName != name {
			name += " (" + e.Node.LegacyDisplayName + ")"
		}
		items = append(items, QueryItem{
			ID:      e.Node.GlobalID,
			Name:    name,
			Type:    "TaxAgency",
			Account: e.Node.LegacyTaxAgencyCode,
		})
	}
	for _, e := range wrap.Data.Groups.Edges {
		items = append(items, QueryItem{ID: e.Node.GlobalID, Name: e.Node.Name, Type: "TaxGroup"})
	}
	if len(wrap.Errors) > 0 {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{Entity: "GST", Counts: map[string]int{"items": len(items)}, Items: items, Note: note}
}

// ReplayGSTFile records a tax payment (CreateTaxPayment). Write: guarded.
func ReplayGSTFile(ctx context.Context, amount float64, date, accountID, agencyID, description string) (*MutateResult, error) {
	plan, err := PlanGSTFile(amount, date, accountID, agencyID, description)
	if err != nil {
		return nil, err
	}
	raw, st, err := execTaxGraphQL(ctx, plan, true)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	_ = json.Unmarshal(raw, &wrap)
	if len(wrap.Errors) > 0 {
		return nil, fmt.Errorf("CreateTaxPayment: %s", wrap.Errors[0].Message)
	}
	return &MutateResult{
		Status: st,
		Op:     "create",
		Entity: "TaxPayment",
		Item:   QueryItem{Type: "TaxPayment", Amount: amount, Date: normalizeDate(date)},
		Note:   "CreateTaxPayment (taxfilingsvc oigql)",
	}, nil
}

// ReplayGSTPayments lists recorded tax payments (GetPayments). Read-only.
func ReplayGSTPayments(ctx context.Context) (*QueryResult, error) {
	body, err := json.Marshal(map[string]any{
		"operationName": "GetPayments",
		"variables":     map[string]any{"filterBy": nil},
		"query":         getTaxPaymentsQuery,
	})
	if err != nil {
		return nil, fmt.Errorf("encoding gst payments: %w", err)
	}
	plan := gqlPlan(taxFilingServiceURL, "salestx.gst.payments", body, "GetPayments (taxfilingsvc oigql); not sent")
	raw, st, err := execTaxGraphQL(ctx, plan, false)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Payments []struct {
				ID            string  `json:"id"`
				PaymentAmount float64 `json:"paymentAmount"`
				PaymentDate   string  `json:"paymentDate"`
				Description   string  `json:"description"`
			} `json:"indirectTaxGetTaxPayments"`
		} `json:"data"`
	}
	items := []QueryItem{}
	if json.Unmarshal(raw, &wrap) == nil {
		for _, p := range wrap.Data.Payments {
			items = append(items, QueryItem{ID: p.ID, Name: p.Description, Date: p.PaymentDate, Amount: p.PaymentAmount, Type: "TaxPayment"})
		}
	}
	note := "GetPayments (taxfilingsvc oigql)"
	if len(wrap.Errors) > 0 {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{Status: st, Entity: "GST", Counts: map[string]int{"items": len(items)}, Items: items, Note: note}, nil
}

// --- bas ---------------------------------------------------------------------

// ReplayBASList returns tax returns for one agency node. Read-only.
func ReplayBASList(ctx context.Context, agencyID string) (*QueryResult, error) {
	plan, err := PlanBASList(agencyID)
	if err != nil {
		return nil, err
	}
	raw, st, err := execTaxGraphQL(ctx, plan, false)
	if err != nil {
		return nil, err
	}
	res := projectTaxReturns(raw)
	res.Status = st
	return res, nil
}

func projectTaxReturns(body []byte) *QueryResult {
	note := "TaxReturns__indirect_tax_ui_qbo (taxconfig)"
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Node struct {
				ID         string `json:"id"`
				TaxReturns struct {
					Edges []struct {
						Node struct {
							ID              string `json:"id"`
							StartDate       string `json:"startDate"`
							ReturnDueStatus string `json:"returnDueStatus"`
							TaxAgency       struct {
								Code string `json:"code"`
							} `json:"taxAgency"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"taxReturns"`
			} `json:"node"`
		} `json:"data"`
	}
	items := []QueryItem{}
	if json.Unmarshal(body, &wrap) == nil {
		for _, e := range wrap.Data.Node.TaxReturns.Edges {
			items = append(items, QueryItem{
				ID:      e.Node.ID,
				Date:    e.Node.StartDate,
				Name:    e.Node.ReturnDueStatus,
				Account: e.Node.TaxAgency.Code,
				Type:    "TaxReturn",
			})
		}
	}
	if len(wrap.Errors) > 0 {
		note += "; gql: " + wrap.Errors[0].Message
	}
	return &QueryResult{Entity: "BAS", Counts: map[string]int{"items": len(items)}, Items: items, Note: note}
}

// ReplayBASLodge marks a return filed (EfileTaxReturn). Write: guarded.
func ReplayBASLodge(ctx context.Context, returnID, eFilingStatus string, entityVersion int) (*MutateResult, error) {
	plan, err := PlanBASLodge(returnID, eFilingStatus, entityVersion)
	if err != nil {
		return nil, err
	}
	raw, st, err := execTaxGraphQL(ctx, plan, true)
	if err != nil {
		return nil, err
	}
	var wrap struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
		Data struct {
			Update struct {
				Return struct {
					ID         string  `json:"id"`
					FileDate   string  `json:"fileDate"`
					StartDate  string  `json:"startDate"`
					EndDate    string  `json:"endDate"`
					NetTaxAmt  float64 `json:"netTaxAmountDue"`
					ReturnType string  `json:"taxReturnType"`
				} `json:"indirecttaxesTaxReturn"`
			} `json:"indirectTaxUpdateTaxReturn"`
		} `json:"data"`
	}
	_ = json.Unmarshal(raw, &wrap)
	if len(wrap.Errors) > 0 {
		return nil, fmt.Errorf("EfileTaxReturn: %s", wrap.Errors[0].Message)
	}
	r := wrap.Data.Update.Return
	item := QueryItem{ID: r.ID, Type: "TaxReturn", Date: r.StartDate, Amount: r.NetTaxAmt, Name: r.ReturnType}
	if item.ID == "" {
		item.ID = returnID
	}
	return &MutateResult{
		Status: st,
		Op:     "lodge",
		Entity: "BAS",
		Item:   item,
		Note:   "EfileTaxReturn (taxfilingsvc oigql)",
	}, nil
}

// --- tpar --------------------------------------------------------------------

// ReplayTPARList delegates to the shared TAXABLE_PAYMENTS instances reader.
func ReplayTPARList(ctx context.Context, limit int) (*QueryResult, error) {
	return replayTPARInstances(ctx, "", "", limit)
}

// ReplayTPARGenerate is intentionally not wired: the capture corpus contains
// only the instances read for TPAR (no generate/lodge API was observed), so
// this returns the documented sentinel rather than inventing an endpoint.
func ReplayTPARGenerate(context.Context) (*MutateResult, error) {
	return nil, ErrTPARGenerateNoAPI
}
