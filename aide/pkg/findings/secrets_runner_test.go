package findings

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/watcher"
)

const fakeSecret = "AWS_ACCESS_KEY_ID=AKIADEADBEEFDEADBEEF\n"

func TestSecretsOnlyChangesAndRunAll(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(map[bool]string{false: "changes", true: "all"}[full], func(t *testing.T) {
			root := t.TempDir()
			files := map[string]fsnotify.Op{}
			for _, name := range []string{".env", "key.pem", "app.ini", "app.properties", "credentials"} {
				path := filepath.Join(root, name)
				if err := os.WriteFile(path, []byte(fakeSecret), 0o600); err != nil {
					t.Fatal(err)
				}
				files[path] = fsnotify.Write
			}
			store := &mockReplaceFindingsStore{}
			r := NewRunner(store, AnalyzerConfig{ProjectRoot: root, Paths: []string{root}, Ignore: aideignore.NewEmpty()}, nil)
			defer r.Stop()
			var projectCalls atomic.Int32
			r.SetDeadCodeRunner(func(context.Context) ([]*Finding, error) { projectCalls.Add(1); return nil, nil })
			r.SetClonesRunner(func(context.Context, []string, ClonesRunnerConfig) ([]*Finding, error) {
				projectCalls.Add(1)
				return nil, nil
			})
			if full {
				if err := r.RunAll(context.Background()); err != nil {
					t.Fatal(err)
				}
			} else {
				r.OnChanges(files)
			}
			r.WaitAll()
			store.mu.Lock()
			defer store.mu.Unlock()
			if len(store.replacedAnalyzerAndFile) != len(files) {
				t.Fatalf("got %d file runs, want %d", len(store.replacedAnalyzerAndFile), len(files))
			}
			for _, call := range store.replacedAnalyzerAndFile {
				if call.Analyzer != AnalyzerSecrets || len(call.Findings) == 0 {
					t.Fatalf("missing secret detection: %+v", call)
				}
				for _, f := range call.Findings {
					if f.FilePath != call.FilePath || filepath.IsAbs(f.FilePath) {
						t.Errorf("inconsistent finding path %q", f.FilePath)
					}
					if strings.Contains(f.Detail, "AKIADEADBEEFDEADBEEF") {
						t.Error("finding exposes secret")
					}
				}
			}
			if !full && projectCalls.Load() != 0 {
				t.Error("non-source changes triggered project analysis")
			}
		})
	}
}

