package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Captured 2026-08-19 from POST https://qbo.intuit.com/app/lists
// (catalog-capture-rest.json screens.lists). Wrap ATS proved HTTP 200
// ListsPrefs graphQuery — honest lists prefs, not the Account stand-in.
const listsEntitiesURL = "https://qbo.intuit.com/api/v4/entities"
const listsReferer = "https://qbo.intuit.com/app/lists"

// listsPrefsGraphQueryTmpl is the captured v4/entities graphQuery. %s is realm.
const listsPrefsGraphQueryTmpl = `query q { company {
      contacts (filterBy: "profiles.user!=null", with: "id='%s'  AND groups='Prefs'") {
          edges {
            node {
              profiles {
                user {
                  qboAppData {
                    accountNumberVisible
                    dismissedSquareFTUTooltip
                  }
                }
              }
            }
          }
        }
       settings (with: "ListsPrefs") { ...settingsListsPrefsFragment ...transactionListsPrefsFragment ...taxListsPrefsFragment ...paymentsListsPrefsFragment ...inventoryListsPrefsFragment }  } }
    fragment settingsListsPrefsFragment on Company_Settings {
      entityVersion
    }

    fragment transactionListsPrefsFragment on Company_Settings {
      transactionSettings {
        locationEnabled
        accountNumbersEnabled
        timeTrackingSettings {
          timeTrackingEnabled
        }
        purchaseAccountingSettings {
          billableExpenseEnabled
          taxOnBillableExpenseEnabled
          customerTrackingOnExpenseEnabled
        }
        salesAccountingSettings {
          trackQuantityRate
          barcodeEnabled
          defaultTransactionDate
        }
        qboAppData {
          assignAccountNumbersVisible
          chartOfAccountImportSupported
          discountAccountingSupported
          discountAtLineLevelSupported
          balanceInHomeCurrencyVisible
          voucherFeatureSupported
          recurringFeatureSupported
          accountNumberEditable
          financiallyRelevantDateRangeStartDate
          useItemForTime
          transactionLocation {
            name
            value
          }
          transactionTaxGroup {
            edges {
              node {
                id
                name
              }
            }
          }
          transactionListSettings {
            accounTypeVisible
            accounDetailTypeVisible
            taxRateVisible
            bankBalanceByDefaultVisible
            journalCodeByDefaultVisible
            descriptionInListViewVisible
            balanceVisible
            journalCodeVisible
            prefixedAccountNoWithNameVisible
            displayNameOnGridVisible
            displayNameByDefaultVisible
            purchaseSaleLocationOnGridVisble
            purchaseSaleLocationByDefaultVisible
            taxCodeEditable
            descriptionByDefaultVisible
          }
        }
      }
    }

    fragment taxListsPrefsFragment on Company_Settings {
      taxSettings {
        defaultTaxAccount {
          id
        }
        defaultTaxGroup {
          id
        }
        qboAppData {
          transactionLineLevelTaxSupported
          transactionTaxTypeSupported
          taxBeforeAfterDiscountSwitch
          taxOnPurchase
          taxOnJournalEntrySupported
          taxOnShippingSupported
        }
      }
    }

    fragment paymentsListsPrefsFragment on Company_Settings {
      paymentSettings {
        paymentsBundleEnabled
       qboAppData {
          onlinePaymentsEnabled
          onlinePaymentsSupported
        }
      }
    }

    fragment inventoryListsPrefsFragment on Company_Settings {
      inventorySettings {
        itemOnSalesTransactionEnabled
        trackQuantityOnHand
        qboAppData {
          itemZeroState
          priceRulesSupported
          categoryMigrationEnabled
          bundlesSupported
          categoriesEnabled
        }
      }
    }
     `

func PlannedListsPrefsURL() string { return listsEntitiesURL }

func listsPrefsGraphQuery(realm string) string {
	if realm == "" {
		realm = "9341457731465363"
	}
	return fmt.Sprintf(listsPrefsGraphQueryTmpl, realm)
}

