package gql

import "strings"

// billsDocument is the complete live QBO bill-list selection captured on
// 2026-09-12. Status is unrestricted so paid and unpaid bills share the walk.
const billsDocument = `query QBCLIBills($filterBy: String, $limit: Int!, $offset: Int!) {
  company {
    transactions(filterBy: $filterBy,with:"status='ALL' && nameId='' && showAttachableCount=true && showCategoryFullName=true && calculateTotals=true ", orderBy: "traits.balance.dueDate asc", limit: $limit, offset: $offset) {
      pageInfo{
          hasNextPage
          hasPreviousPage
        }
         aggregates{
        other(names:["numRows"]){
          name
          value
        }
      }
      edges {
        node {
          ...txnListFragment
        }
      }
    }
  }
}

   fragment txnListFragment on Transactions_Transaction {
    id
    type
    header {
      txnStatus
      allocationStatus
      is_ict_txn
      contact {
        businessDirectoryRefId
        displayName
        contactMethods {
            emails {
                emailAddress
            }
        }
        id
        externalIds {
          localId
          namespaceId
        }
      }
      currencyInfo {
        symbol
        homeAmount
        code
        exchangeRate
        name
        currency
      }
      referenceNumber
      amount
      txnDate
      privateMemo
      department {
        name
      }
    }
    traits {
      approval {
        status
      }
      balance {
        dueDate
        amountPaid
        balance
      }
      
    }
    qboAppData {
      splitTxn
      txnListProps {
        klass
        txnTypeId
        category
        attachmentCount
      }
    }
    
    meta{
     createdByApp {
        name
      }
    }
}
`

// BillsRequest constructs the current UI's real transaction query.
func BillsRequest(window *DateWindow) Request {
	conditions := []string{"header.transactionType in ('PURCHASE_BILL')"}
	if window != nil {
		if !window.From.IsZero() {
			conditions = append(conditions, "header.txnDate >= '"+window.From.Format("2006-01-02")+"'")
		}
		if !window.To.IsZero() {
			conditions = append(conditions, "header.txnDate <= '"+window.To.Format("2006-01-02")+"'")
		}
	}
	return Request{Op: &Op{Name: "QBCLIBills", Kind: KindQuery, Endpoint: EndpointDefault, Document: billsDocument, VarTypes: []string{"String", "Int!", "Int!"}}, Variables: map[string]any{"filterBy": strings.Join(conditions, " && "), "limit": 100, "offset": 0}}
}
