// Package main provides MCP server implementation for aide.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jmylchreest/aide/aide/internal/version"
	"github.com/jmylchreest/aide/aide/pkg/aideignore"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/findings"
	"github.com/jmylchreest/aide/aide/pkg/findings/clone"
	"github.com/jmylchreest/aide/aide/pkg/grammar"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/adapter"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/registry"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/watcher"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mcpLog logs to stderr (stdout is reserved for MCP JSON-RPC protocol)
var mcpLog = log.New(os.Stderr, "[aide-mcp] ", log.Ltime)

// mcpBackend holds the stores a tool call reads through. Client mode fills it
// with gRPC adapters and primary mode with the real stores, and promotion
// swaps one whole set for the other — so it is replaced as a unit, never
// field by field, and a call in flight sees one set or the other.
type mcpBackend struct {
	store         store.Store
	codeStore     store.CodeIndexStore
	findingsStore store.FindingsStore
	surveyStore   store.SurveyStore
	instinctStore store.InstinctProposalStore
	// grpcClient is non-nil in client mode: attached to another process's primary.
	grpcClient *grpcapi.Client
}

// MCPServer wraps the aide store for MCP tool access.
type MCPServer struct {
	backend        atomic.Pointer[mcpBackend]
	codeStoreReady atomic.Bool
	codeInitWg     sync.WaitGroup
	server         *mcp.Server
	// grpcServer is nil until this process becomes primary, which on a
	// promotion happens on the supervisor's goroutine while others read it.
	grpcServer    atomic.Pointer[grpcapi.Server]
	grammarLoader *grammar.CompositeLoader
	dbPath        string // path to the memory database; used to derive project root

	unifiedWatcher   *watcher.Watcher
	findingsRunner   *findings.Runner
	unifiedWatcherMu sync.Mutex

	toolCounts sync.Map // map[string]*atomic.Int64
}

func (s *MCPServer) store() store.Store                 { return s.backend.Load().store }
func (s *MCPServer) findingsStore() store.FindingsStore { return s.backend.Load().findingsStore }
func (s *MCPServer) surveyStore() store.SurveyStore     { return s.backend.Load().surveyStore }
func (s *MCPServer) grpcClient() *grpcapi.Client        { return s.backend.Load().grpcClient }
func (s *MCPServer) instinctStore() store.InstinctProposalStore {
	return s.backend.Load().instinctStore
}

// setBackend installs a new backend set, replacing the current one wholesale.
func (s *MCPServer) setBackend(b *mcpBackend) {
	s.backend.Store(b)
}

// getCodeStore returns the code store (nil during lazy init).
func (s *MCPServer) getCodeStore() store.CodeIndexStore {
	if !s.codeStoreReady.Load() {
		return nil
	}
	return s.backend.Load().codeStore
}

func (s *MCPServer) grpcSrv() *grpcapi.Server { return s.grpcServer.Load() }

// newMCPServer returns a server with a backend installed, so every accessor is
// safe to call before a mode has been chosen. A nil backend means an empty one.
func newMCPServer(b *mcpBackend) *MCPServer {
	if b == nil {
		b = &mcpBackend{}
	}
	s := &MCPServer{}
	s.backend.Store(b)
	return s
}

// mutateBackend applies fn to a copy of the current backend and installs it,
// retrying if another writer got there first. Copy-on-write rather than
// in-place, so readers never observe a torn set.
func (s *MCPServer) mutateBackend(fn func(*mcpBackend)) {
	for {
		old := s.backend.Load()
		next := *old
		fn(&next)
		if s.backend.CompareAndSwap(old, &next) {
			return
		}
	}
}

// setCodeStore attaches the code store once lazy init completes.
func (s *MCPServer) setCodeStore(cs store.CodeIndexStore) {
	s.mutateBackend(func(b *mcpBackend) { b.codeStore = cs })
	s.codeStoreReady.Store(true)
}

// incrementToolCount atomically increments the execution count for a tool.
func (s *MCPServer) incrementToolCount(name string) {
	v, _ := s.toolCounts.LoadOrStore(name, &atomic.Int64{})
	v.(*atomic.Int64).Add(1)
}

// getToolCounts returns a snapshot of tool execution counts.
func (s *MCPServer) getToolCounts() map[string]int64 {
	counts := make(map[string]int64)
	s.toolCounts.Range(func(key, value any) bool {
		counts[key.(string)] = value.(*atomic.Int64).Load()
		return true
	})
	return counts
}

// toolCountMiddleware returns MCP middleware that counts tool invocations.
func (s *MCPServer) toolCountMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/call" {
				if params, ok := req.GetParams().(*mcp.CallToolParamsRaw); ok {
					s.incrementToolCount(params.Name)
				}
			}
			return next(ctx, method, req)
		}
	}
}

