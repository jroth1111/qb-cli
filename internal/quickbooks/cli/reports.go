package cli

import "github.com/spf13/cobra"

func newReportsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reports",
		Short: "Reports and forecasts",
		Long:  "Reports and forecasts. qb reports <entity> <verb>. Stubs return not-wired.",
	}
	account_listEnt := &cobra.Command{Use: "account-list", Short: "account-list"}
	account_listEnt.AddCommand(newStubCmd(flags, "reports account-list", "get", "account-list read"))
	cmd.AddCommand(account_listEnt)
	aged_payablesEnt := &cobra.Command{Use: "aged-payables", Short: "aged-payables"}
	aged_payablesEnt.AddCommand(newStubCmd(flags, "reports aged-payables", "get", "aged-payables read"))
	cmd.AddCommand(aged_payablesEnt)
	aged_payable_detailEnt := &cobra.Command{Use: "aged-payable-detail", Short: "aged-payable-detail"}
	aged_payable_detailEnt.AddCommand(newStubCmd(flags, "reports aged-payable-detail", "get", "aged-payable-detail read"))
	cmd.AddCommand(aged_payable_detailEnt)
	aged_receivablesEnt := &cobra.Command{Use: "aged-receivables", Short: "aged-receivables"}
	aged_receivablesEnt.AddCommand(newStubCmd(flags, "reports aged-receivables", "get", "aged-receivables read"))
	cmd.AddCommand(aged_receivablesEnt)
	aged_receivable_detailEnt := &cobra.Command{Use: "aged-receivable-detail", Short: "aged-receivable-detail"}
	aged_receivable_detailEnt.AddCommand(newStubCmd(flags, "reports aged-receivable-detail", "get", "aged-receivable-detail read"))
	cmd.AddCommand(aged_receivable_detailEnt)
	cash_flowEnt := &cobra.Command{Use: "cash-flow", Short: "cash-flow"}
	cash_flowEnt.AddCommand(newStubCmd(flags, "reports cash-flow", "get", "cash-flow read (not wired)"))
	cmd.AddCommand(cash_flowEnt)
	class_salesEnt := &cobra.Command{Use: "class-sales", Short: "class-sales"}
	class_salesEnt.AddCommand(newStubCmd(flags, "reports class-sales", "get", "class-sales read"))
	cmd.AddCommand(class_salesEnt)
	customEnt := &cobra.Command{Use: "custom", Short: "custom"}
	customEnt.AddCommand(newStubCmd(flags, "reports custom", "create", "custom create (not wired)"))
	customEnt.AddCommand(newStubCmd(flags, "reports custom", "get", "custom read (not wired)"))
	cmd.AddCommand(customEnt)
	department_salesEnt := &cobra.Command{Use: "department-sales", Short: "department-sales"}
	department_salesEnt.AddCommand(newStubCmd(flags, "reports department-sales", "get", "department-sales read"))
	cmd.AddCommand(department_salesEnt)
	forecastEnt := &cobra.Command{Use: "forecast", Short: "forecast"}
	forecastEnt.AddCommand(newStubCmd(flags, "reports forecast", "create", "forecast create (not wired)"))
	forecastEnt.AddCommand(newStubCmd(flags, "reports forecast", "update", "forecast edit (not wired)"))
	forecastEnt.AddCommand(newStubCmd(flags, "reports forecast", "get", "forecast read (not wired)"))
	cmd.AddCommand(forecastEnt)
	general_ledgerEnt := &cobra.Command{Use: "general-ledger", Short: "general-ledger"}
	general_ledgerEnt.AddCommand(newStubCmd(flags, "reports general-ledger", "get", "general-ledger read"))
	cmd.AddCommand(general_ledgerEnt)
	managementEnt := &cobra.Command{Use: "management", Short: "management"}
	managementEnt.AddCommand(newStubCmd(flags, "reports management", "create", "management create (not wired)"))
	managementEnt.AddCommand(newStubCmd(flags, "reports management", "update", "management edit (not wired)"))
	managementEnt.AddCommand(newStubCmd(flags, "reports management", "get", "management read (not wired)"))
	cmd.AddCommand(managementEnt)
	performanceEnt := &cobra.Command{Use: "performance", Short: "performance"}
	performanceEnt.AddCommand(newStubCmd(flags, "reports performance", "create", "performance create (not wired)"))
	performanceEnt.AddCommand(newStubCmd(flags, "reports performance", "delete", "performance delete (not wired)"))
	performanceEnt.AddCommand(newStubCmd(flags, "reports performance", "update", "performance edit (not wired)"))
	performanceEnt.AddCommand(newStubCmd(flags, "reports performance", "get", "performance read (not wired)"))
	cmd.AddCommand(performanceEnt)
	reportEnt := &cobra.Command{Use: "report", Short: "report"}
	reportEnt.AddCommand(newStubCmd(flags, "reports report", "create", "report create (not wired)"))
	reportEnt.AddCommand(newStubCmd(flags, "reports report", "get", "report read (not wired)"))
	cmd.AddCommand(reportEnt)
	balance_sheetEnt := &cobra.Command{Use: "balance-sheet", Short: "balance-sheet"}
	balance_sheetEnt.AddCommand(newStubCmd(flags, "reports balance-sheet", "get", "balance-sheet read"))
	cmd.AddCommand(balance_sheetEnt)
	customer_balanceEnt := &cobra.Command{Use: "customer-balance", Short: "customer-balance"}
	customer_balanceEnt.AddCommand(newStubCmd(flags, "reports customer-balance", "get", "customer-balance read"))
	cmd.AddCommand(customer_balanceEnt)
	customer_balance_detailEnt := &cobra.Command{Use: "customer-balance-detail", Short: "customer-balance-detail"}
	customer_balance_detailEnt.AddCommand(newStubCmd(flags, "reports customer-balance-detail", "get", "customer-balance-detail read"))
	cmd.AddCommand(customer_balance_detailEnt)
	customer_incomeEnt := &cobra.Command{Use: "customer-income", Short: "customer-income"}
	customer_incomeEnt.AddCommand(newStubCmd(flags, "reports customer-income", "get", "customer-income read"))
	cmd.AddCommand(customer_incomeEnt)
	customer_salesEnt := &cobra.Command{Use: "customer-sales", Short: "customer-sales"}
	customer_salesEnt.AddCommand(newStubCmd(flags, "reports customer-sales", "get", "customer-sales read"))
	cmd.AddCommand(customer_salesEnt)
	profit_lossEnt := &cobra.Command{Use: "profit-loss", Short: "profit-loss"}
	profit_lossEnt.AddCommand(newStubCmd(flags, "reports profit-loss", "get", "profit-loss read"))
	cmd.AddCommand(profit_lossEnt)
	vendor_balanceEnt := &cobra.Command{Use: "vendor-balance", Short: "vendor-balance"}
	vendor_balanceEnt.AddCommand(newStubCmd(flags, "reports vendor-balance", "get", "vendor-balance read"))
	cmd.AddCommand(vendor_balanceEnt)
	vendor_balance_detailEnt := &cobra.Command{Use: "vendor-balance-detail", Short: "vendor-balance-detail"}
	vendor_balance_detailEnt.AddCommand(newStubCmd(flags, "reports vendor-balance-detail", "get", "vendor-balance-detail read"))
	cmd.AddCommand(vendor_balance_detailEnt)
	vendor_expensesEnt := &cobra.Command{Use: "vendor-expenses", Short: "vendor-expenses"}
	vendor_expensesEnt.AddCommand(newStubCmd(flags, "reports vendor-expenses", "get", "vendor-expenses read"))
	cmd.AddCommand(vendor_expensesEnt)
	inventory_valuationEnt := &cobra.Command{Use: "inventory-valuation", Short: "inventory-valuation"}
	inventory_valuationEnt.AddCommand(newStubCmd(flags, "reports inventory-valuation", "get", "inventory-valuation read"))
	cmd.AddCommand(inventory_valuationEnt)
	inventory_valuation_detailEnt := &cobra.Command{Use: "inventory-valuation-detail", Short: "inventory-valuation-detail"}
	inventory_valuation_detailEnt.AddCommand(newStubCmd(flags, "reports inventory-valuation-detail", "get", "inventory-valuation-detail read"))
	cmd.AddCommand(inventory_valuation_detailEnt)
	item_salesEnt := &cobra.Command{Use: "item-sales", Short: "item-sales"}
	item_salesEnt.AddCommand(newStubCmd(flags, "reports item-sales", "get", "item-sales read"))
	cmd.AddCommand(item_salesEnt)
	journal_reportEnt := &cobra.Command{Use: "journal-report", Short: "journal-report"}
	journal_reportEnt.AddCommand(newStubCmd(flags, "reports journal-report", "get", "journal-report read"))
	cmd.AddCommand(journal_reportEnt)
	profit_loss_detailEnt := &cobra.Command{Use: "profit-loss-detail", Short: "profit-loss-detail"}
	profit_loss_detailEnt.AddCommand(newStubCmd(flags, "reports profit-loss-detail", "get", "profit-loss-detail read"))
	cmd.AddCommand(profit_loss_detailEnt)
	transaction_listEnt := &cobra.Command{Use: "transaction-list", Short: "transaction-list"}
	transaction_listEnt.AddCommand(newStubCmd(flags, "reports transaction-list", "get", "transaction-list read"))
	cmd.AddCommand(transaction_listEnt)
	trial_balanceEnt := &cobra.Command{Use: "trial-balance", Short: "trial-balance"}
	trial_balanceEnt.AddCommand(newStubCmd(flags, "reports trial-balance", "get", "trial-balance read"))
	cmd.AddCommand(trial_balanceEnt)
	return cmd
}
