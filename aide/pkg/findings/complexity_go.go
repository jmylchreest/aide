package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangGo, &complexityLang{
		funcNodeTypes: []string{
			"function_declaration",
			"method_declaration",
			"func_literal",
		},
		branchTypes: []string{
			"if_statement",
			"for_statement",
			"expression_case",    // each case in a switch
			"type_case",          // each case in a type switch
			"default_case",       // default clause
			"communication_case", // select case
			"go_statement",
			"defer_statement",
			"binary_expression", // will filter to && and ||
		},
		nameField: "name",
	})
}
