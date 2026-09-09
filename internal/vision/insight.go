package vision

type NonObviousInsight struct {
	ObviousFrame   string
	InsightFrame   string
	SignalSentence string
	Implications   []string
}

func (n NonObviousInsight) Formula(apiName string) string {
	if n.InsightFrame == "" {
		return ""
	}
	return apiName + " isn't just " + n.ObviousFrame + ". It's " + n.InsightFrame + ". " + n.SignalSentence
}

func (n NonObviousInsight) HasInsight() bool {
	return n.InsightFrame != "" && len(n.Implications) > 0
}
