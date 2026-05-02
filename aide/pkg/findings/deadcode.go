package findings

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/observe"
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
	// PackProvider returns the grammar pack for a language; nil disables
	// pack-driven exported and suppression checks (analyzer falls back to
	// universal hardcoded skip rules only).
	PackProvider func(language string) *grammar.Pack
	// IncludeExported, when true, analyses exported symbols too. Default false
	// because external consumers cannot be observed from the index.
	IncludeExported bool
	// ConsumerExtensions are additional file extensions the verifier should
	// scan for literal-token references (templating formats like .astro,
	// .svelte, .vue that aren't parsed but consume code by name). Typically
	// supplied as grammar.DefaultPackRegistry().ConsumerExtensions().
	ConsumerExtensions []string
}

// DeadCodeResult holds the output of a dead code analysis run.
type DeadCodeResult struct {
	SymbolsChecked int
	SymbolsSkipped int
	FindingsCount  int
	Duration       time.Duration
}

// candidateFinding pairs a finding with its source symbol so the verification
// pass can exclude the declaration's own body from text matches.
type candidateFinding struct {
	finding *Finding
	symbol  *code.Symbol
}

// AnalyzeDeadCode finds symbols with zero references that are not entrypoints,
// suppressed by a comment, exported public API, or whose name appears as a
// literal token elsewhere in the codebase (text-grep verification catches
// references the index misses: qualified method calls, JSX use sites, function
// values passed by name).
func AnalyzeDeadCode(cfg DeadCodeConfig) ([]*Finding, *DeadCodeResult, error) {
	span := observe.Start("AnalyzeDeadCode", observe.KindSpan).Category("analyzer").Subtype("deadcode")
	defer span.End()
	start := time.Now()
	result := &DeadCodeResult{}

	symbols, err := cfg.GetAllSymbols()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get symbols: %w", err)
	}

	rules := newDeadcodeRuleCache(cfg.PackProvider)
	suppress := newSuppressionScanner(cfg.ProjectRoot, rules)

	var candidates []candidateFinding

	for _, sym := range symbols {
		result.SymbolsChecked++

		if shouldSkipForDeadCode(sym) {
			result.SymbolsSkipped++
			continue
		}

		if !cfg.IncludeExported && rules.isExported(sym) {
			result.SymbolsSkipped++
			continue
		}

		if suppress.isSuppressed(sym) {
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

		candidates = append(candidates, candidateFinding{
			symbol: sym,
			finding: &Finding{
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
			},
		})

		if cfg.ProgressFn != nil && len(candidates)%50 == 0 {
			cfg.ProgressFn(result.SymbolsChecked, len(candidates))
		}
	}

	// Verification pass: drop candidates whose name appears as a literal token
	// anywhere outside the declaring symbol's body. The reference indexer
	// misses qualified call sites (`s.handleX`), JSX components (`<Foo />`),
	// and function-as-value passes (`{Field: foo}`); a literal text scan
	// catches all of these at the cost of accepting false negatives from
	// occurrences inside string literals or comments.
	verifier := newReferenceVerifier(cfg.ProjectRoot, symbols, cfg.ConsumerExtensions)
	findings := make([]*Finding, 0, len(candidates))
	for _, cand := range candidates {
		if verifier.appearsElsewhere(cand.symbol) {
			result.SymbolsSkipped++
			continue
		}
		findings = append(findings, cand.finding)
	}

	result.FindingsCount = len(findings)
	result.Duration = time.Since(start)

	span.Attr("checked", strconv.Itoa(result.SymbolsChecked)).
		Attr("skipped", strconv.Itoa(result.SymbolsSkipped)).
		Attr("findings", strconv.Itoa(result.FindingsCount))

	return findings, result, nil
}

