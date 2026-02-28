package findings

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

var runnerLog = log.New(os.Stderr, "[aide:findings] ", log.Ltime)

const ScopeProject = "<project>"

type AnalyzerConfig struct {
	ComplexityThreshold int
	FanOutThreshold     int
	FanInThreshold      int
	CloneWindowSize     int
	CloneMinLines       int
	CloneMinMatchCount  int
	CloneMaxBucketSize  int
	CloneMinSimilarity  float64
	CloneMinSeverity    string
	Paths               []string
	// ProjectRoot is the absolute path to the project root, used for converting
	// absolute file paths from the watcher to relative paths for aideignore matching.
	ProjectRoot string
	// Ignore is the aideignore matcher used to filter files and directories.
	// If nil, built-in defaults are used.
	Ignore *aideignore.Matcher
}

type RunKey struct {
	Analyzer string
	Scope    string
}

type activeRun struct {
	cancel  context.CancelFunc
	done    chan struct{}
	started time.Time
	id      int64
}

type AnalyzerStatus struct {
	Status       string
	Scope        string
	LastRun      time.Time
	LastDuration time.Duration
	Findings     int
	Error        string
}

// ClonesRunnerConfig holds clone-specific parameters passed to the ClonesRunner.
type ClonesRunnerConfig struct {
	WindowSize    int
	MinLines      int
	MinMatchCount int
	MaxBucketSize int
	MinSimilarity float64
	MinSeverity   string
}

type ClonesRunner func(ctx context.Context, paths []string, cfg ClonesRunnerConfig) ([]*Finding, error)

type Runner struct {
	store        ReplaceFindingsStore
	config       AnalyzerConfig
	clonesRunner ClonesRunner
	loader       grammar.Loader

	mu            sync.Mutex
	runs          map[RunKey]*activeRun
	status        map[string]*AnalyzerStatus
	runIDGen      int64
	defaultIgnore *aideignore.Matcher // Cached default matcher (lazy-init)

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// sem limits concurrent per-file goroutines spawned by RunAll.
	sem chan struct{}
}

type ReplaceFindingsStore interface {
	ReplaceFindingsForAnalyzer(analyzer string, findings []*Finding) error
	ReplaceFindingsForAnalyzerAndFile(analyzer, filePath string, findings []*Finding) error
	Stats(opts SearchOptions) (*Stats, error)
}

func NewRunner(store ReplaceFindingsStore, config AnalyzerConfig, loader grammar.Loader) *Runner {
	ctx, cancel := context.WithCancel(context.Background())

	if loader == nil {
		loader = grammar.NewCompositeLoader()
	}

	return &Runner{
		store:  store,
		config: config,
		loader: loader,
		runs:   make(map[RunKey]*activeRun),
		status: make(map[string]*AnalyzerStatus),
		ctx:    ctx,
		cancel: cancel,
		sem:    make(chan struct{}, DefaultRunnerConcurrency), // Limit concurrent per-file goroutines
	}
}

func (r *Runner) SetClonesRunner(fn ClonesRunner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clonesRunner = fn
}

