package cli

import (
	"context"

	"github.com/mvanhorn/cli-printing-press/v4/internal/quickbooks/client"
	"github.com/spf13/cobra"
)

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
	billEnt.AddCommand(newStubCmd(flags, "expenses bill", "import", "bill import"))
	billEnt.AddCommand(newStubCmd(flags, "expenses bill", "get", "bill read (not wired)"))
	billEnt.AddCommand(newStubCmd(flags, "expenses bill", "search", "bill search (not wired)"))
	billFormEnt := &cobra.Command{Use: "form", Short: "form"}
	billFormEnt.AddCommand(newFormDeleteCmd(flags, "expenses bill form delete", "bill", "PURCHASE_BILL", "QBO.EXPENSES.BILL_FORM_DELETE"))
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
	chequeEnt.AddCommand(newStubCmd(flags, "expenses cheque", "export", "cheque reprint"))
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
	expenseFormEnt.AddCommand(newFormDeleteCmd(flags, "expenses expense form delete", "expense", "PURCHASE", "QBO.EXPENSES.EXPENSE_FORM_DELETE"))
	expenseEnt.AddCommand(expenseFormEnt)
	expenseListEnt := &cobra.Command{Use: "list", Short: "list"}
	expenseListEnt.AddCommand(newStubCmd(flags, "expenses expense list", "create", "expense-list create (not wired)"))
	expenseListEnt.AddCommand(newStubCmd(flags, "expenses expense list", "edit", "expense-list edit (not wired)"))
	expenseEnt.AddCommand(expenseListEnt)
	expenseUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	expenseUpdateEnt.AddCommand(newStubCmd(flags, "expenses expense update", "edit", "expense edit (not wired)"))
	expenseUpdateEnt.AddCommand(newExpenseRecategoriseCmd(flags))
	expenseUpdateEnt.AddCommand(newStubCmd(flags, "expenses expense update", "split", "expense split"))
	expenseEnt.AddCommand(expenseUpdateEnt)
	cmd.AddCommand(expenseEnt)
	gift_certificateEnt := &cobra.Command{Use: "gift-certificate", Short: "gift-certificate"}
	gift_certificateEnt.AddCommand(newStubCmd(flags, "expenses gift-certificate", "create", "gift-certificate create (not wired)"))
	cmd.AddCommand(gift_certificateEnt)
	mileageEnt := &cobra.Command{Use: "mileage", Short: "mileage"}
	mileageEnt.AddCommand(newStubCmd(flags, "expenses mileage", "create", "mileage create (trips updateTrips_Trip)"))
	mileageEnt.AddCommand(newStubCmd(flags, "expenses mileage", "delete", "mileage delete (trips updateTrips_Trip)"))
	mileageEnt.AddCommand(newStubCmd(flags, "expenses mileage", "update", "mileage edit (trips updateTrips_Trip)"))
	mileageEnt.AddCommand(newStubCmd(flags, "expenses mileage", "get", "mileage read"))
	mileageEnt.AddCommand(newStubCmd(flags, "expenses mileage", "search", "mileage search"))
	cmd.AddCommand(mileageEnt)
	overviewEnt := &cobra.Command{Use: "overview", Short: "overview"}
	overviewEnt.AddCommand(newStubCmd(flags, "expenses overview", "get", "dashboardframework EXPENSES_DASHBOARD read"))
	cmd.AddCommand(overviewEnt)
	purchaseEnt := &cobra.Command{Use: "purchase", Short: "purchase"}
	purchaseEnt.AddCommand(newStubCmd(flags, "expenses purchase", "search", "purchase search (not wired)"))
	cmd.AddCommand(purchaseEnt)
	receiptEnt := &cobra.Command{Use: "receipt", Short: "receipt"}
	receiptEnt.AddCommand(newStubCmd(flags, "expenses receipt", "delete", "receipt delete (not wired)"))
	receiptEnt.AddCommand(newStubCmd(flags, "expenses receipt", "get", "receipt read (not wired)"))
	receiptEnt.AddCommand(newStubCmd(flags, "expenses receipt", "search", "receipt search (not wired)"))
	receiptEnt.AddCommand(newStubCmd(flags, "expenses receipt", "import", "receipt upload"))
	cmd.AddCommand(receiptEnt)
	receipt_expenseEnt := &cobra.Command{Use: "receipt-expense", Short: "receipt-expense"}
	receipt_expenseEnt.AddCommand(newStubCmd(flags, "expenses receipt-expense", "create", "receipt-expense create"))
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
	stagetxnEnt.AddCommand(newStubCmd(flags, "expenses stagetxn", "get", "stagetransactions /stage/entities read"))
	cmd.AddCommand(stagetxnEnt)
	stagetxn_testEnt := &cobra.Command{Use: "stagetxn-test", Short: "stagetxn-test"}
	stagetxn_testEnt.AddCommand(newStubCmd(flags, "expenses stagetxn-test", "get", "stagetransactions-test.api.intuit.com/stage/entities get (service map; not wired)"))
	cmd.AddCommand(stagetxn_testEnt)
	txn_ingestionEnt := &cobra.Command{Use: "txn-ingestion", Short: "txn-ingestion"}
	txn_ingestionEnt.AddCommand(newStubCmd(flags, "expenses txn-ingestion", "post", "txn-ingestion-svc.api.intuit.com/v1 post (service map; not wired)"))
	cmd.AddCommand(txn_ingestionEnt)
	payment_accountEnt := &cobra.Command{Use: "payment-account", Short: "payment-account"}
	payment_accountEnt.AddCommand(newStubCmd(flags, "expenses payment-account", "get", "paymentaccount-sbg /v2/accounts"))
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
	b2b_billpayEnt.AddCommand(newStubCmd(flags, "expenses b2b-billpay", "get", "b2bbillpay paymentApproval read"))
	b2b_networkEnt := &cobra.Command{Use: "b2b-network", Short: "b2b-network"}
	b2b_networkEnt.AddCommand(newStubCmd(flags, "expenses b2b-network", "get", "b2bnetworkservice directory/attributes read"))
	bill_pay_onboardingEnt := &cobra.Command{Use: "bill-pay-onboarding", Short: "bill-pay-onboarding"}
	bill_pay_onboardingEnt.AddCommand(newStubCmd(flags, "expenses bill-pay-onboarding", "get", "bill-pay-onboarding-svc bill-payment-eligibility read"))
	return []*cobra.Command{bill_payEnt, b2b_billpayEnt, b2b_networkEnt, bill_pay_onboardingEnt}
}

