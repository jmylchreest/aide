package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangCSharp, &complexityLang{
		funcNodeTypes: []string{
			"method_declaration",
			"constructor_declaration",
			"local_function_statement",
		},
		branchTypes: []string{
			"if_statement",
			"for_statement",
			"for_each_statement",
			"while_statement",
			"do_statement",
			"switch_section",
			"catch_clause",
			"conditional_expression", // ternary
			"binary_expression",      // will filter to && and ||
		},
		nameField: "name",
	})
}