// replayListsPrefs POSTs the captured ListsPrefs v4/entities query.
// id/query are accepted for cobra flag parity but not forwarded.
func replayListsPrefs(ctx context.Context, _, _ string, limit int) (*QueryResult, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	ac, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal([]map[string]any{
		{
			"$type":      "/Query",
			"graphQuery": listsPrefsGraphQuery(ac.realm),
			"variables":  nil,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("lists ListsPrefs: %w", err)
	}
	extra := map[string]string{
		"Referer":      listsReferer,
		"Origin":       "https://qbo.intuit.com",
		"Content-Type": "application/json",
	}
	resp, err := ac.postJSONExtra(ctx, listsEntitiesURL, payload, extra)
	if err != nil {
		return nil, fmt.Errorf("lists ListsPrefs: %w", err)
	}
	defer func(r *http.Response) { _ = drainAndClose(r) }(resp)
	body, err := readBody(resp)
	if err != nil {
		return nil, fmt.Errorf("reading lists ListsPrefs: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &ReplayError{Status: resp.StatusCode, Message: errorMessage(body)}
	}
	res := projectListsPrefs(body)
	res.Status = resp.StatusCode
	_ = limit
	return res, nil
}

func projectListsPrefs(body []byte) *QueryResult {
	note := "v4/entities ListsPrefs"
	var wrap []struct {
		Type string `json:"$type"`
		Data []struct {
			Company *struct {
				Settings *json.RawMessage `json:"settings"`
				Contacts *struct {
					Edges []struct {
						Node *struct {
							Profiles *struct {
								User *struct {
									QboAppData *struct {
										AccountNumberVisible      *bool `json:"accountNumberVisible"`
										DismissedSquareFTUTooltip *bool `json:"dismissedSquareFTUTooltip"`
									} `json:"qboAppData"`
								} `json:"user"`
							} `json:"profiles"`
						} `json:"node"`
					} `json:"edges"`
				} `json:"contacts"`
			} `json:"company"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &wrap); err != nil {
		return &QueryResult{
			Entity: "ListsPrefs",
			Counts: map[string]int{"items": 0},
			Items:  []QueryItem{},
			Note:   note + " (unparsed)",
		}
	}
	contacts := 0
	settings := 0
	items := []QueryItem{}
	for _, row := range wrap {
		if row.Data == nil {
			continue
		}
		for _, d := range row.Data {
			if d.Company == nil {
				continue
			}
			if d.Company.Settings != nil && len(*d.Company.Settings) > 0 && string(*d.Company.Settings) != "null" {
				settings++
			}
			if d.Company.Contacts == nil {
				continue
			}
			for _, e := range d.Company.Contacts.Edges {
				contacts++
				it := QueryItem{ID: "Prefs", Name: "ListsPrefs", Type: "Prefs"}
				if e.Node != nil && e.Node.Profiles != nil && e.Node.Profiles.User != nil && e.Node.Profiles.User.QboAppData != nil {
					if e.Node.Profiles.User.QboAppData.AccountNumberVisible != nil {
						it.Active = e.Node.Profiles.User.QboAppData.AccountNumberVisible
					}
				}
				items = append(items, it)
			}
		}
	}
	if settings > 0 && len(items) == 0 {
		items = append(items, QueryItem{ID: "ListsPrefs", Name: "ListsPrefs", Type: "Prefs"})
	}
	var prefs any
	for _, row := range wrap {
		for _, d := range row.Data {
			if d.Company != nil && d.Company.Settings != nil && len(*d.Company.Settings) > 0 && string(*d.Company.Settings) != "null" {
				var m map[string]any
				if json.Unmarshal(*d.Company.Settings, &m) == nil {
					prefs = m
				}
			}
		}
	}
	note = fmt.Sprintf("%s contacts=%d settings=%d", note, contacts, settings)
	return &QueryResult{
		Entity: "ListsPrefs",
		Counts: map[string]int{"items": len(items), "contacts": contacts, "settings": settings},
		Items:  items,
		Prefs:  prefs,
		Note:   note,
	}
}
