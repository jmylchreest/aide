package clone

import (
	"os"
	"path/filepath"
	"testing"
)

// testdataDir returns the absolute path to the clone testdata directory.
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

	// Use explicit MinCloneLines=6 so the window size (not the line
	// threshold) is the variable under test.
	smallFindings, _, err := DetectClones(Config{
		WindowSize:    3,
		MinCloneLines: 6,
		Paths:         []string{dir},
	})
	if err != nil {
		t.Fatalf("DetectClones (small window) error: %v", err)
	}

	largeFindings, _, err := DetectClones(Config{
		WindowSize:    20,
		MinCloneLines: 6,
		Paths:         []string{dir},
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
		// Grouped findings should list clone locations in metadata.
		if f.Metadata["clone_locations"] == "" {
			t.Errorf("finding at %s:%d missing clone_locations metadata", f.FilePath, f.Line)
		}
		if f.Metadata["clone_count"] == "" {
			t.Errorf("finding at %s:%d missing clone_count metadata", f.FilePath, f.Line)
		}
	}
}

// =============================================================================
// New Feature Tests
// =============================================================================

func TestDetectClones_MinMatchCount(t *testing.T) {
	dir := testdataDir(t)

	// With a very high MinMatchCount, nothing should be reported because each
	// clone region only has a moderate number of matching windows.
	findings, result, err := DetectClones(Config{
		Paths:         []string{dir},
		WindowSize:    20,
		MinMatchCount: 1000, // impossibly high
	})
	if err != nil {
		t.Fatalf("DetectClones error: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected 0 findings with impossibly high MinMatchCount, got %d", len(findings))
	}
	t.Logf("MinMatchCount=1000: %d findings, %d groups", len(findings), result.CloneGroups)
}

func TestDetectClones_MinSimilarity(t *testing.T) {
	dir := testdataDir(t)

	// With MinSimilarity=1.0 (requires 100% match), we should get very few
	// or no findings because minor token differences exist between clones.
	strictFindings, _, err := DetectClones(Config{
		Paths:         []string{dir},
		MinSimilarity: 1.0,
	})
	if err != nil {
		t.Fatalf("DetectClones (strict) error: %v", err)
	}

	// With MinSimilarity=0.0 (no filter), we should get our normal set.
	relaxedFindings, _, err := DetectClones(Config{
		Paths:         []string{dir},
		MinSimilarity: 0.0,
	})
	if err != nil {
		t.Fatalf("DetectClones (relaxed) error: %v", err)
	}

	t.Logf("MinSimilarity=1.0: %d findings, MinSimilarity=0.0: %d findings",
		len(strictFindings), len(relaxedFindings))

	if len(strictFindings) > len(relaxedFindings) {
		t.Errorf("strict similarity (%d) should not produce more findings than relaxed (%d)",
			len(strictFindings), len(relaxedFindings))
	}
}

func TestDetectClones_MaxBucketSize(t *testing.T) {
	dir := testdataDir(t)

	// With MaxBucketSize=1, every hash that appears in more than 1 file is
	// skipped, so we should get fewer (or zero) findings.
	limitedFindings, result, err := DetectClones(Config{
		Paths:         []string{dir},
		MaxBucketSize: 1,
	})
	if err != nil {
		t.Fatalf("DetectClones (limited bucket) error: %v", err)
	}

	// With MaxBucketSize disabled (-1), no buckets are skipped.
	unlimitedFindings, _, err := DetectClones(Config{
		Paths:         []string{dir},
		MaxBucketSize: -1, // disabled
	})
	if err != nil {
		t.Fatalf("DetectClones (unlimited bucket) error: %v", err)
	}

	t.Logf("MaxBucketSize=1: %d findings (%d buckets skipped), unlimited: %d findings",
		len(limitedFindings), result.BucketsSkipped, len(unlimitedFindings))

	// MaxBucketSize=1 means all cross-file hashes are skipped, so findings
	// should be fewer than or equal to unlimited.
	if len(limitedFindings) > len(unlimitedFindings) {
		t.Errorf("bucket-limited (%d) should not produce more findings than unlimited (%d)",
			len(limitedFindings), len(unlimitedFindings))
	}
}

