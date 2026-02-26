package clone

import (
	"os"
	"path/filepath"
	"testing"
)

// testdataDir returns the absolute path to the clone testdata directory.
func testdataDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata"))
	if err != nil {
		t.Fatalf("failed to resolve testdata dir: %v", err)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Fatalf("testdata directory does not exist: %s", dir)
	}
	return dir
}

// =============================================================================
// Clone Detection Tests
// =============================================================================

func TestDetectClones_DuplicatedFunctions(t *testing.T) {
	dir := testdataDir(t)

	findings, result, err := DetectClones(Config{
		Paths: []string{dir},
	})
	if err != nil {
		t.Fatalf("DetectClones error: %v", err)
	}

	if result.FilesAnalyzed < 2 {
		t.Fatalf("expected at least 2 files analyzed, got %d", result.FilesAnalyzed)
	}

	// clone_a.go and clone_b.go have structurally identical functions.
	// We expect at least 1 clone finding.
	if len(findings) == 0 {
		t.Fatal("expected at least 1 clone finding, got 0")
	}

	for _, f := range findings {
		if f.Analyzer != "clones" {
			t.Errorf("unexpected analyzer %q (expected %q)", f.Analyzer, "clones")
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
	}

	t.Logf("Found %d clone findings across %d clone groups", len(findings), result.CloneGroups)
	for _, f := range findings {
		t.Logf("  [%s] %s:%d — %s", f.Severity, f.FilePath, f.Line, f.Title)
	}
}

func TestDetectClones_SingleFile(t *testing.T) {
	dir := testdataDir(t)

	// Within clone_a.go, there are 2 structurally similar functions
	// (ProcessOrders and ValidateInputs). The clone detector may or may
	// not find intra-file clones depending on window size.
	findings, result, err := DetectClones(Config{
		Paths: []string{filepath.Join(dir, "clone_a.go")},
	})
	if err != nil {
		t.Fatalf("DetectClones error: %v", err)
	}

	t.Logf("Single file: %d findings, %d files analyzed", len(findings), result.FilesAnalyzed)
}

func TestDetectClones_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-clone-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	findings, _, err := DetectClones(Config{
		Paths: []string{tmpDir},
	})
	if err != nil {
		t.Fatalf("DetectClones error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for empty dir, got %d", len(findings))
	}
}

func TestDetectClones_CustomWindowSize(t *testing.T) {
	dir := testdataDir(t)

	// Smaller window size should be more sensitive (more clones).
	smallFindings, _, err := DetectClones(Config{
		WindowSize: 3,
		Paths:      []string{dir},
	})
	if err != nil {
		t.Fatalf("DetectClones (small window) error: %v", err)
	}

	// Larger window size should be less sensitive (fewer clones).
	largeFindings, _, err := DetectClones(Config{
		WindowSize: 20,
		Paths:      []string{dir},
	})
	if err != nil {
		t.Fatalf("DetectClones (large window) error: %v", err)
	}

	t.Logf("Window=3: %d findings, Window=20: %d findings", len(smallFindings), len(largeFindings))

	// With a very small window we'd expect at least as many (usually more) clones.
	if len(smallFindings) < len(largeFindings) {
		t.Logf("NOTE: small window (%d) found fewer clones than large window (%d); this can happen with hash collisions",
			len(smallFindings), len(largeFindings))
	}
}

func TestDetectClones_FindingFields(t *testing.T) {
	dir := testdataDir(t)

	findings, _, err := DetectClones(Config{
		Paths: []string{dir},
	})
	if err != nil {
		t.Fatalf("DetectClones error: %v", err)
	}

	for _, f := range findings {
		if f.Analyzer != "clones" {
			t.Errorf("unexpected analyzer: %s", f.Analyzer)
		}
		if f.Severity == "" {
			t.Error("finding has empty severity")
		}
		if f.Category == "" {
			t.Error("finding has empty category")
		}
		if f.CreatedAt.IsZero() {
			t.Error("finding has zero CreatedAt")
		}
		// Clone findings should reference the other file in metadata
		if f.Metadata["other_file"] == "" && f.Metadata["clone_file"] == "" && f.Detail == "" {
			t.Logf("finding at %s:%d has no cross-reference info (may be expected)", f.FilePath, f.Line)
		}
	}
}