// mcpToolTaxonomy maps MCP tool names to (category, subtype) for observe.
// Single source of truth for which class of work each tool represents —
// keeps the dashboard's per-category view honest as new tools are added.
var mcpToolTaxonomy = map[string]struct {
	Category string
	Subtype  string
}{
	// code (consume — manual spans in handler add token math)
	"code_outline":     {"consume", "outline"},
	"code_read_symbol": {"consume", "symbol"},

	// code (navigate)
	"code_search":         {"navigate", "sym_search"},
	"code_symbols":        {"navigate", "file_syms"},
	"code_references":     {"navigate", "refs"},
	"code_top_references": {"navigate", "top_refs"},
	"code_read_check":     {"navigate", "read_check"},
	"code_stats":          {"navigate", "stats"},

	// memory / decisions / state / findings / survey (knowledge)
	"memory_add":       {"knowledge", "memory_add"},
	"memory_search":    {"knowledge", "memory_search"},
	"memory_list":      {"knowledge", "memory_list"},
	"memory_get":       {"knowledge", "memory_get"},
	"decision_get":     {"knowledge", "decision_get"},
	"decision_list":    {"knowledge", "decision_list"},
	"decision_history": {"knowledge", "decision_history"},
	"decision_set":     {"knowledge", "decision_set"},
	"state_get":        {"knowledge", "state_get"},
	"state_list":       {"knowledge", "state_list"},
	"findings_search":  {"knowledge", "findings_search"},
	"findings_list":    {"knowledge", "findings_list"},
	"findings_stats":   {"knowledge", "findings_stats"},
	"findings_accept":  {"knowledge", "findings_accept"},
	"survey_search":    {"knowledge", "survey_search"},
	"survey_list":      {"knowledge", "survey_list"},
	"survey_stats":     {"knowledge", "survey_stats"},
	"survey_run":       {"knowledge", "survey_run"},
	"survey_graph":     {"knowledge", "survey_graph"},

	// coordination
	"task_create":   {"coordinate", "task_create"},
	"task_get":      {"coordinate", "task_get"},
	"task_list":     {"coordinate", "task_list"},
	"task_claim":    {"coordinate", "task_claim"},
	"task_complete": {"coordinate", "task_complete"},
	"task_delete":   {"coordinate", "task_delete"},
	"message_send":  {"coordinate", "message_send"},
	"message_list":  {"coordinate", "message_list"},
	"message_ack":   {"coordinate", "message_ack"},

	// status / introspection
	"instance_info": {"navigate", "instance"},
	"token_stats":   {"navigate", "token_stats"},
}

// toolObserveMiddleware records every MCP tool call as one observe.KindToolCall
// span. The span is attached to the request context so handlers can enrich it
// (e.g. code_outline adds Tokens/Saved) via observe.FromContext — no
// per-handler bookkeeping or skip lists needed.
func (s *MCPServer) toolObserveMiddleware() mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method != "tools/call" {
				return next(ctx, method, req)
			}
			params, ok := req.GetParams().(*mcp.CallToolParamsRaw)
			if !ok {
				return next(ctx, method, req)
			}
			category, subtype := "other", ""
			if tax, found := mcpToolTaxonomy[params.Name]; found {
				category = tax.Category
				subtype = tax.Subtype
			}
			ctx, span := observe.StartCtx(ctx, params.Name, observe.KindToolCall)
			span.Category(category).Subtype(subtype)
			defer span.End()
			result, err := next(ctx, method, req)
			if err != nil {
				span.Err(err)
			}
			// Backfill spent-token cost from the response text length when
			// the handler didn't set it explicitly. Read-side tools
			// (search/list/stats/get) all return text the model consumes —
			// this gives the dashboard a real "spent" number for them
			// without needing per-handler instrumentation. Handlers that
			// compute richer figures (code_outline, code_read_symbol with
			// savings) take precedence.
			if result != nil {
				if call, ok := result.(*mcp.CallToolResult); ok && call != nil {
					total := 0
					for _, c := range call.Content {
						if tc, ok := c.(*mcp.TextContent); ok {
							total += len(tc.Text)
						}
					}
					if total > 0 {
						span.TokensIfUnset((total + 2) / 3)
					}
				}
			}
			return result, err
		}
	}
}

// mcpConfig holds parsed configuration for the MCP server.
type mcpConfig struct {
	codeWatch         bool
	codeWatchPath     string
	codeWatchDelayStr string
	codeStoreEnabled  bool
	codeStoreLazy     bool
}

// parseMCPArgs validates flags and returns parsed config. Returns nil if help was printed.
func parseMCPArgs(args []string) (*mcpConfig, error) {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			printMCPUsage()
			return nil, nil
		}
		if err := validateMCPFlag(arg); err != nil {
			return nil, err
		}
	}

	c := config.Get().Code
	cfg := &mcpConfig{
		codeWatch:         hasFlag(args, "--code-watch") || c.Watch,
		codeWatchPath:     parseFlag(args, "--code-watch="),
		codeWatchDelayStr: parseFlag(args, "--code-watch-delay="),
		codeStoreEnabled:  c.StoreEnabled,
		codeStoreLazy:     !c.StoreSync,
	}
	if cfg.codeWatchPath == "" {
		cfg.codeWatchPath = c.WatchPaths
	}
	if cfg.codeWatchDelayStr == "" {
		cfg.codeWatchDelayStr = c.WatchDelay
	}
	return cfg, nil
}

// validateMCPFlag checks that a flag argument is recognized.
func validateMCPFlag(arg string) error {
	if strings.HasPrefix(arg, "--") {
		known := []string{"--code-watch", "--code-watch=", "--code-watch-delay="}
		for _, k := range known {
			if arg == k || strings.HasPrefix(arg, k) {
				return nil
			}
		}
		return fmt.Errorf("unknown flag: %s\n\nRun 'aide mcp --help' for usage", arg)
	}
	if strings.HasPrefix(arg, "-") {
		return fmt.Errorf("unknown flag: %s\n\nRun 'aide mcp --help' for usage", arg)
	}
	return nil
}

