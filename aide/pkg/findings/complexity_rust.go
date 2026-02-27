package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangRust, &complexityLang{
		funcNodeTypes: []string{
			"function_item",
		},
		branchTypes: []string{
			"if_expression",
			"for_expression",
			"while_expression",
			"loop_expression",
			"match_arm",
			"binary_expression", // will filter to && and ||
		},
		nameField: "name",
	})
}
