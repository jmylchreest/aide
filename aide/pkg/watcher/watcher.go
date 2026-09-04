// Package watcher provides file system watching for aide projects.
package watcher

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/jmylchreest/aide/aide/pkg/aideignore"
)

var watchLog = log.New(os.Stderr, "[aide:watcher] ", log.Ltime)

const DefaultDebounceDelay = 30 * time.Second

type Config struct {
	Paths []string
	// ProjectRoot is the absolute path the ignore matcher resolves against.
	// Defaults to the first entry in Paths, then to the working directory.
	ProjectRoot   string
	DebounceDelay time.Duration
	FileFilter    func(path string) bool
	// Ignore decides which directories and files are watched. Defaults to
	// the built-in patterns. Must be the same matcher the indexer and the
	// analysers use, or incremental and full-scan results diverge.
	Ignore *aideignore.Matcher
}

type FileChangeHandler interface {
	OnChanges(files map[string]fsnotify.Op)
}

type FileChangeHandlerFunc func(files map[string]fsnotify.Op)

func (f FileChangeHandlerFunc) OnChanges(files map[string]fsnotify.Op) {
	f(files)
}

type Watcher struct {
	fsnotify    *fsnotify.Watcher
	config      Config
	projectRoot string
	stop        chan struct{}
	stopOnce    sync.Once
	wg          sync.WaitGroup
	startTime   time.Time

	mu           sync.Mutex
	handlers     []FileChangeHandler
	pending      map[string]fsnotify.Op
	debounceOnce sync.Once
	watchPaths   []string
	dirsWatched  atomic.Int32
}

// isTransientName matches dotfiles and editor scratch files.
func isTransientName(name string) bool {
	return strings.HasPrefix(name, ".") ||
		strings.HasSuffix(name, "~") ||
		strings.HasSuffix(name, ".swp") ||
		strings.HasSuffix(name, ".tmp")
}

// rel returns path relative to the project root, the form the ignore matcher
// expects. Paths outside the root are returned unchanged.
func (w *Watcher) rel(path string) string {
	r, err := filepath.Rel(w.projectRoot, path)
	if err != nil {
		return path
	}
	return r
}

// shouldSkipDir reports whether a directory and its contents go unwatched.
func (w *Watcher) shouldSkipDir(path string) bool {
	r := w.rel(path)
	if r == "." || r == "" {
		return false
	}
	return w.config.Ignore.ShouldIgnoreDir(r)
}

// shouldWatchFile applies every filter a file must pass to reach a handler.
// Both the event path and the backfill walk use it.
func (w *Watcher) shouldWatchFile(path string) bool {
	if isTransientName(filepath.Base(path)) {
		return false
	}
	if w.config.FileFilter != nil && !w.config.FileFilter(path) {
		return false
	}
	return !w.config.Ignore.ShouldIgnoreFile(w.rel(path))
}

func New(config Config, handlers ...FileChangeHandler) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if config.DebounceDelay == 0 {
		config.DebounceDelay = DefaultDebounceDelay
	}
	if config.Ignore == nil {
		config.Ignore = aideignore.NewFromDefaults()
	}

	root := config.ProjectRoot
	if root == "" && len(config.Paths) > 0 {
		root = config.Paths[0]
	}
	if root == "" {
		root, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(root); err == nil {
		root = abs
	}

	return &Watcher{
		fsnotify:    fsWatcher,
		config:      config,
		projectRoot: root,
		handlers:    handlers,
		stop:        make(chan struct{}),
		pending:     make(map[string]fsnotify.Op),
	}, nil
}

func (w *Watcher) AddHandler(h FileChangeHandler) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.handlers = append(w.handlers, h)
}

func (w *Watcher) Start() error {
	paths := w.config.Paths
	if len(paths) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		paths = []string{cwd}
	}

	w.watchPaths = paths

	for _, root := range paths {
		// No backfill: reconciling the existing tree is Indexer.Reconcile's
		// job, and queueing it here would re-analyse the project on every start.
		w.addTree(root, false)
	}

	w.startTime = time.Now()
	w.wg.Add(1)
	go w.processEvents()

	watchLog.Printf("watching %d directories in %v (debounce: %v)", w.dirsWatched.Load(), paths, w.config.DebounceDelay)
	return nil
}

