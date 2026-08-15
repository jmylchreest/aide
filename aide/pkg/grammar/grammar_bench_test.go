package grammar

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
)

// All fixtures in this file are synthetic file trees written into
// b.TempDir(). The loader is built with auto-download disabled and a
// scratch grammar dir, so no network access and no real project tree is
// ever touched.

// benchLangDetect is an extension-based LanguageDetector mirroring the one
// used by TestScanProject.
func benchLangDetect(filePath string, _ []byte) string {
	switch filepath.Ext(filePath) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts":
		return "typescript"
	case ".js":
		return "javascript"
	case ".rs":
		return "rust"
	case ".json":
		return "json"
	}
	return ""
}

// buildBenchScanTree writes a synthetic project tree of ~300 files across
// several languages and nested directories into b.TempDir().
func buildBenchScanTree(b *testing.B) string {
	b.Helper()
	root := b.TempDir()

	specs := []struct {
		dir   string
		ext   string
		count int
		body  string
	}{
		{"src/backend", ".go", 60, "package backend\n\nfunc Work() {}\n"},
		{"src/frontend", ".ts", 40, "export const v = 1;\n"},
		{"src/scripts", ".py", 40, "def main():\n    pass\n"},
		{"src/web", ".js", 30, "module.exports = {};\n"},
		{"src/core", ".go", 60, "package core\n\nfunc Core() {}\n"},
		{"src/native", ".rs", 20, "fn main() {}\n"},
		{"src/types", ".ts", 20, "export type T = number;\n"},
		{"config", ".json", 10, "{}\n"},
	}

	for _, spec := range specs {
		dir := filepath.Join(root, spec.dir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatalf("MkdirAll: %v", err)
		}
		for i := 0; i < spec.count; i++ {
			name := fmt.Sprintf("file%03d%s", i, spec.ext)
			if err := os.WriteFile(filepath.Join(dir, name), []byte(spec.body), 0o644); err != nil {
				b.Fatalf("WriteFile: %v", err)
			}
		}
	}
	return root
}

// newBenchLoader builds a CompositeLoader rooted at the scratch tree with
// auto-download disabled (no network).
func newBenchLoader(root string) *CompositeLoader {
	return NewCompositeLoader(
		WithAutoDownload(false),
		WithGrammarDir(filepath.Join(root, ".aide", "grammars")),
	)
}

var benchScanSink *ScanResult
var benchStatusSink []LanguageStatus
var benchLangSink string

// BenchmarkScanProject measures the full project scan: directory walk,
// marker detection, per-file language detection, and grammar classification.
func BenchmarkScanProject(b *testing.B) {
	root := buildBenchScanTree(b)
	loader := newBenchLoader(root)
	ignore := aideignore.NewFromDefaults()

	// Sanity-check the fixture once.
	res, err := ScanProject(root, loader, benchLangDetect, ignore)
	if err != nil || res == nil || res.TotalFiles == 0 {
		b.Fatalf("fixture check failed: res=%v err=%v", res, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := ScanProject(root, loader, benchLangDetect, ignore)
		if err != nil {
			b.Fatalf("ScanProject: %v", err)
		}
		benchScanSink = got
	}
}

// BenchmarkScanDetail measures ScanProject plus the per-language status
// rollup used by `aide grammar status`.
func BenchmarkScanDetail(b *testing.B) {
	root := buildBenchScanTree(b)
	loader := newBenchLoader(root)
	ignore := aideignore.NewFromDefaults()

	// Sanity-check the fixture once.
	statuses, err := ScanDetail(root, loader, benchLangDetect, ignore)
	if err != nil || len(statuses) == 0 {
		b.Fatalf("fixture check failed: statuses=%v err=%v", statuses, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		got, err := ScanDetail(root, loader, benchLangDetect, ignore)
		if err != nil {
			b.Fatalf("ScanDetail: %v", err)
		}
		benchStatusSink = got
	}
}

// BenchmarkNormaliseLang is a microbenchmark for alias normalisation.
func BenchmarkNormaliseLang(b *testing.B) {
	inputs := []string{"ts", "py", "c++", "shell", "typescript", "Go", "  rs\t", "unknown"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchLangSink = NormaliseLang(inputs[i%len(inputs)])
	}
}
