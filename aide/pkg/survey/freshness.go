// Package survey: freshness.go tags analyzer runs with the git commit they
// ran at, and computes how stale stored entries are relative to HEAD.
//
// The run commit is stamped into each entry's Metadata rather than kept as a
// separate run record: entries for an analyzer are always replaced wholesale
// (ReplaceEntriesForAnalyzer), so every entry in an analyzer shares one run,
// and metadata flows unchanged through every storage path (direct BoltDB,
// gRPC daemon, web dashboard) with no schema or proto change.
package survey

import (
	"fmt"
	"time"
)

// MetaRunCommit is the Entry.Metadata key holding the full git HEAD hash at
// the time the analyzer ran. Empty/absent on entries stored before stamping
// existed or when the project is not a git repo.
const MetaRunCommit = "run_commit"

// DefaultFreshnessMaxWalk caps how far back CommitsBehind walks from HEAD
// looking for a recorded run commit.
const DefaultFreshnessMaxWalk = DefaultMaxCommits

// StampRunCommit records commit on every entry's metadata. No-op for an
// empty commit (not a git repo, or unborn HEAD).
func StampRunCommit(entries []*Entry, commit string) {
	if commit == "" {
		return
	}
	for _, e := range entries {
		if e.Metadata == nil {
			e.Metadata = make(map[string]string, 1)
		}
		e.Metadata[MetaRunCommit] = commit
	}
}

// RunCommitForEntries returns the run commit recorded on stored entries,
// preferring the most recently created entry that carries one. Returns ""
// when no entry was stamped.
func RunCommitForEntries(entries []*Entry) string {
	commit := ""
	var latest time.Time
	for _, e := range entries {
		c := e.Metadata[MetaRunCommit]
		if c == "" {
			continue
		}
		if commit == "" || e.CreatedAt.After(latest) {
			commit = c
			latest = e.CreatedAt
		}
	}
	return commit
}

// Freshness describes how stale a recorded analyzer run is relative to the
// repository HEAD.
type Freshness struct {
	RunCommit string // full hash recorded at run time
	Head      string // current HEAD hash
	Behind    int    // commits between RunCommit and HEAD (0 = current)
	Found     bool   // RunCommit located within MaxWalk commits of HEAD
	MaxWalk   int    // walk cap used when Found is false
}

// String renders a short human-readable staleness description.
func (f *Freshness) String() string {
	switch {
	case !f.Found:
		return fmt.Sprintf("stale: run commit %.8s not within the last %d commits (very old, or history rewritten)", f.RunCommit, f.MaxWalk)
	case f.Behind == 0:
		return "current"
	case f.Behind == 1:
		return "1 commit behind"
	default:
		return fmt.Sprintf("%d commits behind", f.Behind)
	}
}

// ComputeFreshness compares a recorded run commit against the repository at
// rootDir. Returns nil (no error) when runCommit is empty or rootDir is not
// a git repository — freshness is simply unknown/inapplicable there.
func ComputeFreshness(rootDir, runCommit string) (*Freshness, error) {
	if runCommit == "" {
		return nil, nil
	}
	repo, err := OpenGitRepo(rootDir)
	if err != nil || repo == nil {
		return nil, err
	}
	head, err := repo.Head()
	if err != nil || head == "" {
		return nil, err
	}
	f := &Freshness{RunCommit: runCommit, Head: head, MaxWalk: DefaultFreshnessMaxWalk}
	if head == runCommit {
		f.Found = true
		return f, nil
	}
	behind, found, err := repo.CommitsBehind(runCommit, DefaultFreshnessMaxWalk)
	if err != nil {
		return nil, err
	}
	f.Behind = behind
	f.Found = found
	return f, nil
}

// HeadCommit returns the HEAD hash for rootDir, or "" when rootDir is not a
// git repository or has no commits. Errors are deliberately swallowed:
// stamping is best-effort and must never fail an analyzer run.
func HeadCommit(rootDir string) string {
	repo, err := OpenGitRepo(rootDir)
	if err != nil || repo == nil {
		return ""
	}
	head, err := repo.Head()
	if err != nil {
		return ""
	}
	return head
}

// EntryDiff summarises what changed between two runs of the same analyzer.
type EntryDiff struct {
	Added   int
	Removed int
}

// DiffEntries compares two entry sets by (Kind, Name, FilePath) identity.
// Metadata and Detail changes are not counted — the diff answers "what
// appeared or disappeared", not "what was retouched".
func DiffEntries(oldEntries, newEntries []*Entry) EntryDiff {
	key := func(e *Entry) string {
		return e.Kind + "\x00" + e.Name + "\x00" + e.FilePath
	}
	oldSet := make(map[string]struct{}, len(oldEntries))
	for _, e := range oldEntries {
		oldSet[key(e)] = struct{}{}
	}
	var d EntryDiff
	newSet := make(map[string]struct{}, len(newEntries))
	for _, e := range newEntries {
		k := key(e)
		newSet[k] = struct{}{}
		if _, ok := oldSet[k]; !ok {
			d.Added++
		}
	}
	for k := range oldSet {
		if _, ok := newSet[k]; !ok {
			d.Removed++
		}
	}
	return d
}
