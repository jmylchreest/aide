package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangScala, &complexityLang{
		funcNodeTypes: []string{
			"function_definition",
		},
		branchTypes: []string{
			"if_expression",
			"for_expression",
			"while_expression",
			"case_clause",
			"catch_clause",
			"binary_expression", // will filter to && and ||
		},
		nameField: "name",
	})
}
