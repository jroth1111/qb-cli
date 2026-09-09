package cli

func Execute() {
	rootCmd.AddCommand(newLiveCmd())
}

func newLiveCmd() any {
	return struct {
		Use string
	}{Use: "live"}
}

func newOrphanCmd() any {
	return struct {
		Use string
	}{Use: "orphan"}
}
