package survey

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// =============================================================================
// Pure function tests
// =============================================================================

func TestTopChurnFiles_Sorting(t *testing.T) {
	stats := map[string]*ChurnStat{
		"low.go":    {FilePath: "low.go", Commits: 1, LinesChanged: 10},
		"high.go":   {FilePath: "high.go", Commits: 50, LinesChanged: 5000},
		"medium.go": {FilePath: "medium.go", Commits: 10, LinesChanged: 200},
	}

	top := TopChurnFiles(stats, 10)
	if len(top) != 3 {
		t.Fatalf("expected 3 results, got %d", len(top))
	}

	// Highest churn score should be first
	if top[0].FilePath != "high.go" {
		t.Errorf("top[0] = %q, want %q", top[0].FilePath, "high.go")
	}
}

func TestTopChurnFiles_Limit(t *testing.T) {
	stats := map[string]*ChurnStat{
		"a.go": {FilePath: "a.go", Commits: 10, LinesChanged: 100},
		"b.go": {FilePath: "b.go", Commits: 20, LinesChanged: 200},
		"c.go": {FilePath: "c.go", Commits: 30, LinesChanged: 300},
		"d.go": {FilePath: "d.go", Commits: 40, LinesChanged: 400},
		"e.go": {FilePath: "e.go", Commits: 50, LinesChanged: 500},
	}

	top := TopChurnFiles(stats, 2)
	if len(top) != 2 {
		t.Errorf("expected 2 results, got %d", len(top))
	}
}

func TestTopChurnFiles_DefaultLimit(t *testing.T) {
	stats := make(map[string]*ChurnStat)
	for i := 0; i < 100; i++ {
		name := string(rune('a'+i/26)) + string(rune('a'+i%26)) + ".go"
		stats[name] = &ChurnStat{FilePath: name, Commits: i + 1, LinesChanged: (i + 1) * 10}
	}

	// topN=0 defaults to 50
	top := TopChurnFiles(stats, 0)
	if len(top) != 50 {
		t.Errorf("expected 50 results with default limit, got %d", len(top))
	}
}

func TestTopChurnFiles_Empty(t *testing.T) {
	stats := map[string]*ChurnStat{}
	top := TopChurnFiles(stats, 10)
	if len(top) != 0 {
		t.Errorf("expected 0 results for empty stats, got %d", len(top))
	}
}

func TestRunChurn_NotGitRepo(t *testing.T) {
	dir := t.TempDir()

	result, err := RunChurn(dir, 0, 0)
	if err != nil {
		t.Fatalf("RunChurn: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result for non-git dir")
	}
	if len(result.Entries) != 0 {
		t.Errorf("expected 0 entries for non-git dir, got %d", len(result.Entries))
	}
}

func TestOpenGitRepo_NotGitRepo(t *testing.T) {
	dir := t.TempDir()

	repo, err := OpenGitRepo(dir)
	if err != nil {
		t.Fatalf("OpenGitRepo: %v", err)
	}
	if repo != nil {
		t.Error("expected nil repo for non-git directory")
	}
}

func TestChurnResult_EntryFields(t *testing.T) {
	// Directly test that churn entries have the right shape
	cs := &ChurnStat{
		FilePath:     "pkg/server.go",
		Commits:      42,
		LinesChanged: 1500,
	}

	result := &ChurnResult{}
	result.Entries = append(result.Entries, &Entry{
		Analyzer: AnalyzerChurn,
		Kind:     KindChurn,
		Name:     cs.FilePath,
		FilePath: cs.FilePath,
		Title:    "High churn: pkg/server.go (42 commits, 1500 lines changed)",
		Metadata: map[string]string{
			"commits":       "42",
			"lines_changed": "1500",
		},
	})

	e := result.Entries[0]
	if e.Analyzer != AnalyzerChurn {
		t.Errorf("Analyzer = %q, want %q", e.Analyzer, AnalyzerChurn)
	}
	if e.Kind != KindChurn {
		t.Errorf("Kind = %q, want %q", e.Kind, KindChurn)
	}
	if e.Metadata["commits"] != "42" {
		t.Errorf("Metadata[commits] = %q, want %q", e.Metadata["commits"], "42")
	}
	if e.Metadata["lines_changed"] != "1500" {
		t.Errorf("Metadata[lines_changed] = %q, want %q", e.Metadata["lines_changed"], "1500")
	}
}

// =============================================================================
// Git integration test helpers
// =============================================================================

var testCommitTime = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

// initTestRepo creates a temp git repo with an initial commit.
// Returns the repo dir and the go-git Repository.
func initTestRepo(t *testing.T) (string, *git.Repository) {
	t.Helper()
	dir := t.TempDir()

	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("git.PlainInit: %v", err)
	}

	// Create an initial file and commit
	writeTestFile(t, dir, "README.md", "# test repo\n")
	commitAll(t, repo, "initial commit")

	return dir, repo
}