func TestSecretsWatcherPipeline(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ignore, err := aideignore.NewWithOptions(root, aideignore.Options{Gitignore: true})
	if err != nil {
		t.Fatal(err)
	}
	store := &mockReplaceFindingsStore{}
	r := NewRunner(store, AnalyzerConfig{ProjectRoot: root, Ignore: ignore}, nil)
	defer r.Stop()
	w, err := watcher.New(watcher.Config{Paths: []string{root}, ProjectRoot: root, Ignore: ignore, DebounceDelay: 20 * time.Millisecond}, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	for _, name := range []string{".env", "ignored.env", "scratch.tmp"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(fakeSecret), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		store.mu.Lock()
		calls := append([]replaceCall(nil), store.replacedAnalyzerAndFile...)
		store.mu.Unlock()
		for _, call := range calls {
			if call.FilePath == "ignored.env" || call.FilePath == "scratch.tmp" {
				t.Fatalf("excluded file analyzed: %s", call.FilePath)
			}
			if call.FilePath == ".env" && len(call.Findings) > 0 {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("watcher never delivered .env secret to runner")
}

func TestSecretsIgnoreNegationSubdirectoryAndLimits(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".aideignore"), []byte("config/*\n!config/keep.env\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ignore, err := aideignore.NewWithOptions(root, aideignore.Options{})
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]fsnotify.Op{}
	for _, name := range []string{"config/keep.env", "config/skip.env", "asset.png", "large.env"} {
		content := []byte(fakeSecret)
		if name == "large.env" {
			content = append(content, make([]byte, DefaultSecretsMaxFileSize)...)
		}
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		files[path] = fsnotify.Write
	}
	store := &mockReplaceFindingsStore{}
	r := NewRunner(store, AnalyzerConfig{ProjectRoot: root, Ignore: ignore}, nil)
	defer r.Stop()
	r.OnChanges(files)
	r.WaitAll()
	store.mu.Lock()
	defer store.mu.Unlock()
	for _, call := range store.replacedAnalyzerAndFile {
		if call.FilePath == "config/skip.env" || call.FilePath == "asset.png" {
			t.Errorf("excluded file analyzed: %s", call.FilePath)
		}
		if call.FilePath == "large.env" && len(call.Findings) != 0 {
			t.Error("oversized file scanned")
		}
	}
	findings, result, err := AnalyzeSecrets(SecretsConfig{Paths: []string{filepath.Join(root, "config")}, ProjectRoot: root, Ignore: ignore, SkipValidation: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 1 || len(findings) == 0 || findings[0].FilePath != "config/keep.env" {
		t.Fatalf("full/subdirectory scan disagrees: %+v, %+v", result, findings)
	}
}

func TestSecretsRejectSymlinksAndOutsideCheckout(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.env")
	if err := os.WriteFile(outside, []byte(fakeSecret), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.env")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	store := &mockReplaceFindingsStore{}
	r := NewRunner(store, AnalyzerConfig{ProjectRoot: root, Ignore: aideignore.NewEmpty()}, nil)
	defer r.Stop()
	r.OnChanges(map[string]fsnotify.Op{outside: fsnotify.Write, link: fsnotify.Write})
	r.WaitAll()
	if len(store.replacedAnalyzerAndFile) != 0 {
		t.Fatal("outside checkout scanned")
	}
	_, result, err := AnalyzeSecrets(SecretsConfig{Paths: []string{root}, SkipValidation: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.FilesScanned != 0 {
		t.Fatal("standalone scanner followed symlink")
	}
}

func TestDeletedSecretsCancelAndDrainActiveRun(t *testing.T) {
	root := t.TempDir()
	store := &mockReplaceFindingsStore{}
	r := NewRunner(store, AnalyzerConfig{ProjectRoot: root, Ignore: aideignore.NewEmpty()}, nil)
	defer r.Stop()
	started, cancelled, finish := make(chan struct{}), make(chan struct{}), make(chan struct{})
	r.runAnalyzer(RunKey{AnalyzerSecrets, ".env"}, func(ctx context.Context) ([]*Finding, error) {
		close(started)
		<-ctx.Done()
		close(cancelled)
		<-finish
		return []*Finding{{Analyzer: AnalyzerSecrets, FilePath: ".env", Title: "stale"}}, nil
	})
	<-started
	done := make(chan struct{})
	go func() { r.OnChanges(map[string]fsnotify.Op{filepath.Join(root, ".env"): fsnotify.Remove}); close(done) }()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("run not cancelled")
	}
	select {
	case <-done:
		t.Error("deletion did not drain active run")
	default:
	}
	close(finish)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("deletion did not finish")
	}
	r.WaitAll()
	if len(store.replacedAnalyzerAndFile) != 1 || len(store.replacedAnalyzerAndFile[0].Findings) != 0 {
		t.Fatal("deleted findings were repopulated")
	}
}

func TestSecretsBatchReleasesScannerAndCancelledJobs(t *testing.T) {
	batch := &secretsBatch{remaining: 1}
	if _, err := batch.scan(context.Background(), []byte(fakeSecret), ".env"); err != nil {
		t.Fatal(err)
	}
	batch.release()
	if batch.scanner != nil {
		t.Fatal("batch retained idle scanner")
	}
	r := NewRunner(&mockReplaceFindingsStore{}, AnalyzerConfig{}, nil)
	r.sem = make(chan struct{}, 1)
	r.sem <- struct{}{}
	batch = &secretsBatch{remaining: 1}
	r.runAnalyzerWithCleanup(RunKey{AnalyzerSecrets, ".env"}, func(context.Context) ([]*Finding, error) { t.Error("cancelled queued job executed"); return nil, nil }, batch.release)
	r.Stop()
	if batch.remaining != 0 {
		t.Fatal("cancelled job leaked batch ownership")
	}
}