// initMCPCodeStore sets up the code store and returns it with a cleanup. The
// lazy path returns a nil store and publishes it later via setCodeStore.
func (s *MCPServer) initMCPCodeStore(dbPath string, cfg *mcpConfig, grpcServer *grpcapi.Server) (store.CodeIndexStore, func()) {
	if !cfg.codeStoreEnabled {
		mcpLog.Printf("code store: disabled")
		return nil, nil
	}

	indexPath, searchPath := getCodeStorePaths(dbPath)
	openCodeStore := func() (*store.CodeStore, error) {
		codeStart := time.Now()
		cs, err := store.NewCodeStore(indexPath, searchPath)
		if err != nil {
			return nil, err
		}
		mcpLog.Printf("code index opened in %v: %s", time.Since(codeStart), indexPath)
		return cs, nil
	}

	if cfg.codeStoreLazy {
		mcpLog.Printf("code store: lazy init enabled")
		s.codeInitWg.Add(1)
		go func() {
			defer s.codeInitWg.Done()
			time.Sleep(DefaultMCPPollInterval)
			cs, err := openCodeStore()
			if err != nil {
				mcpLog.Printf("WARNING: lazy code store init failed: %v", err)
				return
			}
			s.setCodeStore(cs)
			grpcServer.SetCodeStore(cs)
		}()
		return nil, func() {
			s.codeInitWg.Wait() // Ensure lazy init completes before closing
			if cs := s.getCodeStore(); cs != nil {
				cs.Close()
			}
		}
	}

	// Synchronous initialization (AIDE_CODE_STORE_SYNC=1)
	cs, err := openCodeStore()
	if err != nil {
		mcpLog.Printf("WARNING: failed to open code store: %v (code tools disabled)", err)
		return nil, nil
	}
	grpcServer.SetCodeStore(cs)
	return cs, func() { cs.Close() }
}

// initMCPFindingsStore opens the findings store and registers it with gRPC.
func (s *MCPServer) initMCPFindingsStore(dbPath string, grpcServer *grpcapi.Server) (store.FindingsStore, func()) {
	findingsDir := getFindingsStorePath(dbPath)

	findingsStart := time.Now()
	fs, err := store.NewFindingsStore(findingsDir)
	if err != nil {
		mcpLog.Printf("WARNING: failed to open findings store: %v (findings tools disabled)", err)
		return nil, nil
	}
	mcpLog.Printf("findings store opened in %v: %s", time.Since(findingsStart), findingsDir)

	grpcServer.SetFindingsStore(fs)
	return fs, func() { fs.Close() }
}

// initMCPSurveyStore opens the survey store and registers it with gRPC.
func (s *MCPServer) initMCPSurveyStore(dbPath string, grpcServer *grpcapi.Server) (store.SurveyStore, func()) {
	surveyDir := getSurveyStorePath(dbPath)

	surveyStart := time.Now()
	ss, err := store.NewSurveyStore(surveyDir)
	if err != nil {
		mcpLog.Printf("WARNING: failed to open survey store: %v (survey tools disabled)", err)
		return nil, nil
	}
	mcpLog.Printf("survey store opened in %v: %s", time.Since(surveyStart), surveyDir)

	grpcServer.SetSurveyStore(ss)
	return ss, func() { ss.Close() }
}

// startCodeReconciler runs Indexer.Reconcile once at daemon startup and wires
// the gRPC code-reconciler callback so analyzer RPCs (deadcode, etc.) refresh
// the index before producing findings. Unlike startCodeWatcher this runs
// unconditionally — it's how upgrades from older versions auto-heal stale
// index entries (orphan paths, in-file orphans, paths now matching
// .aideignore) without requiring users to know about `aide code reconcile`.
func (s *MCPServer) startCodeReconciler(dbPath string) {
	projRoot := store.ProjectRootFromDB(dbPath)
	if !isVCSRoot(projRoot) && !config.Get().IndexNonVCS {
		// Same VCS guard as startCodeWatcher — don't touch arbitrary dirs.
		return
	}

	go func() {
		// Wait for the lazy-loaded code store to be ready.
		for i := 0; i < DefaultMCPPollCount; i++ {
			if s.codeStoreReady.Load() {
				break
			}
			time.Sleep(DefaultMCPPollInterval)
		}

		var indexer *Indexer
		if cs := s.getCodeStore(); cs != nil {
			indexer = NewIndexerFromStore(cs, s.grammarLoader, projRoot)
		} else {
			var err error
			indexer, err = NewIndexer(dbPath)
			if err != nil {
				mcpLog.Printf("WARNING: failed to create code indexer for reconcile: %v", err)
				return
			}
		}

		if srv := s.grpcSrv(); srv != nil {
			srv.SetCodeReconciler(func() (int, int, error) {
				res, err := indexer.Reconcile()
				return res.Removed, res.Refreshed, err
			})
		}

		res, err := indexer.Reconcile()
		if err != nil {
			mcpLog.Printf("startup reconcile failed: %v", err)
			return
		}
		if res.Removed > 0 || res.Refreshed > 0 || res.Errors > 0 {
			mcpLog.Printf("startup reconcile: checked %d, removed %d, refreshed %d, errors %d",
				res.Checked, res.Removed, res.Refreshed, res.Errors)
		}

		// Analyse what reconcile indexed, or the index heals but findings don't.
		if len(res.Touched) == 0 {
			return
		}
		runner := s.awaitFindingsRunner()
		if runner == nil {
			return
		}
		files := make(map[string]fsnotify.Op, len(res.Touched))
		for _, p := range res.Touched {
			files[p] = fsnotify.Write
		}
		mcpLog.Printf("startup reconcile: analysing %d reconciled file(s)", len(files))
		runner.OnChanges(files)
	}()
}

