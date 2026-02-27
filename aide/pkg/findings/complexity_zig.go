package findings

// Zig is a compiled-in grammar (one of the 9 core grammars).
// No LangZig constant exists in the code package yet, so we use the string literal.
func init() {
	registerComplexityLang("zig", &complexityLang{
		funcNodeTypes: []string{
			"FnDecl",
			"TestDecl",
		},
		branchTypes: []string{
			"IfExpr",
			"IfStatement",
			"ForStatement",
			"WhileStatement",
			"SwitchExpr",
		},
		nameField: "name",
	})
}
