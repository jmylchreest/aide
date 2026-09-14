// Package checkout identifies a working copy independently of its branch and HEAD.
package checkout

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/jmylchreest/aide/aide/pkg/anchor"
)

type Info struct {
	ID        string `json:"id"`
	Root      string `json:"root"`
	GitDir    string `json:"git_dir,omitempty"`
	CommonDir string `json:"common_dir,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Commit    string `json:"commit,omitempty"`
}

// RootFor selects the caller's checkout only when it belongs to this project.
// An explicit database for a different project continues to address that project.
func RootFor(project, cwd string) string {
	project = canonical(project)
	root, admin, common, err := locate(cwd)
	if err == nil {
		_, _, expected, e := locate(project)
		if e == nil && common == expected && admin != "" {
			return root
		}
	}
	return project
}

// Resolve validates repository membership and persists an opaque identity in
// per-worktree Git metadata. The file survives moves and branch changes, and
// disappears with Git's registration on removal. Non-Git projects use .aide.
func Resolve(project, path string) (Info, error) {
	project = canonical(project)
	root, admin, common, err := locate(path)
	if err != nil {
		if !anchor.Contains(project, canonical(path)) {
			return Info{}, fmt.Errorf("checkout is outside project: %s", path)
		}
		if _, e := os.Stat(filepath.Join(project, ".git")); !os.IsNotExist(e) {
			return Info{}, err
		}
		root, admin, common = project, "", ""
	} else {
		_, _, expected, e := locate(project)
		if e != nil || common != expected {
			return Info{}, fmt.Errorf("checkout %s does not belong to %s", path, project)
		}
	}
	markerDir := admin
	if markerDir == "" {
		markerDir = filepath.Join(root, ".aide", "local")
		if err := EnsureIgnoredDir(markerDir); err != nil {
			return Info{}, err
		}
	}
	if err := os.MkdirAll(markerDir, 0700); err != nil {
		return Info{}, err
	}
	id, err := identity(filepath.Join(markerDir, "aide-checkout-id"))
	if err != nil {
		return Info{}, err
	}
	c := Info{ID: id, Root: root, GitDir: admin, CommonDir: common}
	if repo, e := git.PlainOpenWithOptions(root, &git.PlainOpenOptions{EnableDotGitCommonDir: true}); e == nil {
		if ref, e := repo.Reference(plumbing.HEAD, false); e == nil && ref.Type() == plumbing.SymbolicReference && ref.Target().IsBranch() {
			c.Branch = ref.Target().Short()
		}
		if head, e := repo.Head(); e == nil {
			c.Commit = head.Hash().String()
			if head.Name().IsBranch() {
				c.Branch = head.Name().Short()
			}
		}
	}
	return c, nil
}

// EnsureIgnoredDir installs a default only on first use. Existing ignore files
// belong to the user and are never overwritten, including explicit overrides.
func EnsureIgnoredDir(dir string) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, ".gitignore"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString("# Generated local data; edit this file to explicitly opt into tracking.\n*\n")
	ce := f.Close()
	if err != nil {
		return err
	}
	return ce
}

func canonical(p string) string {
	a, e := filepath.Abs(p)
	if e != nil {
		return anchor.RealPath(p)
	}
	return anchor.RealPath(a)
}

// SourcePath resolves a file only within the selected checkout, including
// symlink resolution. Relative paths never depend on the daemon's cwd.
func SourcePath(root, path string) (string, string, error) {
	if path == "" {
		return "", "", fmt.Errorf("file path is required")
	}
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, path)
	}
	abs = filepath.Clean(abs)
	if !anchor.Contains(root, abs) || !anchor.Contains(anchor.RealPath(root), anchor.RealPath(abs)) {
		return "", "", fmt.Errorf("file %q is outside checkout", path)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", "", err
	}
	return abs, rel, nil
}

func locate(path string) (root, admin, common string, err error) {
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{DetectDotGit: true, EnableDotGitCommonDir: true})
	if err != nil {
		return "", "", "", err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return "", "", "", err
	}
	root = canonical(wt.Filesystem.Root())
	admin = filepath.Join(root, ".git")
	stat, err := os.Stat(admin)
	if err != nil {
		return "", "", "", err
	}
	if !stat.IsDir() {
		data, e := os.ReadFile(admin)
		if e != nil {
			return "", "", "", e
		}
		pointer := strings.TrimSpace(string(data))
		if !strings.HasPrefix(pointer, "gitdir:") {
			return "", "", "", fmt.Errorf("invalid gitdir")
		}
		admin = strings.TrimSpace(strings.TrimPrefix(pointer, "gitdir:"))
		if !filepath.IsAbs(admin) {
			admin = filepath.Join(root, admin)
		}
	}
	admin = canonical(admin)
	common = admin
	if data, e := os.ReadFile(filepath.Join(admin, "commondir")); e == nil {
		common = strings.TrimSpace(string(data))
		if !filepath.IsAbs(common) {
			common = filepath.Join(admin, common)
		}
		common = canonical(common)
	} else if !os.IsNotExist(e) {
		return "", "", "", e
	}
	return root, admin, common, nil
}

func validID(id string) bool { b, e := hex.DecodeString(id); return e == nil && len(b) == 16 }

func identity(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(data))
		if !validID(id) {
			return "", fmt.Errorf("invalid checkout identity at %s", path)
		}
		return id, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(value[:])
	tmp, err := os.CreateTemp(filepath.Dir(path), ".aide-checkout-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.WriteString(id + "\n"); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	// Publish a complete file without replacing another process's identity.
	if err = os.Link(tmp.Name(), path); err != nil && !os.IsExist(err) {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	id = strings.TrimSpace(string(data))
	if !validID(id) {
		return "", fmt.Errorf("invalid checkout identity at %s", path)
	}
	return id, nil
}
