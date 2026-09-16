package gql

// itemsDocument is the version-2 product/service selection used by the live UI.
const itemsDocument = `
  query QBCLIItems(
    $filter: ProductAndServiceListEntityFilterCriteria = {
      status: { value: [ACTIVE], comparator: IN }
      entityType: { value: [BUNDLE, PRODUCT], comparator: IN }
    }
    $sort: [ProductAndServicesListEntitySortCriteria!] = [
      { field: NAME, order: ASC }
    ]
    $offset: Int = 0
    $limit: Int = 50
  ) {
    result: getAllListViewEntities(
      filter: $filter
      sort: $sort
      offset: $offset
      limit: $limit
    ) {
      entities {
        ...ProductAndServicesListEntity
        variants {
          id
          qboItemId
          status
          entityName
          version
        }
      }
      totalCount
      pageInfo {
        nextCursor
        previousCursor
      }
    }
  }
  
  fragment ProductAndServicesListEntity on ProductAndServicesListEntityWithVariants {
    productsAndServicesListEntity {
      assetAccountId
      costGroupId
      categoryId
      categoryFullyQualifiedName
      cogsAccountId
      createdAt
      modifiedAt
      cost
      costMax
      costMin
      deferredRevenue
      entityFullyQualifiedName
      entityName
      entityType
      id
      inventoryItemId
      lowOnStock
      lowOnStockCount
      maxReorderPoint
      outOfStock
      outOfStockCount
      parentId
      parentName
      preferredVendorAccountId
      price
      priceMax
      priceMin
      productPurchaseDescription
      productSalesDescription
      productSourceType
      productType
      purchaseAccountId
      qtyAvailable
      qtyAvailableCount
      qtyCommitted
      qtyCommittedCount
      qtyCommittedOnMO
      qtyCommittedOnSO
      qtyCommittedOnTransfer
      qtyIncoming
      qtyIncomingCount
      qtyIncomingPO
      qtyOnWo
      qtyOnHand
      qtyOnHandCount
      qtyFromWo
      reorderPoint
      salesAccountId
      sellable
      sku
      status
      taxable
      thumbnailUrl
      trackStock
      variantCount
      purchaseRateIncludesTax
      salesRateIncludesTax
      salesTaxCodeId
      purchaseTaxCodeId
      variantPurchaseDescription
      variantSalesDescription
      variabilityDimensions {
        dimension
        value
      }
      flat
      default
      version
      productAccountReferences {
        purchaseDescription
        salesDescription
      }
      isAssembled
      baseUnitAbbreviation
      customExtensions {
        dimensions {
          definition {
            id
          }
          value
        }
      }
      upc
      barcode
    }
  }

`

// ItemsRequest includes product variants and native pagination metadata.
func ItemsRequest() Request {
	return Request{Op: &Op{Name: "QBCLIItems", Kind: KindQuery, Endpoint: "https://commercecontrol.api.intuit.com/graphql", Document: itemsDocument}, Variables: map[string]any{"offset": 0, "limit": 100}}
}