// ignore returns the configured aideignore matcher, falling back to built-in
// defaults. The default matcher is cached on first use to avoid repeated
// allocation.
func (r *Runner) ignore() *aideignore.Matcher {
	if r.config.Ignore != nil {
		return r.config.Ignore
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.defaultIgnore == nil {
		r.defaultIgnore = aideignore.NewFromDefaults()
	}
	return r.defaultIgnore
}

func (r *Runner) OnChanges(files map[string]fsnotify.Op) {
	perFileAnalyzers := []string{AnalyzerComplexity, AnalyzerSecrets}
	projectAnalyzers := []string{AnalyzerCoupling, AnalyzerClones}

	ignore := r.ignore()

	for file := range files {
		if !code.SupportedFile(file) {
			continue
		}

		// Convert to relative path for aideignore matching (watcher sends absolute paths).
		relFile := file
		if r.config.ProjectRoot != "" {
			if rel, err := filepath.Rel(r.config.ProjectRoot, file); err == nil {
				relFile = rel
			}
		}
		if ignore.ShouldIgnoreFile(relFile) {
			continue
		}

		for _, analyzer := range perFileAnalyzers {
			key := RunKey{Analyzer: analyzer, Scope: file}
			r.runAnalyzer(key, func(ctx context.Context) ([]*Finding, error) {
				return r.runPerFileAnalyzer(ctx, analyzer, file)
			})
		}
	}

	for _, analyzer := range projectAnalyzers {
		key := RunKey{Analyzer: analyzer, Scope: ScopeProject}
		r.runAnalyzer(key, func(ctx context.Context) ([]*Finding, error) {
			return r.runProjectAnalyzer(ctx, analyzer)
		})
	}
}

func (r *Runner) runAnalyzer(key RunKey, run func(ctx context.Context) ([]*Finding, error)) {
	r.mu.Lock()

	if existing, ok := r.runs[key]; ok {
		existing.cancel()
		runnerLog.Printf("%s on %s: cancelled existing run", key.Analyzer, key.Scope)
		r.mu.Unlock()
		<-existing.done
		r.mu.Lock()
	}

	ctx, cancel := context.WithCancel(r.ctx)
	done := make(chan struct{})
	r.runIDGen++
	runID := r.runIDGen
	r.runs[key] = &activeRun{
		cancel:  cancel,
		done:    done,
		started: time.Now(),
		id:      runID,
	}

	r.updateStatusLocked(key.Analyzer, key.Scope, "running", 0, 0, "")

	r.mu.Unlock()

	r.wg.Add(1)
	go func() {
		defer close(done)
		defer r.wg.Done()
		defer func() {
			r.mu.Lock()
			if current, ok := r.runs[key]; ok && current.id == runID {
				delete(r.runs, key)
			}
			r.mu.Unlock()
		}()

		// Acquire semaphore slot to limit concurrent goroutines.
		if r.sem != nil {
			select {
			case r.sem <- struct{}{}:
				defer func() { <-r.sem }()
			case <-ctx.Done():
				return
			}
		}

		start := time.Now()
		findings, err := run(ctx)
		duration := time.Since(start)

		if ctx.Err() != nil {
			runnerLog.Printf("%s on %s: cancelled", key.Analyzer, key.Scope)
			return
		}

		if err != nil {
			errStr := err.Error()
			runnerLog.Printf("%s on %s: failed: %v (keeping old findings)", key.Analyzer, key.Scope, err)
			r.updateStatus(key.Analyzer, key.Scope, "error", 0, duration, errStr)
			return
		}

		if key.Scope == ScopeProject {
			if err := r.store.ReplaceFindingsForAnalyzer(key.Analyzer, findings); err != nil {
				runnerLog.Printf("%s: store failed: %v (keeping old findings)", key.Analyzer, err)
				r.updateStatus(key.Analyzer, key.Scope, "error", 0, duration, err.Error())
				return
			}
		} else {
			if err := r.store.ReplaceFindingsForAnalyzerAndFile(key.Analyzer, key.Scope, findings); err != nil {
				runnerLog.Printf("%s on %s: store failed: %v", key.Analyzer, key.Scope, err)
				r.updateStatus(key.Analyzer, key.Scope, "error", 0, duration, err.Error())
				return
			}
		}

		runnerLog.Printf("%s on %s: %d findings in %v", key.Analyzer, key.Scope, len(findings), duration)
		r.updateStatus(key.Analyzer, key.Scope, "idle", len(findings), duration, "")
	}()
}

func (r *Runner) runPerFileAnalyzer(ctx context.Context, analyzer string, file string) ([]*Finding, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	content, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	relPath := file
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, file); err == nil {
			relPath = rel
		}
	}

	switch analyzer {
	case AnalyzerComplexity:
		return r.analyzeFileComplexity(ctx, relPath, content)
	case AnalyzerSecrets:
		return r.analyzeFileSecrets(ctx, relPath, content)
	default:
		return nil, fmt.Errorf("unknown analyzer: %s", analyzer)
	}
}

