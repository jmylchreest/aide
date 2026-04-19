package findings

import (
	"fmt"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
)

// DeadCodeConfig configures the dead code analyzer.
type DeadCodeConfig struct {
	// GetAllSymbols returns all symbols in the code index.
	GetAllSymbols func() ([]*code.Symbol, error)
	// GetRefCount returns the number of references to a symbol name.
	GetRefCount func(name string) (int, error)
	// ProjectRoot is the absolute project root for relative path computation.
	ProjectRoot string
	// ProgressFn is called periodically with progress updates. May be nil.
	ProgressFn func(checked int, found int)
}

// DeadCodeResult holds the output of a dead code analysis run.
type DeadCodeResult struct {
	SymbolsChecked int
	SymbolsSkipped int
	FindingsCount  int
	Duration       time.Duration
}

// AnalyzeDeadCode finds symbols with zero references that are not entrypoints.
//
// A symbol is considered dead if:
//   - It has zero call or type_ref references in the code index
//   - It is not an entrypoint (main, init, TestXxx, BenchmarkXxx, etc.)
//   - It is not an exported method (receiver types may be used via interfaces)
//
// This is a project-wide analyzer that requires the code index to be populated.
func AnalyzeDeadCode(cfg DeadCodeConfig) ([]*Finding, *DeadCodeResult, error) {
	start := time.Now()
	result := &DeadCodeResult{}

	symbols, err := cfg.GetAllSymbols()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get symbols: %w", err)
	}

	var findings []*Finding

	for _, sym := range symbols {
		result.SymbolsChecked++

		if shouldSkipForDeadCode(sym) {
			result.SymbolsSkipped++
			continue
		}

		count, err := cfg.GetRefCount(sym.Name)
		if err != nil {
			continue
		}

		if count > 0 {
			continue
		}

		sev := SevInfo
		if sym.Kind == code.KindFunction || sym.Kind == code.KindMethod {
			sev = SevWarning
		}

		detail := fmt.Sprintf("`%s` (%s) at %s:%d has no references in the codebase. "+
			"It may be unused and safe to remove, or it may be called via reflection, "+
			"code generation, or external consumers not in the index.",
			sym.Name, sym.Kind, sym.FilePath, sym.StartLine)

		findings = append(findings, &Finding{
			Analyzer: AnalyzerDeadCode,
			Severity: sev,
			Category: sym.Kind,
			FilePath: sym.FilePath,
			Line:     sym.StartLine,
			EndLine:  sym.EndLine,
			Title:    fmt.Sprintf("Unreferenced %s: %s", sym.Kind, sym.Name),
			Detail:   detail,
			Metadata: map[string]string{
				"symbol": sym.Name,
				"kind":   sym.Kind,
				"lang":   sym.Language,
			},
		})

		if cfg.ProgressFn != nil && len(findings)%50 == 0 {
			cfg.ProgressFn(result.SymbolsChecked, len(findings))
		}
	}

	result.FindingsCount = len(findings)
	result.Duration = time.Since(start)

	return findings, result, nil
}

// shouldSkipForDeadCode returns true if a symbol should be excluded from
// dead code detection because it is a known entrypoint or framework hook.
func shouldSkipForDeadCode(sym *code.Symbol) bool {
	name := sym.Name

	// Skip test files entirely
	if strings.HasSuffix(sym.FilePath, "_test.go") ||
		strings.HasSuffix(sym.FilePath, ".test.ts") ||
		strings.HasSuffix(sym.FilePath, ".test.tsx") ||
		strings.HasSuffix(sym.FilePath, ".test.js") ||
		strings.HasSuffix(sym.FilePath, ".spec.ts") ||
		strings.HasSuffix(sym.FilePath, ".spec.tsx") ||
		strings.HasSuffix(sym.FilePath, ".spec.js") ||
		strings.Contains(sym.FilePath, "__tests__/") ||
		strings.Contains(sym.FilePath, "__test__/") {
		return true
	}

	// Go entrypoints and lifecycle functions
	if name == "main" || name == "init" {
		return true
	}

	// Go test/benchmark/example/fuzz functions
	if strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") ||
		strings.HasPrefix(name, "Example") || strings.HasPrefix(name, "Fuzz") {
		return true
	}

	// Go exported interface implementations — methods starting with uppercase
	// are often interface implementations accessed via interface types
	if sym.Kind == code.KindMethod && len(name) > 0 && name[0] >= 'A' && name[0] <= 'Z' {
		return true
	}

	// Common framework hooks and lifecycle methods
	lowerName := strings.ToLower(name)
	frameworkHooks := []string{
		"setup", "teardown", "beforeall", "afterall", "beforeeach", "aftereach",
		"run", "execute", "handle", "serve", "listen",
	}
	for _, hook := range frameworkHooks {
		if lowerName == hook {
			return true
		}
	}

	// Skip types/interfaces/classes — they're often used structurally or
	// via reflection and the code index may not capture all type_ref usages
	if sym.Kind == code.KindClass || sym.Kind == code.KindInterface || sym.Kind == code.KindType {
		return true
	}

	// Skip constants and variables — often used in switch/case or as config
	if sym.Kind == code.KindConstant || sym.Kind == code.KindVariable {
		return true
	}

	return false
}
