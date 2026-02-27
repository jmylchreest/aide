package findings

import "github.com/jmylchreest/aide/aide/pkg/code"

func init() {
	registerComplexityLang(code.LangRuby, &complexityLang{
		funcNodeTypes: []string{
			"method",
			"singleton_method",
		},
		branchTypes: []string{
			"if",
			"elsif",
			"unless",
			"while",
			"until",
			"for",
			"when", // case/when
			"rescue",
			"binary", // will filter to && and ||
			"conditional",
		},
		nameField: "name",
	})
}