// awaitFindingsRunner waits for startCodeWatcher to construct the findings
// runner — both start as independent goroutines. Returns nil when the watcher
// is disabled or has no findings store.
func (s *MCPServer) awaitFindingsRunner() *findings.Runner {
	for i := 0; i < DefaultMCPPollCount; i++ {
		s.unifiedWatcherMu.Lock()
		r := s.findingsRunner
		s.unifiedWatcherMu.Unlock()
		if r != nil {
			return r
		}
		time.Sleep(DefaultMCPPollInterval)
	}
	return nil
}

// startCodeWatcher launches the file watcher in the background.
// It reuses the MCPServer's existing code store to avoid double-opening bolt/bleve.
func (s *MCPServer) startCodeWatcher(dbPath string, cfg *mcpConfig) {
	if !cfg.codeWatch && cfg.codeWatchPath == "" {
		return
	}

	projRoot := store.ProjectRootFromDB(dbPath)
	if !isVCSRoot(projRoot) && !config.Get().IndexNonVCS {
		mcpLog.Printf("WARNING: code watcher disabled — project root %q has no VCS marker (.git/.hg/.svn/.bzr/.fossil). Set AIDE_INDEX_NON_VCS=1 to allow watching/indexing in non-version-controlled directories.", projRoot)
		return
	}

	go func() {
		if cfg.codeStoreLazy {
			for i := 0; i < DefaultMCPPollCount; i++ {
				if s.codeStoreReady.Load() {
					break
				}
				time.Sleep(DefaultMCPPollInterval)
			}
		}

		var indexer *Indexer
		if cs := s.getCodeStore(); cs != nil {
			indexer = NewIndexerFromStore(cs, s.grammarLoader, store.ProjectRootFromDB(dbPath))
		} else {
			var err error
			indexer, err = NewIndexer(dbPath)
			if err != nil {
				mcpLog.Printf("WARNING: failed to create code indexer: %v", err)
				return
			}
		}

		debounceDelay := watcher.DefaultDebounceDelay
		if cfg.codeWatchDelayStr != "" {
			if d, err := time.ParseDuration(cfg.codeWatchDelayStr); err == nil {
				debounceDelay = d
			}
		}

		var watchPaths []string
		if cfg.codeWatchPath != "" {
			watchPaths = strings.Split(cfg.codeWatchPath, ",")
		}

		codeHandler := &codeIndexHandler{indexer: indexer}

		// Build handler list — always include code indexer, add findings runner if store is available
		handlers := []watcher.FileChangeHandler{codeHandler}

		// One matcher for the whole pipeline: the watcher decides what to
		// watch with it, the analysers filter with it.
		projectRoot := store.ProjectRootFromDB(dbPath)
		ignore, err := aideignore.New(projectRoot)
		if err != nil {
			mcpLog.Printf("WARNING: failed to load .aideignore: %v (using defaults)", err)
			ignore = aideignore.NewFromDefaults()
		}

		var findingsRunner *findings.Runner
		if s.findingsStore() != nil {
			// Load analyser thresholds from .aide/config/aide.json.
			fcfg := loadFindingsConfig(projectRoot)

			runnerConfig := findings.AnalyzerConfig{
				Paths:               watchPaths,
				Ignore:              ignore,
				ProjectRoot:         projectRoot,
				ComplexityThreshold: fcfg.Complexity.Threshold,
				FanOutThreshold:     fcfg.Coupling.FanOut,
				FanInThreshold:      fcfg.Coupling.FanIn,
				CloneWindowSize:     fcfg.Clones.WindowSize,
				CloneMinLines:       fcfg.Clones.MinLines,
				CloneMinMatchCount:  fcfg.Clones.MinMatchCount,
				CloneMaxBucketSize:  fcfg.Clones.MaxBucketSize,
				CloneMinSimilarity:  fcfg.Clones.MinSimilarity,
				CloneMinSeverity:    fcfg.Clones.MinSeverity,
			}
			findingsRunner = findings.NewRunner(s.findingsStore(), runnerConfig, s.grammarLoader)
			findingsRunner.SetClonesRunner(func(ctx context.Context, paths []string, cfg findings.ClonesRunnerConfig) ([]*findings.Finding, error) {
				cloneCfg := clone.Config{
					Paths:         paths,
					WindowSize:    cfg.WindowSize,
					MinCloneLines: cfg.MinLines,
					MinMatchCount: cfg.MinMatchCount,
					MaxBucketSize: cfg.MaxBucketSize,
					MinSimilarity: cfg.MinSimilarity,
					MinSeverity:   cfg.MinSeverity,
					Ignore:        ignore,
					Loader:        s.grammarLoader,
				}
				f, _, err := clone.DetectClones(cloneCfg)
				return f, err
			})
			if fcfg.DeadCode.Watch == nil || *fcfg.DeadCode.Watch {
				findingsRunner.SetDeadCodeRunner(func(ctx context.Context) ([]*findings.Finding, error) {
					return runWatcherDeadCode(s.getCodeStore(), projectRoot)
				})
			}

			handlers = append(handlers, findingsRunner)
		} else {
			mcpLog.Printf("WARNING: findings store not available, findings analysis disabled in watcher")
		}

		// Register grammar install callback: when a new grammar is downloaded,
		// re-scan the project tree for files matching its extensions.
		root := projectRoot
		s.grammarLoader.SetOnInstall(func(name string) {
			// Run re-scan in a goroutine to avoid blocking the parse call
			// that triggered the download.
			go func() {
				rescanForGrammar(name, indexer, findingsRunner, root, ignore)
				// Mark re-scan complete in the manifest so it won't be
				// re-triggered on restart.
				s.grammarLoader.MarkRescanComplete(name)
			}()
		})

		w, err := watcher.New(watcher.Config{
			Paths:         watchPaths,
			ProjectRoot:   projectRoot,
			DebounceDelay: debounceDelay,
			FileFilter:    code.SupportedFile,
			Ignore:        ignore,
		}, handlers...)
		if err != nil {
			mcpLog.Printf("WARNING: failed to create unified watcher: %v", err)
			return
		}

		if err := w.Start(); err != nil {
			mcpLog.Printf("WARNING: failed to start unified watcher: %v", err)
			return
		}

		s.unifiedWatcherMu.Lock()
		s.unifiedWatcher = w
		s.findingsRunner = findingsRunner
		s.unifiedWatcherMu.Unlock()

		// Expose watcher/runner to gRPC services. The reconciler is wired
		// separately by startCodeReconciler so it runs unconditionally.
		if srv := s.grpcSrv(); srv != nil {
			srv.SetWatcher(w)
			srv.SetFindingsRunner(findingsRunner)
		}

		if len(watchPaths) > 0 {
			mcpLog.Printf("unified watcher enabled for: %s (debounce: %v)", strings.Join(watchPaths, ", "), debounceDelay)
		} else {
			mcpLog.Printf("unified watcher enabled for current directory (debounce: %v)", debounceDelay)
		}

		// Startup re-scan: check for grammars that were installed but whose
		// project re-scan didn't complete (e.g. process was killed mid-scan).
		if pending := s.grammarLoader.GrammarsNeedingRescan(); len(pending) > 0 {
			mcpLog.Printf("found %d grammar(s) with pending re-scan: %s", len(pending), strings.Join(pending, ", "))
			go func() {
				for _, name := range pending {
					rescanForGrammar(name, indexer, findingsRunner, root, ignore)
					s.grammarLoader.MarkRescanComplete(name)
				}
			}()
		}
	}()
}

