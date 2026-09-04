package watcher

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
)

const (
	testDebounce = 150 * time.Millisecond
	// settle covers inotify, the debounce timer and the handler, with enough
	// headroom that a loaded CI box doesn't flake.
	settle = 800 * time.Millisecond
)

type collector struct {
	mu    sync.Mutex
	seen  map[string]fsnotify.Op
	calls int
}

func newCollector() *collector {
	return &collector{seen: map[string]fsnotify.Op{}}
}

func (c *collector) OnChanges(files map[string]fsnotify.Op) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	for k, v := range files {
		c.seen[k] |= v
	}
}

func (c *collector) op(path string) (fsnotify.Op, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	op, ok := c.seen[path]
	return op, ok
}

func (c *collector) has(path string) bool {
	_, ok := c.op(path)
	return ok
}

func (c *collector) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.seen)
}

// testRoot returns a temp directory with symlinks resolved: on macOS /tmp is a
// symlink, so fsnotify would report paths the test can't match.
func testRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func startWatcher(t *testing.T, root string, ignore *aideignore.Matcher) *collector {
	t.Helper()
	c := newCollector()
	w, err := New(Config{
		Paths:         []string{root},
		ProjectRoot:   root,
		DebounceDelay: testDebounce,
		FileFilter:    func(p string) bool { return strings.HasSuffix(p, ".go") },
		Ignore:        ignore,
	}, c)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Stop() })
	time.Sleep(100 * time.Millisecond) // let the watches register
	return c
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Guards the rest of the suite against a watcher that reports nothing at all.
func TestWatcherReportsWriteInWatchedDir(t *testing.T) {
	root := testRoot(t)
	c := startWatcher(t, root, nil)

	path := filepath.Join(root, "a.go")
	writeFile(t, path)

	time.Sleep(settle)
	if !c.has(path) {
		t.Fatalf("write in watched directory not reported: %s", path)
	}
}

// A file written immediately after mkdir emits no event — the directory has no
// watch yet — so only the backfill walk sees it.
func TestWatcherBackfillsNewDirectory(t *testing.T) {
	root := testRoot(t)
	c := startWatcher(t, root, nil)

	dir := filepath.Join(root, "pkg")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "new.go")
	writeFile(t, path)

	time.Sleep(settle)
	if !c.has(path) {
		t.Fatalf("file in newly created directory not reported: %s", path)
	}
}

// `mkdir -p a/b/c` reports only "a"; without recursing, b and c never get
// watched and nothing inside them is seen again.
func TestWatcherWatchesNestedNewDirectories(t *testing.T) {
	root := testRoot(t)
	c := startWatcher(t, root, nil)

	dir := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "deep.go")
	writeFile(t, path)

	time.Sleep(settle)
	if !c.has(path) {
		t.Fatalf("file in nested new directory not reported: %s", path)
	}

	// A later write proves the leaf got a watch, rather than the file merely
	// being caught by the backfill.
	c.mu.Lock()
	delete(c.seen, path)
	c.mu.Unlock()

	writeFile(t, path)
	time.Sleep(settle)
	if !c.has(path) {
		t.Fatalf("nested directory did not get an inotify watch: %s", path)
	}
}

// A directory renamed into place arrives with its contents already inside, so
// no per-file event is emitted. This is what a branch checkout looks like.
func TestWatcherBackfillsDirectoryRenamedIntoPlace(t *testing.T) {
	root := testRoot(t)
	staging := testRoot(t)
	c := startWatcher(t, root, nil)

	src := filepath.Join(staging, "mod")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "x.go"))

	dst := filepath.Join(root, "mod")
	if err := os.Rename(src, dst); err != nil {
		t.Skipf("cross-device rename unavailable: %v", err)
	}

	time.Sleep(settle)
	if !c.has(filepath.Join(dst, "x.go")) {
		t.Fatalf("contents of directory renamed into place not reported")
	}
}