func (r *Runner) runProjectAnalyzer(ctx context.Context, analyzer string) ([]*Finding, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	paths := r.config.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}

	switch analyzer {
	case AnalyzerCoupling:
		cfg := CouplingConfig{
			Paths:           paths,
			FanOutThreshold: r.config.FanOutThreshold,
			FanInThreshold:  r.config.FanInThreshold,
			Ignore:          r.ignore(),
		}
		if cfg.FanOutThreshold <= 0 {
			cfg.FanOutThreshold = DefaultFanOutThreshold
		}
		if cfg.FanInThreshold <= 0 {
			cfg.FanInThreshold = DefaultFanInThreshold
		}
		findings, _, err := AnalyzeCoupling(cfg)
		return findings, err

	case AnalyzerClones:
		if r.clonesRunner == nil {
			return nil, fmt.Errorf("clones runner not configured")
		}
		// Defaults mirror clone.DefaultWindowSize / clone.DefaultMinCloneLines.
		// Can't import clone (cycle), but DetectClones.defaults() applies
		// the canonical values for any zero fields anyway.
		windowSize := r.config.CloneWindowSize
		if windowSize <= 0 {
			windowSize = DefaultCloneWindowSize
		}
		minLines := r.config.CloneMinLines
		if minLines <= 0 {
			minLines = DefaultCloneMinLines
		}
		return r.clonesRunner(ctx, paths, ClonesRunnerConfig{
			WindowSize:    windowSize,
			MinLines:      minLines,
			MinMatchCount: r.config.CloneMinMatchCount,
			MaxBucketSize: r.config.CloneMaxBucketSize,
			MinSimilarity: r.config.CloneMinSimilarity,
			MinSeverity:   r.config.CloneMinSeverity,
		})

	default:
		return nil, fmt.Errorf("unknown analyzer: %s", analyzer)
	}
}

func (r *Runner) analyzeFileComplexity(ctx context.Context, filePath string, content []byte) ([]*Finding, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	lang := code.DetectLanguage(filePath, content)
	if lang == "" {
		return nil, nil
	}

	langCfg := getComplexityLang(lang)

	threshold := r.config.ComplexityThreshold
	if threshold <= 0 {
		threshold = DefaultComplexityThreshold
	}

	return analyzeFileComplexity(ctx, r.loader, content, filePath, lang, langCfg, threshold), nil
}

func (r *Runner) analyzeFileSecrets(ctx context.Context, filePath string, _ []byte) ([]*Finding, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	cfg := SecretsConfig{
		Paths:          []string{filePath},
		SkipValidation: true,
		MaxFileSize:    DefaultRunnerSecretsMaxFileSize,
	}

	findings, _, err := AnalyzeSecrets(cfg)
	return findings, err
}

func (r *Runner) updateStatus(analyzer, scope, status string, findings int, duration time.Duration, err string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.updateStatusLocked(analyzer, scope, status, findings, duration, err)
}

func (r *Runner) updateStatusLocked(analyzer, scope, status string, findings int, duration time.Duration, err string) {
	s := r.status[analyzer]
	if s == nil {
		s = &AnalyzerStatus{}
		r.status[analyzer] = s
	}

	s.Status = status
	s.Scope = scope
	s.LastRun = time.Now()
	s.LastDuration = duration
	s.Findings = findings
	s.Error = err
}

func (r *Runner) GetStatus() map[string]AnalyzerStatus {
	r.mu.Lock()
	defer r.mu.Unlock()

	result := make(map[string]AnalyzerStatus)
	for k, v := range r.status {
		result[k] = *v
	}
	return result
}

// WaitAll blocks until all running analysers have completed.
func (r *Runner) WaitAll() {
	r.wg.Wait()
}

func (r *Runner) Stop() {
	r.cancel()
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(DefaultRunnerStopTimeout):
		runnerLog.Printf("timeout waiting for analyzers to stop")
	}
}

// RunAll schedules analysis of all supported files in the configured paths.
// Per-file analysers (complexity, secrets) are launched per file; project-wide
// analysers (coupling, clones) are launched once. All analysers run
// asynchronously via runAnalyzer — use WaitAll() to block until completion,
// or Stop() to cancel and drain.
func (r *Runner) RunAll(ctx context.Context) error {
	paths := r.config.Paths
	if len(paths) == 0 {
		paths = []string{"."}
	}

	ignore := r.ignore()

	for _, root := range paths {
		absRoot, _ := filepath.Abs(root)
		shouldSkip := ignore.WalkFunc(absRoot)

		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if skip, skipDir := shouldSkip(path, info); skip {
				if skipDir {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if !code.SupportedFile(path) {
				return nil
			}

			for _, analyzer := range []string{AnalyzerComplexity, AnalyzerSecrets} {
				key := RunKey{Analyzer: analyzer, Scope: path}
				r.runAnalyzer(key, func(ctx context.Context) ([]*Finding, error) {
					return r.runPerFileAnalyzer(ctx, analyzer, path)
				})
			}
			return nil
		})
		if err != nil {
			return err
		}
	}

	for _, analyzer := range []string{AnalyzerCoupling, AnalyzerClones} {
		key := RunKey{Analyzer: analyzer, Scope: ScopeProject}
		r.runAnalyzer(key, func(ctx context.Context) ([]*Finding, error) {
			return r.runProjectAnalyzer(ctx, analyzer)
		})
	}

	return nil
}
