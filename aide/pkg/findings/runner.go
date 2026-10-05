package findings

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/checkout"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
)

var runnerLog = log.New(os.Stderr, "[aide:findings] ", log.Ltime)

const ScopeProject = "<project>"

// toRelPath converts an absolute or mixed-format path to a path relative to root.
// This ensures a consistent path format across findings storage and RunKey
// scoping, preventing duplicates caused by path mismatches (e.g. the watcher
// sends absolute paths, while findings store uses relative paths).
func toRelPath(root, file string) string {
	if abs, err := filepath.Abs(file); err == nil {
		if rel, err := filepath.Rel(root, abs); err == nil {
			return rel
		}
	}
	return file
}

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

// DeadCodeRunner runs the dead-code analyser. Like ClonesRunner it is injected
// rather than called directly: the analyser needs the code index, which this
// package cannot import without a cycle.
type DeadCodeRunner func(ctx context.Context) ([]*Finding, error)

type Runner struct {
	store          ReplaceFindingsStore
	config         AnalyzerConfig
	clonesRunner   ClonesRunner
	deadCodeRunner DeadCodeRunner
	loader         grammar.Loader

	scheduleMu    sync.Mutex // Serializes change batches, RunAll and deletion cleanup.
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

func (r *Runner) SetDeadCodeRunner(fn DeadCodeRunner) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deadCodeRunner = fn
}

func (r *Runner) clones() ClonesRunner {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.clonesRunner
}

func (r *Runner) deadCode() DeadCodeRunner {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.deadCodeRunner
}

// perFileAnalyzers lists the analysers that run against one file's contents.
func perFileAnalyzers() []string {
	return []string{AnalyzerComplexity, AnalyzerSecrets, AnalyzerSecurity, AnalyzerTodos}
}

