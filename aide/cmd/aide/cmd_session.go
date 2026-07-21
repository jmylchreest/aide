package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"context"

	"github.com/jmylchreest/aide/aide/pkg/anchor"
	"github.com/jmylchreest/aide/aide/pkg/code"
	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/contextshare"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi/adapter"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// SessionInitResult is the JSON output of `aide session init`.
type SessionInitResult struct {
	// State cleanup results
	StateKeysDeleted   int `json:"state_keys_deleted"`
	StaleAgentsCleaned int `json:"stale_agents_cleaned"`

	// Share auto-import results
	SharedImported int `json:"shared_imported,omitempty"`

	// Retention sweep results (direct mode only), per-bucket pruned counts
	RetentionPruned map[string]int `json:"retention_pruned,omitempty"`

	// Memory injection data
	GlobalMemories        []SessionMemory   `json:"global_memories"`
	ProjectMemories       []SessionMemory   `json:"project_memories"`
	ProjectMemoryOverflow bool              `json:"project_memory_overflow,omitempty"`
	Decisions             []SessionDecision `json:"decisions"`
	RecentSessions        []*SessionGroup   `json:"recent_sessions"`

	// Codebase Map — module entries from the survey modules analyzer,
	// largest first, capped. Empty when the analyzer has never run.
	CodebaseMap     []SessionModule `json:"codebase_map,omitempty"`
	CodebaseMapNote string          `json:"codebase_map_note,omitempty"` // freshness, e.g. "as of a1b2c3d4 — 3 commits behind"

	// Estate — parent projects from the anchor chain (upward) and direct
	// child subprojects from survey (downward). Omitted for standalone
	// repos with no surveyed children.
	Estate *SessionEstate `json:"estate,omitempty"`
}

// SessionEstate is the estate picture for session injection.
type SessionEstate struct {
	Parents     []SessionEstateNode `json:"parents,omitempty"`
	Subprojects []SessionEstateNode `json:"subprojects,omitempty"`
}

// SessionEstateNode is one related project scope.
type SessionEstateNode struct {
	Name     string `json:"name,omitempty"`
	Path     string `json:"path"`
	Evidence string `json:"evidence,omitempty"`
	HasStore bool   `json:"has_store,omitempty"`
}

// SessionModule is one Codebase Map line for JSON output.
type SessionModule struct {
	Name string `json:"name"`
	Size int    `json:"size"`
	Hub  string `json:"hub"`
}

// sessionModuleLimit caps how many modules the Codebase Map carries — it is
// a session-start orientation aid, not the full survey.
const sessionModuleLimit = 12

