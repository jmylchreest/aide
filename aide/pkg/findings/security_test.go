package findings

import (
	"context"
	"path/filepath"
	"testing"
)

// =============================================================================
// Security Analyzer Tests
// =============================================================================

func TestSecurityAnalyzer_GoFile(t *testing.T) {
	dir := testdataDir(t)

	findings, result, err := AnalyzeSecurity(SecurityConfig{
		Paths: []string{filepath.Join(dir, "security_go.go")},
	})
	if err != nil {
		t.Fatalf("AnalyzeSecurity error: %v", err)
	}

	if result.FilesAnalyzed == 0 {
		t.Fatal("expected at least 1 file analyzed, got 0")
	}

	if len(findings) == 0 {
		t.Fatal("expected security findings for test file with known vulnerabilities")
	}

	// Check that all findings have correct analyzer
	for _, f := range findings {
		if f.Analyzer != AnalyzerSecurity {
			t.Errorf("unexpected analyzer %q (expected %q)", f.Analyzer, AnalyzerSecurity)
		}
	}

	// Verify specific patterns are detected
	foundPatterns := make(map[string]bool)
	for _, f := range findings {
		foundPatterns[f.Metadata["rule_id"]] = true
	}

	expectedRules := []string{
		"go-command-injection", // exec.Command
		"go-command-shell",     // exec.Command("bash"...)
		"go-weak-crypto-md5",   // md5.Sum
		"go-tls-insecure-skip", // InsecureSkipVerify: true
	}

	for _, rule := range expectedRules {
		if !foundPatterns[rule] {
			t.Errorf("expected rule %q to fire but it did not", rule)
		}
	}

	// Check categories are set correctly
	for _, f := range findings {
		if f.Category == "" {
			t.Errorf("finding %q has empty category", f.Title)
		}
	}
}

func TestSecurityAnalyzer_PythonFile(t *testing.T) {
	dir := testdataDir(t)

	findings, result, err := AnalyzeSecurity(SecurityConfig{
		Paths: []string{filepath.Join(dir, "security_py.py")},
	})
	if err != nil {
		t.Fatalf("AnalyzeSecurity error: %v", err)
	}

	if result.FilesAnalyzed == 0 {
		t.Fatal("expected at least 1 file analyzed, got 0")
	}

	if len(findings) == 0 {
		t.Fatal("expected security findings for Python test file")
	}

	foundPatterns := make(map[string]bool)
	for _, f := range findings {
		foundPatterns[f.Metadata["rule_id"]] = true
	}

	expectedRules := []string{
		"py-eval",               // eval()
		"py-pickle-deserialize", // pickle.loads
		"py-weak-hash",          // hashlib.md5
	}

	for _, rule := range expectedRules {
		if !foundPatterns[rule] {
			t.Errorf("expected rule %q to fire but it did not", rule)
		}
	}
}

func TestSecurityAnalyzer_CommentSkipping(t *testing.T) {
	dir := testdataDir(t)

	findings, _, err := AnalyzeSecurity(SecurityConfig{
		Paths: []string{filepath.Join(dir, "security_py.py")},
	})
	if err != nil {
		t.Fatalf("AnalyzeSecurity error: %v", err)
	}

	// Comments mentioning eval() and pickle.loads should NOT trigger findings
	for _, f := range findings {
		if f.Line >= 46 { // Comment lines are at end of file
			t.Errorf("finding at line %d appears to be from a comment: %s", f.Line, f.Title)
		}
	}
}

func TestSecurityAnalyzer_NoRules(t *testing.T) {
	// Test with a file type that has no security rules (e.g., JSON)
	findings := analyzeFileSecurity(context.Background(), "test.json", []byte(`{"key": "value"}`))
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for JSON file, got %d", len(findings))
	}
}

func TestSecurityAnalyzer_EmptyFile(t *testing.T) {
	findings := analyzeFileSecurity(context.Background(), "test.go", []byte{})
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty file, got %d", len(findings))
	}
}

func TestSecurityAnalyzer_SeverityLevels(t *testing.T) {
	dir := testdataDir(t)

	findings, _, err := AnalyzeSecurity(SecurityConfig{
		Paths: []string{filepath.Join(dir, "security_go.go")},
	})
	if err != nil {
		t.Fatalf("AnalyzeSecurity error: %v", err)
	}

	// Verify severity levels are valid
	validSeverities := map[string]bool{
		SevCritical: true,
		SevWarning:  true,
		SevInfo:     true,
	}

	for _, f := range findings {
		if !validSeverities[f.Severity] {
			t.Errorf("finding %q has invalid severity %q", f.Title, f.Severity)
		}
	}
}

func TestSecurityAnalyzer_CommandSeverityCalibration(t *testing.T) {
	dir := testdataDir(t)

	findings, _, err := AnalyzeSecurity(SecurityConfig{
		Paths: []string{filepath.Join(dir, "security_go.go")},
	})
	if err != nil {
		t.Fatalf("AnalyzeSecurity error: %v", err)
	}

	severityByRule := make(map[string]string)
	for _, f := range findings {
		severityByRule[f.Metadata["rule_id"]] = f.Severity
	}

	if got := severityByRule["go-command-injection"]; got != SevWarning {
		t.Errorf("go-command-injection severity = %q, want %q", got, SevWarning)
	}
	if got := severityByRule["go-command-shell"]; got != SevCritical {
		t.Errorf("go-command-shell severity = %q, want %q", got, SevCritical)
	}
}

func TestSecurityAnalyzer_RuleMetadata(t *testing.T) {
	dir := testdataDir(t)

	findings, _, err := AnalyzeSecurity(SecurityConfig{
		Paths: []string{filepath.Join(dir, "security_go.go")},
	})
	if err != nil {
		t.Fatalf("AnalyzeSecurity error: %v", err)
	}

	for _, f := range findings {
		if f.Metadata["rule_id"] == "" {
			t.Errorf("finding %q has empty rule_id metadata", f.Title)
		}
		if f.Metadata["language"] == "" {
			t.Errorf("finding %q has empty language metadata", f.Title)
		}
	}
}

func TestIsCommentLine(t *testing.T) {
	tests := []struct {
		line string
		lang string
		want bool
	}{
		{"// this is a comment", "go", true},
		{"/* block comment", "go", true},
		{"* continuation", "go", true},
		{"code := 1", "go", false},
		{"# this is a comment", "python", true},
		{"code = 1", "python", false},
		{"# comment", "ruby", true},
		{"puts 'hello'", "ruby", false},
	}

	for _, tt := range tests {
		got := isCommentLine(tt.line, tt.lang)
		if got != tt.want {
			t.Errorf("isCommentLine(%q, %q) = %v, want %v", tt.line, tt.lang, got, tt.want)
		}
	}
}
