package findings

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/observe"
)

// todoKeywordSeverity maps the canonical TODO-style keyword to a severity.
var todoKeywordSeverity = map[string]string{
	"TODO":       SevInfo,
	"NOTE":       SevInfo,
	"DEPRECATED": SevInfo,
	"FIXME":      SevWarning,
	"XXX":        SevWarning,
	"HACK":       SevWarning,
	"BUG":        SevCritical,
	"BROKEN":     SevCritical,
}

// todoLine captures keyword, optional owner/ticket in parens, and remaining text.
// Case-sensitive — these keywords are an ALL-CAPS convention; matching "broken" or
// "note" case-insensitively produces endless false positives from ordinary prose.
var todoLine = regexp.MustCompile(
	`\b(TODO|FIXME|XXX|HACK|BUG|BROKEN|NOTE|DEPRECATED)\b(?:\(([^)]*)\))?\s*:?\s*(.*)$`,
)

// TodosConfig holds configuration for standalone TODO analysis (CLI).
type TodosConfig struct {
	Paths       []string
	ProjectRoot string
	Ignore      *aideignore.Matcher
	MaxFileSize int64
	ProgressFn  func(path string, findings int)
}

// TodosResult holds summary statistics from a TODO analysis run.
type TodosResult struct {
	FilesAnalyzed int
	FilesSkipped  int
	FindingsCount int
	Duration      time.Duration
}

// AnalyzeTodos emits one Finding per TODO/FIXME-style comment found in source files.
func AnalyzeTodos(cfg TodosConfig) ([]*Finding, *TodosResult, error) {
	span := observe.Start("AnalyzeTodos", observe.KindSpan).Category("analyzer").Subtype("todos")
	defer span.End()
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
	result := &TodosResult{}
	var allFindings []*Finding

	var err error
	result.FilesAnalyzed, result.FilesSkipped, err = walkSourceFiles(cfg.Paths, ignore, maxFileSize, func(path string, content []byte) {
		relPath := toRelPath(cfg.ProjectRoot, path)
		findings := analyzeFileTodos(relPath, content)
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
	span.Attr("files", strconv.Itoa(result.FilesAnalyzed)).
		Attr("findings", strconv.Itoa(result.FindingsCount))
	return allFindings, result, nil
}

// analyzeFileTodos scans a single file's content for TODO-style comments.
func analyzeFileTodos(filePath string, content []byte) []*Finding {
	lang := code.DetectLanguage(filePath, content)
	var ff []*Finding

	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		raw := scanner.Text()
		commentText := grammar.ExtractCommentText(raw, lang)
		if commentText == "" {
			continue
		}

		m := todoLine.FindStringSubmatch(commentText)
		if m == nil {
			continue
		}

		keyword := strings.ToUpper(m[1])
		owner := strings.TrimSpace(m[2])
		text := strings.TrimSpace(m[3])

		severity, ok := todoKeywordSeverity[keyword]
		if !ok {
			severity = SevInfo
		}

		title := fmt.Sprintf("%s: %s", keyword, summarise(text, 80))
		if text == "" {
			title = keyword
		}

		meta := map[string]string{
			"keyword":  keyword,
			"language": lang,
		}
		if owner != "" {
			meta["owner"] = owner
		}
		if t := extractTicket(text); t != "" {
			meta["ticket"] = t
		}

		ff = append(ff, &Finding{
			Analyzer:  AnalyzerTodos,
			Severity:  severity,
			Category:  strings.ToLower(keyword),
			FilePath:  filePath,
			Line:      lineNum,
			Title:     title,
			Detail:    text,
			Metadata:  meta,
			CreatedAt: time.Now(),
		})
	}
	return ff
}

var ticketPattern = regexp.MustCompile(`\b([A-Z][A-Z0-9]+-\d+)\b`)

func extractTicket(s string) string {
	m := ticketPattern.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

func summarise(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