type codeIndexHandler struct {
	indexer *Indexer
}

func (h *codeIndexHandler) OnChanges(files map[string]fsnotify.Op) {
	for path, op := range files {
		if watcher.IsRemove(op) {
			if err := h.indexer.RemoveFile(path); err != nil {
				mcpLog.Printf("failed to remove %s: %v", path, err)
			} else {
				mcpLog.Printf("removed %s from index", path)
			}
		} else {
			count, err := h.indexer.IndexFile(path)
			switch {
			case err != nil:
				mcpLog.Printf("failed to index %s: %v", path, err)
			case count == 0:
				// Zero symbols may indicate the grammar isn't available yet.
				// Log at a higher level to distinguish from genuinely empty files.
				if lang := code.GetLanguageForFile(path); lang != "" {
					mcpLog.Printf("indexed %s: 0 symbols (grammar %q may not be installed yet)", path, lang)
				} else {
					mcpLog.Printf("indexed %s: 0 symbols", path)
				}
			default:
				mcpLog.Printf("indexed %s: %d symbols", path, count)
			}
		}
	}
}

// rescanForGrammar walks the project tree and re-indexes files matching
// the given grammar's extensions. This is called after a grammar is newly
// installed to pick up files that were previously skipped (zero symbols).
// It also notifies the findings runner if available.
func rescanForGrammar(name string, indexer *Indexer, runner *findings.Runner, root string, ignore *aideignore.Matcher) {
	pack := grammar.DefaultPackRegistry().Get(name)
	if pack == nil {
		return
	}

	// Build a set of extensions to match.
	extSet := make(map[string]bool, len(pack.Meta.Extensions))
	for _, ext := range pack.Meta.Extensions {
		extSet[strings.ToLower(ext)] = true
	}
	fnSet := make(map[string]bool, len(pack.Meta.Filenames))
	for _, fn := range pack.Meta.Filenames {
		fnSet[fn] = true
	}
	if len(extSet) == 0 && len(fnSet) == 0 {
		return
	}

	mcpLog.Printf("re-scanning project for %s files after grammar install", name)
	var count int
	var findingsFiles map[string]fsnotify.Op

	if runner != nil {
		findingsFiles = make(map[string]fsnotify.Op)
	}

	shouldSkip := ignore.WalkFunc(root)
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
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

		ext := strings.ToLower(filepath.Ext(path))
		base := filepath.Base(path)
		if !extSet[ext] && !fnSet[base] {
			return nil
		}

		n, indexErr := indexer.IndexFile(path)
		if indexErr != nil {
			mcpLog.Printf("re-scan: failed to index %s: %v", path, indexErr)
		} else if n > 0 {
			count++
		}

		if findingsFiles != nil {
			findingsFiles[path] = fsnotify.Write
		}
		return nil
	})

	mcpLog.Printf("re-scan complete for %s: %d files indexed", name, count)

	// Notify findings runner about the re-scanned files.
	if runner != nil && len(findingsFiles) > 0 {
		runner.OnChanges(findingsFiles)
	}
}

