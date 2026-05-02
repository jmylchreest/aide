package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// SessionInitResult is the JSON output of `aide session init`.
type SessionInitResult struct {
	// State cleanup results
	StateKeysDeleted   int `json:"state_keys_deleted"`
	StaleAgentsCleaned int `json:"stale_agents_cleaned"`

	// Share auto-import results
	SharedImported int `json:"shared_imported,omitempty"`

	// Memory injection data
	GlobalMemories        []SessionMemory   `json:"global_memories"`
	ProjectMemories       []SessionMemory   `json:"project_memories"`
	ProjectMemoryOverflow bool              `json:"project_memory_overflow,omitempty"`
	Decisions             []SessionDecision `json:"decisions"`
	RecentSessions        []*SessionGroup   `json:"recent_sessions"`
}

// SessionMemory is a memory entry for JSON output.
type SessionMemory struct {
	ID        string   `json:"id"`
	Category  string   `json:"category"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	CreatedAt string   `json:"created_at"`
}

// SessionDecision is a decision entry for JSON output.
type SessionDecision struct {
	Topic     string `json:"topic"`
	Value     string `json:"value"`
	Rationale string `json:"rationale,omitempty"`
	CreatedAt string `json:"created_at"`
}

func cmdSession(dbPath string, args []string) error {
	return dispatchSubcmd("session", args, printSessionUsage, []subcmd{
		{name: "init", handler: func(a []string) error { return sessionInit(dbPath, a) }},
	})
}

func printSessionUsage() {
	fmt.Println(`aide session - Session lifecycle commands

Usage:
  aide session <subcommand> [arguments]

Subcommands:
  init       Initialize a new session (reset state, cleanup, fetch context)

Options:
  init:
    --project=NAME       Project name for memory filtering
    --cleanup-age=DUR    Max age for stale agent entries (default: 30m)
    --session-limit=N    Number of recent sessions to include (default: 3)
    --share-import       Import shared data from .aide/shared/ on session start
    --format=json        Output as JSON (default)

Environment:
  AIDE_SHARE_AUTO_IMPORT=1   Enable auto-import of shared data on session init

Examples:
  aide session init --project=myapp
  aide session init --project=myapp --share-import
  aide session init --project=myapp --cleanup-age=1h --session-limit=5`)
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
	sessionLimit := DefaultSessionLimit

	if dur := parseFlag(args, "--cleanup-age="); dur != "" {
		if d, err := time.ParseDuration(dur); err == nil {
			cleanupAge = d
		}
	}
	if l := parseFlag(args, "--session-limit="); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil {
			return fmt.Errorf("invalid --session-limit= value %q: %w", l, err)
		}
		sessionLimit = n
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		return err
	}
	defer backend.Close()

	result := SessionInitResult{}

	// 1. Delete global state keys
	globalKeys := []string{
		"mode",
		"startedAt",
		"lastToolUse",
		"toolCalls",
		"agentCount",
		"lastTool",
	}
	for _, key := range globalKeys {
		if err := backend.DeleteState(key); err == nil {
			result.StateKeysDeleted++
		}
	}

	// 2. Cleanup stale agent state
	cleaned, err := backend.CleanupState(cleanupAge)
	if err == nil {
		result.StaleAgentsCleaned = cleaned
	}

	// 3. Auto-import shared data if enabled
	shareImport := hasFlag(args, "--share-import") || config.Get().Share.AutoImport
	if shareImport {
		projectRoot := projectRoot(dbPath)
		sharedDir := filepath.Join(projectRoot, ".aide", "shared")
		if _, statErr := os.Stat(sharedDir); statErr == nil {
			imported, _, _ := shareImportDecisions(backend, sharedDir, false)
			memImported, _, _ := shareImportMemories(backend, sharedDir, false)
			result.SharedImported = imported + memImported
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

// sessionFetchContext gathers memories, decisions, and recent sessions.
// When memory scoring is enabled (default), memories are sorted by computed
// priority score (highest first). When scoring is disabled via
// AIDE_MEMORY_SCORING_DISABLED=1, chronological ULID order is preserved.
func sessionFetchContext(backend *Backend, project string, sessionLimit int, result *SessionInitResult) {
	now := time.Now()
	cfg := memoryScoringConfig()

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
			result.GlobalMemories = append(result.GlobalMemories, memoryToSession(sm.Memory))
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
				result.ProjectMemories = append(result.ProjectMemories, memoryToSession(sm.Memory))
			}
		}
	}

	// Decisions (latest per topic)
	decisions, err := backend.ListDecisions()
	if err == nil {
		latest := make(map[string]*memory.Decision)
		for _, d := range decisions {
			if existing, ok := latest[d.Topic]; !ok || d.CreatedAt.After(existing.CreatedAt) {
				latest[d.Topic] = d
			}
		}
		for _, d := range latest {
			result.Decisions = append(result.Decisions, SessionDecision{
				Topic:     d.Topic,
				Value:     d.Decision,
				Rationale: d.Rationale,
				CreatedAt: d.CreatedAt.Format(time.RFC3339),
			})
		}
	}

	// Recent sessions (grouped by session tag)
	if project != "" && sessionLimit > 0 {
		result.RecentSessions = fetchRecentSessions(backend, project, sessionLimit)
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

func memoryToSession(m *memory.Memory) SessionMemory {
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
	}
}