// shouldSkipForDeadCode returns true if a symbol should be excluded from
// dead code detection because it is a known entrypoint or framework hook.
// These are universal sanity rules independent of the language pack.
func shouldSkipForDeadCode(sym *code.Symbol) bool {
	name := sym.Name

	// Corrupt or orphan symbol rows (empty FilePath, no language, line 0)
	// can't be reasoned about — drop them silently rather than producing
	// findings that point to a phantom location. Reconcile cleans them up
	// from the index but the analyzer still needs a guard for cases where
	// reconcile hasn't run yet.
	if sym.FilePath == "" || sym.StartLine == 0 {
		return true
	}

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

	if name == "main" || name == "init" {
		return true
	}

	if strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") ||
		strings.HasPrefix(name, "Example") || strings.HasPrefix(name, "Fuzz") {
		return true
	}

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

	if sym.Kind == code.KindClass || sym.Kind == code.KindInterface || sym.Kind == code.KindType {
		return true
	}

	if sym.Kind == code.KindConstant || sym.Kind == code.KindVariable {
		return true
	}

	return false
}

// deadcodeRuleCache resolves and caches per-language pack rules.
type deadcodeRuleCache struct {
	provider func(string) *grammar.Pack
	mu       sync.Mutex
	packs    map[string]*deadcodeLangRules
}

type deadcodeLangRules struct {
	exportedRule  string
	suppression   []*regexp.Regexp
	blockSuppress []*regexp.Regexp
}

func newDeadcodeRuleCache(provider func(string) *grammar.Pack) *deadcodeRuleCache {
	return &deadcodeRuleCache{
		provider: provider,
		packs:    make(map[string]*deadcodeLangRules),
	}
}

func (c *deadcodeRuleCache) get(lang string) *deadcodeLangRules {
	if c.provider == nil || lang == "" {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if r, ok := c.packs[lang]; ok {
		return r
	}
	pack := c.provider(lang)
	if pack == nil || pack.Deadcode == nil {
		c.packs[lang] = nil
		return nil
	}
	r := &deadcodeLangRules{exportedRule: pack.Deadcode.ExportedRule}
	for _, p := range pack.Deadcode.SuppressionPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		r.suppression = append(r.suppression, re)
	}
	for _, p := range pack.Deadcode.BlockSuppressionPatterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		r.blockSuppress = append(r.blockSuppress, re)
	}
	c.packs[lang] = r
	return r
}

func (c *deadcodeRuleCache) isExported(sym *code.Symbol) bool {
	r := c.get(sym.Language)
	if r == nil {
		return false
	}
	return isExportedByRule(sym.Name, r.exportedRule)
}

func isExportedByRule(name, rule string) bool {
	if name == "" {
		return false
	}
	switch rule {
	case "first_char_uppercase":
		c := name[0]
		return c >= 'A' && c <= 'Z'
	case "no_leading_underscore":
		return name[0] != '_'
	default:
		return false
	}
}

// suppressionScanner reads file content (cached per file) and checks whether
// the lines above a symbol's declaration contain a pack-defined suppression
// pattern, or whether the symbol falls inside a block-suppressed range.
type suppressionScanner struct {
	root       string
	rules      *deadcodeRuleCache
	mu         sync.Mutex
	cache      map[string][]string
	blockCache map[string][]lineRange
}

type lineRange struct {
	start int // 1-indexed inclusive
	end   int // 1-indexed inclusive
}

func newSuppressionScanner(root string, rules *deadcodeRuleCache) *suppressionScanner {
	return &suppressionScanner{
		root:       root,
		rules:      rules,
		cache:      make(map[string][]string),
		blockCache: make(map[string][]lineRange),
	}
}

