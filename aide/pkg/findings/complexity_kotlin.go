package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangKotlin, &complexityLang{
		funcNodeTypes: []string{
			"function_declaration",
		},
		branchTypes: []string{
			"if_expression",
			"for_statement",
			"while_statement",
			"do_while_statement",
			"when_entry",
			"catch_block",
			"binary_expression",      // will filter to && and ||
			"conjunction_expression", // &&
			"disjunction_expression", // ||
			"elvis_expression",       // ?:
		},
		nameField: "simple_identifier",
	})
}
