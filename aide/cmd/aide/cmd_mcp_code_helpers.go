package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// ============================================================================
// Code formatting helpers
// ============================================================================

func formatCodeSearchResults(results []*store.CodeSearchResult) string {
	if len(results) == 0 {
		return "No matching symbols found.\n\nTip: Run `aide code index` to index your codebase."
	}

	var sb strings.Builder
	sb.WriteString("# Code Search Results\n\n")

	for _, r := range results {
		sym := r.Symbol
		fmt.Fprintf(&sb, "## `%s` [%s]\n", sym.Name, sym.Kind)
		fmt.Fprintf(&sb, "**File:** `%s:%d`\n", sym.FilePath, sym.StartLine)
		fmt.Fprintf(&sb, "**Signature:** `%s`\n", sym.Signature)
		if sym.DocComment != "" {
			fmt.Fprintf(&sb, "**Doc:** %s\n", sym.DocComment)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// containsBleveSyntax checks if a query contains Bleve query syntax characters.
func containsBleveSyntax(query string) bool {
	special := []string{"*", "?", "+", "-", ":", "^", "~", "(", ")", "[", "]", "{", "}", "\"", "&&", "||", "!"}
	for _, s := range special {
		if strings.Contains(query, s) {
			return true
		}
	}
	return false
}

func formatCodeSymbols(filePath string, symbols []*code.Symbol) string {
	if len(symbols) == 0 {
		return fmt.Sprintf("No symbols found in `%s`", filePath)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# Symbols in `%s`\n\n", filePath)
	fmt.Fprintf(&sb, "_Total: %d symbols_\n\n", len(symbols))

	grouped := make(map[string][]*code.Symbol)
	for _, sym := range symbols {
		grouped[sym.Kind] = append(grouped[sym.Kind], sym)
	}

	kindOrder := []string{"interface", "class", "type", "function", "method"}
	for _, kind := range kindOrder {
		syms := grouped[kind]
		if len(syms) == 0 {
			continue
		}

		fmt.Fprintf(&sb, "## %ss\n\n", titleCase(kind))
		for _, sym := range syms {
			fmt.Fprintf(&sb, "- **%s** (line %d): `%s`\n", sym.Name, sym.StartLine, sym.Signature)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func formatCodeReferences(symbolName string, refs []*code.Reference) string {
	if len(refs) == 0 {
		return fmt.Sprintf("No references found for `%s`.\n\nTip: Run `aide code index` to index your codebase.", symbolName)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# References to `%s`\n\n", symbolName)
	fmt.Fprintf(&sb, "_Found %d references_\n\n", len(refs))

	grouped := make(map[string][]*code.Reference)
	for _, ref := range refs {
		grouped[ref.FilePath] = append(grouped[ref.FilePath], ref)
	}

	for filePath, fileRefs := range grouped {
		fmt.Fprintf(&sb, "## `%s`\n\n", filePath)
		for _, ref := range fileRefs {
			kindTag := ""
			switch ref.Kind {
			case code.RefKindCall:
				kindTag = "[call]"
			case code.RefKindTypeRef:
				kindTag = "[type]"
			}
			fmt.Fprintf(&sb, "- **Line %d** %s: `%s`\n", ref.Line, kindTag, ref.Context)
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// readFileLines reads lines startLine..endLine (1-indexed, inclusive) from a file.
func readFileLines(path string, startLine, endLine int) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		if lineNum >= startLine && lineNum <= endLine {
			lines = append(lines, scanner.Text())
		}
		if lineNum >= endLine {
			break
		}
	}
	return lines, scanner.Err()
}

// ============================================================================
// Code outline helpers
// ============================================================================

// bodyRange represents a collapsible body region in a file.
type bodyRange struct {
	startLine int // 1-indexed, first line of body (e.g., the { line)
	endLine   int // 1-indexed, last line of body
	symbol    *code.Symbol
}

// commentPattern matches common single-line comment patterns.
var commentPattern = regexp.MustCompile(`^\s*(//|#|/\*|\*|\*/|--)`)

// buildOutline creates a collapsed view of a file using symbol body ranges.
// Lines inside function/method bodies are replaced with a single "{ ... }" marker.
// If stripComments is true, standalone comment lines outside bodies are removed.
func buildOutline(content []byte, symbols []*code.Symbol, stripComments bool) string {
	var lines []string
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	totalLines := len(lines)

	if totalLines == 0 {
		return "(empty file)"
	}

	var ranges []bodyRange
	for _, sym := range symbols {
		if sym.BodyStartLine > 0 && sym.BodyEndLine > 0 && sym.BodyEndLine > sym.BodyStartLine {
			ranges = append(ranges, bodyRange{
				startLine: sym.BodyStartLine,
				endLine:   sym.BodyEndLine,
				symbol:    sym,
			})
		}
	}
	sort.Slice(ranges, func(i, j int) bool {
		return ranges[i].startLine < ranges[j].startLine
	})

	// Only collapse leaf-level bodies (functions/methods), not containers (classes).
	leafRanges := filterLeafBodies(ranges, symbols)

	collapseStart := make(map[int]*bodyRange)
	collapsedLines := make(map[int]bool)

	for i := range leafRanges {
		r := &leafRanges[i]
		collapseStart[r.startLine] = r
		for line := r.startLine + 1; line <= r.endLine; line++ {
			collapsedLines[line] = true
		}
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "// Outline: %d symbols, %d lines total\n\n", len(symbols), totalLines)

	for lineNum := 1; lineNum <= totalLines; lineNum++ {
		lineIdx := lineNum - 1
		line := lines[lineIdx]

		if collapsedLines[lineNum] {
			continue
		}

		if r, ok := collapseStart[lineNum]; ok {
			indent := extractIndent(line)
			fmt.Fprintf(&sb, "%s%s{ ... }  // lines %d-%d\n", lineNumPrefix(lineNum), indent, r.startLine, r.endLine)
			continue
		}

		if stripComments && isCommentLine(line) {
			continue
		}

		if strings.TrimSpace(line) == "" {
			continue
		}

		fmt.Fprintf(&sb, "%s%s\n", lineNumPrefix(lineNum), line)
	}

	return sb.String()
}

// filterLeafBodies returns only body ranges for leaf symbols (functions, methods)
// and not for container symbols (classes, interfaces) that contain other symbols.
func filterLeafBodies(ranges []bodyRange, _ []*code.Symbol) []bodyRange {
	var result []bodyRange
	for _, r := range ranges {
		kind := r.symbol.Kind
		if kind == code.KindClass || kind == code.KindInterface {
			continue
		}
		result = append(result, r)
	}
	return result
}

// isCommentLine checks if a line is a standalone comment (not code with trailing comment).
func isCommentLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	return commentPattern.MatchString(trimmed)
}

// extractIndent returns the leading whitespace of a line.
func extractIndent(line string) string {
	for i, ch := range line {
		if ch != ' ' && ch != '\t' {
			return line[:i]
		}
	}
	return line
}

// lineNumPrefix formats a line number for the outline output.
func lineNumPrefix(lineNum int) string {
	return fmt.Sprintf("%-4d: ", lineNum)
}

// countLines counts the number of lines in a string.
func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
