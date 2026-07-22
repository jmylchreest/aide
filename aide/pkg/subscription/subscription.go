// Package subscription reads peer context trees named in config
// subscriptions: git repositories fetched into .aide/cache/remotes/<name>/
// or local directories read in place. Peers are a read-only layer —
// decisions only (memories never cross project boundaries), surfaced with
// origin provenance and never re-exported. Promotion into the local store
// is `aide decision adopt`.
package subscription

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	git "github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	gitmemory "github.com/go-git/go-git/v5/storage/memory"
	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/contextshare"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// nameRe bounds subscription names: they become cache directory names, so
// path separators and traversal sequences must be impossible.
var nameRe = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

// syncStamp marks the last successful git fetch; its mtime drives the
// session-init staleness check.
const syncStamp = ".aide-last-sync"

// CacheDir is where a git subscription's checkout lives.
func CacheDir(projectRoot, name string) string {
	return filepath.Join(projectRoot, ".aide", "cache", "remotes", name)
}

func resolvePath(projectRoot, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(projectRoot, p)
}

func validate(sub config.SubscriptionConfig) error {
	if !nameRe.MatchString(sub.Name) {
		return fmt.Errorf("subscription name %q must match %s", sub.Name, nameRe)
	}
	if (sub.URL == "") == (sub.Path == "") {
		return fmt.Errorf("subscription %q must set exactly one of url and path", sub.Name)
	}
	return nil
}

// Sync makes a subscription's records locally readable and returns the
// share-tree root. Git subscriptions clone or pull the cache checkout;
// path subscriptions are validated and read in place (no copy — they are
// already local and always fresh).
func Sync(ctx context.Context, projectRoot string, sub config.SubscriptionConfig) (string, error) {
	if err := validate(sub); err != nil {
		return "", err
	}
	if sub.Path != "" {
		p := resolvePath(projectRoot, sub.Path)
		if info, err := os.Stat(p); err != nil || !info.IsDir() {
			return "", fmt.Errorf("subscription %q path %s is not a directory", sub.Name, p)
		}
		return shareRoot(sub.Name, p)
	}

	dir := CacheDir(projectRoot, sub.Name)
	if err := gitSync(ctx, dir, sub.URL, sub.Branch); err != nil {
		return "", fmt.Errorf("subscription %q: %w", sub.Name, err)
	}
	stampSync(dir)
	return shareRoot(sub.Name, dir)
}

func gitSync(ctx context.Context, dir, url, branch string) error {
	var ref plumbing.ReferenceName
	if branch != "" {
		ref = plumbing.NewBranchReferenceName(branch)
	}
	repo, err := git.PlainOpen(dir)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		_, err = cloneWithFallback(ctx, dir, url, branch)
		return err
	}
	if err != nil {
		return err
	}
	wt, err := repo.Worktree()
	if err != nil {
		return err
	}
	err = wt.PullContext(ctx, &git.PullOptions{
		ReferenceName: ref,
		SingleBranch:  branch != "",
		Force:         true,
	})
	if err != nil && branch == "" && errors.Is(err, plumbing.ErrReferenceNotFound) {
		if db := remoteDefaultBranch(ctx, url); db != "" {
			err = wt.PullContext(ctx, &git.PullOptions{
				ReferenceName: plumbing.NewBranchReferenceName(db),
				SingleBranch:  true,
				Force:         true,
			})
		}
	}
	if errors.Is(err, git.NoErrAlreadyUpToDate) {
		return nil
	}
	return err
}

