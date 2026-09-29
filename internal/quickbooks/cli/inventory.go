package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

// inventory_adj.go entity/path constants re-declared here as CLI-local
// aliases so this file stays the single wiring point for the four SPA
// routes (see client/inventory_adj.go for endpoint evidence).
const (
	inventoryCountEntity         = client.InventoryCountEntity
	inventoryBuildAssemblyEntity = client.BuildAssemblyEntity
	inventoryStartingValueEntity = client.StartingValueEntity
	inventoryCountPath           = client.InventoryCountPath
	inventoryBuildAssemblyPath   = client.BuildAssemblyPath
	inventoryStartingValuePath   = client.InventoryStartingValuePath
)

// newInventoryV3MutateCmd is the inventory-domain mutate runner. It mirrors
// newV3MutateCmd's dry-run/live contract but posts through a fixed v3 path
// via client.ReplayInventory*Create / ReplayMutate(delete), because these
// entities are not in the v3MutateByID catalog map.
func newInventoryV3MutateCmd(flags *rootFlags, e primitiveEntry, command, entity, path string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (v3 create)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				url := client.PlannedInventoryV3URL(path)
				note := "v3 POST; not sent"
				if strings.HasSuffix(command, "delete") {
					url += "&operation=delete"
					note = "v3 POST delete; not sent"
				}
				return writePlan(cmd, flags, planEnvelope{
					Command: command,
					ID:      e.ID,
					Mode:    modeWired,
					Method:  "POST",
					URL:     url,
					Flags:   fm,
					Note:    note,
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			var res *client.MutateResult
			var err error
			switch {
			case strings.HasSuffix(command, "delete"):
				res, err = client.ReplayMutate(ctx, entity, "delete", fm["id"], nil)
			case e.ID == "QBO.INVENTORY.COUNT_CREATE":
				res, err = client.ReplayInventoryCountCreate(ctx, fm)
			case e.ID == "QBO.INVENTORY.BUILD_ASSEMBLY_CREATE":
				res, err = client.ReplayBuildAssemblyCreate(ctx, fm)
			default:
				res, err = client.ReplayStartingValueCreate(ctx, fm)
			}
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.ID)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "id", "target QBO id")
	// Each inline mutation binds exactly its dedicatedConsumed contract
	// entry — count/build-assembly/starting-value/adjust-delete read
	// different subsets, so the contract drives attachment per command.
	attachDedicatedConsumed(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newItemReceiptMutateCmd runs item-receipt create/update against the
// warehouse-management-svc GraphQL host via client.ReplayItemReceipt*;
// the create/update contract was captured from /app/itemreceipt on
// Test Company 2 (2026-09-16).
func newItemReceiptMutateCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	use := verbOf(command)
	cmd := &cobra.Command{
		Use:   use,
		Short: command + " (warehouse-management-svc)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command,
					ID:      e.ID,
					Mode:    modeWired,
					Method:  "POST",
					URL:     client.PlannedItemReceiptURL(),
					Flags:   fm,
					Note:    "warehouse-management-svc " + map[string]string{"create": "CreateItemReceipt", "update": "UpdateItemReceipt"}[use] + " POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			var res *client.MutateResult
			var err error
			if use == "create" {
				res, err = client.ReplayItemReceiptCreate(ctx, fm)
			} else {
				res, err = client.ReplayItemReceiptUpdate(ctx, fm)
			}
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.ID)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	if use == "create" {
		ensureFlag(cmd, "vendor-id", "supplier id the goods were received from (create)")
	}
	attachDedicatedConsumed(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

// newPOPartialCmd runs `inventory purchase-order update partial`: it loads the
// PO via v3, resolves the --items subset against its lines, and posts the
// proven CommerceReceiveInventory mutation with the partial quantities.
func newPOPartialCmd(flags *rootFlags, e primitiveEntry, command string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "partial",
		Short: command + " (warehouse-management-svc CommerceReceiveInventory)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := collectFlags(cmd)
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command,
					ID:      e.ID,
					Mode:    modeWired,
					Method:  "POST",
					URL:     client.PlannedItemReceiptURL(),
					Flags:   fm,
					Note:    "warehouse-management-svc CommerceReceiveInventory POST; not sent",
				})
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayPurchaseOrderPartial(ctx, fm)
			if err != nil {
				return feedErr(flags, err)
			}
			if flags.asJSON {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s %s\n", res.Op, res.Entity, res.Item.ID)
			return nil
		},
	}
	attachParamFlags(cmd, e.ID)
	ensureFlag(cmd, "id", "purchase order id to receive against")
	attachDedicatedConsumed(cmd, e.ID)
	applyCatalogHelp(cmd, e.ID)
	return cmd
}

func newInventoryCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Items and purchase orders",
		Long:  "Items and purchase orders. qb inventory <entity> <verb>. Stubs return not-wired.",
	}
	adjustEnt := &cobra.Command{Use: "adjust", Short: "adjust"}
	adjustEnt.AddCommand(newStubCmd(flags, "inventory adjust", "create", "adjust create (not wired)"))
	// Read/search are not catalog ids (YAML has ADJUST_CREATE only). Same
	// v3 entity as create: InventoryAdjustment. Query only — no POST.
	adjustGet := primitiveEntry{ID: "QBO.INVENTORY.ADJUST_READ", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory adjust get"}
	adjustSearch := primitiveEntry{ID: "QBO.INVENTORY.ADJUST_SEARCH", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory adjust search"}
	adjustEnt.AddCommand(newV3QueryCmd(flags, adjustGet, adjustGet.Command, "InventoryAdjustment"))
	adjustEnt.AddCommand(newV3QueryCmd(flags, adjustSearch, adjustSearch.Command, "InventoryAdjustment"))
	adjustDelete := primitiveEntry{ID: "QBO.INVENTORY.ADJUST_DELETE", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory adjust delete"}
	adjustEnt.AddCommand(newInventoryV3MutateCmd(flags, adjustDelete, adjustDelete.Command, "InventoryAdjustment", "inventoryadjustment"))
	cmd.AddCommand(adjustEnt)
	countEnt := &cobra.Command{Use: "count", Short: "count"}
	countCreate := primitiveEntry{ID: "QBO.INVENTORY.COUNT_CREATE", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory count create"}
	countGet := primitiveEntry{ID: "QBO.INVENTORY.COUNT_READ", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory count get"}
	countList := primitiveEntry{ID: "QBO.INVENTORY.COUNT_SEARCH", Domain: "inventory", Risk: "R0", Mode: modeRead, Command: "inventory count list"}
	// create posts /api/v3/company/{realm}/inventory-counts (route-derived
	// path; see client/inventory_adj.go). get/list are v3 queries on the
	// same entity. Not catalog ids — same inline pattern as adjust get/search.
	countEnt.AddCommand(newInventoryV3MutateCmd(flags, countCreate, countCreate.Command, inventoryCountEntity, inventoryCountPath))
	countEnt.AddCommand(newV3QueryCmd(flags, countGet, countGet.Command, inventoryCountEntity))
	countEnt.AddCommand(newV3QueryCmd(flags, countList, countList.Command, inventoryCountEntity))
	cmd.AddCommand(countEnt)
	buildAssemblyEnt := &cobra.Command{Use: "buildassembly", Short: "buildassembly"}
	buildAssemblyCreate := primitiveEntry{ID: "QBO.INVENTORY.BUILD_ASSEMBLY_CREATE", Domain: "inventory", Risk: "R3", Mode: modeWired, Command: "inventory buildassembly create"}
	buildAssemblyEnt.AddCommand(newInventoryV3MutateCmd(flags, buildAssemblyCreate, buildAssemblyCreate.Command, inventoryBuildAssemblyEntity, inventoryBuildAssemblyPath))
	cmd.AddCommand(buildAssemblyEnt)
	startingValueEnt := &cobra.Command{Use: "startingvalue", Short: "startingvalue"}
	startingValueCreate := primitiveEntry{ID: "QBO.INVENTORY.STARTING_VALUE_CREATE", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory startingvalue create"}
	startingValueEnt.AddCommand(newInventoryV3MutateCmd(flags, startingValueCreate, startingValueCreate.Command, inventoryStartingValueEntity, inventoryStartingValuePath))
	cmd.AddCommand(startingValueEnt)
	consignmentEnt := &cobra.Command{Use: "consignment", Short: "consignment"}
	consignmentEnt.AddCommand(newStubCmd(flags, "inventory consignment", "create", "consignment create (not wired)"))
	cmd.AddCommand(consignmentEnt)
	itemEnt := &cobra.Command{Use: "item", Short: "item"}
	itemEnt.AddCommand(newStubCmd(flags, "inventory item", "create", "item create (not wired)"))
	itemEnt.AddCommand(newStubCmd(flags, "inventory item", "delete", "item delete (not wired)"))
	itemEnt.AddCommand(newStubCmd(flags, "inventory item", "update", "item edit (not wired)"))
	itemEnt.AddCommand(newStubCmd(flags, "inventory item", "get", "item read (not wired)"))
	itemEnt.AddCommand(newStubCmd(flags, "inventory item", "search", "item search (not wired)"))
	cmd.AddCommand(itemEnt)
	item_receiptEnt := &cobra.Command{Use: "item-receipt", Short: "item-receipt"}
	irCreate := primitiveEntry{ID: "QBO.INVENTORY.ITEM_RECEIPT_CREATE", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory item-receipt create"}
	irUpdate := primitiveEntry{ID: "QBO.INVENTORY.ITEM_RECEIPT_EDIT", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory item-receipt update"}
	item_receiptEnt.AddCommand(newItemReceiptMutateCmd(flags, irCreate, irCreate.Command))
	item_receiptEnt.AddCommand(newItemReceiptMutateCmd(flags, irUpdate, irUpdate.Command))
	item_receiptEnt.AddCommand(newStubCmd(flags, "inventory item-receipt", "get", "item-receipt read (not wired)"))
	item_receiptEnt.AddCommand(newStubCmd(flags, "inventory item-receipt", "search", "item-receipt search (not wired)"))
	cmd.AddCommand(item_receiptEnt)
	overviewEnt := &cobra.Command{Use: "overview", Short: "overview"}
	overviewEnt.AddCommand(newStubCmd(flags, "inventory overview", "get", "overview read (not wired)"))
	overviewEnt.AddCommand(newStubCmd(flags, "inventory overview", "search", "overview search (not wired)"))
	cmd.AddCommand(overviewEnt)
	purchase_orderEnt := &cobra.Command{Use: "purchase-order", Short: "purchase-order"}
	purchase_orderEnt.AddCommand(newStubCmd(flags, "inventory purchase-order", "create", "purchase-order create (not wired)"))
	purchase_orderEnt.AddCommand(newStubCmd(flags, "inventory purchase-order", "delete", "purchase-order delete (not wired)"))
	poUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	poUpdateEnt.AddCommand(newStubCmd(flags, "inventory purchase-order update", "edit", "purchase-order edit (not wired)"))
	poPartial := primitiveEntry{ID: "QBO.INVENTORY.PURCHASE_ORDER_PARTIAL", Domain: "inventory", Risk: "R2", Mode: modeWired, Command: "inventory purchase-order update partial"}
	poUpdateEnt.AddCommand(newPOPartialCmd(flags, poPartial, poPartial.Command))
	purchase_orderEnt.AddCommand(poUpdateEnt)
	purchase_orderEnt.AddCommand(newStubCmd(flags, "inventory purchase-order", "get", "purchase-order read (not wired)"))
	purchase_orderEnt.AddCommand(newStubCmd(flags, "inventory purchase-order", "search", "purchase-order search (not wired)"))
	cmd.AddCommand(purchase_orderEnt)
	// Service map v2 (2026-08-24): never-triggered production services, blocked.
	bulk_actionEnt := &cobra.Command{Use: "bulk-action", Short: "bulk-action"}
	bulk_actionEnt.AddCommand(newStubCmd(flags, "inventory bulk-action", "post", "qbbulkaction.api.intuit.com/v4/graphql post (service map; not wired)"))
	cmd.AddCommand(bulk_actionEnt)
	item_svcEnt := &cobra.Command{Use: "item-svc", Short: "item-svc"}
	item_svcEnt.AddCommand(newStubCmd(flags, "inventory item-svc", "get", "item-management-svc.api.intuit.com/graphql get (service map; not wired)"))
	cmd.AddCommand(item_svcEnt)
	agentsEnt := &cobra.Command{Use: "agents", Short: "agents"}
	agentsEnt.AddCommand(newStubCmd(flags, "inventory agents", "post", "inventory-agents.api.intuit.com post (service map; not wired)"))
	cmd.AddCommand(agentsEnt)
	cost_accountingEnt := &cobra.Command{Use: "cost-accounting", Short: "cost-accounting"}
	cost_accountingEnt.AddCommand(newStubCmd(flags, "inventory cost-accounting", "get", "inventorycostaccounting.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(cost_accountingEnt)
	warehouse_serviceEnt := &cobra.Command{Use: "warehouse-service", Short: "warehouse-service"}
	warehouse_serviceEnt.AddCommand(newStubCmd(flags, "inventory warehouse-service", "get", "warehouse-management-svc.api.intuit.com/graphql get (service map; not wired)"))
	cmd.AddCommand(warehouse_serviceEnt)
	return cmd
}
