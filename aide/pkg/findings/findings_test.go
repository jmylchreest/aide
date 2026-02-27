package findings

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

// testdataDir returns the absolute path to the testdata directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("failed to resolve testdata dir: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("testdata directory does not exist: %s", dir)
	}
	return dir
}

// =============================================================================
// Complexity Analyzer Tests
// =============================================================================

func TestComplexityAnalyzer_HighComplexity(t *testing.T) {
	dir := testdataDir(t)

	findings, result, err := AnalyzeComplexity(ComplexityConfig{
		Threshold: 10,
		Paths:     []string{filepath.Join(dir, "complex_high.go")},
	})
	if err != nil {
		t.Fatalf("AnalyzeComplexity error: %v", err)
	}

	if result.FilesAnalyzed == 0 {
		t.Fatal("expected at least 1 file analyzed, got 0")
	}

	// We expect at least 2 findings: HighComplexity (critical) and ModerateComplexity (warning).
	// SimpleFunction should NOT appear (complexity < threshold).
	if len(findings) < 2 {
		t.Fatalf("expected at least 2 findings, got %d", len(findings))
	}

	var foundHigh, foundModerate bool
	for _, f := range findings {
		if f.Analyzer != AnalyzerComplexity {
			t.Errorf("unexpected analyzer %q (expected %q)", f.Analyzer, AnalyzerComplexity)
		}

		name := f.Metadata["function"]
		switch name {
		case "HighComplexity":
			foundHigh = true
			if f.Severity != SevCritical {
				t.Errorf("HighComplexity: expected severity %q, got %q", SevCritical, f.Severity)
			}
		case "ModerateComplexity":
			foundModerate = true
			if f.Severity != SevWarning {
				t.Errorf("ModerateComplexity: expected severity %q, got %q", SevWarning, f.Severity)
			}
		case "SimpleFunction":
			t.Error("SimpleFunction should NOT be flagged (below threshold)")
		}
	}

	if !foundHigh {
		t.Error("expected finding for HighComplexity function")
	}
	if !foundModerate {
		t.Error("expected finding for ModerateComplexity function")
	}
}

func TestComplexityAnalyzer_CustomThreshold(t *testing.T) {
	dir := testdataDir(t)

	// With a very high threshold, nothing should be flagged.
	findings, _, err := AnalyzeComplexity(ComplexityConfig{
		Threshold: 100,
		Paths:     []string{filepath.Join(dir, "complex_high.go")},
	})
	if err != nil {
		t.Fatalf("AnalyzeComplexity error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings with threshold 100, got %d", len(findings))
	}
}

func TestComplexityAnalyzer_LowThreshold(t *testing.T) {
	dir := testdataDir(t)

	// With threshold 2, even SimpleFunction (complexity ~2) may be flagged.
	findings, _, err := AnalyzeComplexity(ComplexityConfig{
		Threshold: 2,
		Paths:     []string{filepath.Join(dir, "complex_high.go")},
	})
	if err != nil {
		t.Fatalf("AnalyzeComplexity error: %v", err)
	}

	// All 3 functions should appear.
	if len(findings) < 3 {
		t.Errorf("expected at least 3 findings with threshold 2, got %d", len(findings))
	}
}

func TestComplexityAnalyzer_FindingFields(t *testing.T) {
	dir := testdataDir(t)

	findings, _, err := AnalyzeComplexity(ComplexityConfig{
		Threshold: 10,
		Paths:     []string{filepath.Join(dir, "complex_high.go")},
	})
	if err != nil {
		t.Fatalf("AnalyzeComplexity error: %v", err)
	}

	for _, f := range findings {
		// Verify required fields are populated
		if f.Analyzer == "" {
			t.Error("finding has empty Analyzer")
		}
		if f.Severity == "" {
			t.Error("finding has empty Severity")
		}
		if f.FilePath == "" {
			t.Error("finding has empty FilePath")
		}
		if f.Line == 0 {
			t.Error("finding has zero Line")
		}
		if f.Title == "" {
			t.Error("finding has empty Title")
		}
		if f.Detail == "" {
			t.Error("finding has empty Detail")
		}
		if f.Metadata["complexity"] == "" {
			t.Error("finding missing complexity metadata")
		}
		if f.Metadata["function"] == "" {
			t.Error("finding missing function metadata")
		}
		if f.Metadata["language"] == "" {
			t.Error("finding missing language metadata")
		}
		if f.CreatedAt.IsZero() {
			t.Error("finding has zero CreatedAt")
		}
	}
}

