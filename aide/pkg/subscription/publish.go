package subscription

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/jmylchreest/aide/aide/pkg/config"
)

// publishRetries bounds the fetch/reset/write/commit/push loop when
// another publisher lands a commit between our fetch and push.
const publishRetries = 3

// Publish writes this project's own records into a publish-enabled
// subscription and returns whether anything new shipped. write receives
// the share-tree root and applies the records (write-once files, so
// re-applying after a retry reset is idempotent — the format's
// multi-writer safety rules, not git's).
//
// Git subscriptions run fetch → hard-reset to the remote head → write →
// commit → push, refetching and retrying on a push race. Path
// subscriptions just apply write in place — the directory's versioning,
// if any, is the user's.
func Publish(ctx context.Context, projectRoot string, sub config.SubscriptionConfig, write func(shareRoot string) error) (bool, error) {
	if err := validate(sub); err != nil {
		return false, err
	}
	if !sub.Publish {
		return false, fmt.Errorf("subscription %q is not publish-enabled", sub.Name)
	}

	if sub.Path != "" {
		root, err := publishRoot(sub.Name, resolvePath(projectRoot, sub.Path))
		if err != nil {
			return false, err
		}
		return true, write(root)
	}

	dir := CacheDir(projectRoot, sub.Name)
	var lastErr error
	for attempt := 0; attempt < publishRetries; attempt++ {
		repo, empty, err := openOrClone(ctx, dir, sub.URL, sub.Branch)
		if err != nil {
			return false, fmt.Errorf("subscription %q: %w", sub.Name, err)
		}
		if !empty {
			if err := resetToRemote(ctx, repo, sub.Branch); err != nil {
				return false, fmt.Errorf("subscription %q: %w", sub.Name, err)
			}
		}

		root, err := publishRoot(sub.Name, dir)
		if err != nil {
			return false, err
		}
		if err := write(root); err != nil {
			return false, err
		}

		wt, err := repo.Worktree()
		if err != nil {
			return false, err
		}
		if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
			return false, err
		}
		status, err := wt.Status()
		if err != nil {
			return false, err
		}
		if onlyManifestChanged(status) {
			return false, nil
		}

		if _, err := wt.Commit("aide sync: publish context records", &git.CommitOptions{
			Author: commitAuthor(repo),
		}); err != nil {
			return false, err
		}

		err = repo.PushContext(ctx, &git.PushOptions{})
		if err == nil || errors.Is(err, git.NoErrAlreadyUpToDate) {
			stampSync(dir)
			return true, nil
		}
		if !isPushRace(err) {
			return false, fmt.Errorf("subscription %q: push: %w", sub.Name, err)
		}
		lastErr = err
	}
	return false, fmt.Errorf("subscription %q: push kept racing after %d attempts: %w", sub.Name, publishRetries, lastErr)
}

// openOrClone opens the cache checkout, cloning when absent. A brand-new
// empty remote (nothing to clone) is initialised locally with origin
// configured; the first publish's push creates the branch.
func openOrClone(ctx context.Context, dir, url, branch string) (repo *git.Repository, emptyRemote bool, err error) {
	repo, err = git.PlainOpen(dir)
	if err == nil {
		return repo, false, nil
	}
	if !errors.Is(err, git.ErrRepositoryNotExists) {
		return nil, false, err
	}

	var ref plumbing.ReferenceName
	if branch != "" {
		ref = plumbing.NewBranchReferenceName(branch)
	}
	repo, err = git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		URL:           url,
		ReferenceName: ref,
		SingleBranch:  branch != "",
	})
	if err == nil {
		return repo, false, nil
	}
	if !errors.Is(err, transport.ErrEmptyRemoteRepository) {
		return nil, false, err
	}

	repo, err = git.PlainInit(dir, false)
	if err != nil {
		return nil, false, err
	}
	if _, err := repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{url},
	}); err != nil {
		return nil, false, err
	}
	if branch != "" {
		head := plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.NewBranchReferenceName(branch))
		if err := repo.Storer.SetReference(head); err != nil {
			return nil, false, err
		}
	}
	return repo, true, nil
}

// resetToRemote fetches origin and hard-resets the worktree to the remote
// head, discarding any local commit left by a failed prior push so every
// attempt builds directly on what the remote has.
func resetToRemote(ctx context.Context, repo *git.Repository, branch string) error {
	err := repo.FetchContext(ctx, &git.FetchOptions{})
	if err != nil && !errors.Is(err, git.NoErrAlreadyUpToDate) {
		return err
	}
	if branch == "" {
		head, err := repo.Head()
		if err != nil {
			return err
		}
		branch = head.Name().Short()
	}
	remote, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	return wt.Reset(&git.ResetOptions{Mode: git.HardReset, Commit: remote.Hash()})
}

// publishRoot is shareRoot with a fallback: a tree with no records yet
// publishes at the directory root (the dedicated-context-repo layout).
func publishRoot(name, dir string) (string, error) {
	if root, err := shareRoot(name, dir); err == nil {
		return root, nil
	}
	return dir, nil
}

// onlyManifestChanged reports whether the staged changes carry no record
// content — nothing, or just the manifest watermark, which is the only
// byte an export of unchanged content rewrites. Publishing a
// watermark-only commit every sync would be pure churn.
func onlyManifestChanged(status git.Status) bool {
	for path, s := range status {
		if s.Staging == git.Unmodified && s.Worktree == git.Unmodified {
			continue
		}
		if !strings.HasSuffix(path, "/"+"manifest.json") && path != "manifest.json" {
			return false
		}
	}
	return true
}

// commitAuthor prefers the user's real git identity so context-repo
// history reads like any other repo; falls back to a fixed aide identity
// when none is configured.
func commitAuthor(repo *git.Repository) *object.Signature {
	sig := &object.Signature{Name: "aide sync", Email: "aide-sync@localhost", When: time.Now()}
	cfg, err := repo.ConfigScoped(gitconfig.GlobalScope)
	if err == nil && cfg.User.Name != "" {
		sig.Name = cfg.User.Name
		sig.Email = cfg.User.Email
	}
	return sig
}

func isPushRace(err error) bool {
	return err != nil && strings.Contains(err.Error(), "non-fast-forward")
}
