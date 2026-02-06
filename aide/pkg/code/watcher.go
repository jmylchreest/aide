// Package code provides code indexing and symbol extraction.
package code

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher watches for file changes and triggers reindexing.
type Watcher struct {
	watcher  *fsnotify.Watcher
	config   WatcherConfig
	onChange func(path string, op fsnotify.Op) // Callback for file changes
	stop     chan struct{}
	wg       sync.WaitGroup

	// Debouncing
	mu           sync.Mutex
	pending      map[string]fsnotify.Op
	debounceOnce sync.Once
}

// NewWatcher creates a new file watcher.
func NewWatcher(config WatcherConfig, onChange func(path string, op fsnotify.Op)) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	if config.DebounceDelay == 0 {
		config.DebounceDelay = DefaultDebounceDelay
	}

	return &Watcher{
		watcher:  fsWatcher,
		config:   config,
		onChange: onChange,
		stop:     make(chan struct{}),
		pending:  make(map[string]fsnotify.Op),
	}, nil
}

// Start begins watching for file changes.
func (w *Watcher) Start() error {
	// Determine paths to watch
	paths := w.config.Paths
	if len(paths) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		paths = []string{cwd}
	}

	// Add all directories recursively
	for _, root := range paths {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				// Skip common non-source directories
				name := info.Name()
				if name == "node_modules" || name == ".git" || name == "vendor" ||
					name == "__pycache__" || name == ".venv" || name == "dist" ||
					name == "build" || name == ".aide" {
					return filepath.SkipDir
				}
				return w.watcher.Add(path)
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	// Start event processing goroutine
	w.wg.Add(1)
	go w.processEvents()

	return nil
}

// Stop stops the watcher.
func (w *Watcher) Stop() error {
	close(w.stop)
	w.wg.Wait()
	return w.watcher.Close()
}

// processEvents handles fsnotify events with debouncing.
func (w *Watcher) processEvents() {
	defer w.wg.Done()

	for {
		select {
		case <-w.stop:
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Handle new directory creation - add to watcher
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					name := filepath.Base(event.Name)
					// Skip excluded directories
					if name != "node_modules" && name != ".git" && name != "vendor" &&
						name != "__pycache__" && name != ".venv" && name != "dist" &&
						name != "build" && name != ".aide" && !strings.HasPrefix(name, ".") {
						if err := w.watcher.Add(event.Name); err == nil {
							log.Printf("[aide:watcher] watching new directory: %s", event.Name)
						}
					}
					continue
				}
			}

			// Only process supported file types
			ext := filepath.Ext(event.Name)
			if !SupportedExtension(ext) {
				continue
			}

			// Skip temporary files
			name := filepath.Base(event.Name)
			if strings.HasPrefix(name, ".") || strings.HasSuffix(name, "~") ||
				strings.HasSuffix(name, ".swp") || strings.HasSuffix(name, ".tmp") {
				continue
			}

			// Handle relevant operations
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
				w.queueChange(event.Name, event.Op)
			} else if event.Op&fsnotify.Remove != 0 {
				w.queueChange(event.Name, event.Op)
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("[aide:watcher] error: %v", err)
		}
	}
}

// queueChange queues a file change for debounced processing.
func (w *Watcher) queueChange(path string, op fsnotify.Op) {
	w.mu.Lock()
	w.pending[path] = op
	w.mu.Unlock()

	// Start debounce timer (only once until flush)
	w.debounceOnce.Do(func() {
		go func() {
			time.Sleep(w.config.DebounceDelay)
			w.flushPending()
		}()
	})
}

// flushPending processes all pending changes.
func (w *Watcher) flushPending() {
	w.mu.Lock()
	pending := w.pending
	w.pending = make(map[string]fsnotify.Op)
	w.debounceOnce = sync.Once{} // Reset for next batch
	w.mu.Unlock()

	if len(pending) == 0 {
		return
	}

	log.Printf("[aide:watcher] processing %d file changes after debounce", len(pending))

	for path, op := range pending {
		if w.onChange != nil {
			w.onChange(path, op)
		}
	}
}

// WatchAndIndex creates a watcher that automatically indexes changed files.
// indexFn should index the file and return the number of symbols, or error.
// removeFn should remove the file from the index.
func WatchAndIndex(
	config WatcherConfig,
	indexFn func(path string) (int, error),
	removeFn func(path string) error,
) (*Watcher, error) {
	onChange := func(path string, op fsnotify.Op) {
		if op&fsnotify.Remove != 0 {
			if err := removeFn(path); err != nil {
				log.Printf("[aide:watcher] failed to remove %s: %v", path, err)
			} else {
				log.Printf("[aide:watcher] removed %s from index", path)
			}
		} else {
			// Create, Write, or Rename - reindex
			count, err := indexFn(path)
			if err != nil {
				log.Printf("[aide:watcher] failed to index %s: %v", path, err)
			} else {
				log.Printf("[aide:watcher] indexed %s: %d symbols", path, count)
			}
		}
	}

	return NewWatcher(config, onChange)
}
