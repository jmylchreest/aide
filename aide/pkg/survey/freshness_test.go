package survey

import (
	"testing"
	"time"
)

// =============================================================================
// Pure function tests
// =============================================================================

func TestStampRunCommit(t *testing.T) {
	entries := []*Entry{
		{Name: "a"},
		{Name: "b", Metadata: map[string]string{"language": "go"}},
	}
	StampRunCommit(entries, "abc123")

	for _, e := range entries {
		if e.Metadata[MetaRunCommit] != "abc123" {
			t.Errorf("entry %q: run_commit = %q, want %q", e.Name, e.Metadata[MetaRunCommit], "abc123")
		}
	}
	if entries[1].Metadata["language"] != "go" {
		t.Error("existing metadata was clobbered")
	}
}

func TestStampRunCommit_EmptyCommitIsNoop(t *testing.T) {
	entries := []*Entry{{Name: "a"}}
	StampRunCommit(entries, "")
	if entries[0].Metadata != nil {
		t.Errorf("expected no metadata for empty commit, got %v", entries[0].Metadata)
	}
}

func TestRunCommitForEntries(t *testing.T) {
	now := time.Now()
	entries := []*Entry{
		{Name: "old", CreatedAt: now.Add(-time.Hour), Metadata: map[string]string{MetaRunCommit: "old-commit"}},
		{Name: "unstamped", CreatedAt: now},
		{Name: "new", CreatedAt: now, Metadata: map[string]string{MetaRunCommit: "new-commit"}},
	}
	if got := RunCommitForEntries(entries); got != "new-commit" {
		t.Errorf("RunCommitForEntries = %q, want %q", got, "new-commit")
	}
	if got := RunCommitForEntries(nil); got != "" {
		t.Errorf("RunCommitForEntries(nil) = %q, want empty", got)
	}
}

func TestDiffEntries(t *testing.T) {
	oldEntries := []*Entry{
		{Kind: KindModule, Name: "pkg/a"},
		{Kind: KindModule, Name: "pkg/b"},
		{Kind: KindChurn, Name: "pkg/a"}, // same name, different kind — distinct identity
	}
	newEntries := []*Entry{
		{Kind: KindModule, Name: "pkg/a"},                   // kept
		{Kind: KindModule, Name: "pkg/c"},                   // added
		{Kind: KindChurn, Name: "pkg/a", FilePath: "pkg/a"}, // FilePath differs — counts as add+remove
	}
	d := DiffEntries(oldEntries, newEntries)
	if d.Added != 2 || d.Removed != 2 {
		t.Errorf("DiffEntries = +%d/-%d, want +2/-2", d.Added, d.Removed)
	}

	same := DiffEntries(oldEntries, oldEntries)
	if same.Added != 0 || same.Removed != 0 {
		t.Errorf("identical sets diff = +%d/-%d, want +0/-0", same.Added, same.Removed)
	}
}

func TestFreshnessString(t *testing.T) {
	cases := []struct {
		f    Freshness
		want string
	}{
		{Freshness{Found: true, Behind: 0}, "current"},
		{Freshness{Found: true, Behind: 1}, "1 commit behind"},
		{Freshness{Found: true, Behind: 5}, "5 commits behind"},
	}
	for _, c := range cases {
		if got := c.f.String(); got != c.want {
			t.Errorf("Freshness%+v.String() = %q, want %q", c.f, got, c.want)
		}
	}
	notFound := Freshness{RunCommit: "deadbeefdeadbeef", Found: false, MaxWalk: 500}
	if got := notFound.String(); got == "" || got == "current" {
		t.Errorf("not-found freshness rendered as %q", got)
	}
}

// =============================================================================
// Git integration tests (uses initTestRepo/commitAll from churn_test.go)
// =============================================================================

func TestHeadCommit_NotGitRepo(t *testing.T) {
	if got := HeadCommit(t.TempDir()); got != "" {
		t.Errorf("HeadCommit on non-repo = %q, want empty", got)
	}
}

func TestComputeFreshness_EmptyCommit(t *testing.T) {
	f, err := ComputeFreshness(t.TempDir(), "")
	if err != nil || f != nil {
		t.Errorf("ComputeFreshness with empty commit = (%v, %v), want (nil, nil)", f, err)
	}
}

func TestComputeFreshness_NotGitRepo(t *testing.T) {
	f, err := ComputeFreshness(t.TempDir(), "abc123")
	if err != nil || f != nil {
		t.Errorf("ComputeFreshness on non-repo = (%v, %v), want (nil, nil)", f, err)
	}
}

func TestCommitsBehind(t *testing.T) {
	dir, repo := initTestRepo(t)

	g, err := OpenGitRepo(dir)
	if err != nil || g == nil {
		t.Fatalf("OpenGitRepo: %v", err)
	}

	first, err := g.Head()
	if err != nil || first == "" {
		t.Fatalf("Head after init: %q, %v", first, err)
	}

	// HEAD itself is 0 behind.
	behind, found, err := g.CommitsBehind(first, 0)
	if err != nil || !found || behind != 0 {
		t.Errorf("CommitsBehind(HEAD) = (%d, %v, %v), want (0, true, nil)", behind, found, err)
	}

	// Three more commits: the first commit is now 3 behind.
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		writeTestFile(t, dir, name, "content\n")
		commitAll(t, repo, "commit "+name)
	}
	behind, found, err = g.CommitsBehind(first, 0)
	if err != nil || !found || behind != 3 {
		t.Errorf("CommitsBehind(first) = (%d, %v, %v), want (3, true, nil)", behind, found, err)
	}

	// Unknown hash: walks entire (4-commit) history without a match.
	behind, found, err = g.CommitsBehind("0000000000000000000000000000000000000000", 0)
	if err != nil || found {
		t.Errorf("CommitsBehind(unknown) = (%d, %v, %v), want found=false", behind, found, err)
	}

	// Walk cap respected: cap of 1 cannot reach the first commit.
	_, found, err = g.CommitsBehind(first, 1)
	if err != nil || found {
		t.Errorf("CommitsBehind(first, cap=1) found=%v, want false", found)
	}

	// End-to-end: ComputeFreshness against the same repo.
	f, err := ComputeFreshness(dir, first)
	if err != nil || f == nil {
		t.Fatalf("ComputeFreshness: (%v, %v)", f, err)
	}
	if !f.Found || f.Behind != 3 {
		t.Errorf("ComputeFreshness = %+v, want Found=true Behind=3", f)
	}
	if f.String() != "3 commits behind" {
		t.Errorf("Freshness.String() = %q, want %q", f.String(), "3 commits behind")
	}
}
