package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangJava, &complexityLang{
		funcNodeTypes: []string{
			"method_declaration",
			"constructor_declaration",
		},
		branchTypes: []string{
			"if_statement",
			"for_statement",
			"enhanced_for_statement",
			"while_statement",
			"do_statement",
			"switch_block_statement_group",
			"catch_clause",
			"ternary_expression",
			"binary_expression", // will filter to && and ||
		},
		nameField: "name",
	})
}