func TestComplexityAnalyzer_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-complexity-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	findings, result, err := AnalyzeComplexity(ComplexityConfig{
		Threshold: 10,
		Paths:     []string{tmpDir},
	})
	if err != nil {
		t.Fatalf("AnalyzeComplexity error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty dir, got %d", len(findings))
	}
	if result.FilesAnalyzed != 0 {
		t.Errorf("expected 0 files analyzed for empty dir, got %d", result.FilesAnalyzed)
	}
}

// =============================================================================
// Coupling Analyzer Tests
// =============================================================================

func TestCouplingAnalyzer_HighFanOut(t *testing.T) {
	dir := testdataDir(t)

	findings, result, err := AnalyzeCoupling(CouplingConfig{
		FanOutThreshold: 15,
		FanInThreshold:  20,
		Paths:           []string{dir},
	})
	if err != nil {
		t.Fatalf("AnalyzeCoupling error: %v", err)
	}

	if result.FilesAnalyzed == 0 {
		t.Fatal("expected at least 1 file analyzed")
	}

	// coupling_high_fanout.go has 20 imports — should trigger fan-out warning.
	var foundFanOut bool
	for _, f := range findings {
		if f.Analyzer != AnalyzerCoupling {
			t.Errorf("unexpected analyzer %q", f.Analyzer)
		}
		if f.Category == "fan-out" {
			foundFanOut = true
			if f.Severity != SevWarning && f.Severity != SevCritical {
				t.Errorf("expected severity warning or critical, got %q", f.Severity)
			}
		}
	}

	if !foundFanOut {
		t.Errorf("expected at least one fan-out finding; got %d total findings: %v", len(findings), findingSummary(findings))
	}
}

func TestCouplingAnalyzer_ThresholdAbove(t *testing.T) {
	dir := testdataDir(t)

	// With a very high fan-out threshold, no findings expected.
	findings, _, err := AnalyzeCoupling(CouplingConfig{
		FanOutThreshold: 100,
		FanInThreshold:  100,
		Paths:           []string{dir},
	})
	if err != nil {
		t.Fatalf("AnalyzeCoupling error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings with high thresholds, got %d", len(findings))
	}
}

func TestCouplingAnalyzer_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-coupling-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	findings, _, err := AnalyzeCoupling(CouplingConfig{
		Paths: []string{tmpDir},
	})
	if err != nil {
		t.Fatalf("AnalyzeCoupling error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty dir, got %d", len(findings))
	}
}

// =============================================================================
// Secrets Analyzer Tests
// =============================================================================

func TestSecretsAnalyzer_EmbeddedSecrets(t *testing.T) {
	dir := testdataDir(t)

	findings, result, err := AnalyzeSecrets(SecretsConfig{
		Paths:          []string{filepath.Join(dir, "secrets_embedded.go")},
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeSecrets error: %v", err)
	}

	if result.FilesScanned == 0 {
		t.Fatal("expected at least 1 file scanned")
	}

	// We embedded ~9 distinct secrets. The scanner should find at least some.
	if len(findings) == 0 {
		t.Fatal("expected at least 1 secret finding, got 0")
	}

	for _, f := range findings {
		if f.Analyzer != AnalyzerSecrets {
			t.Errorf("unexpected analyzer %q (expected %q)", f.Analyzer, AnalyzerSecrets)
		}
		if f.Severity == "" {
			t.Error("finding has empty severity")
		}
		if f.FilePath == "" {
			t.Error("finding has empty file path")
		}
		// Note: Titus ScanFile may not provide line numbers (returns 0).
		// This is a known limitation — line numbers are best-effort.
		if f.Title == "" {
			t.Error("finding has empty title")
		}
	}

	// Log what was found for debugging.
	t.Logf("Found %d secret findings:", len(findings))
	for _, f := range findings {
		t.Logf("  [%s] line %d: %s (category: %s)", f.Severity, f.Line, f.Title, f.Category)
	}
}

func TestSecretsAnalyzer_CleanFile(t *testing.T) {
	dir := testdataDir(t)

	// complex_high.go has no secrets — should return 0 findings.
	findings, _, err := AnalyzeSecrets(SecretsConfig{
		Paths:          []string{filepath.Join(dir, "complex_high.go")},
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeSecrets error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for clean file, got %d", len(findings))
		for _, f := range findings {
			t.Logf("  unexpected: line %d: %s", f.Line, f.Title)
		}
	}
}

func TestSecretsAnalyzer_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-secrets-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	findings, _, err := AnalyzeSecrets(SecretsConfig{
		Paths:          []string{tmpDir},
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeSecrets error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty dir, got %d", len(findings))
	}
}

func TestSecretsAnalyzer_FindingFields(t *testing.T) {
	dir := testdataDir(t)

	findings, _, err := AnalyzeSecrets(SecretsConfig{
		Paths:          []string{filepath.Join(dir, "secrets_embedded.go")},
		SkipValidation: true,
	})
	if err != nil {
		t.Fatalf("AnalyzeSecrets error: %v", err)
	}

	for _, f := range findings {
		if f.Analyzer != AnalyzerSecrets {
			t.Errorf("unexpected analyzer: %s", f.Analyzer)
		}
		if f.Category == "" {
			t.Error("secret finding has empty category")
		}
		if f.CreatedAt.IsZero() {
			t.Error("finding has zero CreatedAt")
		}
	}
}

// =============================================================================
// Helpers
// =============================================================================

// findingSummary returns a human-readable summary of findings (for error messages).
func findingSummary(ff []*Finding) []string {
	ss := make([]string, 0, len(ff))
	for _, f := range ff {
		ss = append(ss, f.Severity+":"+f.Category+":"+f.FilePath)
	}
	return ss
}

// =============================================================================
// Pack-based import extraction — verify parity with hardcoded extractors
// =============================================================================

func TestExtractImportsFromPack_Go(t *testing.T) {
	content := []byte(`package main

import "fmt"

import (
	"os"
	"strings"
)

func main() {}
`)
	reg := grammar.DefaultPackRegistry()
	pack := reg.Get("go")
	if pack == nil || pack.Imports == nil {
		t.Fatal("go pack has no imports config")
	}

	got := extractImportsFromPack(content, "go", pack.Imports)
	want := []string{"fmt", "os", "strings"}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("go imports: got %v, want %v", got, want)
	}
}

func TestExtractImportsFromPack_Python(t *testing.T) {
	content := []byte(`import os
from pathlib import Path
import sys
`)
	reg := grammar.DefaultPackRegistry()
	pack := reg.Get("python")
	if pack == nil || pack.Imports == nil {
		t.Fatal("python pack has no imports config")
	}

	got := extractImportsFromPack(content, "python", pack.Imports)
	want := []string{"os", "pathlib", "sys"}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("python imports: got %v, want %v", got, want)
	}
}

