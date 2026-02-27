package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangPHP, &complexityLang{
		funcNodeTypes: []string{
			"function_definition",
			"method_declaration",
		},
		branchTypes: []string{
			"if_statement",
			"for_statement",
			"foreach_statement",
			"while_statement",
			"do_statement",
			"switch_case",
			"catch_clause",
			"binary_expression", // will filter to && and ||
		},
		nameField: "name",
	})
}
