//go:build gofuzz

package actionlint_fuzz

import "github.com/rhysd/actionlint"

func parseWorkflowPanicFree(data []byte) *actionlint.Workflow {
	// Avoid Parse() panicking. It panics when go-yaml panics
	defer func() { recover() }()
	w, _ := actionlint.Parse(data)
	return w
}

func FuzzCheck(data []byte) int {
	w := parseWorkflowPanicFree(data)
	if w == nil {
		return 0
	}

	ac := actionlint.NewLocalActionsCache(nil, nil)
	wc := actionlint.NewLocalReusableWorkflowCache(nil, "", nil)

	rules := []actionlint.Rule{
		actionlint.NewRuleMatrix(),
		actionlint.NewRuleCredentials(),
		actionlint.NewRuleShellName(),
		actionlint.NewRuleRunnerLabel(),
		actionlint.NewRuleEvents(),
		actionlint.NewRuleGlob(),
		actionlint.NewRuleJobNeeds(),
		actionlint.NewRuleAction(ac),
		actionlint.NewRuleEnvVar(),
		actionlint.NewRuleID(),
		actionlint.NewRuleExpression(ac, wc),
		actionlint.NewRuleWorkflowCall("test.yaml", wc),
		actionlint.NewRulePermissions(),
		actionlint.NewRuleDeprecatedCommands(),
		actionlint.NewRuleIfCond(),
	}

	v := actionlint.NewVisitor()
	for _, rule := range rules {
		v.AddPass(rule)
	}

	if err := v.Visit(w); err != nil {
		return 0
	}

	return 1
}