// fsnotify reports a rename as RENAME on the source path; handlers must see a
// removal or they keep the vanished file's stale rows forever.
func TestWatcherNormalisesRenameToRemove(t *testing.T) {
	root := testRoot(t)
	old := filepath.Join(root, "old.go")
	writeFile(t, old)

	c := startWatcher(t, root, nil)

	renamed := filepath.Join(root, "new.go")
	if err := os.Rename(old, renamed); err != nil {
		t.Fatal(err)
	}

	time.Sleep(settle)

	op, ok := c.op(old)
	if !ok {
		t.Fatalf("rename source not reported: %s", old)
	}
	if !IsRemove(op) {
		t.Errorf("rename source op = %v, want a removal", op)
	}

	op, ok = c.op(renamed)
	if !ok {
		t.Fatalf("rename destination not reported: %s", renamed)
	}
	if IsRemove(op) {
		t.Errorf("rename destination op = %v, want a live file", op)
	}
}

// A delete arriving after a write in the same window is still a delete.
func TestWatcherCoalescesWriteThenRemoveAsRemove(t *testing.T) {
	root := testRoot(t)
	path := filepath.Join(root, "doomed.go")
	writeFile(t, path)

	c := startWatcher(t, root, nil)

	writeFile(t, path)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}

	time.Sleep(settle)

	op, ok := c.op(path)
	if !ok {
		t.Fatalf("write-then-remove not reported: %s", path)
	}
	if !IsRemove(op) {
		t.Errorf("op = %v, want a removal", op)
	}
}

// Start must not replay the existing tree — that is Indexer.Reconcile's job.
func TestWatcherStartDoesNotBackfillExistingTree(t *testing.T) {
	root := testRoot(t)
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "a.go"))
	writeFile(t, filepath.Join(root, "sub", "b.go"))

	c := startWatcher(t, root, nil)
	time.Sleep(settle)

	if n := c.count(); n != 0 {
		t.Errorf("Start reported %d pre-existing files, want 0", n)
	}
}

// The ignore matcher applies to directories that appear after startup too.
func TestWatcherHonoursIgnoreMatcherForNewDirectory(t *testing.T) {
	root := testRoot(t)
	c := startWatcher(t, root, nil)

	dir := filepath.Join(root, "node_modules", "pkg")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "ignored.go"))

	time.Sleep(settle)
	if n := c.count(); n != 0 {
		t.Errorf("reported %d files under an ignored directory, want 0", n)
	}
}

// A .aideignore negation re-includes a directory the built-in defaults exclude.
// The watcher could not honour this while it carried its own skip list.
func TestWatcherHonoursIgnoreNegation(t *testing.T) {
	root := testRoot(t)
	if err := os.WriteFile(filepath.Join(root, ".aideignore"), []byte("!dist/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignore, err := aideignore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if ignore.ShouldIgnoreDir("dist") {
		t.Skip("aideignore does not re-include dist/; negation semantics differ")
	}

	c := startWatcher(t, root, ignore)

	dir := filepath.Join(root, "dist")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "kept.go")
	writeFile(t, path)

	time.Sleep(settle)
	if !c.has(path) {
		t.Errorf("re-included directory not watched: %s", path)
	}
}

// Rejected and scratch files reach handlers from neither path.
func TestWatcherFiltersNonMatchingAndTransientFiles(t *testing.T) {
	root := testRoot(t)
	c := startWatcher(t, root, nil)

	dir := filepath.Join(root, "pkg")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "keep.go"))
	writeFile(t, filepath.Join(dir, "skip.md"))
	writeFile(t, filepath.Join(dir, ".hidden.go"))
	writeFile(t, filepath.Join(dir, "scratch.go~"))
	writeFile(t, filepath.Join(dir, "scratch.go.swp"))

	time.Sleep(settle)

	if !c.has(filepath.Join(dir, "keep.go")) {
		t.Error("matching file not reported")
	}
	for _, name := range []string{"skip.md", ".hidden.go", "scratch.go~", "scratch.go.swp"} {
		if c.has(filepath.Join(dir, name)) {
			t.Errorf("filtered file reported: %s", name)
		}
	}
}
