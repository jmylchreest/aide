// Package findings provides static analysis analyzers for aide.
package findings

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/golang"
	"github.com/smacker/go-tree-sitter/java"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/python"
	"github.com/smacker/go-tree-sitter/rust"
	"github.com/smacker/go-tree-sitter/typescript/typescript"
)

// ComplexityConfig configures the complexity analyzer.
type ComplexityConfig struct {
	// Threshold is the minimum complexity to report (default 10).
	Threshold int
	// Paths to analyze (default: current directory).
	Paths []string
	// ProgressFn is called after each file is analyzed. May be nil.
	ProgressFn func(path string, findings int)
}

// ComplexityResult holds the output of a complexity analysis run.
type ComplexityResult struct {
	FilesAnalyzed int
	FilesSkipped  int
	FindingsCount int
	Duration      time.Duration
}

// complexityLang holds per-language tree-sitter config for complexity analysis.
type complexityLang struct {
	provider      func() *sitter.Language
	funcNodeTypes []string // Node types that represent functions/methods
	branchTypes   []string // Node types that add a decision point (+1 complexity)
	nameField     string   // Field name for function name (default: "name")
}

// complexityLanguages defines supported languages for complexity analysis.
var complexityLanguages = map[string]*complexityLang{
	code.LangGo: {
		provider: golang.GetLanguage,
		funcNodeTypes: []string{
			"function_declaration",
			"method_declaration",
			"func_literal",
		},
		branchTypes: []string{
			"if_statement",
			"for_statement",
			"expression_case",    // each case in a switch
			"type_case",          // each case in a type switch
			"default_case",       // default clause
			"communication_case", // select case
			"go_statement",
			"defer_statement",
			"binary_expression", // will filter to && and ||
		},
		nameField: "name",
	},
	code.LangTypeScript: {
		provider: typescript.GetLanguage,
		funcNodeTypes: []string{
			"function_declaration",
			"method_definition",
			"arrow_function",
			"function",
		},
		branchTypes: []string{
			"if_statement",
			"for_statement",
			"for_in_statement",
			"while_statement",
			"do_statement",
			"switch_case",
			"catch_clause",
			"ternary_expression",
			"binary_expression", // will filter to && and ||
			"optional_chain_expression",
		},
		nameField: "name",
	},
	code.LangJavaScript: {
		provider: javascript.GetLanguage,
		funcNodeTypes: []string{
			"function_declaration",
			"method_definition",
			"arrow_function",
			"function",
		},
		branchTypes: []string{
			"if_statement",
			"for_statement",
			"for_in_statement",
			"while_statement",
			"do_statement",
			"switch_case",
			"catch_clause",
			"ternary_expression",
			"binary_expression", // will filter to && and ||
		},
		nameField: "name",
	},
	code.LangPython: {
		provider: python.GetLanguage,
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
	},
	code.LangRust: {
		provider: rust.GetLanguage,
		funcNodeTypes: []string{
			"function_item",
		},
		branchTypes: []string{
			"if_expression",
			"for_expression",
			"while_expression",
			"loop_expression",
			"match_arm",
			"binary_expression", // will filter to && and ||
		},
		nameField: "name",
	},
	code.LangJava: {
		provider: java.GetLanguage,
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
	},
}

// AnalyzeComplexity analyzes files for cyclomatic complexity.
// It returns findings for functions/methods that exceed the configured threshold.
func AnalyzeComplexity(cfg ComplexityConfig) ([]*Finding, *ComplexityResult, error) {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 10
	}
	if len(cfg.Paths) == 0 {
		cfg.Paths = []string{"."}
	}

	start := time.Now()
	result := &ComplexityResult{}
	var allFindings []*Finding

	for _, root := range cfg.Paths {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				if name == "node_modules" || name == ".git" || name == "vendor" ||
					name == "__pycache__" || name == ".venv" || name == "dist" ||
					name == "build" || name == ".aide" {
					return filepath.SkipDir
				}
				return nil
			}

			if !code.SupportedFile(path) {
				return nil
			}

			lang := code.DetectLanguage(path, nil)
			langCfg, ok := complexityLanguages[lang]
			if !ok {
				result.FilesSkipped++
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			relPath := path
			if cwd, err := os.Getwd(); err == nil {
				if rel, err := filepath.Rel(cwd, path); err == nil {
					relPath = rel
				}
			}

			findings := analyzeFileComplexity(content, relPath, lang, langCfg, cfg.Threshold)
			allFindings = append(allFindings, findings...)
			result.FilesAnalyzed++

			if cfg.ProgressFn != nil {
				cfg.ProgressFn(relPath, len(findings))
			}

			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("walk %s: %w", root, err)
		}
	}

	result.FindingsCount = len(allFindings)
	result.Duration = time.Since(start)
	return allFindings, result, nil
}