// runWatcherDeadCode runs the dead-code analyser against the live code index.
// An empty index is not an error here — the watcher runs before anything has
// been indexed on a fresh project, and reporting "no dead code" is the honest
// answer until there are symbols to check.
func runWatcherDeadCode(cs store.CodeIndexStore, projectRoot string) ([]*findings.Finding, error) {
	if cs == nil {
		return nil, nil
	}
	stats, err := cs.Stats()
	if err != nil {
		return nil, fmt.Errorf("read code stats: %w", err)
	}
	if stats.Symbols == 0 {
		return nil, nil
	}

	registry := grammar.DefaultPackRegistry()
	ff, _, err := findings.AnalyzeDeadCode(findings.DeadCodeConfig{
		GetAllSymbols: func() ([]*code.Symbol, error) {
			return cs.ListAllSymbols(-1)
		},
		GetRefCount: func(name string) (int, error) {
			refs, err := cs.SearchReferences(code.ReferenceSearchOptions{
				SymbolName: name,
				Limit:      1,
			})
			if err != nil {
				return 0, err
			}
			return len(refs), nil
		},
		ProjectRoot:        projectRoot,
		PackProvider:       registry.Get,
		ConsumerExtensions: registry.ConsumerExtensions(),
	})
	return ff, err
}

// stopCodeWatcher gracefully stops the file watcher if running.
func (s *MCPServer) stopCodeWatcher() {
	s.unifiedWatcherMu.Lock()
	w := s.unifiedWatcher
	runner := s.findingsRunner
	s.unifiedWatcher = nil
	s.findingsRunner = nil
	s.unifiedWatcherMu.Unlock()

	if runner != nil {
		runner.Stop()
	}
	if w != nil {
		if err := w.Stop(); err != nil {
			mcpLog.Printf("WARNING: watcher stop error: %v", err)
		}
	}
}

// Election and failover timings. The lock timeout is short because losing the
// election must fail fast: the loser's next move is to attach to the winner.
const (
	electionLockTimeout = 750 * time.Millisecond
	electionBackoff     = 50 * time.Millisecond
	electionBackoffMax  = 1 * time.Second
	electionDeadline    = 5 * time.Second
	primaryPingInterval = 2 * time.Second
)

// attachToPrimary connects to a primary already serving the socket and points
// every store at it over gRPC. Reports whether it got a healthy connection.
func (s *MCPServer) attachToPrimary(dbPath string) bool {
	if !grpcapi.SocketExistsForDB(dbPath) {
		return false
	}
	client, err := grpcapi.NewClientForDB(dbPath)
	if err != nil {
		os.Remove(grpcapi.SocketPathFromDB(dbPath))
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), DefaultPingTimeout)
	pingErr := client.Ping(ctx)
	cancel()
	if pingErr != nil {
		client.Close()
		// A socket nobody answers on is stale; clear it so the next promotion
		// attempt isn't waved off by its mere existence.
		os.Remove(grpcapi.SocketPathFromDB(dbPath))
		return false
	}

	s.setBackend(&mcpBackend{
		store:         adapter.NewStoreAdapter(client),
		findingsStore: adapter.NewFindingsAdapter(client),
		surveyStore:   adapter.NewSurveyAdapter(client),
		instinctStore: adapter.NewInstinctAdapter(client),
		// Code tools read the primary's index over gRPC; without this the
		// whole code_* family (and survey_graph) is dead in client mode.
		codeStore:  adapter.NewCodeAdapter(client),
		grpcClient: client,
	})
	s.codeStoreReady.Store(true)
	return true
}

