package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangSwift, &complexityLang{
		funcNodeTypes: []string{
			"function_declaration",
		},
		branchTypes: []string{
			"if_statement",
			"for_statement",
			"while_statement",
			"repeat_while_statement",
			"switch_case",
			"catch_clause",
			"guard_statement",
			"ternary_expression",
			"binary_expression", // will filter to && and ||
		},
		nameField: "name",
	})
}