// analyzeFileComplexity parses a single file and computes complexity for each function.
func analyzeFileComplexity(content []byte, filePath, lang string, langCfg *complexityLang, threshold int) []*Finding {
	sitterLang := langCfg.provider()
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(sitterLang)

	tree, err := parser.ParseCtx(context.Background(), nil, content)
	if err != nil || tree == nil {
		return nil
	}
	defer tree.Close()

	root := tree.RootNode()

	// Build lookup sets for fast node type checking
	funcTypes := make(map[string]bool, len(langCfg.funcNodeTypes))
	for _, t := range langCfg.funcNodeTypes {
		funcTypes[t] = true
	}
	branchTypes := make(map[string]bool, len(langCfg.branchTypes))
	for _, t := range langCfg.branchTypes {
		branchTypes[t] = true
	}

	var findings []*Finding

	// Walk AST to find function nodes
	var walk func(node *sitter.Node)
	walk = func(node *sitter.Node) {
		if funcTypes[node.Type()] {
			complexity := countComplexity(node, branchTypes, content)
			if complexity >= threshold {
				name := extractFuncName(node, content, langCfg.nameField)
				severity := SevInfo
				if complexity >= threshold*2 {
					severity = SevCritical
				} else if complexity >= threshold {
					severity = SevWarning
				}

				startLine := int(node.StartPoint().Row) + 1
				endLine := int(node.EndPoint().Row) + 1

				finding := &Finding{
					Analyzer: AnalyzerComplexity,
					Severity: severity,
					Category: lang,
					FilePath: filePath,
					Line:     startLine,
					EndLine:  endLine,
					Title:    fmt.Sprintf("%s has complexity %d", name, complexity),
					Detail:   fmt.Sprintf("Cyclomatic complexity of %d exceeds threshold of %d. Consider refactoring into smaller functions.", complexity, threshold),
					Metadata: map[string]string{
						"complexity": strconv.Itoa(complexity),
						"threshold":  strconv.Itoa(threshold),
						"function":   name,
						"language":   lang,
					},
					CreatedAt: time.Now(),
				}
				findings = append(findings, finding)
			}
		}

		// Don't recurse into nested functions from the outer walk — they'll be
		// found at the top-level walk. However, we DO recurse into all children
		// to find function nodes at any depth (e.g., methods inside classes).
		for i := 0; i < int(node.ChildCount()); i++ {
			walk(node.Child(i))
		}
	}

	for i := 0; i < int(root.ChildCount()); i++ {
		walk(root.Child(i))
	}

	return findings
}

// countComplexity counts the cyclomatic complexity of a function node.
// Cyclomatic complexity = 1 (base) + number of decision points.
func countComplexity(funcNode *sitter.Node, branchTypes map[string]bool, content []byte) int {
	complexity := 1 // Base complexity

	var count func(node *sitter.Node)
	count = func(node *sitter.Node) {
		nodeType := node.Type()

		if branchTypes[nodeType] {
			// Special handling for binary expressions: only count && and ||
			if nodeType == "binary_expression" || nodeType == "boolean_operator" {
				op := getOperator(node, content)
				if op == "&&" || op == "||" || op == "and" || op == "or" {
					complexity++
				}
			} else {
				complexity++
			}
		}

		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			// Don't recurse into nested function definitions
			if isNestedFunction(child) {
				continue
			}
			count(child)
		}
	}

	// Start counting from the function body
	for i := 0; i < int(funcNode.ChildCount()); i++ {
		count(funcNode.Child(i))
	}

	return complexity
}

// getOperator extracts the operator from a binary expression node.
func getOperator(node *sitter.Node, content []byte) string {
	// The operator is typically a named or anonymous child
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		text := child.Content(content)
		if text == "&&" || text == "||" || text == "and" || text == "or" {
			return text
		}
	}
	return ""
}

// isNestedFunction checks if a node is a nested function definition.
func isNestedFunction(node *sitter.Node) bool {
	nodeType := node.Type()
	switch nodeType {
	case "function_declaration", "function_definition", "method_declaration",
		"method_definition", "function_item", "func_literal",
		"arrow_function", "function", "constructor_declaration":
		return true
	}
	return false
}

// extractFuncName gets the function name from a function node.
func extractFuncName(node *sitter.Node, content []byte, nameField string) string {
	// Try the configured name field
	if nameField != "" {
		if nameNode := node.ChildByFieldName(nameField); nameNode != nil {
			return nameNode.Content(content)
		}
	}

	// Fallback: try common field names
	for _, field := range []string{"name", "declarator"} {
		if nameNode := node.ChildByFieldName(field); nameNode != nil {
			text := nameNode.Content(content)
			// For Go methods, the name might be a field_identifier
			if len(text) > 0 {
				return text
			}
		}
	}

	// Fallback: anonymous function at line
	return fmt.Sprintf("<anonymous:%d>", node.StartPoint().Row+1)
}

// skipDirs returns true for directories that should be skipped during analysis.
func skipDirs(name string) bool {
	switch name {
	case "node_modules", ".git", "vendor", "__pycache__", ".venv",
		"dist", "build", ".aide", ".next", "coverage", ".cache":
		return true
	}
	return strings.HasPrefix(name, ".")
}