// becomePrimary opens the store and serves gRPC on the socket for other
// instances. Bolt's exclusive lock is the election: exactly one process can
// hold the store, so whoever opens it first wins and the rest get an error.
//
// Returns a teardown for everything it opened.
func (s *MCPServer) becomePrimary(dbPath string, cfg *mcpConfig) (func(), error) {
	socketPath := grpcapi.SocketPathFromDB(dbPath)

	storeStart := time.Now()
	st, err := store.NewCombinedStoreWithTimeout(dbPath, electionLockTimeout)
	if err != nil {
		return nil, err
	}
	mcpLog.Printf("database opened in %v (bolt + bleve search)", time.Since(storeStart))

	var cleanups []func()
	defer func() {
		if err != nil {
			runTeardowns(cleanups)
		}
	}()
	cleanups = append(cleanups, func() { st.Close() })

	observeSink := store.NewObserveSink(st.Bolt())
	observe.SetDefault(observeSink)
	cleanups = append(cleanups, func() { observe.SetDefault(nil) })

	// The MCP primary holds the store locks for its whole lifetime, which
	// makes Backend.RetentionSweep (direct-mode only) a no-op for every CLI
	// invocation — so the primary must run the retention loop itself.
	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	cleanups = append(cleanups, cleanupCancel)
	go runCleanupLoop(cleanupCtx, st, mcpLog.Printf)

	if migrated, mErr := st.Bolt().MigrateTokenEventsToObserve(); mErr != nil {
		mcpLog.Printf("WARNING: token-event migration failed: %v", mErr)
	} else if migrated > 0 {
		mcpLog.Printf("migrated %d legacy token events into observe store", migrated)
	}

	grpcServer := grpcapi.NewServer(st, dbPath, socketPath, s.grammarLoader)
	observeSink.SetBus(grpcServer.ObserveBus())
	grpcServer.SetInstinctStore(st)
	s.grpcServer.Store(grpcServer)
	// Wired here rather than in Run: a process that starts as a client has run
	// Run already by the time it promotes, so the status service would never
	// learn its tool list.
	grpcServer.SetMCPTools(mcpToolList())
	grpcServer.SetToolCountFunc(s.getToolCounts)
	grpcServer.SetPprofURLFunc(pprofURL)
	mcpLog.Printf("gRPC socket: %s", socketPath)

	// Initialize stores BEFORE starting gRPC server.
	// grpcServer.Start() registers service implementations that capture store
	// references at registration time, so stores must be set first.
	cs, csCleanup := s.initMCPCodeStore(dbPath, cfg, grpcServer)
	fs, fsCleanup := s.initMCPFindingsStore(dbPath, grpcServer)
	ss, ssCleanup := s.initMCPSurveyStore(dbPath, grpcServer)
	for _, cleanup := range []func(){csCleanup, fsCleanup, ssCleanup} {
		if cleanup != nil {
			cleanups = append(cleanups, cleanup)
		}
	}

	// Published as one set, only now that every store behind it is open — a
	// promotion must not expose a half-built primary to a call in flight.
	s.setBackend(&mcpBackend{
		store:         st,
		instinctStore: st,
		codeStore:     cs,
		findingsStore: fs,
		surveyStore:   ss,
	})
	s.codeStoreReady.Store(cs != nil)

	go func() {
		if err := grpcServer.Start(); err != nil {
			mcpLog.Printf("gRPC server error: %v", err)
		}
	}()
	cleanups = append(cleanups, grpcServer.Stop)

	// Register instance for discovery by aide-web
	projRoot := store.ProjectRootFromDB(dbPath)
	var chainParents []string
	for _, link := range resolveAnchor(projRoot).Chain[1:] {
		chainParents = append(chainParents, link.Root)
	}
	if rErr := registry.RegisterWithParents(projRoot, socketPath, dbPath, chainParents); rErr != nil {
		mcpLog.Printf("warning: failed to register instance: %v", rErr)
	} else {
		cleanups = append(cleanups, func() {
			if err := registry.Unregister(projRoot); err != nil {
				mcpLog.Printf("warning: failed to unregister instance: %v", err)
			}
		})
	}

	// Reconcile heals whatever changed while no primary was watching, which
	// on a promotion is exactly the gap since the last one died.
	s.startCodeReconciler(dbPath)
	s.startCodeWatcher(dbPath, cfg)
	cleanups = append(cleanups, s.stopCodeWatcher)

	return func() { runTeardowns(cleanups) }, nil
}

func runTeardowns(cleanups []func()) {
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
}

// join attaches to a running primary, or becomes one. Whoever opens the store
// first wins; the losers back off and retry, by which time the winner is
// serving the socket and they attach to it instead.
func (s *MCPServer) join(dbPath string, cfg *mcpConfig) (func(), error) {
	deadline := time.Now().Add(electionDeadline)
	wait := electionBackoff

	for attempt := 1; ; attempt++ {
		if s.attachToPrimary(dbPath) {
			mcpLog.Printf("client mode: attached to primary via %s", grpcapi.SocketPathFromDB(dbPath))
			return func() {
				if c := s.grpcClient(); c != nil {
					c.Close()
				}
			}, nil
		}

		teardown, err := s.becomePrimary(dbPath, cfg)
		if err == nil {
			mcpLog.Printf("primary mode: holding the store, serving gRPC")
			return teardown, nil
		}

		if time.Now().After(deadline) {
			return nil, fmt.Errorf("could not attach to or become primary after %v: %w", electionDeadline, err)
		}
		mcpLog.Printf("election attempt %d lost (%v), retrying in %v", attempt, err, wait)
		time.Sleep(wait)
		if wait *= 2; wait > electionBackoffMax {
			wait = electionBackoffMax
		}
	}
}

// supervisePrimary re-runs the election when the primary this client is
// attached to goes away. A primary never demotes — holding the bolt lock means
// nothing can take it — so this only ever runs in client mode.
func (s *MCPServer) supervisePrimary(ctx context.Context, dbPath string, cfg *mcpConfig, swap func(func())) {
	ticker := time.NewTicker(primaryPingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		client := s.grpcClient()
		if client == nil {
			return // promoted; nothing left to supervise
		}
		pingCtx, cancel := context.WithTimeout(ctx, DefaultPingTimeout)
		err := client.Ping(pingCtx)
		cancel()
		if err == nil {
			continue
		}

		mcpLog.Printf("primary unreachable (%v), re-running election", err)
		client.Close()
		next, err := s.join(dbPath, cfg)
		if err != nil {
			mcpLog.Printf("ERROR: no primary available: %v", err)
			continue
		}
		swap(next)
		if s.grpcClient() == nil {
			return // we are the primary now
		}
	}
}

