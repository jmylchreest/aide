package watcher

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

var watchLog = log.New(os.Stderr, "[aide:watcher] ", log.Ltime)

const DefaultDebounceDelay = 30 * time.Second

// DefaultSkipDirs contains directories to skip during file watching.
// Organized by language/ecosystem but applied universally for simplicity.
var DefaultSkipDirs = map[string]bool{
	// Version control
	".git": true, ".svn": true, ".hg": true,

	// Aide internal
	".aide": true,

	// Node/JavaScript/TypeScript
	"node_modules": true,
	"dist":         true,
	".next":        true,
	".nuxt":        true,
	"coverage":     true,
	".cache":       true,

	// Python
	"__pycache__":   true,
	".venv":         true,
	"venv":          true,
	".tox":          true,
	".mypy_cache":   true,
	".pytest_cache": true,
	"*.egg-info":    true,
	"site-packages": true,

	// Go
	"vendor": true,

	// Rust
	"target": true,

	// Java/Kotlin/Gradle
	"build":   true,
	".gradle": true,
	"out":     true,

	// C/C++
	"cmake-build-debug":   true,
	"cmake-build-release": true,
	".cmake":              true,
	".deps":               true,
	"Debug":               true,
	"Release":             true,

	// Ruby
	".bundle": true,

	// C#
	"bin": true,
	"obj": true,

	// Elixir
	"_build": true,
	"deps":   true,

	// OCaml
	"_opam": true,

	// Scala
	".bloop":  true,
	".metals": true,

	// Swift
	".build": true,

	// IDE/Editor
	".idea":   true,
	".vscode": true,

	// OS
	".DS_Store": true,
}

type Config struct {
	Paths         []string
	DebounceDelay time.Duration
	SkipDirs      []string
	FileFilter    func(path string) bool
}

type FileChangeHandler interface {
	OnChanges(files map[string]fsnotify.Op)
}

type FileChangeHandlerFunc func(files map[string]fsnotify.Op)

func (f FileChangeHandlerFunc) OnChanges(files map[string]fsnotify.Op) {
	f(files)
}

type Watcher struct {
	fsnotify  *fsnotify.Watcher
	config    Config
	handlers  []FileChangeHandler
	stop      chan struct{}
	stopOnce  sync.Once
	wg        sync.WaitGroup
	startTime time.Time

	mu           sync.Mutex
	pending      map[string]fsnotify.Op
	debounceOnce sync.Once
	watchPaths   []string
	dirsWatched  int
}

func New(config Config, handlers ...FileChangeHandler) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if config.DebounceDelay == 0 {
		config.DebounceDelay = DefaultDebounceDelay
	}

	skipSet := make(map[string]bool)
	for k, v := range DefaultSkipDirs {
		skipSet[k] = v
	}
	for _, d := range config.SkipDirs {
		skipSet[d] = true
	}

	return &Watcher{
		fsnotify: fsWatcher,
		config:   config,
		handlers: handlers,
		stop:     make(chan struct{}),
		pending:  make(map[string]fsnotify.Op),
	}, nil
}

func (w *Watcher) AddHandler(h FileChangeHandler) {
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
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				name := info.Name()
				if DefaultSkipDirs[name] || (len(name) > 1 && name[0] == '.') {
					return filepath.SkipDir
				}
				if err := w.fsnotify.Add(path); err == nil {
					w.dirsWatched++
				}
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	w.startTime = time.Now()
	w.wg.Add(1)
	go w.processEvents()

	watchLog.Printf("watching %d directories in %v (debounce: %v)", w.dirsWatched, paths, w.config.DebounceDelay)
	return nil
}

func (w *Watcher) Stop() error {
	w.stopOnce.Do(func() { close(w.stop) })
	w.wg.Wait()
	return w.fsnotify.Close()
}

func (w *Watcher) Stats() WatcherStats {
	w.mu.Lock()
	pending := len(w.pending)
	w.mu.Unlock()

	return WatcherStats{
		Enabled:      true,
		Paths:        w.watchPaths,
		DirsWatched:  w.dirsWatched,
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
					name := filepath.Base(event.Name)
					if !DefaultSkipDirs[name] && !(len(name) > 1 && name[0] == '.') {
						if err := w.fsnotify.Add(event.Name); err == nil {
							w.dirsWatched++
							watchLog.Printf("watching new directory: %s", event.Name)
						}
					}
					continue
				}
			}

			if w.config.FileFilter != nil && !w.config.FileFilter(event.Name) {
				continue
			}

			name := filepath.Base(event.Name)
			if strings.HasPrefix(name, ".") || strings.HasSuffix(name, "~") ||
				strings.HasSuffix(name, ".swp") || strings.HasSuffix(name, ".tmp") {
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
	w.pending[path] = op
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
	w.mu.Unlock()

	if len(pending) == 0 {
		return
	}

	watchLog.Printf("processing %d file changes", len(pending))

	for _, h := range w.handlers {
		h.OnChanges(pending)
	}
}

func IsRemove(op fsnotify.Op) bool {
	return op&fsnotify.Remove != 0
}