// projectAnalyzers lists the whole-project analysers to run. Dead code joins
// them only once a runner is wired, since without the code index it has
// nothing to analyse.
func (r *Runner) projectAnalyzers() []string {
	analyzers := []string{AnalyzerCoupling, AnalyzerClones}
	if r.deadCode() != nil {
		analyzers = append(analyzers, AnalyzerDeadCode)
	}
	return analyzers
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

// fileAnalysis shares one read across a file's source and secrets analyzers.
// The content is loaded only after the job acquires the concurrency semaphore.
type fileAnalysis struct {
	path    string
	maxSize int64
	once    sync.Once
	content []byte
	err     error
}

func (f *fileAnalysis) read() ([]byte, error) {
	f.once.Do(func() { f.content, f.err = readAnalysisFile(f.path, f.maxSize) })
	return f.content, f.err
}

func analyzersForFile(path string) []string {
	if code.SupportedFile(path) {
		return perFileAnalyzers()
	}
	if !secretsSkipExtensions[strings.ToLower(filepath.Ext(path))] {
		return []string{AnalyzerSecrets}
	}
	return nil
}

func (r *Runner) OnChanges(files map[string]fsnotify.Op) {
	r.scheduleMu.Lock()
	defer r.scheduleMu.Unlock()
	if r.scheduleFiles(files) {
		r.scheduleProjectAnalyzers()
	}
}

// scheduleFiles admits files by ignore policy, then selects applicable analyzers.
// Non-source changes never trigger project-wide code analysis.
func (r *Runner) scheduleFiles(files map[string]fsnotify.Op) bool {
	type task struct {
		file            *fileAnalysis
		analyzer, scope string
	}
	var tasks []task
	batch := &secretsBatch{}
	sourceChanged := false
	for file, op := range files {
		if r.ctx.Err() != nil {
			break
		}
		if r.config.ProjectRoot != "" {
			var err error
			file, _, err = checkout.SourcePath(r.config.ProjectRoot, file)
			if err != nil {
				continue
			}
		}
		scope := toRelPath(r.config.ProjectRoot, file)
		if r.ignore().ShouldIgnoreFile(scope) {
			continue
		}
		analyzers := analyzersForFile(file)
		if len(analyzers) == 0 {
			continue
		}
		if code.SupportedFile(file) {
			sourceChanged = true
		}
		if op&fsnotify.Remove != 0 {
			for _, analyzer := range analyzers {
				key := RunKey{Analyzer: analyzer, Scope: scope}
				r.cancelRun(key)
				if err := r.store.ReplaceFindingsForAnalyzerAndFile(analyzer, scope, nil); err != nil {
					runnerLog.Printf("%s on %s: failed to clear deleted file: %v", analyzer, scope, err)
				}
			}
			continue
		}
		input := &fileAnalysis{path: file}
		if !code.SupportedFile(file) {
			input.maxSize = DefaultRunnerSecretsMaxFileSize
		}
		for _, analyzer := range analyzers {
			tasks = append(tasks, task{input, analyzer, scope})
			if analyzer == AnalyzerSecrets {
				batch.remaining++
			}
		}
	}
	for _, task := range tasks {
		var cleanup func()
		if task.analyzer == AnalyzerSecrets {
			cleanup = batch.release
		}
		r.runAnalyzerWithCleanup(RunKey{Analyzer: task.analyzer, Scope: task.scope}, func(ctx context.Context) ([]*Finding, error) {
			return r.runPerFileAnalyzer(ctx, task.analyzer, task.file, batch)
		}, cleanup)
	}
	return sourceChanged
}

func (r *Runner) scheduleProjectAnalyzers() {
	for _, analyzer := range r.projectAnalyzers() {
		r.runAnalyzer(RunKey{Analyzer: analyzer, Scope: ScopeProject}, func(ctx context.Context) ([]*Finding, error) {
			return r.runProjectAnalyzer(ctx, analyzer)
		})
	}
}

// Wait for the cancelled run to finish committing before clearing its findings.
func (r *Runner) cancelRun(key RunKey) {
	r.mu.Lock()
	run := r.runs[key]
	if run != nil {
		run.cancel()
	}
	r.mu.Unlock()
	if run != nil {
		<-run.done
	}
}

func (r *Runner) runAnalyzer(key RunKey, run func(ctx context.Context) ([]*Finding, error)) {
	r.runAnalyzerWithCleanup(key, run, nil)
}

func (r *Runner) runAnalyzerWithCleanup(key RunKey, run func(ctx context.Context) ([]*Finding, error), cleanup func()) {
	r.mu.Lock()
	for {
		existing := r.runs[key]
		if existing == nil {
			break
		}
		existing.cancel()
		r.mu.Unlock()
		<-existing.done
		r.mu.Lock()
	}
	if r.ctx.Err() != nil {
		r.mu.Unlock()
		if cleanup != nil {
			cleanup()
		}
		return
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

	r.wg.Add(1)
	r.mu.Unlock()

	go func() {
		defer close(done)
		defer r.wg.Done()
		if cleanup != nil {
			defer cleanup()
		}
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

func (r *Runner) runPerFileAnalyzer(ctx context.Context, analyzer string, input *fileAnalysis, batch *secretsBatch) ([]*Finding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if analyzer == AnalyzerSecrets {
		info, err := os.Lstat(input.path)
		if err != nil {
			return nil, err
		}
		if !secretsEligible(input.path, info, DefaultRunnerSecretsMaxFileSize) {
			return nil, nil
		}
	}
	content, err := input.read()
	if errors.Is(err, errAnalysisSkipped) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relPath := toRelPath(r.config.ProjectRoot, input.path)
	switch analyzer {
	case AnalyzerComplexity:
		return r.analyzeFileComplexity(ctx, relPath, content)
	case AnalyzerSecrets:
		if int64(len(content)) > DefaultRunnerSecretsMaxFileSize {
			return nil, nil
		}
		return batch.scan(ctx, content, relPath)
	case AnalyzerSecurity:
		return r.analyzeFileSecurity(ctx, relPath, content)
	case AnalyzerTodos:
		return analyzeFileTodos(relPath, content), nil
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
		paths = []string{r.config.ProjectRoot}
		if paths[0] == "" {
			paths[0] = "."
		}
	}
	if r.config.ProjectRoot != "" {
		resolved := make([]string, 0, len(paths))
		for _, path := range paths {
			abs, _, err := checkout.SourcePath(r.config.ProjectRoot, path)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, abs)
		}
		paths = resolved
	}

	switch analyzer {
	case AnalyzerCoupling:
		cfg := CouplingConfig{
			Paths:           paths,
			ProjectRoot:     r.config.ProjectRoot,
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

	case AnalyzerDeadCode:
		run := r.deadCode()
		if run == nil {
			return nil, fmt.Errorf("deadcode runner not configured")
		}
		return run(ctx)

	case AnalyzerClones:
		clones := r.clones()
		if clones == nil {
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
		return clones(ctx, paths, ClonesRunnerConfig{
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

func (r *Runner) analyzeFileSecurity(ctx context.Context, filePath string, content []byte) ([]*Finding, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	return analyzeFileSecurity(ctx, filePath, content), nil
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
	r.scheduleMu.Lock()
	// Start the waiter only after in-flight scheduling has finished adding jobs.
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	r.scheduleMu.Unlock()

	select {
	case <-done:
	case <-time.After(DefaultRunnerStopTimeout):
		runnerLog.Printf("timeout waiting for analyzers to stop")
	}
}

// RunAll uses the same applicability and ignore policy as incremental changes.
// Jobs remain asynchronous; WaitAll drains them and releases batch resources.
func (r *Runner) RunAll(ctx context.Context) error {
	r.scheduleMu.Lock()
	defer r.scheduleMu.Unlock()
	paths := r.config.Paths
	if len(paths) == 0 {
		paths = []string{r.config.ProjectRoot}
		if paths[0] == "" {
			paths[0] = "."
		}
	}
	files := make(map[string]fsnotify.Op)
	for _, root := range paths {
		if r.config.ProjectRoot != "" {
			var err error
			root, _, err = checkout.SourcePath(r.config.ProjectRoot, root)
			if err != nil {
				return err
			}
		}
		ignoreRoot := r.config.ProjectRoot
		if ignoreRoot == "" {
			ignoreRoot, _ = filepath.Abs(root)
		}
		shouldSkip := r.ignore().WalkFunc(ignoreRoot)
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if r.ctx.Err() != nil {
				return r.ctx.Err()
			}
			if err != nil {
				return nil
			}
			if skip, dir := shouldSkip(path, info); skip {
				if dir {
					return filepath.SkipDir
				}
				return nil
			}
			if info.Mode().IsRegular() && len(analyzersForFile(path)) > 0 {
				files[path] = fsnotify.Write
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	r.scheduleFiles(files)
	r.scheduleProjectAnalyzers()
	return nil
}