// cmdMCP starts the MCP server over stdio. Only one process per project can
// hold the store, so it either attaches to whoever already does or becomes
// that process itself, and keeps checking that the one it attached to is
// still there.
func cmdMCP(dbPath string, args []string) error {
	cfg, err := parseMCPArgs(args)
	if err != nil {
		return err
	}
	if cfg == nil {
		return nil // --help was printed
	}

	startTime := time.Now()

	if config.Get().Pprof.Enable {
		initPprof()
		defer stopPprof()
	}

	mcpLog.Printf("aide MCP server starting")
	mcpLog.Printf("version: %s", version.String())
	mcpLog.Printf("database: %s", dbPath)

	mcpServer := newMCPServer(nil)
	mcpServer.grammarLoader = newGrammarLoader(dbPath, mcpLog)
	mcpServer.dbPath = dbPath

	// Registered first so it runs last — after every store Close has run,
	// leaving the bolt files unlocked for compaction. No-op unless enabled.
	defer compactStoresOnExit(dbPath)

	teardown, err := mcpServer.join(dbPath, cfg)
	if err != nil {
		return err
	}

	// The supervisor replaces the teardown when it promotes, so exit must run
	// whichever one is current rather than the one join happened to return.
	var teardownMu sync.Mutex
	current := teardown
	swap := func(next func()) {
		teardownMu.Lock()
		prev := current
		current = next
		teardownMu.Unlock()
		prev()
	}
	defer func() {
		teardownMu.Lock()
		defer teardownMu.Unlock()
		current()
	}()

	superviseCtx, stopSupervisor := context.WithCancel(context.Background())
	defer stopSupervisor()
	go mcpServer.supervisePrimary(superviseCtx, dbPath, cfg, swap)

	mcpLog.Printf("MCP server ready in %v, listening on stdio", time.Since(startTime))
	return mcpServer.Run()
}

// Run starts the MCP server and registers all tools.
func (s *MCPServer) Run() error {
	srv := mcp.NewServer(
		&mcp.Implementation{
			Name:    "aide",
			Version: version.Short(),
		},
		nil, // Use default capabilities
	)
	s.server = srv

	// Track tool execution counts + emit observe events
	srv.AddReceivingMiddleware(s.toolCountMiddleware())
	srv.AddReceivingMiddleware(s.toolObserveMiddleware())

	// Register tools — data layer + task coordination
	s.registerMemoryTools()
	s.registerStateReadTools() // Read-only state access
	s.registerDecisionTools()
	s.registerMessageTools()
	s.registerTaskTools()         // Shared task management (swarm coordination, persistence)
	s.registerCodeTools()         // Code indexing and search
	s.registerFindingsTools()     // Findings search and stats
	s.registerSurveyTools()       // Survey search, list, stats, run
	s.registerInstinctTools()     // Instinct proposals (reflect output) list/accept/reject
	s.registerInstanceInfoTools() // Instance identity: project root, version, paths
	s.registerTokenTools()        // Token intelligence and statistics

	// Run over stdio
	return srv.Run(context.Background(), &mcp.StdioTransport{})
}

// mcpToolList returns the static list of MCP tools registered by the server.
func mcpToolList() []*grpcapi.StatusMCPTool {
	return []*grpcapi.StatusMCPTool{
		{Name: "memory_search", Category: "memory"},
		{Name: "memory_list", Category: "memory"},
		{Name: "state_get", Category: "state"},
		{Name: "state_list", Category: "state"},
		{Name: "decision_get", Category: "decision"},
		{Name: "decision_history", Category: "decision"},
		{Name: "decision_list", Category: "decision"},
		{Name: "message_list", Category: "message"},
		{Name: "message_send", Category: "message"},
		{Name: "message_ack", Category: "message"},
		{Name: "task_create", Category: "task"},
		{Name: "task_get", Category: "task"},
		{Name: "task_list", Category: "task"},
		{Name: "task_claim", Category: "task"},
		{Name: "task_complete", Category: "task"},
		{Name: "task_delete", Category: "task"},
		{Name: "code_search", Category: "code"},
		{Name: "code_symbols", Category: "code"},
		{Name: "code_stats", Category: "code"},
		{Name: "code_references", Category: "code"},
		{Name: "code_outline", Category: "code"},
		{Name: "code_top_references", Category: "code"},
		{Name: "code_read_symbol", Category: "code"},
		{Name: "code_read_check", Category: "code"},
		{Name: "findings_search", Category: "findings"},
		{Name: "findings_list", Category: "findings"},
		{Name: "findings_stats", Category: "findings"},
		{Name: "findings_accept", Category: "findings"},
		{Name: "survey_search", Category: "survey"},
		{Name: "survey_list", Category: "survey"},
		{Name: "survey_stats", Category: "survey"},
		{Name: "survey_run", Category: "survey"},
		{Name: "survey_graph", Category: "survey"},
		{Name: "instance_info", Category: "instance"},
		{Name: "token_stats", Category: "token"},
	}
}

// printMCPUsage prints help for the mcp command.
func printMCPUsage() {
	fmt.Printf(`aide mcp - Start MCP server for Claude Code plugin integration

Usage:
  aide mcp [flags]

Flags:
  --code-watch           Enable file watching for code index updates
  --code-watch=<paths>   Comma-separated paths to watch
  --code-watch-delay=<d> Debounce delay for watcher (e.g., 30s)
  --help, -h             Show this help

Environment Variables:
  AIDE_CODE_WATCH=1         Enable file watching
  AIDE_CODE_WATCH_PATHS     Comma-separated paths to watch
  AIDE_CODE_WATCH_DELAY     Debounce delay (default: 30s)
  AIDE_INDEX_NON_VCS=1      Allow watcher in non-VCS dirs (default: refuse)
  AIDE_INDEX_WORKERS=N      Parallel parser workers for code indexing (default: NumCPU, max 32)
  AIDE_CODE_STORE_DISABLE=1 Disable code store entirely
  AIDE_CODE_STORE_SYNC=1    Force synchronous code store init (default: lazy)
  AIDE_PPROF_ENABLE=1       Enable pprof profiling (requires -tags pprof build)
  AIDE_PPROF_ADDR           pprof server address (default: localhost:6060)

The MCP server communicates over stdio using JSON-RPC protocol.
It is typically started by Claude Code via the plugin configuration.
`)
}