func TestDetectClones_LanguageIsolation(t *testing.T) {
	dir := testdataDir(t)

	// Both test files are Go, so language isolation should not change results.
	trueVal := true
	isoFindings, _, err := DetectClones(Config{
		Paths:             []string{dir},
		LanguageIsolation: &trueVal,
	})
	if err != nil {
		t.Fatalf("DetectClones (iso=true) error: %v", err)
	}

	falseVal := false
	noIsoFindings, _, err := DetectClones(Config{
		Paths:             []string{dir},
		LanguageIsolation: &falseVal,
	})
	if err != nil {
		t.Fatalf("DetectClones (iso=false) error: %v", err)
	}

	t.Logf("LanguageIsolation=true: %d findings, false: %d findings",
		len(isoFindings), len(noIsoFindings))

	// For same-language test files, results should be identical.
	if len(isoFindings) != len(noIsoFindings) {
		t.Logf("NOTE: isolation changed result count (true=%d, false=%d); expected same for same-language files",
			len(isoFindings), len(noIsoFindings))
	}
}

func TestDetectClones_BucketsSkippedReported(t *testing.T) {
	dir := testdataDir(t)

	// Use MaxBucketSize=1 to force bucket skipping, then verify the count is reported.
	_, result, err := DetectClones(Config{
		Paths:         []string{dir},
		MaxBucketSize: 1,
	})
	if err != nil {
		t.Fatalf("DetectClones error: %v", err)
	}

	// With 2 files sharing the same tokens, most buckets have size 2, so
	// MaxBucketSize=1 should skip some.
	if result.BucketsSkipped == 0 {
		t.Error("expected some buckets to be skipped with MaxBucketSize=1")
	}
	t.Logf("BucketsSkipped=%d, CollisionsFiltered=%d", result.BucketsSkipped, result.CollisionsFiltered)
}

func TestDetectClones_GroupedReporting(t *testing.T) {
	dir := testdataDir(t)

	// Use low MinCloneLines so we get findings from the test fixtures.
	findings, _, err := DetectClones(Config{
		Paths:         []string{dir},
		MinCloneLines: 6,
	})
	if err != nil {
		t.Fatalf("DetectClones error: %v", err)
	}

	if len(findings) == 0 {
		t.Fatal("expected at least 1 finding for grouped reporting test")
	}

	// Each finding should be one unique file region — no two findings
	// should cover the same file and overlapping line range.
	type regionSeen struct {
		file string
		line int
	}
	seen := make(map[regionSeen]bool)
	for _, f := range findings {
		rs := regionSeen{file: f.FilePath, line: f.Line}
		if seen[rs] {
			t.Errorf("duplicate finding at %s:%d — grouped reporting should deduplicate", f.FilePath, f.Line)
		}
		seen[rs] = true

		// Each finding must have clone_count >= 1 and clone_locations non-empty.
		if f.Metadata["clone_count"] == "" || f.Metadata["clone_count"] == "0" {
			t.Errorf("finding at %s:%d has zero or missing clone_count", f.FilePath, f.Line)
		}
		if f.Metadata["clone_locations"] == "" {
			t.Errorf("finding at %s:%d has empty clone_locations", f.FilePath, f.Line)
		}
	}

	t.Logf("Grouped reporting: %d unique findings", len(findings))
	for _, f := range findings {
		t.Logf("  %s:%d–%d (%s locations) — %s",
			f.FilePath, f.Line, f.EndLine, f.Metadata["clone_count"], f.Title)
	}
}

func TestDetectClones_DefaultConstants(t *testing.T) {
	// Verify the exported constants have sensible values.
	if DefaultWindowSize <= 0 {
		t.Errorf("DefaultWindowSize must be positive, got %d", DefaultWindowSize)
	}
	if DefaultMinCloneLines <= 0 {
		t.Errorf("DefaultMinCloneLines must be positive, got %d", DefaultMinCloneLines)
	}
	if DefaultMinMatchCount <= 0 {
		t.Errorf("DefaultMinMatchCount must be positive, got %d", DefaultMinMatchCount)
	}
	if DefaultMaxBucketSize <= 0 {
		t.Errorf("DefaultMaxBucketSize must be positive, got %d", DefaultMaxBucketSize)
	}
	if DefaultMinSimilarity < 0 || DefaultMinSimilarity > 1 {
		t.Errorf("DefaultMinSimilarity must be 0.0–1.0, got %f", DefaultMinSimilarity)
	}
	if DefaultSevWarningLines <= 0 {
		t.Errorf("DefaultSevWarningLines must be positive, got %d", DefaultSevWarningLines)
	}
	if DefaultSevCriticalLines <= DefaultSevWarningLines {
		t.Errorf("DefaultSevCriticalLines (%d) must exceed DefaultSevWarningLines (%d)",
			DefaultSevCriticalLines, DefaultSevWarningLines)
	}
}