// cloneWithFallback clones dir from url. A dangling remote HEAD (the
// server's default branch was never pushed — common when a publisher
// bootstrapped an empty repo under a different default) fails ref
// resolution; the retry targets the branch the remote actually has.
func cloneWithFallback(ctx context.Context, dir, url, branch string) (*git.Repository, error) {
	var ref plumbing.ReferenceName
	if branch != "" {
		ref = plumbing.NewBranchReferenceName(branch)
	}
	repo, err := git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
		URL:           url,
		ReferenceName: ref,
		SingleBranch:  branch != "",
	})
	if err != nil && branch == "" && errors.Is(err, plumbing.ErrReferenceNotFound) {
		if db := remoteDefaultBranch(ctx, url); db != "" {
			_ = os.RemoveAll(dir)
			repo, err = git.PlainCloneContext(ctx, dir, false, &git.CloneOptions{
				URL:           url,
				ReferenceName: plumbing.NewBranchReferenceName(db),
				SingleBranch:  true,
			})
		}
	}
	return repo, err
}

// remoteDefaultBranch asks the remote which branch to treat as default:
// HEAD's symref target when advertised, else the branch HEAD's hash
// points at (main/master preferred), else main/master by presence, else
// the first branch alphabetically. Empty when the remote is unreachable
// or has no branches.
func remoteDefaultBranch(ctx context.Context, url string) string {
	rem := git.NewRemote(gitmemory.NewStorage(), &gitconfig.RemoteConfig{
		Name: "origin",
		URLs: []string{url},
	})
	refs, err := rem.ListContext(ctx, &git.ListOptions{})
	if err != nil {
		return ""
	}
	var headHash plumbing.Hash
	branches := map[string]plumbing.Hash{}
	names := []string{}
	for _, r := range refs {
		if r.Name() == plumbing.HEAD {
			if r.Type() == plumbing.SymbolicReference {
				return r.Target().Short()
			}
			headHash = r.Hash()
			continue
		}
		if r.Name().IsBranch() {
			branches[r.Name().Short()] = r.Hash()
			names = append(names, r.Name().Short())
		}
	}
	if !headHash.IsZero() {
		for _, cand := range []string{"main", "master"} {
			if h, ok := branches[cand]; ok && h == headHash {
				return cand
			}
		}
		for _, n := range names {
			if branches[n] == headHash {
				return n
			}
		}
	}
	for _, cand := range []string{"main", "master"} {
		if _, ok := branches[cand]; ok {
			return cand
		}
	}
	sort.Strings(names)
	if len(names) > 0 {
		return names[0]
	}
	return ""
}

func stampSync(dir string) {
	p := filepath.Join(dir, syncStamp)
	if err := os.WriteFile(p, nil, 0o644); err == nil {
		now := time.Now()
		_ = os.Chtimes(p, now, now)
	}
}

// CachedRoot resolves a subscription's share root from local data only —
// no network. Git subscriptions require an existing cache (run `aide sync`
// first); path subscriptions resolve directly.
func CachedRoot(projectRoot string, sub config.SubscriptionConfig) (string, error) {
	if err := validate(sub); err != nil {
		return "", err
	}
	if sub.Path != "" {
		return shareRoot(sub.Name, resolvePath(projectRoot, sub.Path))
	}
	dir := CacheDir(projectRoot, sub.Name)
	if _, err := os.Stat(dir); err != nil {
		return "", fmt.Errorf("subscription %q has no local cache yet — run `aide sync`", sub.Name)
	}
	return shareRoot(sub.Name, dir)
}

// EnsureFresh returns the share root, fetching first only when a git
// subscription has no cache or last synced longer than maxAge ago. Session
// init uses this with a short shared deadline so a dead network never
// stalls startup: when the fetch fails but a cache exists, the stale cache
// is served silently.
func EnsureFresh(ctx context.Context, projectRoot string, sub config.SubscriptionConfig, maxAge time.Duration) (string, error) {
	if err := validate(sub); err != nil {
		return "", err
	}
	if sub.Path != "" {
		return Sync(ctx, projectRoot, sub)
	}
	dir := CacheDir(projectRoot, sub.Name)
	if info, err := os.Stat(filepath.Join(dir, syncStamp)); err == nil && time.Since(info.ModTime()) < maxAge {
		return shareRoot(sub.Name, dir)
	}
	root, syncErr := Sync(ctx, projectRoot, sub)
	if syncErr == nil {
		return root, nil
	}
	if cached, err := CachedRoot(projectRoot, sub); err == nil {
		return cached, nil
	}
	// No cache to fall back on: the fetch failure is the actionable cause,
	// not "run aide sync" (which would fail the same way).
	return "", syncErr
}

