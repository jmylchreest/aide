package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangC, &complexityLang{
		funcNodeTypes: []string{
			"function_definition",
		},
		branchTypes: []string{
			"if_statement",
			"for_statement",
			"while_statement",
			"do_statement",
			"case_statement",
			"conditional_expression", // ternary
			"binary_expression",      // will filter to && and ||
		},
		nameField: "declarator",
	})

	registerComplexityLang(code.LangCPP, &complexityLang{
		funcNodeTypes: []string{
			"function_definition",
		},
		branchTypes: []string{
			"if_statement",
			"for_statement",
			"for_range_loop",
			"while_statement",
			"do_statement",
			"case_statement",
			"catch_clause",
			"conditional_expression", // ternary
			"binary_expression",      // will filter to && and ||
		},
		nameField: "declarator",
	})
}
