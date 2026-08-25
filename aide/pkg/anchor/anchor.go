// Package anchor holds the project-scope classification primitives shared by
// the CLI resolver (aide/cmd/aide/cmd_anchor.go) and the survey subproject
// analyzer. There is exactly one implementation of gitdir-shape
// classification and project-identity derivation — do not re-grow copies.
package anchor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	git "github.com/go-git/go-git/v5"
)

// VCSMarkerNames are the recognized VCS root markers, .git first.
var VCSMarkerNames = []string{".git", ".hg", ".svn", ".bzr", ".fossil"}

// Gitdir shape classifications for a .git marker.
const (
	ShapeDirectory = "directory" // plain repo (.git is a directory)
	ShapeWorktree  = "worktree"  // linked worktree (anchors at the main repo)
	ShapeSubmodule = "submodule" // submodule checkout (anchors itself)
	ShapeInvalid   = "invalid"   // .git file that is not a gitdir pointer
)

// IsSubmoduleGitdir reports whether a resolved gitdir path points into a
// superproject's modules tree. Two shapes qualify:
//
//	<super>/.git/modules/<path>                    plain submodule
//	<super>/.git/worktrees/<wt>/modules/<path>     submodule inside a
//	                                               linked worktree of the
//	                                               superproject
func IsSubmoduleGitdir(gitdir string) bool {
	parts := strings.Split(filepath.ToSlash(gitdir), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] != ".git" {
			continue
		}
		j := i + 1
		if parts[j] == "worktrees" && j+2 < len(parts) {
			j += 2
		}
		if j < len(parts) && parts[j] == "modules" {
			return true
		}
	}
	return false
}

// GitfileInfo parses a .git file ("gitdir: <path>") and classifies it:
// ShapeWorktree or ShapeSubmodule, plus the owning repository's root — the
// main repo for a worktree, the superproject for a submodule. Returns
// ("", "") when the file is unreadable or not a gitdir pointer. A dangling
// gitdir target keeps its shape but reports no owner, so callers fall back
// to the checkout directory instead of resurrecting state at a dead path.
func GitfileInfo(gitFilePath string) (shape string, ownerRoot string) {
	data, err := os.ReadFile(gitFilePath)
	if err != nil {
		return "", ""
	}
	line := strings.TrimSpace(string(data))
	if !strings.HasPrefix(line, "gitdir:") {
		return "", ""
	}
	gitdir := strings.TrimSpace(strings.TrimPrefix(line, "gitdir:"))
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(filepath.Dir(gitFilePath), gitdir)
	}

	shape = ShapeWorktree
	if IsSubmoduleGitdir(gitdir) {
		shape = ShapeSubmodule
	}

	if _, err := os.Stat(gitdir); err != nil {
		return shape, ""
	}

	candidate := gitdir
	for {
		parent := filepath.Dir(candidate)
		if parent == candidate {
			break
		}
		if filepath.Base(candidate) == ".git" {
			return shape, parent
		}
		candidate = parent
	}
	return shape, ""
}

// ClassifyDir reports the project marker present at dir and, for a .git
// marker, its shape. Returns ("", "") when dir carries no marker.
func ClassifyDir(dir string) (marker, gitdirShape string) {
	gitPath := filepath.Join(dir, ".git")
	if st, err := os.Stat(gitPath); err == nil {
		if st.IsDir() {
			return ".git", ShapeDirectory
		}
		shape, _ := GitfileInfo(gitPath)
		if shape == "" {
			shape = ShapeInvalid
		}
		return ".git", shape
	}
	for _, m := range VCSMarkerNames[1:] {
		if _, err := os.Stat(filepath.Join(dir, m)); err == nil {
			return m, ""
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".aide")); err == nil {
		return ".aide", ""
	}
	return "", ""
}

// ProjectIdentity derives the project name for root: the last path segment
// of the origin remote URL when one exists (rename-stable), else the
// directory basename. Mirrored by the TS layer's getProjectName.
func ProjectIdentity(root string) (name, source string) {
	if repo, err := git.PlainOpenWithOptions(root, &git.PlainOpenOptions{}); err == nil {
		if remote, err := repo.Remote("origin"); err == nil {
			urls := remote.Config().URLs
			if len(urls) > 0 {
				if n := LastURLSegment(urls[0]); n != "" {
					return n, "git-remote"
				}
			}
		}
	}
	return filepath.Base(root), "basename"
}

// LastURLSegment extracts the repository name from a git remote URL:
// "git@host:org/repo.git" and "https://host/org/repo.git" both yield "repo".
func LastURLSegment(url string) string {
	s := strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(url), "/"), ".git")
	if i := strings.LastIndexAny(s, "/:"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// RealPath resolves symlinks so aliased spellings of one project map to one
// identity — a ~/src tree pointing at a data volume on Unix, a directory
// junction or mapped drive on Windows. On error (the path does not exist yet,
// or the OS refuses to resolve it) it falls back to filepath.Clean, which
// still normalises separators and .. segments rather than handing back a raw
// string. This is the single implementation: callers needing a canonical root
// use it rather than calling filepath.EvalSymlinks themselves.
func RealPath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(p)
}

// Contains reports whether path sits inside root, or is root itself.
//
// It accepts containment under EITHER the literal spellings or the resolved
// ones, because the two answer different questions and both are legitimate:
//
//   - Literal catches a symlinked directory inside the project — a vendored
//     or generated tree linked out to another volume. The link is inside the
//     project as written, so work there belongs to this project, even though
//     its resolved path lands elsewhere entirely.
//   - Resolved catches the project itself reached by an alias — a ~/src
//     symlink onto a data volume, a Windows junction — where the recorded
//     root and the caller's cwd are two spellings of one directory.
//
// Requiring both would reject the first case, which is a containment the
// plain string comparison this replaced got right.
//
// Each comparison walks path segments via filepath.Rel rather than matching a
// string prefix, so "/src/aide-web" is not inside "/src/aide". Case is folded
// on Windows because the filesystem folds it, and Rel's error across volumes
// (C: vs D:) is correctly read as outside.
func Contains(root, path string) bool {
	return containsLexical(root, path) ||
		containsLexical(RealPath(root), RealPath(path))
}

// containsLexical is Contains without the symlink resolution: a segment-wise
// containment test on the paths exactly as given.
func containsLexical(root, path string) bool {
	root, path = filepath.Clean(root), filepath.Clean(path)
	if runtime.GOOS == "windows" {
		root, path = strings.ToLower(root), strings.ToLower(path)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false // different volumes, or otherwise unrelatable
	}
	return filepath.IsLocal(rel) || rel == "."
}

// HasAideStore reports whether dir carries its own .aide directory.
func HasAideStore(dir string) bool {
	st, err := os.Stat(filepath.Join(dir, ".aide"))
	return err == nil && st.IsDir()
}
