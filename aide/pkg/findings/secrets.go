package findings

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/praetorian-inc/titus"
)

// SecretsConfig configures the secrets analyzer.
type SecretsConfig struct {
	// Paths to scan (default: current directory).
	Paths []string
	// ProjectRoot anchors ignore matching and finding paths when scanning individual files.
	ProjectRoot string
	// SkipValidation disables live credential validation (default true — no network calls).
	SkipValidation bool
	// MaxFileSize is the maximum file size in bytes to scan (default 1MB).
	MaxFileSize int64
	// ProgressFn is called after each file is scanned. May be nil.
	ProgressFn func(path string, secrets int)
	// Ignore is the aideignore matcher for filtering files/directories.
	// If nil, built-in defaults are used.
	Ignore *aideignore.Matcher
}

// SecretsResult holds the output of a secrets analysis run.
type SecretsResult struct {
	FilesScanned  int
	FilesSkipped  int
	FindingsCount int
	RulesLoaded   int
	Duration      time.Duration
}

// defaultSecretsPaths returns fallback paths when none are configured.
func defaultSecretsPaths(paths []string) []string {
	if len(paths) > 0 {
		return paths
	}
	return []string{"."}
}

// defaultMaxFileSize returns the max file size, defaulting to DefaultSecretsMaxFileSize.
func defaultMaxFileSize(size int64) int64 {
	if size > 0 {
		return size
	}
	return DefaultSecretsMaxFileSize
}

// secretsSkipExtensions contains file extensions to skip (binary/large files).
var secretsSkipExtensions = map[string]bool{
	// Binary / compiled
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".o": true, ".a": true,
	".pyc": true, ".pyo": true, ".class": true, ".jar": true, ".war": true,
	// Archives
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".xz": true, ".7z": true, ".rar": true,
	// Media
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".bmp": true, ".ico": true,
	".svg": true, ".mp3": true, ".mp4": true, ".avi": true, ".mov": true, ".wav": true,
	".woff": true, ".woff2": true, ".ttf": true, ".eot": true, ".otf": true,
	// Data / large
	".db": true, ".sqlite": true, ".sqlite3": true, ".bleve": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	// Lock files (unlikely to contain secrets, very large)
	".lock": true,
}

// AnalyzeSecrets scans files for hardcoded secrets using Titus.
// It returns the findings, a result summary, and any error.
func AnalyzeSecrets(cfg SecretsConfig) ([]*Finding, *SecretsResult, error) {
	start := time.Now()
	paths := defaultSecretsPaths(cfg.Paths)
	maxSize := defaultMaxFileSize(cfg.MaxFileSize)

	result := &SecretsResult{}

	// Create Titus scanner — no validation by default (no network calls).
	var opts []titus.Option
	if !cfg.SkipValidation {
		opts = append(opts, titus.WithValidation())
	}

	scanner, err := titus.NewScanner(opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create secrets scanner: %w", err)
	}
	defer scanner.Close()

	result.RulesLoaded = scanner.RuleCount()

	ignore := cfg.Ignore
	if ignore == nil {
		ignore = aideignore.NewFromDefaults()
	}

	var allFindings []*Finding

	for _, root := range paths {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, nil, fmt.Errorf("abs path %s: %w", root, err)
		}
		ignoreRoot := cfg.ProjectRoot
		if ignoreRoot == "" {
			ignoreRoot = absRoot
		}
		shouldSkip := ignore.WalkFunc(ignoreRoot)

		err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil // skip inaccessible files
			}

			// Skip ignored directories and files.
			if skip, skipDir := shouldSkip(path, info); skip {
				if skipDir {
					return filepath.SkipDir
				}
				result.FilesSkipped++
				return nil
			}
			if info.IsDir() {
				return nil
			}

			if !secretsEligible(path, info, maxSize) {
				result.FilesSkipped++
				return nil
			}
			content, readErr := readAnalysisFile(path, maxSize)
			if readErr != nil {
				result.FilesSkipped++
				return nil
			}
			matches, scanErr := scanner.ScanBytes(content)
			if scanErr != nil {
				result.FilesSkipped++
				return nil
			}

			result.FilesScanned++

			pathRoot := cfg.ProjectRoot
			if pathRoot == "" {
				pathRoot, _ = os.Getwd()
			}
			relPath := toRelPath(pathRoot, path)
			if relPath == "" {
				relPath = path
			}

			allFindings = append(allFindings, secretFindings(matches, relPath)...)

			if cfg.ProgressFn != nil {
				cfg.ProgressFn(relPath, len(matches))
			}

			return nil
		})
		if err != nil {
			return nil, nil, fmt.Errorf("error walking %s: %w", root, err)
		}
	}

	result.FindingsCount = len(allFindings)
	result.Duration = time.Since(start)

	return allFindings, result, nil
}

// secretsEligible is shared by standalone and incremental analysis. It deliberately
// preserves the existing extension exclusions; relevance is independent of grammars.
func secretsEligible(path string, info os.FileInfo, maxSize int64) bool {
	return info != nil && info.Mode().IsRegular() && info.Size() <= maxSize &&
		!secretsSkipExtensions[strings.ToLower(filepath.Ext(path))]
}

var errAnalysisSkipped = errors.New("file is not a regular file or exceeds analysis limit")

