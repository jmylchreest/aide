package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangBash, &complexityLang{
		funcNodeTypes: []string{
			"function_definition",
		},
		branchTypes: []string{
			"if_statement",
			"elif_clause",
			"for_statement",
			"while_statement",
			"case_item",
			"binary_expression", // will filter to && and ||
		},
		nameField: "name",
	})
}