// addTree watches dir and every non-ignored directory beneath it, recursing
// because `mkdir -p a/b/c` reports only "a".
//
// When backfill is true it also queues the files already present. Nothing
// else ever sees them: a directory has no watch until it is registered, so
// anything written before that — a file created right after mkdir, or the
// contents of a directory renamed into place — emits no event at all.
func (w *Watcher) addTree(dir string, backfill bool) {
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			// The caller vetted dir; skipping it here would make an
			// explicitly configured watch path unwatchable.
			if path != dir && w.shouldSkipDir(path) {
				return filepath.SkipDir
			}
			if err := w.fsnotify.Add(path); err == nil {
				w.dirsWatched.Add(1)
			}
			return nil
		}
		if !backfill || !w.shouldWatchFile(path) {
			return nil
		}
		w.queueChange(path, fsnotify.Create)
		return nil
	})
}

func (w *Watcher) Stop() error {
	w.stopOnce.Do(func() { close(w.stop) })
	w.wg.Wait()
	return w.fsnotify.Close()
}

// Close releases all resources. It is safe to call Close without ever
// calling Start — the underlying fsnotify watcher is always closed.
// Close is equivalent to Stop but provided for symmetry with New.
func (w *Watcher) Close() error {
	return w.Stop()
}

func (w *Watcher) Stats() WatcherStats {
	w.mu.Lock()
	pending := len(w.pending)
	w.mu.Unlock()

	return WatcherStats{
		Enabled:      true,
		Paths:        w.watchPaths,
		DirsWatched:  int(w.dirsWatched.Load()),
		Debounce:     w.config.DebounceDelay,
		PendingFiles: pending,
		Uptime:       time.Since(w.startTime),
	}
}

type WatcherStats struct {
	Enabled      bool
	Paths        []string
	DirsWatched  int
	Debounce     time.Duration
	PendingFiles int
	Uptime       time.Duration
}

func (w *Watcher) processEvents() {
	defer w.wg.Done()

	for {
		select {
		case <-w.stop:
			return

		case event, ok := <-w.fsnotify.Events:
			if !ok {
				return
			}

			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if !w.shouldSkipDir(event.Name) {
						watchLog.Printf("watching new directory: %s", event.Name)
						w.addTree(event.Name, true)
					}
					continue
				}
			}

			if !w.shouldWatchFile(event.Name) {
				continue
			}

			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) != 0 {
				w.queueChange(event.Name, event.Op)
			}

		case err, ok := <-w.fsnotify.Errors:
			if !ok {
				return
			}
			watchLog.Printf("error: %v", err)
		}
	}
}

func (w *Watcher) queueChange(path string, op fsnotify.Op) {
	w.mu.Lock()
	// Union, not assignment: a write then a delete in one window is a delete.
	w.pending[path] |= op
	w.debounceOnce.Do(func() {
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			select {
			case <-time.After(w.config.DebounceDelay):
				w.flushPending()
			case <-w.stop:
				return
			}
		}()
	})
	w.mu.Unlock()
}

func (w *Watcher) flushPending() {
	w.mu.Lock()
	pending := w.pending
	w.pending = make(map[string]fsnotify.Op)
	w.debounceOnce = sync.Once{}
	// Copy handlers under lock to avoid racing with AddHandler.
	handlers := make([]FileChangeHandler, len(w.handlers))
	copy(handlers, w.handlers)
	w.mu.Unlock()

	if len(pending) == 0 {
		return
	}

	normaliseRemovals(pending)

	watchLog.Printf("processing %d file changes", len(pending))

	for _, h := range handlers {
		func() {
			defer func() {
				if r := recover(); r != nil {
					watchLog.Printf("handler panicked: %v", r)
				}
			}()
			h.OnChanges(pending)
		}()
	}
}

// normaliseRemovals marks any renamed-or-removed path that is gone from disk
// as a removal. fsnotify reports a rename as RENAME on the source path, and
// backends disagree on which op a delete produces; without this a handler
// re-indexes a file that no longer exists and keeps its stale rows forever.
func normaliseRemovals(pending map[string]fsnotify.Op) {
	for path, op := range pending {
		if op&(fsnotify.Rename|fsnotify.Remove) == 0 {
			continue
		}
		if _, err := os.Lstat(path); os.IsNotExist(err) {
			pending[path] = op | fsnotify.Remove
		}
	}
}

func IsRemove(op fsnotify.Op) bool {
	return op&fsnotify.Remove != 0
}