// writeTestFile writes content to a file in the repo dir.
func writeTestFile(t *testing.T, repoDir, name, content string) {
	t.Helper()
	path := filepath.Join(repoDir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}

// commitAll stages all changes and commits with the given message.
func commitAll(t *testing.T, repo *git.Repository, msg string) {
	t.Helper()
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}

	// Stage all files
	if _, err := wt.Add("."); err != nil {
		t.Fatalf("wt.Add: %v", err)
	}

	testCommitTime = testCommitTime.Add(time.Second)

	_, err = wt.Commit(msg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test Author",
			Email: "test@example.com",
			When:  testCommitTime,
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
}

// =============================================================================
// Git integration tests: OpenGitRepo
// =============================================================================

func TestOpenGitRepo_ValidRepo(t *testing.T) {
	dir, _ := initTestRepo(t)

	repo, err := OpenGitRepo(dir)
	if err != nil {
		t.Fatalf("OpenGitRepo: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repo for valid git directory")
	}
}

func TestOpenGitRepo_SubdirectoryDetection(t *testing.T) {
	dir, _ := initTestRepo(t)

	// Create a nested subdirectory
	subDir := filepath.Join(dir, "pkg", "deep")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// OpenGitRepo should detect the repo from a subdirectory (DetectDotGit=true)
	repo, err := OpenGitRepo(subDir)
	if err != nil {
		t.Fatalf("OpenGitRepo from subdir: %v", err)
	}
	if repo == nil {
		t.Fatal("expected non-nil repo when opening from subdirectory")
	}
}

// =============================================================================
// Git integration tests: FileChurnStats
// =============================================================================

func TestFileChurnStats_HappyPath(t *testing.T) {
	dir, repo := initTestRepo(t)

	// Make several commits modifying different files
	writeTestFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	commitAll(t, repo, "add main.go")

	writeTestFile(t, dir, "main.go", "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n")
	commitAll(t, repo, "update main.go")

	writeTestFile(t, dir, "util.go", "package main\n\nfunc helper() {}\n")
	commitAll(t, repo, "add util.go")

	writeTestFile(t, dir, "main.go", "package main\n\nfunc main() {\n\tprintln(\"world\")\n\thelper()\n}\n")
	commitAll(t, repo, "update main.go again")

	gitRepo, err := OpenGitRepo(dir)
	if err != nil {
		t.Fatalf("OpenGitRepo: %v", err)
	}

	stats, err := gitRepo.FileChurnStats(0)
	if err != nil {
		t.Fatalf("FileChurnStats: %v", err)
	}

	// main.go should appear in multiple commits
	mainStat, ok := stats["main.go"]
	if !ok {
		t.Fatal("expected stats for main.go")
	}
	if mainStat.Commits < 2 {
		t.Errorf("main.go commits = %d, want >= 2", mainStat.Commits)
	}
	if mainStat.LinesChanged == 0 {
		t.Error("main.go LinesChanged should be > 0")
	}

	// util.go should appear in at least 1 commit
	utilStat, ok := stats["util.go"]
	if !ok {
		t.Fatal("expected stats for util.go")
	}
	if utilStat.Commits < 1 {
		t.Errorf("util.go commits = %d, want >= 1", utilStat.Commits)
	}
}

func TestFileChurnStats_DefaultMaxCommits(t *testing.T) {
	dir, repo := initTestRepo(t)

	writeTestFile(t, dir, "a.go", "package main\n")
	commitAll(t, repo, "add a.go")

	gitRepo, err := OpenGitRepo(dir)
	if err != nil {
		t.Fatalf("OpenGitRepo: %v", err)
	}

	// maxCommits=0 should default to DefaultMaxCommits (500)
	stats, err := gitRepo.FileChurnStats(0)
	if err != nil {
		t.Fatalf("FileChurnStats(0): %v", err)
	}
	if len(stats) == 0 {
		t.Error("expected non-empty stats with default maxCommits")
	}
}

func TestFileChurnStats_MaxCommitsLimit(t *testing.T) {
	dir, repo := initTestRepo(t)

	// Create 5 commits modifying the same file
	for i := 0; i < 5; i++ {
		writeTestFile(t, dir, "hot.go", fmt.Sprintf("package main\n// version %d\n", i))
		commitAll(t, repo, fmt.Sprintf("update hot.go v%d", i))
	}

	gitRepo, err := OpenGitRepo(dir)
	if err != nil {
		t.Fatalf("OpenGitRepo: %v", err)
	}

	// Limit to only 2 most recent commits
	stats, err := gitRepo.FileChurnStats(2)
	if err != nil {
		t.Fatalf("FileChurnStats(2): %v", err)
	}

	// hot.go should have at most 2 commit mentions since we limited traversal
	hotStat, ok := stats["hot.go"]
	if !ok {
		t.Fatal("expected stats for hot.go")
	}
	if hotStat.Commits > 2 {
		t.Errorf("hot.go commits = %d with maxCommits=2, want <= 2", hotStat.Commits)
	}
}

func TestFileChurnStats_EmptyRepo(t *testing.T) {
	dir, _ := initTestRepo(t)

	// Repo has only the initial commit (README.md)
	gitRepo, err := OpenGitRepo(dir)
	if err != nil {
		t.Fatalf("OpenGitRepo: %v", err)
	}

	stats, err := gitRepo.FileChurnStats(0)
	if err != nil {
		t.Fatalf("FileChurnStats: %v", err)
	}

	// Should still have at least README.md from the initial commit
	if len(stats) == 0 {
		t.Error("expected at least README.md in stats for repo with initial commit")
	}
}

// =============================================================================
// Git integration tests: Submodules
// =============================================================================

func TestSubmodules_NoSubmodules(t *testing.T) {
	dir, _ := initTestRepo(t)

	gitRepo, err := OpenGitRepo(dir)
	if err != nil {
		t.Fatalf("OpenGitRepo: %v", err)
	}

	subs, err := gitRepo.Submodules()
	if err != nil {
		t.Fatalf("Submodules: %v", err)
	}
	if len(subs) != 0 {
		t.Errorf("expected 0 submodules, got %d", len(subs))
	}
}

// =============================================================================
// Git integration tests: RunChurn end-to-end
// =============================================================================

func TestRunChurn_EndToEnd(t *testing.T) {
	dir, repo := initTestRepo(t)

	// Create some commits with varied churn
	writeTestFile(t, dir, "main.go", "package main\n\nfunc main() {}\n")
	commitAll(t, repo, "add main.go")

	writeTestFile(t, dir, "main.go", "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n")
	commitAll(t, repo, "update main.go")

	writeTestFile(t, dir, "util.go", "package main\n\nfunc helper() {}\n")
	commitAll(t, repo, "add util.go")

	result, err := RunChurn(dir, 0, 10)
	if err != nil {
		t.Fatalf("RunChurn: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Should have entries for files that changed
	if len(result.Entries) == 0 {
		t.Error("expected at least 1 churn entry")
	}

	// All entries should be churn analyzer with valid kinds
	for _, e := range result.Entries {
		if e.Analyzer != AnalyzerChurn {
			t.Errorf("entry %q Analyzer = %q, want %q", e.Name, e.Analyzer, AnalyzerChurn)
		}
		if e.Kind != KindChurn && e.Kind != KindSubmodule {
			t.Errorf("entry %q Kind = %q, want churn or submodule", e.Name, e.Kind)
		}
	}
}

func TestRunChurn_TopNLimit(t *testing.T) {
	dir, repo := initTestRepo(t)

	// Create enough files to test topN limiting
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("file%d.go", i)
		writeTestFile(t, dir, name, fmt.Sprintf("package main\n// file %d\n", i))
		commitAll(t, repo, fmt.Sprintf("add %s", name))
	}

	// Request only top 3
	result, err := RunChurn(dir, 0, 3)
	if err != nil {
		t.Fatalf("RunChurn: %v", err)
	}

	// Count churn entries (not submodule entries)
	churnCount := 0
	for _, e := range result.Entries {
		if e.Kind == KindChurn {
			churnCount++
		}
	}

	if churnCount > 3 {
		t.Errorf("expected at most 3 churn entries with topN=3, got %d", churnCount)
	}
}

func TestRunChurn_EntryMetadata(t *testing.T) {
	dir, repo := initTestRepo(t)

	writeTestFile(t, dir, "server.go", "package main\n\nfunc serve() {}\n")
	commitAll(t, repo, "add server.go")

	writeTestFile(t, dir, "server.go", "package main\n\nfunc serve() {\n\tlistenAndServe()\n}\n")
	commitAll(t, repo, "update server.go")

	result, err := RunChurn(dir, 0, 50)
	if err != nil {
		t.Fatalf("RunChurn: %v", err)
	}

	// Find the server.go entry
	var serverEntry *Entry
	for _, e := range result.Entries {
		if e.Kind == KindChurn && e.FilePath == "server.go" {
			serverEntry = e
			break
		}
	}

	if serverEntry == nil {
		t.Fatal("expected churn entry for server.go")
	}

	// Check metadata fields
	if serverEntry.Metadata["commits"] == "" {
		t.Error("expected non-empty commits metadata")
	}
	if serverEntry.Metadata["lines_changed"] == "" {
		t.Error("expected non-empty lines_changed metadata")
	}
	if serverEntry.Title == "" {
		t.Error("expected non-empty Title")
	}
	if serverEntry.Detail == "" {
		t.Error("expected non-empty Detail")
	}
	if serverEntry.Name != "server.go" {
		t.Errorf("Name = %q, want %q", serverEntry.Name, "server.go")
	}
}
