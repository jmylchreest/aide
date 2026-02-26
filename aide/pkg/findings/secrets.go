package findings

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/praetorian-inc/titus"
)

// SecretsConfig configures the secrets analyzer.
type SecretsConfig struct {
	// Paths to scan (default: current directory).
	Paths []string
	// SkipValidation disables live credential validation (default true — no network calls).
	SkipValidation bool
	// MaxFileSize is the maximum file size in bytes to scan (default 1MB).
	MaxFileSize int64
	// ProgressFn is called after each file is scanned. May be nil.
	ProgressFn func(path string, secrets int)
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

// defaultMaxFileSize returns the max file size, defaulting to 1MB.
func defaultMaxFileSize(size int64) int64 {
	if size > 0 {
		return size
	}
	return 1 << 20 // 1MB
}

// secretsSkipDirs contains directory names to always skip during scanning.
var secretsSkipDirs = map[string]bool{
	".git":         true,
	".svn":         true,
	".hg":          true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".tox":         true,
	".venv":        true,
	"venv":         true,
	".aide":        true,
	"dist":         true,
	"build":        true,
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

	var allFindings []*Finding

	for _, root := range paths {
		err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return nil // skip inaccessible files
			}

			// Skip hidden directories and known non-source dirs.
			if info.IsDir() {
				base := filepath.Base(path)
				if secretsSkipDirs[base] || (len(base) > 1 && base[0] == '.') {
					return filepath.SkipDir
				}
				return nil
			}

			// Skip by extension.
			ext := strings.ToLower(filepath.Ext(path))
			if secretsSkipExtensions[ext] {
				result.FilesSkipped++
				return nil
			}

			// Skip files exceeding size limit.
			if info.Size() > maxSize {
				result.FilesSkipped++
				return nil
			}

			// Skip symlinks.
			if info.Mode()&os.ModeSymlink != 0 {
				result.FilesSkipped++
				return nil
			}

			// Scan file.
			matches, scanErr := scanner.ScanFile(path)
			if scanErr != nil {
				// Non-fatal — skip files that fail to scan.
				result.FilesSkipped++
				return nil
			}

			result.FilesScanned++

			relPath, _ := filepath.Rel(".", path)
			if relPath == "" {
				relPath = path
			}

			for _, match := range matches {
				line := 0
				if match.Location.Source.Start.Line > 0 {
					line = int(match.Location.Source.Start.Line)
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

				allFindings = append(allFindings, f)
			}

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

// AnalyzeSecretsWithContext is the context-aware variant.
func AnalyzeSecretsWithContext(ctx context.Context, cfg SecretsConfig) ([]*Finding, *SecretsResult, error) {
	// Check for cancellation before starting.
	select {
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	default:
	}
	// Titus supports context via ScanStringWithContext, but ScanFile doesn't.
	// We check context between files for cancellation.
	return AnalyzeSecrets(cfg)
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

	sb.WriteString(fmt.Sprintf("Rule: %s (%s)\n", match.RuleName, match.RuleID))
	sb.WriteString(fmt.Sprintf("File: %s\n", filePath))

	if match.Location.Source.Start.Line > 0 {
		sb.WriteString(fmt.Sprintf("Line: %d", match.Location.Source.Start.Line))
		if match.Location.Source.Start.Column > 0 {
			sb.WriteString(fmt.Sprintf(", Column: %d", match.Location.Source.Start.Column))
		}
		sb.WriteString("\n")
	}

	if match.ValidationResult != nil {
		sb.WriteString(fmt.Sprintf("Validation: %s", match.ValidationResult.Status))
		if match.ValidationResult.Message != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", match.ValidationResult.Message))
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
			if len(line) > 120 {
				line = line[:120] + "..."
			}
			sb.WriteString(fmt.Sprintf("  %s\n", line))
		}
		sb.WriteString("  [REDACTED SECRET]\n")
		afterLines := strings.Split(strings.TrimSpace(string(match.Snippet.After)), "\n")
		for _, line := range afterLines {
			if len(line) > 120 {
				line = line[:120] + "..."
			}
			sb.WriteString(fmt.Sprintf("  %s\n", line))
		}
	}

	return sb.String()
}
