package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangPython, &complexityLang{
		funcNodeTypes: []string{
			"function_definition",
		},
		branchTypes: []string{
			"if_statement",
			"elif_clause",
			"for_statement",
			"while_statement",
			"except_clause",
			"with_statement",
			"assert_statement",
			"boolean_operator", // and/or
			"conditional_expression",
			"list_comprehension",
			"dictionary_comprehension",
			"set_comprehension",
			"generator_expression",
		},
		nameField: "name",
	})
}
