package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangJavaScript, &complexityLang{
		funcNodeTypes: []string{
			"function_declaration",
			"method_definition",
			"arrow_function",
			"function",
		},
		branchTypes: []string{
			"if_statement",
			"for_statement",
			"for_in_statement",
			"while_statement",
			"do_statement",
			"switch_case",
			"catch_clause",
			"ternary_expression",
			"binary_expression", // will filter to && and ||
		},
		nameField: "name",
	})
}
