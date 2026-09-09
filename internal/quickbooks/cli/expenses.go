package cli

import "github.com/spf13/cobra"

func newExpensesCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "expenses",
		Short: "Bills, expenses, suppliers",
		Long:  "Bills, expenses, suppliers. qb expenses <entity> <verb>. Stubs return not-wired.",
	}
	bank_feeEnt := &cobra.Command{Use: "bank-fee", Short: "bank-fee"}
	bank_feeEnt.AddCommand(newStubCmd(flags, "expenses bank-fee", "create", "bank-fee create (not wired)"))
	cmd.AddCommand(bank_feeEnt)
	billEnt := &cobra.Command{Use: "bill", Short: "bill"}
	billEnt.AddCommand(newStubCmd(flags, "expenses bill", "create", "bill create (not wired)"))
	billEnt.AddCommand(newStubCmd(flags, "expenses bill", "delete", "bill delete (not wired)"))
	billEnt.AddCommand(newStubCmd(flags, "expenses bill", "import", "bill import (not wired)"))
	billEnt.AddCommand(newStubCmd(flags, "expenses bill", "get", "bill read (not wired)"))
	billEnt.AddCommand(newStubCmd(flags, "expenses bill", "search", "bill search (not wired)"))
	billFormEnt := &cobra.Command{Use: "form", Short: "form"}
	billFormEnt.AddCommand(newStubCmd(flags, "expenses bill form", "delete", "bill-form delete (not wired)"))
	billListEnt := &cobra.Command{Use: "list", Short: "list"}
	billListEnt.AddCommand(newStubCmd(flags, "expenses bill list", "create", "bill-list create (not wired)"))
	billListEnt.AddCommand(newStubCmd(flags, "expenses bill list", "edit", "bill-list edit (not wired)"))
	billUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	billUpdateEnt.AddCommand(newStubCmd(flags, "expenses bill update", "edit", "bill edit (not wired)"))
	billUpdateEnt.AddCommand(newStubCmd(flags, "expenses bill update", "pay", "bill pay (not wired)"))
	billEnt.AddCommand(billListEnt, billUpdateEnt, billFormEnt)
	cmd.AddCommand(billEnt)
	billpaymentEnt := &cobra.Command{Use: "bill-payment", Short: "bill-payment"}
	billpaymentEnt.AddCommand(newStubCmd(flags, "expenses bill-payment", "create", "bill-payment create"))
	billpaymentEnt.AddCommand(newStubCmd(flags, "expenses bill-payment", "update", "bill-payment edit"))
	billpaymentEnt.AddCommand(newStubCmd(flags, "expenses bill-payment", "get", "bill-payment read"))
	billpaymentEnt.AddCommand(newStubCmd(flags, "expenses bill-payment", "delete", "bill-payment delete"))
	cmd.AddCommand(billpaymentEnt)
	chequeEnt := &cobra.Command{Use: "cheque", Short: "cheque"}
	chequeEnt.AddCommand(newStubCmd(flags, "expenses cheque", "create", "cheque create (not wired)"))
	chequeEnt.AddCommand(newStubCmd(flags, "expenses cheque", "delete", "cheque delete (not wired)"))
	chequeEnt.AddCommand(newStubCmd(flags, "expenses cheque", "get", "cheque read (not wired)"))
	chequeUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	chequeUpdateEnt.AddCommand(newStubCmd(flags, "expenses cheque update", "edit", "cheque edit (not wired)"))
	chequeUpdateEnt.AddCommand(newStubCmd(flags, "expenses cheque update", "bounce", "cheque bounce (not wired)"))
	chequeEnt.AddCommand(chequeUpdateEnt)
	chequeEnt.AddCommand(newStubCmd(flags, "expenses cheque", "export reprint", "cheque reprint (not wired)"))
	cmd.AddCommand(chequeEnt)
	ccpEnt := &cobra.Command{Use: "credit-card-payment", Short: "credit-card-payment"}
	ccpEnt.AddCommand(newStubCmd(flags, "expenses credit-card-payment", "get", "credit-card-payment read"))
	ccpEnt.AddCommand(newStubCmd(flags, "expenses credit-card-payment", "search", "credit-card-payment search"))
	cmd.AddCommand(ccpEnt)
	expenseEnt := &cobra.Command{Use: "expense", Short: "expense"}
	expenseEnt.AddCommand(newStubCmd(flags, "expenses expense", "create", "expense create (not wired)"))
	expenseEnt.AddCommand(newStubCmd(flags, "expenses expense", "delete", "expense delete (not wired)"))
	expenseEnt.AddCommand(newStubCmd(flags, "expenses expense", "get", "expense read (not wired)"))
	expenseEnt.AddCommand(newStubCmd(flags, "expenses expense", "search", "expense search (not wired)"))
	expenseFormEnt := &cobra.Command{Use: "form", Short: "form"}
	expenseFormEnt.AddCommand(newStubCmd(flags, "expenses expense form", "delete", "expense-form delete (not wired)"))
	expenseEnt.AddCommand(expenseFormEnt)
	expenseListEnt := &cobra.Command{Use: "list", Short: "list"}
	expenseListEnt.AddCommand(newStubCmd(flags, "expenses expense list", "create", "expense-list create (not wired)"))
	expenseListEnt.AddCommand(newStubCmd(flags, "expenses expense list", "edit", "expense-list edit (not wired)"))
	expenseEnt.AddCommand(expenseListEnt)
	expenseUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	expenseUpdateEnt.AddCommand(newStubCmd(flags, "expenses expense update", "edit", "expense edit (not wired)"))
	expenseUpdateEnt.AddCommand(newStubCmd(flags, "expenses expense update", "recategorise", "expense recategorise (not wired)"))
	expenseUpdateEnt.AddCommand(newStubCmd(flags, "expenses expense update", "split", "expense split (not wired)"))
	expenseEnt.AddCommand(expenseUpdateEnt)
	cmd.AddCommand(expenseEnt)
	gift_certificateEnt := &cobra.Command{Use: "gift-certificate", Short: "gift-certificate"}
	gift_certificateEnt.AddCommand(newStubCmd(flags, "expenses gift-certificate", "create", "gift-certificate create (not wired)"))
	cmd.AddCommand(gift_certificateEnt)
	mileageEnt := &cobra.Command{Use: "mileage", Short: "mileage"}
	mileageEnt.AddCommand(newStubCmd(flags, "expenses mileage", "create", "mileage create (not wired)"))
	mileageEnt.AddCommand(newStubCmd(flags, "expenses mileage", "delete", "mileage delete (not wired)"))
	mileageEnt.AddCommand(newStubCmd(flags, "expenses mileage", "update", "mileage edit (not wired)"))
	mileageEnt.AddCommand(newStubCmd(flags, "expenses mileage", "get", "mileage read (not wired)"))
	mileageEnt.AddCommand(newStubCmd(flags, "expenses mileage", "search", "mileage search (not wired)"))
	cmd.AddCommand(mileageEnt)
	overviewEnt := &cobra.Command{Use: "overview", Short: "overview"}
	overviewEnt.AddCommand(newStubCmd(flags, "expenses overview", "get", "overview read (not wired)"))
	cmd.AddCommand(overviewEnt)
	purchaseEnt := &cobra.Command{Use: "purchase", Short: "purchase"}
	purchaseEnt.AddCommand(newStubCmd(flags, "expenses purchase", "search", "purchase search (not wired)"))
	cmd.AddCommand(purchaseEnt)
	receiptEnt := &cobra.Command{Use: "receipt", Short: "receipt"}
	receiptEnt.AddCommand(newStubCmd(flags, "expenses receipt", "delete", "receipt delete (not wired)"))
	receiptEnt.AddCommand(newStubCmd(flags, "expenses receipt", "get", "receipt read (not wired)"))
	receiptEnt.AddCommand(newStubCmd(flags, "expenses receipt", "search", "receipt search (not wired)"))
	receiptEnt.AddCommand(newStubCmd(flags, "expenses receipt", "import", "receipt upload (not wired)"))
	cmd.AddCommand(receiptEnt)
	receipt_expenseEnt := &cobra.Command{Use: "receipt-expense", Short: "receipt-expense"}
	receipt_expenseEnt.AddCommand(newStubCmd(flags, "expenses receipt-expense", "create", "receipt-expense create (not wired)"))
	cmd.AddCommand(receipt_expenseEnt)
	supplierEnt := &cobra.Command{Use: "supplier", Short: "supplier"}
	supplierEnt.AddCommand(newStubCmd(flags, "expenses supplier", "create", "supplier create (not wired)"))
	supplierEnt.AddCommand(newStubCmd(flags, "expenses supplier", "delete", "supplier delete (not wired)"))
	supplierEnt.AddCommand(newStubCmd(flags, "expenses supplier", "get", "supplier read (not wired)"))
	supplierEnt.AddCommand(newStubCmd(flags, "expenses supplier", "search", "supplier search (not wired)"))
	supplierUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	supplierUpdateEnt.AddCommand(newStubCmd(flags, "expenses supplier update", "edit", "supplier edit (not wired)"))
	supplierUpdateEnt.AddCommand(newStubCmd(flags, "expenses supplier update", "merge", "supplier merge (not wired)"))
	supplierEnt.AddCommand(supplierUpdateEnt)
	cmd.AddCommand(supplierEnt)
	time_activityEnt := &cobra.Command{Use: "time-activity", Short: "time-activity"}
	time_activityEnt.AddCommand(newStubCmd(flags, "expenses time-activity", "create", "time-activity create (not wired)"))
	time_activityEnt.AddCommand(newStubCmd(flags, "expenses time-activity", "delete", "time-activity delete (not wired)"))
	time_activityEnt.AddCommand(newStubCmd(flags, "expenses time-activity", "update", "time-activity edit (not wired)"))
	time_activityEnt.AddCommand(newStubCmd(flags, "expenses time-activity", "get", "time-activity read (not wired)"))
	time_activityEnt.AddCommand(newStubCmd(flags, "expenses time-activity", "search", "time-activity search (not wired)"))
	cmd.AddCommand(time_activityEnt)
	vendor_creditEnt := &cobra.Command{Use: "vendor-credit", Short: "vendor-credit"}
	vendor_creditEnt.AddCommand(newStubCmd(flags, "expenses vendor-credit", "create", "vendor-credit create (not wired)"))
	vendor_creditEnt.AddCommand(newStubCmd(flags, "expenses vendor-credit", "delete", "vendor-credit delete (not wired)"))
	vendor_creditEnt.AddCommand(newStubCmd(flags, "expenses vendor-credit", "update", "vendor-credit edit (not wired)"))
	vendor_creditEnt.AddCommand(newStubCmd(flags, "expenses vendor-credit", "get", "vendor-credit read (not wired)"))
	cmd.AddCommand(vendor_creditEnt)
	for _, svcCmd := range newExpensesServiceMapCmds(flags) {
		cmd.AddCommand(svcCmd)
	}
	// Service map v2 (2026-08-24): never-triggered production services, blocked.
	spendmgmtEnt := &cobra.Command{Use: "spendmgmt", Short: "spendmgmt"}
	spendmgmtEnt.AddCommand(newStubCmd(flags, "expenses spendmgmt", "get", "spendmgmt.api.intuit.com/graphql get (service map; not wired)"))
	cmd.AddCommand(spendmgmtEnt)
	stagetxnEnt := &cobra.Command{Use: "stagetxn", Short: "stagetxn"}
	stagetxnEnt.AddCommand(newStubCmd(flags, "expenses stagetxn", "get", "stagetransactions.api.intuit.com/stage/entities get (service map; not wired)"))
	cmd.AddCommand(stagetxnEnt)
	stagetxn_testEnt := &cobra.Command{Use: "stagetxn-test", Short: "stagetxn-test"}
	stagetxn_testEnt.AddCommand(newStubCmd(flags, "expenses stagetxn-test", "get", "stagetransactions-test.api.intuit.com/stage/entities get (service map; not wired)"))
	cmd.AddCommand(stagetxn_testEnt)
	txn_ingestionEnt := &cobra.Command{Use: "txn-ingestion", Short: "txn-ingestion"}
	txn_ingestionEnt.AddCommand(newStubCmd(flags, "expenses txn-ingestion", "post", "txn-ingestion-svc.api.intuit.com/v1 post (service map; not wired)"))
	cmd.AddCommand(txn_ingestionEnt)
	payment_accountEnt := &cobra.Command{Use: "payment-account", Short: "payment-account"}
	payment_accountEnt.AddCommand(newStubCmd(flags, "expenses payment-account", "get", "paymentaccount.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(payment_accountEnt)
	payments_fundingEnt := &cobra.Command{Use: "payments-funding", Short: "payments-funding"}
	payments_fundingEnt.AddCommand(newStubCmd(flags, "expenses payments-funding", "get", "paymentsfunding-aws.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(payments_fundingEnt)
	pymts_apps_orchEnt := &cobra.Command{Use: "pymts-apps-orch", Short: "pymts-apps-orch"}
	pymts_apps_orchEnt.AddCommand(newStubCmd(flags, "expenses pymts-apps-orch", "post", "pymtsappsorchestration.api.intuit.com post (service map; not wired)"))
	cmd.AddCommand(pymts_apps_orchEnt)
	resolution_centerEnt := &cobra.Command{Use: "resolution-center", Short: "resolution-center"}
	resolution_centerEnt.AddCommand(newStubCmd(flags, "expenses resolution-center", "get", "resolution-center-svc.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(resolution_centerEnt)
	walletEnt := &cobra.Command{Use: "wallet", Short: "wallet"}
	walletEnt.AddCommand(newStubCmd(flags, "expenses wallet", "get", "wallet-aws.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(walletEnt)
	subscription_sbgEnt := &cobra.Command{Use: "subscription-sbg", Short: "subscription-sbg"}
	subscription_sbgEnt.AddCommand(newStubCmd(flags, "expenses subscription-sbg", "get", "subscription-sbg-aws.api.intuit.com/v4/graphql get (service map; not wired)"))
	cmd.AddCommand(subscription_sbgEnt)
	return cmd
}

// Service map (2026-08-24): never-triggered production services documented as
// blocked stubs. See .audit/qb/2026-08-24/SERVICE_MAP.md.
func newExpensesServiceMapCmds(flags *rootFlags) []*cobra.Command {
	bill_payEnt := &cobra.Command{Use: "bill-pay-service", Short: "bill-pay-service"}
	bill_payEnt.AddCommand(newStubCmd(flags, "expenses bill-pay-service", "create", "online bill pay (service map; not wired)"))
	b2b_billpayEnt := &cobra.Command{Use: "b2b-billpay", Short: "b2b-billpay"}
	b2b_billpayEnt.AddCommand(newStubCmd(flags, "expenses b2b-billpay", "get", "B2B bill pay read (service map; not wired)"))
	b2b_networkEnt := &cobra.Command{Use: "b2b-network", Short: "b2b-network"}
	b2b_networkEnt.AddCommand(newStubCmd(flags, "expenses b2b-network", "get", "B2B network read (service map; not wired)"))
	bill_pay_onboardingEnt := &cobra.Command{Use: "bill-pay-onboarding", Short: "bill-pay-onboarding"}
	bill_pay_onboardingEnt.AddCommand(newStubCmd(flags, "expenses bill-pay-onboarding", "get", "bill-pay onboarding read (service map; not wired)"))
	return []*cobra.Command{bill_payEnt, b2b_billpayEnt, b2b_networkEnt, bill_pay_onboardingEnt}
}
