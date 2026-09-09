package cli

type rootFlags struct {
	deadOne   string
	deadTwo   string
	deadThree string
}

func Execute() {
	rootCmd.PersistentFlags().StringVar(&flags.deadOne, "dead-one", "", "")
	rootCmd.PersistentFlags().StringVar(&flags.deadTwo, "dead-two", "", "")
	rootCmd.PersistentFlags().StringVar(&flags.deadThree, "dead-three", "", "")
	rootCmd.AddCommand(newLiveCmd())
}

func newLiveCmd() any {
	return struct {
		Use string
	}{Use: "live"}
}
