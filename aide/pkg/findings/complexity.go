// Package findings provides static analysis analyzers for aide.
package findings

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// ComplexityConfig configures the complexity analyzer.
type ComplexityConfig struct {
	// Threshold is the minimum complexity to report (default 10).
	Threshold int
	// Paths to analyze (default: current directory).
	Paths []string
	// ProgressFn is called after each file is analyzed. May be nil.
	ProgressFn func(path string, findings int)
	// Ignore is the aideignore matcher for filtering files/directories.
	// If nil, built-in defaults are used.
	Ignore *aideignore.Matcher
	// Loader is the grammar loader for tree-sitter languages.
	// If nil, a default CompositeLoader is created.
	Loader grammar.Loader
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
	funcNodeTypes []string // Node types that represent functions/methods
	branchTypes   []string // Node types that add a decision point (+1 complexity)
	nameField     string   // Field name for function name (default: "name")
}

// getComplexityLang returns the complexity config for a language. It checks:
// 1. PackRegistry (pack.json complexity data)
// 2. genericComplexityLang fallback
func getComplexityLang(lang string) *complexityLang {
	if pack := grammar.DefaultPackRegistry().Get(lang); pack != nil && pack.Complexity != nil {
		return &complexityLang{
			funcNodeTypes: pack.Complexity.FuncNodeTypes,
			branchTypes:   pack.Complexity.BranchTypes,
			nameField:     pack.Complexity.NameField,
		}
	}
	return genericComplexityLang
}

// genericComplexityLang is a minimal fallback config covering the most common
// tree-sitter node types. Used when no language-specific pack.json complexity
// config exists (e.g. data/config languages, or newly-added packs that haven't
// defined one yet). Unrecognised node types are harmlessly ignored.
var genericComplexityLang = &complexityLang{
	funcNodeTypes: []string{
		"function_declaration",
		"function_definition",
		"method_declaration",
		"method_definition",
		"func_literal",
		"arrow_function",
		"function_item",
	},
	branchTypes: []string{
		"if_statement", "if_expression",
		"for_statement", "for_in_statement",
		"while_statement",
		"do_statement",
		"switch_case", "case_clause",
		"catch_clause", "except_clause",
		"ternary_expression", "conditional_expression",
		"binary_expression", "boolean_operator",
	},
	nameField: "name",
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
	if cfg.Loader == nil {
		cfg.Loader = grammar.NewCompositeLoader()
	}

	ignore := cfg.Ignore
	if ignore == nil {
		ignore = aideignore.NewFromDefaults()
	}

	start := time.Now()
	result := &ComplexityResult{}
	var allFindings []*Finding

	for _, root := range cfg.Paths {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, nil, fmt.Errorf("abs path %s: %w", root, err)
		}
		shouldSkip := ignore.WalkFunc(absRoot)

		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if skip, skipDir := shouldSkip(path, info); skip {
				if skipDir {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() {
				return nil
			}

			if !code.SupportedFile(path) {
				return nil
			}

			lang := code.DetectLanguage(path, nil)
			langCfg := getComplexityLang(lang)

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

			findings := analyzeFileComplexity(context.Background(), cfg.Loader, content, relPath, lang, langCfg, cfg.Threshold)
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
func analyzeFileComplexity(ctx context.Context, loader grammar.Loader, content []byte, filePath, lang string, langCfg *complexityLang, threshold int) []*Finding {
	sitterLang, err := loader.Load(ctx, lang)
	if err != nil {
		return nil
	}

	parser := tree_sitter.NewParser()
	defer parser.Close()
	if err := parser.SetLanguage(sitterLang); err != nil {
		return nil
	}

	tree := parser.Parse(content, nil)
	if tree == nil {
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
	var walk func(node *tree_sitter.Node)
	walk = func(node *tree_sitter.Node) {
		if funcTypes[node.Kind()] {
			complexity := countComplexity(node, branchTypes, funcTypes, content)
			if complexity >= threshold {
				name := extractFuncName(node, content, langCfg.nameField)
				severity := SevInfo
				if complexity >= threshold*2 {
					severity = SevCritical
				} else if complexity >= threshold {
					severity = SevWarning
				}

				startLine := int(node.StartPosition().Row) + 1
				endLine := int(node.EndPosition().Row) + 1

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
		for i := uint(0); i < node.ChildCount(); i++ {
			walk(node.Child(i))
		}
	}

	for i := uint(0); i < root.ChildCount(); i++ {
		walk(root.Child(i))
	}

	return findings
}

// countComplexity counts the cyclomatic complexity of a function node.
// funcTypes is used to identify nested function boundaries (don't recurse into them).
func countComplexity(funcNode *tree_sitter.Node, branchTypes, funcTypes map[string]bool, content []byte) int {
	complexity := 1 // Base complexity

	var count func(node *tree_sitter.Node)
	count = func(node *tree_sitter.Node) {
		nodeType := node.Kind()

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

		for i := uint(0); i < node.ChildCount(); i++ {
			child := node.Child(i)
			// Don't recurse into nested function definitions
			if funcTypes[child.Kind()] {
				continue
			}
			count(child)
		}
	}

	// Start counting from the function body
	for i := uint(0); i < funcNode.ChildCount(); i++ {
		count(funcNode.Child(i))
	}

	return complexity
}

// getOperator extracts the operator from a binary expression node.
func getOperator(node *tree_sitter.Node, content []byte) string {
	// The operator is typically a named or anonymous child
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		text := child.Utf8Text(content)
		if text == "&&" || text == "||" || text == "and" || text == "or" {
			return text
		}
	}
	return ""
}

// extractFuncName gets the function name from a function node.
func extractFuncName(node *tree_sitter.Node, content []byte, nameField string) string {
	// Try the configured name field
	if nameField != "" {
		if nameNode := node.ChildByFieldName(nameField); nameNode != nil {
			return nameNode.Utf8Text(content)
		}
	}

	// Fallback: try common field names
	for _, field := range []string{"name", "declarator"} {
		if nameNode := node.ChildByFieldName(field); nameNode != nil {
			text := nameNode.Utf8Text(content)
			// For Go methods, the name might be a field_identifier
			if len(text) > 0 {
				return text
			}
		}
	}

	// Fallback: anonymous function at line
	return fmt.Sprintf("<anonymous:%d>", node.StartPosition().Row+1)
}