// isSuppressed checks the immediately-preceding non-blank line for a pack
// suppression pattern, and whether the symbol falls inside a block-suppressed
// range (e.g. a `#[cfg(test)] mod tests { ... }` body). Linter directives by
// convention attach to the next declaration, so we walk past blank lines but
// stop at any other line for the line-level check.
func (s *suppressionScanner) isSuppressed(sym *code.Symbol) bool {
	r := s.rules.get(sym.Language)
	if r == nil || (len(r.suppression) == 0 && len(r.blockSuppress) == 0) {
		return false
	}
	lines := s.lines(sym.FilePath)
	if lines == nil {
		return false
	}

	if len(r.blockSuppress) > 0 {
		for _, rng := range s.blocks(sym.FilePath, lines, r.blockSuppress) {
			if sym.StartLine >= rng.start && sym.StartLine <= rng.end {
				return true
			}
		}
	}

	if len(r.suppression) == 0 {
		return false
	}

	for i := sym.StartLine - 2; i >= 0 && i < len(lines); i-- {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		for _, re := range r.suppression {
			if re.MatchString(lines[i]) {
				return true
			}
		}
		return false
	}
	return false
}

// blocks returns cached line ranges for the file where any block_suppression
// pattern matched. Each range covers the body of the next braced block opened
// after the matching attribute line.
func (s *suppressionScanner) blocks(relPath string, lines []string, rules []*regexp.Regexp) []lineRange {
	s.mu.Lock()
	defer s.mu.Unlock()
	if cached, ok := s.blockCache[relPath]; ok {
		return cached
	}
	ranges := computeBlockRanges(lines, rules)
	s.blockCache[relPath] = ranges
	return ranges
}

// computeBlockRanges scans the file for lines matching any of the given
// patterns, then for each match locates the nearest opening `{` on or after
// that line and follows brace depth forward to find the matching `}`. The
// returned ranges are 1-indexed inclusive and cover the lines from the
// attribute through the closing brace, so any symbol declared inside the
// block is included. Comments and string literals are not parsed — the
// scanner counts braces naively, which is good enough for Rust attribute
// blocks where braces inside strings are rare and cause at worst an
// over-broad suppression range.
func computeBlockRanges(lines []string, rules []*regexp.Regexp) []lineRange {
	var ranges []lineRange
	for i, line := range lines {
		matched := false
		for _, re := range rules {
			if re.MatchString(line) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		openLine := findOpeningBrace(lines, i)
		if openLine < 0 {
			continue
		}
		closeLine := findMatchingClose(lines, openLine)
		if closeLine < 0 {
			continue
		}
		ranges = append(ranges, lineRange{start: i + 1, end: closeLine + 1})
	}
	return ranges
}

func findOpeningBrace(lines []string, from int) int {
	for i := from; i < len(lines); i++ {
		if strings.Contains(lines[i], "{") {
			return i
		}
	}
	return -1
}

func findMatchingClose(lines []string, openLine int) int {
	depth := 0
	started := false
	for i := openLine; i < len(lines); i++ {
		for _, ch := range lines[i] {
			switch ch {
			case '{':
				depth++
				started = true
			case '}':
				depth--
				if started && depth == 0 {
					return i
				}
			}
		}
	}
	return -1
}

// referenceVerifier checks whether a symbol's bare name appears as a literal
// token anywhere in the indexed file set, outside the symbol's own declaration
// body. It catches references the tree-sitter index misses (qualified method
// calls, JSX, function-as-value passes).
type referenceVerifier struct {
	root    string
	files   []string
	mu      sync.Mutex
	content map[string][]byte
}

func newReferenceVerifier(root string, symbols []*code.Symbol, consumerExts []string) *referenceVerifier {
	seen := make(map[string]struct{}, len(symbols))
	for _, s := range symbols {
		seen[s.FilePath] = struct{}{}
	}
	for _, p := range walkConsumerFiles(root, consumerExts) {
		seen[p] = struct{}{}
	}
	files := make([]string, 0, len(seen))
	for p := range seen {
		files = append(files, p)
	}
	return &referenceVerifier{root: root, files: files, content: make(map[string][]byte)}
}

// walkConsumerFiles returns project-relative paths of files whose extension
// matches one of the registered consumer formats. These files are not in the
// code index but are scanned by the verifier as plain text.
func walkConsumerFiles(root string, exts []string) []string {
	if root == "" || len(exts) == 0 {
		return nil
	}
	ignore, _ := aideignore.New(root)
	if ignore == nil {
		ignore = aideignore.NewFromDefaults()
	}
	shouldSkip := ignore.WalkFunc(root)

	extSet := make(map[string]struct{}, len(exts))
	for _, e := range exts {
		extSet[strings.ToLower(e)] = struct{}{}
	}

	var out []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
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
		if !matchesExt(strings.ToLower(path), extSet) {
			return nil
		}
		if rel, err := filepath.Rel(root, path); err == nil {
			out = append(out, rel)
		} else {
			out = append(out, path)
		}
		return nil
	})
	return out
}

