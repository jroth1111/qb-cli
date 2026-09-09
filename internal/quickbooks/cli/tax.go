package cli

import "github.com/spf13/cobra"

func newTaxCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tax",
		Short: "GST, BAS, TPAR",
		Long:  "GST, BAS, TPAR. qb tax <entity> <verb>. Stubs return not-wired.",
	}
	atoEnt := &cobra.Command{Use: "ato", Short: "ato"}
	atoEnt.AddCommand(newStubCmd(flags, "tax ato", "run", "ato connect (not wired)"))
	cmd.AddCommand(atoEnt)
	basEnt := &cobra.Command{Use: "bas", Short: "bas"}
	basEnt.AddCommand(newStubCmd(flags, "tax bas", "run", "bas lodge (not wired)"))
	basEnt.AddCommand(newStubCmd(flags, "tax bas", "get", "bas read (not wired)"))
	cmd.AddCommand(basEnt)
	codeEnt := &cobra.Command{Use: "code", Short: "code"}
	codeEnt.AddCommand(newTaxCodeCreateCmd(flags))
	codeEnt.AddCommand(newStubCmd(flags, "tax code", "delete", "code delete (not wired)"))
	codeEnt.AddCommand(newStubCmd(flags, "tax code", "update", "code edit (not wired)"))
	codeEnt.AddCommand(newStubCmd(flags, "tax code", "get", "code read (not wired)"))
	codeEnt.AddCommand(newStubCmd(flags, "tax code", "search", "code search (not wired)"))
	cmd.AddCommand(codeEnt)
	gstEnt := &cobra.Command{Use: "gst", Short: "gst"}
	gstEnt.AddCommand(newStubCmd(flags, "tax gst", "run setup", "gst setup (not wired)"))
	cmd.AddCommand(gstEnt)
	gst_amendmentsEnt := &cobra.Command{Use: "gst-amendments", Short: "gst-amendments"}
	gst_amendmentsEnt.AddCommand(newStubCmd(flags, "tax gst-amendments", "get", "gst-amendments read (not wired)"))
	cmd.AddCommand(gst_amendmentsEnt)
	gst_deferredEnt := &cobra.Command{Use: "gst-deferred", Short: "gst-deferred"}
	gst_deferredEnt.AddCommand(newStubCmd(flags, "tax gst-deferred", "create", "gst-deferred create (not wired)"))
	cmd.AddCommand(gst_deferredEnt)
	gst_paymentEnt := &cobra.Command{Use: "gst-payment", Short: "gst-payment"}
	gst_paymentEnt.AddCommand(newStubCmd(flags, "tax gst-payment", "create", "gst-payment create (not wired)"))
	gst_paymentEnt.AddCommand(newStubCmd(flags, "tax gst-payment", "delete", "gst-payment delete (not wired)"))
	cmd.AddCommand(gst_paymentEnt)
	gst_refundEnt := &cobra.Command{Use: "gst-refund", Short: "gst-refund"}
	gst_refundEnt.AddCommand(newStubCmd(flags, "tax gst-refund", "create", "gst-refund create (not wired)"))
	cmd.AddCommand(gst_refundEnt)
	gst_settingsEnt := &cobra.Command{Use: "gst-settings", Short: "gst-settings"}
	gst_settingsEnt.AddCommand(newStubCmd(flags, "tax gst-settings", "update", "gst-settings edit (not wired)"))
	cmd.AddCommand(gst_settingsEnt)
	iasEnt := &cobra.Command{Use: "ias", Short: "ias"}
	iasEnt.AddCommand(newStubCmd(flags, "tax ias", "get", "ias read (not wired)"))
	cmd.AddCommand(iasEnt)
	lodgeitEnt := &cobra.Command{Use: "lodgeit", Short: "lodgeit"}
	lodgeitEnt.AddCommand(newStubCmd(flags, "tax lodgeit", "export", "lodgeit export (not wired)"))
	cmd.AddCommand(lodgeitEnt)
	tparEnt := &cobra.Command{Use: "tpar", Short: "tpar"}
	tparEnt.AddCommand(newStubCmd(flags, "tax tpar", "export", "tpar export (not wired)"))
	tparEnt.AddCommand(newStubCmd(flags, "tax tpar", "get", "tpar read (not wired)"))
	cmd.AddCommand(tparEnt)
	tax_agencyEnt := &cobra.Command{Use: "tax-agency", Short: "tax-agency"}
	tax_agencyEnt.AddCommand(newStubCmd(flags, "tax tax-agency", "get", "tax-agency read"))
	tax_agencyEnt.AddCommand(newStubCmd(flags, "tax tax-agency", "create", "tax-agency create"))
	cmd.AddCommand(tax_agencyEnt)
	tax_rateEnt := &cobra.Command{Use: "tax-rate", Short: "tax-rate"}
	tax_rateEnt.AddCommand(newStubCmd(flags, "tax tax-rate", "create", "tax-rate create"))
	tax_rateEnt.AddCommand(newStubCmd(flags, "tax tax-rate", "get", "tax-rate read"))
	cmd.AddCommand(tax_rateEnt)
	// Service map v2 (2026-08-24): never-triggered production services, blocked.
	nexusEnt := &cobra.Command{Use: "nexus", Short: "nexus"}
	nexusEnt.AddCommand(newStubCmd(flags, "tax nexus", "get", "salestax-nexus.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(nexusEnt)
	taxcalcEnt := &cobra.Command{Use: "taxcalc", Short: "taxcalc"}
	taxcalcEnt.AddCommand(newStubCmd(flags, "tax taxcalc", "post", "taxcalculationsvc.api.intuit.com post (service map; not wired)"))
	cmd.AddCommand(taxcalcEnt)
	filing_svcEnt := &cobra.Command{Use: "filing-svc", Short: "filing-svc"}
	filing_svcEnt.AddCommand(newStubCmd(flags, "tax filing-svc", "get", "taxfilingsvc.api.intuit.com[/graphql] get (service map; not wired)"))
	cmd.AddCommand(filing_svcEnt)
	indirect_reportsEnt := &cobra.Command{Use: "indirect-reports", Short: "indirect-reports"}
	indirect_reportsEnt.AddCommand(newStubCmd(flags, "tax indirect-reports", "get", "indirecttaxreports.api.intuit.com get (service map; not wired)"))
	cmd.AddCommand(indirect_reportsEnt)
	experiment_assignmentEnt := &cobra.Command{Use: "experiment-assignment", Short: "experiment-assignment"}
	experiment_assignmentEnt.AddCommand(newStubCmd(flags, "tax experiment-assignment", "get", "experimentassignment.api.intuit.com/api/v3/assignments/ get (service map; not wired)"))
	cmd.AddCommand(experiment_assignmentEnt)
	return cmd
}