// SessionMemory is a memory entry for JSON output.
type SessionMemory struct {
	ID        string   `json:"id"`
	Category  string   `json:"category"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
	Score     float64  `json:"score"`
}

// SessionDecision is a decision entry for JSON output. Origin fields are
// set only for decisions cascaded from an ancestor store (provenance, per
// decision terminology-axes): Origin is the ancestor's root, OriginName
// its project identity.
type SessionDecision struct {
	Topic      string `json:"topic"`
	Value      string `json:"value"`
	Rationale  string `json:"rationale,omitempty"`
	CreatedAt  string `json:"created_at"`
	Origin     string `json:"origin,omitempty"`
	OriginName string `json:"origin_name,omitempty"`
}

func cmdSession(dbPath string, args []string) error {
	return dispatchSubcmd("session", args, printSessionUsage, []subcmd{
		{name: "init", handler: func(a []string) error { return sessionInit(dbPath, a) }},
		{name: "end", handler: func(a []string) error { return sessionEnd(dbPath, a) }},
	})
}

func printSessionUsage() {
	fmt.Println(`aide session - Session lifecycle commands

Usage:
  aide session <subcommand> [arguments]

Subcommands:
  init       Initialize a new session (reset state, cleanup, fetch context)
  end        End a session (record end message, clear transient state)

Options:
  init:
    --project=NAME       Project name for memory filtering
    --cleanup-age=DUR    Max age for stale agent entries (default: 30m)
    --session-limit=N    Number of recent sessions to include (default: 3)
    --share-import       Import shared data from .aide/shared/ on session start
    --format=json        Output as JSON (default)
  end:
    --session=ID         Session ID to end (required)
    --duration=MS        Session duration in milliseconds (optional)

Environment:
  AIDE_SHARE_AUTO_IMPORT=1   Enable auto-import of shared data on session init

Examples:
  aide session init --project=myapp
  aide session init --project=myapp --share-import
  aide session init --project=myapp --cleanup-age=1h --session-limit=5
  aide session end --session=abc123 --duration=45000`)
}

// sessionStateKeys: counters are written session-scoped by hooks; `mode`
// stays GLOBAL by design (sessionless writers, shared swarm mode — see the
// note in src/core/aide-client.ts; promotion was tried and reverted).
// Session init deletes the global spellings as stale-value hygiene; session
// end deliberately does NOT — another session may still be live.
var sessionStateKeys = []string{
	"mode",
	"startedAt",
	"modelTier",
	"agentCount",
	"toolCalls",
	"lastToolUse",
	"lastTool",
}

// sessionEnd performs session teardown in a single invocation, mirroring the
// startup consolidation in sessionInit. It is spawned detached by the
// session-end hooks, so teardown is best-effort: every step runs and failures
// are reported together rather than aborting at the first error.
//  1. Broadcast a system message recording the session end
//  2. Clear session-scoped state (agent:<sessionID>:*) — this covers the
//     session's own mode/counters; global keys are left alone because other
//     sessions sharing the store may still be live
//  3. Record last-session metrics
func sessionEnd(dbPath string, args []string) error {
	sessionID := parseFlag(args, "--session=")
	if sessionID == "" {
		return fmt.Errorf("usage: aide session end --session=SESSION_ID [--duration=MS]")
	}
	// A bad duration must not abort teardown (the hook spawns this detached
	// and discards stderr — aborting here would silently skip cleanup).
	// Degrade to no-duration and report the error alongside the teardown.
	durationMS := 0
	var errs []error
	if s := parseFlag(args, "--duration="); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			durationMS = n
		} else {
			errs = append(errs, fmt.Errorf("invalid --duration= value %q: expected milliseconds (teardown ran without it)", s))
		}
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	content := fmt.Sprintf("Session %s ended", sessionID)
	if durationMS > 0 {
		content = fmt.Sprintf("Session %s ended (%ds)", sessionID, (durationMS+500)/1000)
	}
	if _, err := backend.SendMessageWithOpts("system", "", content, "system", DefaultMessageTTLSeconds, MessageSendOpts{}); err != nil {
		errs = append(errs, fmt.Errorf("send session-end message: %w", err))
	}

	cleared, err := backend.ClearState(sessionID)
	if err != nil {
		errs = append(errs, fmt.Errorf("clear agent state: %w", err))
	}

	if durationMS > 0 {
		if err := backend.SetState("last_session_duration", strconv.Itoa(durationMS), ""); err != nil {
			errs = append(errs, fmt.Errorf("set last_session_duration: %w", err))
		}
	}
	if err := backend.SetState("last_session_end", time.Now().UTC().Format(time.RFC3339), ""); err != nil {
		errs = append(errs, fmt.Errorf("set last_session_end: %w", err))
	}

	deleteSessionAnchor(sessionID)

	fmt.Printf("Session %s ended: cleared %d session-scoped state entries\n", sessionID, cleared)
	return errors.Join(errs...)
}

// sessionInit performs all session startup in a single DB open:
// 1. Delete global state keys (mode, startedAt, etc.)
// 2. Cleanup stale agent state entries
// 3. Fetch global memories (scope:global)
// 4. Fetch project memories (project:<name>)
// 5. Fetch decisions (latest per topic)
// 6. Fetch recent session memories
func sessionInit(dbPath string, args []string) error {
	project := parseFlag(args, "--project=")
	cleanupAge := DefaultSessionCleanupAge

	if dur := parseFlag(args, "--cleanup-age="); dur != "" {
		if d, err := time.ParseDuration(dur); err == nil {
			cleanupAge = d
		}
	}
	sessionLimit, err := parseIntFlag(args, "--session-limit=", DefaultSessionLimit)
	if err != nil {
		return err
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	result := SessionInitResult{}

	// 1. Delete global state keys
	for _, key := range sessionStateKeys {
		if err := backend.DeleteState(key); err == nil {
			result.StateKeysDeleted++
		}
	}

	// 2. Cleanup stale agent state
	cleaned, err := backend.CleanupState(cleanupAge)
	if err == nil {
		result.StaleAgentsCleaned = cleaned
	}

	// 2b. Retention sweep (direct mode only; the daemon runs its own loop).
	// Rate-limited to once per hour so swarm session starts stay cheap.
	if pruned := backend.RetentionSweep(); len(pruned) > 0 {
		result.RetentionPruned = pruned
	}

	// 3. Auto-import shared data if enabled. The per-record and legacy layouts
	// share .aide/shared/. Import is governed by the share.{decisions,memories}
	// .import policy and filters (decisions on, memories off by default), so the
	// documented behaviour change applies here too: teammate memories no longer
	// auto-import unless opted in. Share errors never fail session init.
	shareImport := hasFlag(args, "--share-import") || config.Get().Share.AutoImport
	if shareImport {
		sharedDir := filepath.Join(projectRoot(dbPath), ".aide", "shared")
		if _, statErr := os.Stat(sharedDir); statErr == nil {
			result.SharedImported = autoImportShared(backend, sharedDir)
		}
	}

	// 4-7. Gather context (skip if project root is invalid)
	if validateProjectRoot(dbPath) {
		sessionFetchContext(backend, project, sessionLimit, &result)
	}

	// Ensure nil slices become empty arrays in JSON
	sessionInitDefaults(&result)

	out, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(out))
	return nil
}

// autoImportShared imports shared data from sharedDir at session init under the
// share.{decisions,memories}.import policy. It runs the per-record importer when
// that layout is present and the legacy importer when flat aggregate files
// remain. All errors are warnings on stderr — session init must never fail on a
// share error. Returns the number of records imported.
func autoImportShared(backend *Backend, sharedDir string) int {
	share := config.Get().Share
	importDecisions := share.DecisionImportEnabled()
	importMemories := share.MemoryImportEnabled()

	imported := 0
	if hasPerRecordLayout(sharedDir) {
		stats, err := contextshare.Import(backend.Store(), backend.TombstoneStore(), sharedDir, contextshare.ImportOptions{
			Decisions:      importDecisions,
			Memories:       importMemories,
			DecisionFilter: toFilter(share.DecisionImportFilter()),
			MemoryFilter:   toFilter(share.MemoryImportFilter()),
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: shared auto-import skipped: %v\n", err)
		} else {
			imported += stats.DecisionsImported + stats.MemoriesImported
		}
	}
	if hasLegacyRecords(sharedDir) {
		if importDecisions {
			n, _, err := shareImportDecisions(backend, sharedDir, false)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: legacy decision auto-import skipped: %v\n", err)
			} else {
				imported += n
			}
		}
		if importMemories {
			n, _, err := shareImportMemories(backend, sharedDir, false)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: legacy memory auto-import skipped: %v\n", err)
			} else {
				imported += n
			}
		}
	}
	return imported
}

// sessionFetchContext gathers memories, decisions, and recent sessions.
// When memory scoring is enabled (default), memories are sorted by computed
// priority score (highest first). When scoring is disabled via
// AIDE_MEMORY_SCORING_DISABLED=1, chronological ULID order is preserved.
func sessionFetchContext(backend *Backend, project string, sessionLimit int, result *SessionInitResult) {
	now := time.Now()
	cfg := memoryScoringConfig()

	// Shared token budget across global + project injected memories. Highest-
	// scored first; once the budget is exhausted, remaining (lower-ranked)
	// memories are dropped whole. 0 disables the budget. Global goes first
	// because user preferences carry the top category weight. budgetState
	// carries the running remaining-tokens and whether anything has been kept
	// yet (the single top-scored memory is always kept — see takeWithinBudget).
	bs := &budgetState{budget: config.Get().Memory.InjectionTokenBudget}
	bs.remaining = bs.budget

	// Global memories (scope:global tag) — exclude forgotten and partials
	globalMems, err := backend.ListMemories("global", 100, nil)
	if err == nil {
		var filtered []*memory.Memory
		for _, m := range globalMems {
			if hasAllTags(m.Tags, []string{"scope:global"}) && !hasAnyTag(m.Tags, []string{"partial"}) {
				filtered = append(filtered, m)
			}
		}
		sorted := scoreAndSort(filtered, now, cfg)
		for _, sm := range sorted {
			if !bs.take(sm.Memory.Content) {
				break // budget exhausted — drop this and all lower-ranked
			}
			result.GlobalMemories = append(result.GlobalMemories, memoryToSession(sm.Memory, sm.Score))
		}
	}

	// Project memories — exclude partials, cap at DefaultProjectMemoryLimit
	if project != "" {
		projectMems, err := backend.ListMemories("", 1000, nil)
		if err == nil {
			projectTag := "project:" + project
			var filtered []*memory.Memory
			for _, m := range projectMems {
				if hasAllTags(m.Tags, []string{projectTag}) && !hasAnyTag(m.Tags, []string{"partial"}) {
					filtered = append(filtered, m)
				}
			}
			sorted := scoreAndSort(filtered, now, cfg)
			// Cap project memories with overflow hint
			if len(sorted) > DefaultProjectMemoryLimit {
				sorted = sorted[:DefaultProjectMemoryLimit]
				result.ProjectMemoryOverflow = true
			}
			for _, sm := range sorted {
				if !bs.take(sm.Memory.Content) {
					// Budget exhausted: drop this and all lower-ranked project
					// memories, and flag the overflow so the plugin hints at it.
					result.ProjectMemoryOverflow = true
					break
				}
				result.ProjectMemories = append(result.ProjectMemories, memoryToSession(sm.Memory, sm.Score))
			}
		}
	}

	// Decisions (latest per topic), then the estate cascade: ancestors'
	// decisions join nearest-wins — a topic decided locally shadows every
	// ancestor's version of it, and nearer ancestors shadow farther ones.
	seenTopics := make(map[string]bool)
	decisions, err := backend.ListDecisions()
	if err == nil {
		latest := latestDecisionsByTopic(decisions)
		for _, d := range latest {
			seenTopics[d.Topic] = true
			result.Decisions = append(result.Decisions, SessionDecision{
				Topic:     d.Topic,
				Value:     d.Decision,
				Rationale: d.Rationale,
				CreatedAt: d.CreatedAt.Format(time.RFC3339),
			})
		}
	}
	sessionCascadeDecisions(backend, seenTopics, result)

	// Recent sessions (grouped by session tag)
	if project != "" && sessionLimit > 0 {
		result.RecentSessions = fetchRecentSessions(backend, project, sessionLimit)
	}

	sessionFetchCodebaseMap(backend, result)
	sessionFetchEstate(backend, result)
}

// latestDecisionsByTopic reduces an append-only decision list to the
// newest entry per topic.
func latestDecisionsByTopic(decisions []*memory.Decision) map[string]*memory.Decision {
	latest := make(map[string]*memory.Decision)
	for _, d := range decisions {
		if existing, ok := latest[d.Topic]; !ok || d.CreatedAt.After(existing.CreatedAt) {
			latest[d.Topic] = d
		}
	}
	return latest
}

// sessionCascadeDecisions layers ancestors' decisions into the session
// context, nearest-first, for topics not already decided nearer. Cascaded
// entries carry origin provenance. Kill switch: AIDE_CASCADE_DISABLED=1.
// Solo repos are unaffected — no parents, no cascade, no cost.
func sessionCascadeDecisions(backend *Backend, seenTopics map[string]bool, result *SessionInitResult) {
	if v := os.Getenv("AIDE_CASCADE_DISABLED"); v == "1" || strings.EqualFold(v, "true") {
		return
	}
	a := resolveAnchor(projectRoot(backend.dbPath))
	for _, link := range a.Chain[1:] {
		parentDecisions := fetchAncestorDecisions(link.Root)
		if len(parentDecisions) == 0 {
			continue
		}
		name, _ := anchor.ProjectIdentity(link.Root)
		// Deterministic order within one ancestor: topic-sorted.
		latest := latestDecisionsByTopic(parentDecisions)
		topics := make([]string, 0, len(latest))
		for t := range latest {
			topics = append(topics, t)
		}
		sort.Strings(topics)
		for _, topic := range topics {
			if seenTopics[topic] {
				continue
			}
			seenTopics[topic] = true
			d := latest[topic]
			result.Decisions = append(result.Decisions, SessionDecision{
				Topic:      d.Topic,
				Value:      d.Decision,
				Rationale:  d.Rationale,
				CreatedAt:  d.CreatedAt.Format(time.RFC3339),
				Origin:     link.Root,
				OriginName: name,
			})
		}
	}
}

// fetchAncestorDecisions reads an ancestor store's decisions through the
// access ladder: its live daemon socket when one exists; a short-timeout
// READ-ONLY direct open when none does (never a writable open — that
// would create buckets and contend for the write lock); skipped entirely
// when the socket exists but is unreachable, since the daemon behind it
// holds the store locks.
func fetchAncestorDecisions(root string) []*memory.Decision {
	dbPath := computeDBPath(root)
	if grpcapi.SocketExistsForDB(dbPath) {
		client, err := grpcapi.NewClientForDB(dbPath)
		if err != nil {
			return nil
		}
		defer client.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		resp, err := client.Decision.List(ctx, &grpcapi.DecisionListRequest{})
		if err != nil {
			return nil
		}
		return adapter.ProtoToDecisions(resp.Decisions)
	}

	st, err := store.NewReadOnlyBoltStore(dbPath)
	if err != nil {
		return nil
	}
	defer st.Close()
	ds, err := st.ListDecisions()
	if err != nil {
		return nil
	}
	return ds
}

// sessionFetchEstate assembles the estate picture: parents from the anchor
// chain (re-resolved, cheap), direct children from the survey subproject
// entries when the topology analyzer has run.
func sessionFetchEstate(backend *Backend, result *SessionInitResult) {
	estate := &SessionEstate{}

	a := resolveAnchor(projectRoot(backend.dbPath))
	for _, link := range a.Chain[1:] {
		name, _ := anchor.ProjectIdentity(link.Root)
		estate.Parents = append(estate.Parents, SessionEstateNode{
			Name:     name,
			Path:     link.Root,
			Evidence: link.Evidence,
			HasStore: anchor.HasAideStore(link.Root),
		})
	}

	if entries, err := backend.ListSurvey(survey.SearchOptions{Kind: survey.KindSubproject, Limit: 100}); err == nil {
		for _, e := range entries {
			estate.Subprojects = append(estate.Subprojects, SessionEstateNode{
				Name:     e.Name,
				Path:     e.FilePath,
				Evidence: e.Metadata["evidence"],
				HasStore: e.Metadata["has_aide_store"] == "true",
			})
		}
	}

	if len(estate.Parents) > 0 || len(estate.Subprojects) > 0 {
		result.Estate = estate
	}
}

// sessionFetchCodebaseMap loads the module map produced by the survey
// modules analyzer: largest modules first, capped, with a freshness note so
// a stale map says so instead of being silently trusted. Absent entries
// (analyzer never ran) leave the section empty — no nagging.
func sessionFetchCodebaseMap(backend *Backend, result *SessionInitResult) {
	entries, err := backend.ListSurvey(survey.SearchOptions{Analyzer: survey.AnalyzerModules, Limit: 1000})
	if err != nil || len(entries) == 0 {
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		si, _ := strconv.Atoi(entries[i].Metadata["size"])
		sj, _ := strconv.Atoi(entries[j].Metadata["size"])
		if si != sj {
			return si > sj
		}
		return entries[i].Name < entries[j].Name
	})

	for _, e := range entries {
		if len(result.CodebaseMap) >= sessionModuleLimit {
			break
		}
		size, _ := strconv.Atoi(e.Metadata["size"])
		result.CodebaseMap = append(result.CodebaseMap, SessionModule{
			Name: e.Name,
			Size: size,
			Hub:  e.Metadata["hub"],
		})
	}

	if runCommit := survey.RunCommitForEntries(entries); runCommit != "" {
		note := fmt.Sprintf("as of %.8s", runCommit)
		if f, ferr := survey.ComputeFreshness(projectRoot(backend.dbPath), runCommit); ferr == nil && f != nil && (f.Behind > 0 || !f.Found) {
			note += fmt.Sprintf(" — %s; run survey_run to refresh", f)
		}
		result.CodebaseMapNote = note
	}
}

// memoryScoringConfig builds a ScoringConfig from defaults and env vars.
func memoryScoringConfig() memory.ScoringConfig {
	cfg := memory.DefaultScoringConfig()
	mc := config.Get().Memory
	cfg.ScoringEnabled = mc.ScoringEnabled
	cfg.DecayEnabled = mc.DecayEnabled
	return cfg
}

// budgetState tracks a shared token budget consumed across the injected
// memory buckets (global first, then project), in score order.
type budgetState struct {
	budget    int  // configured ceiling; <= 0 disables the budget
	remaining int  // tokens left
	anyTaken  bool // whether any memory has been kept yet
}

// take reports whether a memory of the given content fits the remaining
// budget and accounts for it. When the budget is disabled (<= 0) it always
// returns true. The single highest-scored memory across the whole injection
// is always kept even if it alone exceeds the budget — an over-budget top hit
// is still more useful than injecting nothing.
func (b *budgetState) take(content string) bool {
	if b.budget <= 0 {
		return true
	}
	cost := code.EstimateTokensForText(content)
	if cost > b.remaining && b.anyTaken {
		return false
	}
	b.remaining -= cost
	if b.remaining < 0 {
		b.remaining = 0
	}
	b.anyTaken = true
	return true
}

// scoreAndSort scores a slice of memories and returns them sorted by score
// (highest first). When scoring is disabled, memories retain their original
// ULID chronological order. Uses sort.SliceStable so that ULID order is the
// tiebreaker when scores are equal.
func scoreAndSort(mems []*memory.Memory, now time.Time, cfg memory.ScoringConfig) []memory.ScoredMemory {
	scored := make([]memory.ScoredMemory, len(mems))
	for i, m := range mems {
		scored[i] = memory.ScoredMemory{
			Memory: m,
			Score:  memory.ScoreMemory(m, now, cfg),
		}
	}
	if cfg.ScoringEnabled {
		sort.SliceStable(scored, func(i, j int) bool {
			return scored[i].Score > scored[j].Score
		})
	}
	return scored
}

// validateProjectRoot checks that the current working directory is inside
// the resolved project root. Returns true if valid. When invalid (e.g.,
// ~/.aide/ was incorrectly resolved as project root), it logs a warning
// and returns false — the caller should skip memory injection but still
// allow the session to start.
func validateProjectRoot(dbPath string) bool {
	root := projectRoot(dbPath)
	cwd, err := os.Getwd()
	if err != nil {
		return true // can't validate, assume OK
	}
	// Resolve symlinks for accurate comparison
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return true
	}
	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return true
	}
	// cwd must be inside (or equal to) the project root
	if !strings.HasPrefix(absCwd, absRoot) {
		fmt.Fprintf(os.Stderr, "warning: cwd %q is not inside project root %q — skipping memory injection\n", absCwd, absRoot)
		return false
	}
	return true
}

// fetchRecentSessions returns the most recent session groups for a project.
// For each session, it prefers session-summary memories over raw partials.
// If a session-summary exists, only that is included. Otherwise, non-partial
// memories are included (decisions, learnings, etc.) as a fallback.
func fetchRecentSessions(backend *Backend, project string, limit int) []*SessionGroup {
	allMems, err := backend.ListMemories("", 1000, nil)
	if err != nil {
		return nil
	}

	projectTag := "project:" + project

	// Track per-session: summaries, non-partial memories, and latest timestamp
	type sessionData struct {
		summaries  []*memory.Memory
		nonPartial []*memory.Memory
		lastAt     string
	}
	sessionMap := make(map[string]*sessionData)

	for _, m := range allMems {
		hasProject := false
		var sessionID string

		for _, tag := range m.Tags {
			if tag == projectTag {
				hasProject = true
			}
			if strings.HasPrefix(tag, "session:") {
				sessionID = strings.TrimPrefix(tag, "session:")
			}
		}

		if !hasProject || sessionID == "" {
			continue
		}

		data, ok := sessionMap[sessionID]
		if !ok {
			data = &sessionData{}
			sessionMap[sessionID] = data
		}

		ts := m.CreatedAt.Format(time.RFC3339)
		if data.lastAt == "" || ts > data.lastAt {
			data.lastAt = ts
		}

		isSummary := hasAnyTag(m.Tags, []string{"session-summary"})
		isPartial := hasAnyTag(m.Tags, []string{"partial"})

		if isSummary && !isPartial {
			// Non-partial session summary — highest priority
			data.summaries = append(data.summaries, m)
		} else if !isPartial {
			// Non-partial, non-summary memory (decisions, learnings, etc.)
			data.nonPartial = append(data.nonPartial, m)
		}
		// Skip raw partials entirely — they are granular tool-event records
	}

	// Build SessionGroups, preferring summaries over raw memories
	sessions := make([]*SessionGroup, 0, len(sessionMap))
	for sid, data := range sessionMap {
		group := &SessionGroup{
			SessionID: sid,
			LastAt:    data.lastAt,
		}
		switch {
		case len(data.summaries) > 0:
			// Use the most recent summary only
			latest := data.summaries[0]
			for _, s := range data.summaries[1:] {
				if s.CreatedAt.After(latest.CreatedAt) {
					latest = s
				}
			}
			group.Memories = []*memory.Memory{latest}
		case len(data.nonPartial) > 0:
			group.Memories = data.nonPartial
		default:
			// Session has only partials — skip it
			continue
		}
		sessions = append(sessions, group)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastAt > sessions[j].LastAt
	})

	if len(sessions) > limit {
		sessions = sessions[:limit]
	}
	return sessions
}

// sessionInitDefaults ensures nil slices become empty arrays in JSON output.
func sessionInitDefaults(result *SessionInitResult) {
	if result.GlobalMemories == nil {
		result.GlobalMemories = []SessionMemory{}
	}
	if result.ProjectMemories == nil {
		result.ProjectMemories = []SessionMemory{}
	}
	if result.Decisions == nil {
		result.Decisions = []SessionDecision{}
	}
	if result.RecentSessions == nil {
		result.RecentSessions = []*SessionGroup{}
	}
}

func memoryToSession(m *memory.Memory, score float64) SessionMemory {
	tags := m.Tags
	if tags == nil {
		tags = []string{}
	}
	return SessionMemory{
		ID:        m.ID,
		Category:  string(m.Category),
		Content:   m.Content,
		Tags:      tags,
		CreatedAt: m.CreatedAt.Format(time.RFC3339),
		Score:     score,
	}
}
