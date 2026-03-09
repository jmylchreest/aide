// Package survey provides codebase structural analysis.
// gitrepo.go wraps go-git for survey-specific operations.
package survey

import (
	"fmt"
	"sort"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// DefaultMaxCommits is the default number of commits to analyze for churn.
const DefaultMaxCommits = 500

// GitRepo wraps go-git for survey-specific operations.
type GitRepo struct {
	repo *git.Repository
}

// ChurnStat tracks change frequency for a single file.
type ChurnStat struct {
	FilePath     string
	Commits      int
	LinesChanged int // Total lines added + removed
}

// OpenGitRepo opens a git repository. Returns nil, nil if not a git repo.
// This follows the design decision: not-a-git-repo is a graceful skip, not an error.
func OpenGitRepo(dir string) (*GitRepo, error) {
	repo, err := git.PlainOpenWithOptions(dir, &git.PlainOpenOptions{DetectDotGit: true})
	if err == git.ErrRepositoryNotExists {
		return nil, nil // Not a git repo — not an error for survey purposes
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open git repo: %w", err)
	}
	return &GitRepo{repo: repo}, nil
}

// Submodules returns the list of configured submodules.
func (g *GitRepo) Submodules() ([]SubmoduleInfo, error) {
	wt, err := g.repo.Worktree()
	if err != nil {
		return nil, fmt.Errorf("failed to get worktree: %w", err)
	}

	subs, err := wt.Submodules()
	if err != nil {
		return nil, fmt.Errorf("failed to list submodules: %w", err)
	}

	var result []SubmoduleInfo
	for _, sub := range subs {
		cfg := sub.Config()
		info := SubmoduleInfo{
			Name: cfg.Name,
			Path: cfg.Path,
			URL:  cfg.URL,
		}

		// Check if submodule is checked out by trying to get its status.
		status, err := sub.Status()
		if err != nil || !status.IsClean() {
			info.Status = "not_checked_out"
		} else {
			info.Status = "checked_out"
		}

		result = append(result, info)
	}
	return result, nil
}

// SubmoduleInfo holds metadata about a git submodule.
type SubmoduleInfo struct {
	Name   string
	Path   string
	URL    string
	Status string // "checked_out" or "not_checked_out"
}

// FileChurnStats walks the commit history and aggregates per-file change statistics.
// maxCommits limits how far back to look (0 = DefaultMaxCommits).
func (g *GitRepo) FileChurnStats(maxCommits int) (map[string]*ChurnStat, error) {
	if maxCommits <= 0 {
		maxCommits = DefaultMaxCommits
	}

	logIter, err := g.repo.Log(&git.LogOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get commit log: %w", err)
	}
	defer logIter.Close()

	stats := make(map[string]*ChurnStat)
	count := 0

	err = logIter.ForEach(func(c *object.Commit) error {
		if count >= maxCommits {
			return fmt.Errorf("stop") // Use error to break iteration
		}
		count++

		// Get file stats for this commit
		fileStats, err := commitFileStats(c)
		if err != nil {
			// Skip commits we can't diff (e.g., initial commit in shallow clone)
			return nil
		}

		for _, fs := range fileStats {
			s, ok := stats[fs.Name]
			if !ok {
				s = &ChurnStat{FilePath: fs.Name}
				stats[fs.Name] = s
			}
			s.Commits++
			s.LinesChanged += fs.Addition + fs.Deletion
		}

		return nil
	})
	// "stop" error is expected — it's our way to break the iteration
	if err != nil && err.Error() != "stop" {
		return nil, fmt.Errorf("failed to iterate commits: %w", err)
	}

	return stats, nil
}

// TopChurnFiles returns the top N files by churn score (commits × change magnitude).
func TopChurnFiles(stats map[string]*ChurnStat, topN int) []*ChurnStat {
	if topN <= 0 {
		topN = 50
	}

	// Collect into slice
	all := make([]*ChurnStat, 0, len(stats))
	for _, s := range stats {
		all = append(all, s)
	}

	// Sort by combined score: commits * (1 + linesChanged/100) for balanced ranking
	sort.Slice(all, func(i, j int) bool {
		scoreI := float64(all[i].Commits) * (1.0 + float64(all[i].LinesChanged)/100.0)
		scoreJ := float64(all[j].Commits) * (1.0 + float64(all[j].LinesChanged)/100.0)
		return scoreI > scoreJ
	})

	if len(all) > topN {
		all = all[:topN]
	}
	return all
}

// commitFileStats gets the file-level diff stats for a single commit.
// go-git's Commit.Stats() handles parent diffing internally for all cases,
// including initial commits (no parents) and merge commits.
func commitFileStats(c *object.Commit) (object.FileStats, error) {
	return c.Stats()
}