// newFormDeleteCmd wires `expenses <entity> form delete` to the captured v4
// form-delete mutation (updateTransactions_Transaction with deleted:true +
// header.closeBooksPassword), proven on the expense form's More → Delete flow.
func newFormDeleteCmd(flags *rootFlags, command, entity, txnType, catalogID string) *cobra.Command {
	var id string
	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Delete a " + entity + " via the transaction form (v4)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := map[string]string{}
			if id != "" {
				fm["id"] = id
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: command,
					ID:      catalogID,
					Method:  "POST",
					URL:     "https://qbo.intuit.com/api/v4/graphql",
					Note:    "deleteSavedTransaction__transactions_experiences_qbo (deleted:true); not sent",
					Flags:   fm,
				})
			}
			if err := client.RequireSession(); err != nil {
				return feedErr(flags, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayFormDelete(ctx, id, txnType)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "v3 transaction id to delete (required)")
	applyCatalogHelp(cmd, catalogID)
	return cmd
}

// newExpenseRecategoriseCmd wires `expenses expense update recategorise` to a
// real v3 Purchase sparse update: the category account on account-based lines
// is swapped to --category-id (optionally only --line-id), and --payee-id can
// move the EntityRef. This is the same account swap the banking UI performs.
func newExpenseRecategoriseCmd(flags *rootFlags) *cobra.Command {
	var (
		id, catID, classID, payee, lineID              string
		expectedToken, expectedCategory, expectedClass string
		repairCreditCardType                           bool
	)
	cmd := &cobra.Command{
		Use:   "recategorise",
		Short: "Recategorise a posted expense (v3 Purchase sparse update)",
		RunE: func(cmd *cobra.Command, args []string) error {
			fm := map[string]string{}
			if id != "" {
				fm["id"] = id
			}
			if catID != "" {
				fm["category-id"] = catID
			}
			if classID != "" {
				fm["class-id"] = classID
			}
			if payee != "" {
				fm["payee-id"] = payee
			}
			if lineID != "" {
				fm["line-id"] = lineID
			}
			if cmd.Flags().Changed("expected-sync-token") {
				fm["expected-sync-token"] = expectedToken
			}
			if cmd.Flags().Changed("expected-category-id") {
				fm["expected-category-id"] = expectedCategory
			}
			if cmd.Flags().Changed("expected-class-id") {
				fm["expected-class-id"] = expectedClass
			}
			if repairCreditCardType {
				fm["repair-credit-card-type"] = "true"
			}
			if flags.dryRun {
				return writePlan(cmd, flags, planEnvelope{
					Command: "expenses expense update recategorise",
					ID:      "QBO.EXPENSES.EXPENSE_RECATEGORISE",
					Method:  "POST",
					URL:     "https://qbo.intuit.com/api/v3/company/{realm}/purchase",
					Note:    "v3 Purchase sparse update; not sent",
					Flags:   fm,
				})
			}
			if err := client.RequireSession(); err != nil {
				return feedErr(flags, err)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), feedTimeout(flags))
			defer cancel()
			res, err := client.ReplayExpenseRecategorise(ctx, fm)
			if err != nil {
				return feedErr(flags, err)
			}
			printMutateResult(cmd, flags, res)
			return nil
		},
	}
	cmd.Flags().StringVar(&id, "id", "", "v3 Purchase (expense) id to recategorise (required)")
	cmd.Flags().StringVar(&catID, "category-id", "", "GL account id applied as the new category on account-based lines")
	cmd.Flags().StringVar(&classID, "class-id", "", "Class id applied to the selected account-based line; none clears its Class")
	cmd.Flags().StringVar(&payee, "payee-id", "", "vendor/customer id applied as the new payee (EntityRef)")
	cmd.Flags().StringVar(&lineID, "line-id", "", "only recategorise this Purchase line id")
	cmd.Flags().StringVar(&expectedToken, "expected-sync-token", "", "abort if the Purchase SyncToken changed")
	cmd.Flags().StringVar(&expectedCategory, "expected-category-id", "", "abort if the selected line's category account changed")
	cmd.Flags().StringVar(&expectedClass, "expected-class-id", "", "abort if the selected line's Class changed; use none for blank")
	cmd.Flags().BoolVar(&repairCreditCardType, "repair-credit-card-type", false, "explicitly convert a legacy Check funded by a credit-card account to CreditCard; may not be reversible")
	applyCatalogHelp(cmd, "QBO.EXPENSES.EXPENSE_RECATEGORISE")
	return cmd
}