func TestExtractImportsFromPack_TypeScript(t *testing.T) {
	content := []byte(`import { foo } from './foo';
import bar from 'bar';
const x = require('baz');
export { thing } from '@scope/pkg';
`)
	reg := grammar.DefaultPackRegistry()
	pack := reg.Get("typescript")
	if pack == nil || pack.Imports == nil {
		t.Fatal("typescript pack has no imports config")
	}

	got := extractImportsFromPack(content, "typescript", pack.Imports)
	want := []string{"./foo", "bar", "baz", "@scope/pkg"}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("typescript imports: got %v, want %v", got, want)
	}
}

func TestExtractImportsFromPack_Java(t *testing.T) {
	content := []byte(`import java.util.List;
import static org.junit.Assert.assertEquals;
`)
	reg := grammar.DefaultPackRegistry()
	pack := reg.Get("java")
	if pack == nil || pack.Imports == nil {
		t.Fatal("java pack has no imports config")
	}

	got := extractImportsFromPack(content, "java", pack.Imports)
	want := []string{"java.util.List", "org.junit.Assert.assertEquals"}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("java imports: got %v, want %v", got, want)
	}
}

func TestExtractImportsFromPack_Rust(t *testing.T) {
	content := []byte(`use std::collections::HashMap;
use crate::config;
`)
	reg := grammar.DefaultPackRegistry()
	pack := reg.Get("rust")
	if pack == nil || pack.Imports == nil {
		t.Fatal("rust pack has no imports config")
	}

	got := extractImportsFromPack(content, "rust", pack.Imports)
	want := []string{"std::collections::HashMap", "config"}
	sort.Strings(got)
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("rust imports: got %v, want %v", got, want)
	}
}

func TestSeverityRank(t *testing.T) {
	tests := []struct {
		sev  string
		want int
	}{
		{SevInfo, 0},
		{SevWarning, 1},
		{SevCritical, 2},
		{"", -1},
		{"unknown", -1},
	}
	for _, tt := range tests {
		got := SeverityRank(tt.sev)
		if got != tt.want {
			t.Errorf("SeverityRank(%q) = %d, want %d", tt.sev, got, tt.want)
		}
	}

	// Verify ordering: info < warning < critical.
	if SeverityRank(SevInfo) >= SeverityRank(SevWarning) {
		t.Error("info should rank below warning")
	}
	if SeverityRank(SevWarning) >= SeverityRank(SevCritical) {
		t.Error("warning should rank below critical")
	}
}
