package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// SessionInitResult is the JSON output of `aide session init`.
type SessionInitResult struct {
	// State cleanup results
	StateKeysDeleted   int `json:"state_keys_deleted"`
	StaleAgentsCleaned int `json:"stale_agents_cleaned"`

	// Memory injection data
	GlobalMemories  []SessionMemory   `json:"global_memories"`
	ProjectMemories []SessionMemory   `json:"project_memories"`
	Decisions       []SessionDecision `json:"decisions"`
	RecentSessions  []*SessionGroup   `json:"recent_sessions"`
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
	if len(args) < 1 {
		printSessionUsage()
		return nil
	}

	subcmd := args[0]

	if subcmd == "help" || subcmd == "-h" || subcmd == "--help" {
		printSessionUsage()
		return nil
	}

	switch subcmd {
	case "init":
		return sessionInit(dbPath, args[1:])
	default:
		return fmt.Errorf("unknown session subcommand: %s", subcmd)
	}
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
    --format=json        Output as JSON (default)

Examples:
  aide session init --project=myapp
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
	cleanupAge := 30 * time.Minute
	sessionLimit := 3

	if dur := parseFlag(args, "--cleanup-age="); dur != "" {
		if d, err := time.ParseDuration(dur); err == nil {
			cleanupAge = d
		}
	}
	if l := parseFlag(args, "--session-limit="); l != "" {
		fmt.Sscanf(l, "%d", &sessionLimit)
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

	// 3. Fetch global memories (scope:global tag)
	globalMems, err := backend.ListMemories("global", 100)
	if err == nil {
		for _, m := range globalMems {
			if hasAllTags(m.Tags, []string{"scope:global"}) {
				result.GlobalMemories = append(result.GlobalMemories, memoryToSession(m))
			}
		}
	}

	// 4. Fetch project memories
	if project != "" {
		projectMems, err := backend.ListMemories("", 1000)
		if err == nil {
			projectTag := "project:" + project
			for _, m := range projectMems {
				if hasAllTags(m.Tags, []string{projectTag}) {
					result.ProjectMemories = append(result.ProjectMemories, memoryToSession(m))
				}
			}
		}
	}

	// 5. Fetch decisions (latest per topic)
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

	// 6. Fetch recent sessions (grouped by session tag)
	if project != "" && sessionLimit > 0 {
		allMems, err := backend.ListMemories("", 1000)
		if err == nil {
			projectTag := "project:" + project
			sessionMap := make(map[string]*SessionGroup)

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

				group, ok := sessionMap[sessionID]
				if !ok {
					group = &SessionGroup{
						SessionID: sessionID,
						Memories:  make([]*memory.Memory, 0),
					}
					sessionMap[sessionID] = group
				}
				group.Memories = append(group.Memories, m)

				ts := m.CreatedAt.Format(time.RFC3339)
				if group.LastAt == "" || ts > group.LastAt {
					group.LastAt = ts
				}
			}

			sessions := make([]*SessionGroup, 0, len(sessionMap))
			for _, group := range sessionMap {
				sessions = append(sessions, group)
			}
			sort.Slice(sessions, func(i, j int) bool {
				return sessions[i].LastAt > sessions[j].LastAt
			})

			if len(sessions) > sessionLimit {
				sessions = sessions[:sessionLimit]
			}

			result.RecentSessions = sessions
		}
	}

	// Ensure nil slices become empty arrays in JSON
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

	out, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}
	fmt.Println(string(out))
	return nil
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
