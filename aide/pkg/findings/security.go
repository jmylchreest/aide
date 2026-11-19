package findings

import (
	"bufio"
	"context"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// SecurityConfig holds configuration for standalone security analysis (CLI).
type SecurityConfig struct {
	// Paths to scan.
	Paths []string
	// ProjectRoot for relative path computation.
	ProjectRoot string
	// ProgressFn is called per file with the number of findings.
	ProgressFn func(path string, findings int)
	// Ignore matcher for skipping files/directories.
	Ignore *aideignore.Matcher
	// MaxFileSize limits files scanned. Zero means DefaultSecurityMaxFileSize.
	MaxFileSize int64
}

// SecurityResult holds summary statistics from a security analysis run.
type SecurityResult struct {
	FilesAnalyzed int
	FilesSkipped  int
	FindingsCount int
	RulesChecked  int
	Duration      time.Duration
}

// compiledSecurityRule holds a pre-compiled security rule ready for matching.
type compiledSecurityRule struct {
	rule grammar.SecurityRule
	re   *regexp.Regexp // Compiled from rule.Pattern (nil when using Query)
}

// securityRuleCache caches compiled rules per language.
var (
	securityRuleCache   = make(map[string][]compiledSecurityRule)
	securityRuleCacheMu sync.RWMutex
)

// getSecurityRules returns compiled security rules for a language from the pack registry.
// Results are cached so regex compilation happens only once per language.
func getSecurityRules(lang string) []compiledSecurityRule {
	securityRuleCacheMu.RLock()
	if cached, ok := securityRuleCache[lang]; ok {
		securityRuleCacheMu.RUnlock()
		return cached
	}
	securityRuleCacheMu.RUnlock()

	securityRuleCacheMu.Lock()
	defer securityRuleCacheMu.Unlock()

	// Double-check after acquiring write lock.
	if cached, ok := securityRuleCache[lang]; ok {
		return cached
	}

	pack := grammar.DefaultPackRegistry().Get(lang)
	if pack == nil || pack.Security == nil || len(pack.Security.Rules) == 0 {
		securityRuleCache[lang] = nil
		return nil
	}

	compiled := make([]compiledSecurityRule, 0, len(pack.Security.Rules))
	for _, rule := range pack.Security.Rules {
		cr := compiledSecurityRule{rule: rule}
		if rule.Pattern != "" {
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				continue // Skip rules with invalid regex
			}
			cr.re = re
		}
		// Rules with Query (tree-sitter) but no Pattern are still added;
		// tree-sitter query matching is handled separately.
		// Rules with neither are skipped.
		if cr.re == nil && rule.Query == "" {
			continue
		}
		compiled = append(compiled, cr)
	}

	securityRuleCache[lang] = compiled
	return compiled
}

// AnalyzeSecurity runs security pattern analysis across all files in the given paths.
// This is the standalone entry point for CLI usage.
func AnalyzeSecurity(cfg SecurityConfig) ([]*Finding, *SecurityResult, error) {
	if len(cfg.Paths) == 0 {
		cfg.Paths = []string{"."}
	}
	maxFileSize := cfg.MaxFileSize
	if maxFileSize <= 0 {
		maxFileSize = DefaultSecurityMaxFileSize
	}

	ignore := cfg.Ignore
	if ignore == nil {
		ignore = aideignore.NewFromDefaults()
	}

	start := time.Now()
	result := &SecurityResult{}
	var allFindings []*Finding

	var err error
	result.FilesAnalyzed, result.FilesSkipped, err = walkSourceFiles(cfg.Paths, ignore, maxFileSize, func(path string, content []byte) {
		relPath := toRelPath(cfg.ProjectRoot, path)
		findings := analyzeFileSecurity(context.Background(), relPath, content)
		allFindings = append(allFindings, findings...)
		if cfg.ProgressFn != nil {
			cfg.ProgressFn(relPath, len(findings))
		}
	})
	if err != nil {
		return nil, nil, err
	}

	result.FindingsCount = len(allFindings)
	result.Duration = time.Since(start)
	return allFindings, result, nil
}

// analyzeFileSecurity scans a single file for security patterns using regex-based rules
// from the language pack.
func analyzeFileSecurity(_ context.Context, filePath string, content []byte) []*Finding {
	lang := code.DetectLanguage(filePath, content)
	if lang == "" {
		return nil
	}

	rules := getSecurityRules(lang)
	if len(rules) == 0 {
		return nil
	}

	var findings []*Finding

	// Regex-based matching: scan line by line
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Skip comments (basic heuristic — not perfect, but avoids flagging
		// commented-out code for most languages).
		if grammar.IsCommentLine(trimmed, lang) {
			continue
		}

		for _, cr := range rules {
			if cr.re == nil {
				continue // Query-only rules are not handled by line scanning
			}

			if cr.re.MatchString(line) {
				severity := cr.rule.Severity
				if severity == "" {
					severity = SevWarning
				}

				finding := &Finding{
					Analyzer: AnalyzerSecurity,
					Severity: severity,
					Category: cr.rule.Category,
					FilePath: filePath,
					Line:     lineNum,
					Title:    cr.rule.Name,
					Detail:   cr.rule.Description,
					Metadata: map[string]string{
						"rule_id":  cr.rule.ID,
						"language": lang,
						"pattern":  cr.rule.Pattern,
					},
					CreatedAt: time.Now(),
				}
				findings = append(findings, finding)
			}
		}
	}

	return findings
}

// isCommentLine returns true if the line appears to be a comment.
// This is a best-effort heuristic to avoid flagging code in comments.
