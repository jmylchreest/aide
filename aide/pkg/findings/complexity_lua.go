package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangLua, &complexityLang{
		funcNodeTypes: []string{
			"function_declaration",
			"local_function_declaration_statement",
			"function_definition",
		},
		branchTypes: []string{
			"if_statement",
			"elseif_clause",
			"for_statement",
			"for_generic_statement",
			"while_statement",
			"repeat_statement",
			"binary_expression", // will filter to and/or
		},
		nameField: "name",
	})
}