// readAnalysisFile bounds allocation even if a file grows after the stat check.
// Symlinks and special files are skipped rather than followed or blocked on.
func readAnalysisFile(path string, maxSize int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || (maxSize > 0 && info.Size() > maxSize) {
		return nil, errAnalysisSkipped
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || (maxSize > 0 && info.Size() > maxSize) {
		return nil, errAnalysisSkipped
	}
	var reader io.Reader = f
	if maxSize > 0 {
		reader = io.LimitReader(f, maxSize+1)
	}
	content, err := io.ReadAll(reader)
	if maxSize > 0 && int64(len(content)) > maxSize {
		return nil, errAnalysisSkipped
	}
	return content, err
}

func secretFindings(matches []*titus.Match, relPath string) []*Finding {
	findings := make([]*Finding, 0, len(matches))
	for _, match := range matches {
		line := 0
		if match.Location.Source.Start.Line > 0 {
			line = match.Location.Source.Start.Line
		}

		severity := SevWarning
		// Elevated severity for validated active secrets.
		if match.ValidationResult != nil && match.ValidationResult.Status == titus.StatusValid {
			severity = SevCritical
		}

		category := categorizeSecretRule(match.RuleID)

		metadata := map[string]string{
			"rule_id":   match.RuleID,
			"rule_name": match.RuleName,
		}
		if match.ValidationResult != nil {
			metadata["validation"] = string(match.ValidationResult.Status)
		}
		if line > 0 {
			metadata["line"] = strconv.Itoa(line)
		}

		f := &Finding{
			Analyzer:  AnalyzerSecrets,
			Severity:  severity,
			Category:  category,
			FilePath:  relPath,
			Line:      line,
			Title:     fmt.Sprintf("Potential secret: %s", match.RuleName),
			Detail:    buildSecretDetail(match, relPath),
			Metadata:  metadata,
			CreatedAt: time.Now(),
		}

		findings = append(findings, f)
	}
	return findings
}

// secretsBatch owns one lazy scanner for a batch, and releases it after every
// scheduled secrets task has finished (including cancellation before execution).
// Serial matching keeps scanner working memory bounded independently of file count.
type secretsBatch struct {
	mu        sync.Mutex
	remaining int
	scanner   *titus.Scanner
}

func (b *secretsBatch) scan(ctx context.Context, content []byte, path string) ([]*Finding, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if b.scanner == nil {
		var err error
		b.scanner, err = titus.NewScanner()
		if err != nil {
			return nil, fmt.Errorf("create secrets scanner: %w", err)
		}
	}
	matches, err := b.scanner.ScanBytesWithContext(ctx, content)
	if err != nil {
		return nil, err
	}
	return secretFindings(matches, path), nil
}

func (b *secretsBatch) release() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.remaining--
	if b.remaining == 0 && b.scanner != nil {
		_ = b.scanner.Close()
		b.scanner = nil
	}
}

// categorizeSecretRule maps a Titus rule ID prefix to a human-readable category.
func categorizeSecretRule(ruleID string) string {
	// Titus uses NoseyParker rule IDs like "np.aws.1", "np.github.1", etc.
	parts := strings.SplitN(ruleID, ".", 3)
	if len(parts) < 2 {
		return "generic"
	}

	switch parts[1] {
	case "aws":
		return "aws"
	case "azure":
		return "azure"
	case "gcp":
		return "gcp"
	case "github":
		return "github"
	case "gitlab":
		return "gitlab"
	case "slack":
		return "slack"
	case "stripe":
		return "stripe"
	case "twilio":
		return "twilio"
	case "sendgrid":
		return "sendgrid"
	case "npm":
		return "npm"
	case "pypi":
		return "pypi"
	case "docker", "dockerhub":
		return "docker"
	case "heroku":
		return "heroku"
	case "ssh":
		return "ssh"
	case "pem", "rsa":
		return "crypto_key"
	case "jwt":
		return "jwt"
	case "generic":
		return "generic"
	default:
		return parts[1]
	}
}

// buildSecretDetail generates a detailed explanation for a secret finding.
func buildSecretDetail(match *titus.Match, filePath string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "Rule: %s (%s)\n", match.RuleName, match.RuleID)
	fmt.Fprintf(&sb, "File: %s\n", filePath)

	if match.Location.Source.Start.Line > 0 {
		fmt.Fprintf(&sb, "Line: %d", match.Location.Source.Start.Line)
		if match.Location.Source.Start.Column > 0 {
			fmt.Fprintf(&sb, ", Column: %d", match.Location.Source.Start.Column)
		}
		sb.WriteString("\n")
	}

	if match.ValidationResult != nil {
		fmt.Fprintf(&sb, "Validation: %s", match.ValidationResult.Status)
		if match.ValidationResult.Message != "" {
			fmt.Fprintf(&sb, " (%s)", match.ValidationResult.Message)
		}
		sb.WriteString("\n")
	}

	// Show a redacted snippet context (avoid leaking the actual secret).
	if len(match.Snippet.Before) > 0 {
		sb.WriteString("\nContext:\n")
		sb.WriteString("  ...")
		// Only show context lines, NOT the matching line (which contains the secret).
		beforeLines := strings.Split(strings.TrimSpace(string(match.Snippet.Before)), "\n")
		for _, line := range beforeLines {
			if len(line) > DefaultSnippetTruncateLen {
				line = line[:DefaultSnippetTruncateLen] + "..."
			}
			fmt.Fprintf(&sb, "  %s\n", line)
		}
		sb.WriteString("  [REDACTED SECRET]\n")
		afterLines := strings.Split(strings.TrimSpace(string(match.Snippet.After)), "\n")
		for _, line := range afterLines {
			if len(line) > DefaultSnippetTruncateLen {
				line = line[:DefaultSnippetTruncateLen] + "..."
			}
			fmt.Fprintf(&sb, "  %s\n", line)
		}
	}

	return sb.String()
}