func matchesExt(lowerPath string, extSet map[string]struct{}) bool {
	for ext := range extSet {
		if strings.HasSuffix(lowerPath, ext) {
			return true
		}
	}
	return false
}

func (v *referenceVerifier) appearsElsewhere(sym *code.Symbol) bool {
	if sym.Name == "" {
		return false
	}
	name := []byte(sym.Name)
	for _, f := range v.files {
		body := v.read(f)
		if len(body) == 0 {
			continue
		}
		if !containsToken(body, name) {
			continue
		}
		if f != sym.FilePath {
			return true
		}
		// Same file as the declaration: count occurrences outside the body lines.
		if extraOccurrenceOutsideBody(body, name, sym) {
			return true
		}
	}
	return false
}

func (v *referenceVerifier) read(relPath string) []byte {
	v.mu.Lock()
	defer v.mu.Unlock()
	if c, ok := v.content[relPath]; ok {
		return c
	}
	abs := relPath
	if !filepath.IsAbs(abs) && v.root != "" {
		abs = filepath.Join(v.root, relPath)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		v.content[relPath] = nil
		return nil
	}
	v.content[relPath] = data
	return data
}

// containsToken reports whether name appears in body with non-identifier
// characters on both sides (a "word" match), so that searching for "Foo"
// does not match inside "FooBar" or "MyFoo".
func containsToken(body, name []byte) bool {
	idx := 0
	for {
		rel := bytesIndex(body[idx:], name)
		if rel < 0 {
			return false
		}
		pos := idx + rel
		if !isIdentChar(prevByte(body, pos)) && !isIdentChar(nextByte(body, pos+len(name))) {
			return true
		}
		idx = pos + 1
	}
}

// extraOccurrenceOutsideBody reports whether the name appears outside the
// symbol's declaration range, line-by-line. EndLine may be 0 or smaller than
// StartLine when the indexer didn't capture a body span; in that case the
// body is treated as just the declaration line.
func extraOccurrenceOutsideBody(body, name []byte, sym *code.Symbol) bool {
	endLine := sym.EndLine
	if endLine < sym.StartLine {
		endLine = sym.StartLine
	}
	line := 1
	lineStart := 0
	for i := 0; i <= len(body); i++ {
		if i == len(body) || (i < len(body) && body[i] == '\n') {
			if line < sym.StartLine || line > endLine {
				if containsToken(body[lineStart:i], name) {
					return true
				}
			}
			line++
			lineStart = i + 1
		}
	}
	return false
}

func bytesIndex(haystack, needle []byte) int {
	if len(needle) == 0 || len(haystack) < len(needle) {
		return -1
	}
	first := needle[0]
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i] != first {
			continue
		}
		match := true
		for j := 1; j < len(needle); j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func prevByte(b []byte, pos int) byte {
	if pos <= 0 {
		return 0
	}
	return b[pos-1]
}

func nextByte(b []byte, pos int) byte {
	if pos >= len(b) {
		return 0
	}
	return b[pos]
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func (s *suppressionScanner) lines(relPath string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if l, ok := s.cache[relPath]; ok {
		return l
	}
	abs := relPath
	if !filepath.IsAbs(abs) && s.root != "" {
		abs = filepath.Join(s.root, relPath)
	}
	f, err := os.Open(abs)
	if err != nil {
		s.cache[relPath] = nil
		return nil
	}
	defer f.Close()

	var out []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		out = append(out, scanner.Text())
	}
	s.cache[relPath] = out
	return out
}
