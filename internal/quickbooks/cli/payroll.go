package cli

import "github.com/spf13/cobra"

func newPayrollCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "payroll",
		Short: "Pay runs, STP, super",
		Long:  "Pay runs, STP, super. qb payroll <entity> <verb>. Stubs return not-wired.",
	}
	bpayEnt := &cobra.Command{Use: "bpay", Short: "bpay"}
	bpayEnt.AddCommand(newStubCmd(flags, "payroll bpay", "create", "bpay create (not wired)"))
	cmd.AddCommand(bpayEnt)
	deductionEnt := &cobra.Command{Use: "deduction", Short: "deduction"}
	deductionEnt.AddCommand(newStubCmd(flags, "payroll deduction", "create", "deduction create (not wired)"))
	deductionEnt.AddCommand(newStubCmd(flags, "payroll deduction", "update", "deduction edit (not wired)"))
	deductionEnt.AddCommand(newStubCmd(flags, "payroll deduction", "get", "deduction read (not wired)"))
	cmd.AddCommand(deductionEnt)
	employeeEnt := &cobra.Command{Use: "employee", Short: "employee"}
	employeeEnt.AddCommand(newStubCmd(flags, "payroll employee", "create", "employee create"))
	employeeEnt.AddCommand(newStubCmd(flags, "payroll employee", "get", "employee read (not wired)"))
	employeeEnt.AddCommand(newStubCmd(flags, "payroll employee", "search", "employee search (not wired)"))
	employeeEnt.AddCommand(newStubCmd(flags, "payroll employee", "delete", "employee terminate (not wired)"))
	employeeUpdateEnt := &cobra.Command{Use: "update", Short: "update"}
	employeeUpdateEnt.AddCommand(newStubCmd(flags, "payroll employee update", "edit", "employee edit"))
	employeeEnt.AddCommand(employeeUpdateEnt)
	cmd.AddCommand(employeeEnt)
	leave_balanceEnt := &cobra.Command{Use: "leave-balance", Short: "leave-balance"}
	leave_balanceEnt.AddCommand(newStubCmd(flags, "payroll leave-balance", "update", "leave-balance edit (not wired)"))
	cmd.AddCommand(leave_balanceEnt)
	leave_categoryEnt := &cobra.Command{Use: "leave-category", Short: "leave-category"}
	leave_categoryEnt.AddCommand(newStubCmd(flags, "payroll leave-category", "create", "leave-category create (not wired)"))
	leave_categoryEnt.AddCommand(newStubCmd(flags, "payroll leave-category", "update", "leave-category edit (not wired)"))
	leave_categoryEnt.AddCommand(newStubCmd(flags, "payroll leave-category", "get", "leave-category read (not wired)"))
	cmd.AddCommand(leave_categoryEnt)
	lump_sumEnt := &cobra.Command{Use: "lump-sum", Short: "lump-sum"}
	lump_sumEnt.AddCommand(newStubCmd(flags, "payroll lump-sum", "create", "lump-sum create (not wired)"))
	cmd.AddCommand(lump_sumEnt)
	overviewEnt := &cobra.Command{Use: "overview", Short: "overview"}
	overviewEnt.AddCommand(newStubCmd(flags, "payroll overview", "search", "overview search (not wired)"))
	cmd.AddCommand(overviewEnt)
	pay_categoryEnt := &cobra.Command{Use: "pay-category", Short: "pay-category"}
	pay_categoryEnt.AddCommand(newStubCmd(flags, "payroll pay-category", "create", "pay-category create (not wired)"))
	pay_categoryEnt.AddCommand(newStubCmd(flags, "payroll pay-category", "update", "pay-category edit (not wired)"))
	pay_categoryEnt.AddCommand(newStubCmd(flags, "payroll pay-category", "get", "pay-category read (not wired)"))
	cmd.AddCommand(pay_categoryEnt)
	pay_runEnt := &cobra.Command{Use: "pay-run", Short: "pay-run"}
	pay_runEnt.AddCommand(newStubCmd(flags, "payroll pay-run", "create", "pay-run create (not wired)"))
	pay_runEnt.AddCommand(newStubCmd(flags, "payroll pay-run", "get", "pay-run read (not wired)"))
	cmd.AddCommand(pay_runEnt)
	pay_run_inclusionEnt := &cobra.Command{Use: "pay-run-inclusion", Short: "pay-run-inclusion"}
	pay_run_inclusionEnt.AddCommand(newStubCmd(flags, "payroll pay-run-inclusion", "create", "pay-run-inclusion create (not wired)"))
	cmd.AddCommand(pay_run_inclusionEnt)
	payg_summaryEnt := &cobra.Command{Use: "payg-summary", Short: "payg-summary"}
	payg_summaryEnt.AddCommand(newStubCmd(flags, "payroll payg-summary", "create", "payg-summary create (not wired)"))
	cmd.AddCommand(payg_summaryEnt)
	setupEnt := &cobra.Command{Use: "setup", Short: "setup"}
	setupEnt.AddCommand(newStubCmd(flags, "payroll setup", "run", "setup run (not wired)"))
	cmd.AddCommand(setupEnt)
	stpEnt := &cobra.Command{Use: "stp", Short: "stp"}
	stpRunEnt := &cobra.Command{Use: "run", Short: "run"}
	stpRunEnt.AddCommand(newStubCmd(flags, "payroll stp run", "finalise", "stp finalise (not wired)"))
	stpRunEnt.AddCommand(newStubCmd(flags, "payroll stp run", "reset", "stp reset (not wired)"))
	stpEnt.AddCommand(stpRunEnt)
	cmd.AddCommand(stpEnt)
	stp_closely_heldEnt := &cobra.Command{Use: "stp-closely-held", Short: "stp-closely-held"}
	stp_closely_heldEnt.AddCommand(newStubCmd(flags, "payroll stp-closely-held", "run", "stp-closely-held lodge (not wired)"))
	cmd.AddCommand(stp_closely_heldEnt)
	stp_microEnt := &cobra.Command{Use: "stp-micro", Short: "stp-micro"}
	stp_microEnt.AddCommand(newStubCmd(flags, "payroll stp-micro", "run", "stp-micro lodge (not wired)"))
	cmd.AddCommand(stp_microEnt)
	stp_payEnt := &cobra.Command{Use: "stp-pay", Short: "stp-pay"}
	stp_payEnt.AddCommand(newStubCmd(flags, "payroll stp-pay", "create", "stp-pay create (not wired)"))
	cmd.AddCommand(stp_payEnt)
	stp_updateEnt := &cobra.Command{Use: "stp-update", Short: "stp-update"}
	stp_updateEnt.AddCommand(newStubCmd(flags, "payroll stp-update", "create", "stp-update create (not wired)"))
	cmd.AddCommand(stp_updateEnt)
	superEnt := &cobra.Command{Use: "super", Short: "super"}
	superEnt.AddCommand(newStubCmd(flags, "payroll super", "update", "super pay (not wired)"))
	superEnt.AddCommand(newStubCmd(flags, "payroll super", "get", "super read (not wired)"))
	cmd.AddCommand(superEnt)
	super_rescEnt := &cobra.Command{Use: "super-resc", Short: "super-resc"}
	super_rescEnt.AddCommand(newStubCmd(flags, "payroll super-resc", "update", "super-resc edit (not wired)"))
	cmd.AddCommand(super_rescEnt)
	teamEnt := &cobra.Command{Use: "team", Short: "team"}
	teamEnt.AddCommand(newStubCmd(flags, "payroll team", "get", "team read (not wired)"))
	teamEnt.AddCommand(newStubCmd(flags, "payroll team", "search", "team search (not wired)"))
	cmd.AddCommand(teamEnt)
	timesheetEnt := &cobra.Command{Use: "timesheet", Short: "timesheet"}
	timesheetEnt.AddCommand(newStubCmd(flags, "payroll timesheet", "create", "timesheet create (not wired)"))
	timesheetEnt.AddCommand(newStubCmd(flags, "payroll timesheet", "update", "timesheet edit (not wired)"))
	timesheetEnt.AddCommand(newStubCmd(flags, "payroll timesheet", "get", "timesheet read (not wired)"))
	cmd.AddCommand(timesheetEnt)
	workcoverEnt := &cobra.Command{Use: "workcover", Short: "workcover"}
	workcoverEnt.AddCommand(newStubCmd(flags, "payroll workcover", "create", "workcover create (not wired)"))
	cmd.AddCommand(workcoverEnt)
	return cmd
}