// shareRoot locates the record tree inside a subscription target: the
// directory itself (a dedicated context repo), its .aide/shared/ (a normal
// aide project), or context/.
func shareRoot(name, dir string) (string, error) {
	for _, cand := range []string{dir, filepath.Join(dir, ".aide", "shared"), filepath.Join(dir, "context")} {
		if st, err := os.Stat(filepath.Join(cand, "decisions")); err == nil && st.IsDir() {
			return cand, nil
		}
		if _, err := os.Stat(filepath.Join(cand, contextshare.ManifestName)); err == nil {
			return cand, nil
		}
	}
	return "", fmt.Errorf("subscription %q: no context records under %s (expected decisions/ or %s at the root, .aide/shared/, or context/)",
		name, dir, contextshare.ManifestName)
}

// ReadDecisions parses a share tree into its live decisions, latest per
// topic, honouring tombstones. Store-free by construction: records exist
// only in the returned map, so peer content can never leak into a local
// export. The stale-manifest guard is bypassed — this is a read of
// whatever the peer last published, not a merge.
func ReadDecisions(root string) (map[string]*memory.Decision, error) {
	c := newCollector()
	if _, err := contextshare.Import(c, c, root, contextshare.ImportOptions{
		Decisions: true,
		Force:     true,
	}); err != nil {
		return nil, err
	}
	latest := make(map[string]*memory.Decision, len(c.history))
	for topic, versions := range c.history {
		for _, d := range versions {
			if cur, ok := latest[topic]; !ok || d.CreatedAt.After(cur.CreatedAt) {
				latest[topic] = d
			}
		}
	}
	return latest, nil
}

// collector satisfies contextshare.Target and TombstoneAccess in memory,
// turning Import into a pure reader with its tombstone semantics intact.
type collector struct {
	history map[string][]*memory.Decision
	tombs   map[string]*memory.Tombstone
}

func newCollector() *collector {
	return &collector{
		history: make(map[string][]*memory.Decision),
		tombs:   make(map[string]*memory.Tombstone),
	}
}

func (c *collector) GetDecisionHistory(topic string) ([]*memory.Decision, error) {
	return c.history[topic], nil
}

func (c *collector) SetDecision(d *memory.Decision) error {
	c.history[d.Topic] = append(c.history[d.Topic], d)
	return nil
}

func (c *collector) DeleteDecision(topic string) (int, error) {
	n := len(c.history[topic])
	delete(c.history, topic)
	return n, nil
}

func (c *collector) GetMemory(string) (*memory.Memory, error) { return nil, nil }
func (c *collector) AddMemory(*memory.Memory) error           { return nil }
func (c *collector) UpdateMemory(*memory.Memory) error        { return nil }
func (c *collector) DeleteMemory(string) error                { return nil }

func tombKey(kind, id string) string { return kind + "\x00" + id }

func (c *collector) AddTombstone(t *memory.Tombstone) error {
	c.tombs[tombKey(t.Kind, t.ID)] = t
	return nil
}

func (c *collector) GetTombstone(kind, id string) (*memory.Tombstone, error) {
	return c.tombs[tombKey(kind, id)], nil
}

func (c *collector) ListTombstones() ([]*memory.Tombstone, error) {
	out := make([]*memory.Tombstone, 0, len(c.tombs))
	for _, t := range c.tombs {
		out = append(out, t)
	}
	return out, nil
}

func (c *collector) DeleteTombstone(kind, id string) error {
	delete(c.tombs, tombKey(kind, id))
	return nil
}
