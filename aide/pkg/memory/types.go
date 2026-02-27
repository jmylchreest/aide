// Package memory provides the core data types for aide.
package memory

import "time"

// Category represents the type of memory entry.
type Category string

const (
	CategoryLearning  Category = "learning"  // Technical discoveries
	CategoryDecision  Category = "decision"  // Choices made with rationale
	CategoryIssue     Category = "issue"     // Known problems/workarounds
	CategoryDiscovery Category = "discovery" // Swarm findings (shared)
	CategoryBlocker   Category = "blocker"   // Things that stopped progress
)

// Memory represents a single memory entry.
// Memories are short, transient user preferences or instructions.
type Memory struct {
	ID           string    `json:"id"`
	Category     Category  `json:"category"`
	Content      string    `json:"content"`
	Tags         []string  `json:"tags,omitempty"`
	Priority     float32   `json:"priority"`            // 0.0-1.0, decays over time
	Plan         string    `json:"plan,omitempty"`      // Plan context
	Agent        string    `json:"agent,omitempty"`     // Agent that created it
	Namespace    string    `json:"namespace,omitempty"` // Swarm scope (empty = global)
	AccessCount  uint32    `json:"accessCount"`         // Number of times this memory was retrieved
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
	LastAccessed time.Time `json:"lastAccessed,omitempty"` // Last time this memory was read/searched
}

// TaskStatus represents the state of a swarm task.
type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusClaimed TaskStatus = "claimed"
	TaskStatusDone    TaskStatus = "done"
	TaskStatusBlocked TaskStatus = "blocked"
)

// Task represents a swarm task that can be claimed by agents.
type Task struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Status      TaskStatus `json:"status"`
	ClaimedBy   string     `json:"claimedBy,omitempty"`
	ClaimedAt   time.Time  `json:"claimedAt,omitempty"`
	CompletedAt time.Time  `json:"completedAt,omitempty"`
	Worktree    string     `json:"worktree,omitempty"`
	Result      string     `json:"result,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// Message represents inter-agent communication.
type Message struct {
	ID        uint64    `json:"id"`
	From      string    `json:"from"`
	To        string    `json:"to,omitempty"` // Empty = broadcast
	Content   string    `json:"content"`
	Type      string    `json:"type,omitempty"` // Optional: info, warning, error, etc.
	ReadBy    []string  `json:"readBy,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"` // TTL - auto-prune after this time
}

// Decision represents a shared architectural decision with full context.
// Decisions are append-only (latest wins) and can contain rich details.
type Decision struct {
	Topic      string    `json:"topic"`                // Unique key (e.g., "auth-strategy", "db-schema")
	Decision   string    `json:"decision"`             // Short summary of the decision
	Rationale  string    `json:"rationale,omitempty"`  // Why this decision was made
	Details    string    `json:"details,omitempty"`    // Full content: schemas, code examples, specs
	References []string  `json:"references,omitempty"` // External links, docs, related files
	DecidedBy  string    `json:"decidedBy,omitempty"`  // Who made the decision (agent/user)
	CreatedAt  time.Time `json:"createdAt"`
}

// State represents session/agent state (mode, model, etc.)
type State struct {
	Key       string    `json:"key"`             // Unique key (e.g., "mode", "modelTier", "agent:abc:mode")
	Value     string    `json:"value"`           // State value
	Agent     string    `json:"agent,omitempty"` // Agent ID if agent-specific
	UpdatedAt time.Time `json:"updatedAt"`
}

// DefaultExcludeTags are tags excluded from all memory queries by default.
// Use --all (CLI) or IncludeAll (programmatic) to bypass this filter.
var DefaultExcludeTags = []string{"forget"}

// SearchOptions configures memory search behavior.
type SearchOptions struct {
	Category    Category
	Plan        string
	Tags        []string
	ExcludeTags []string // Exclude memories with any of these tags (default: DefaultExcludeTags)
	Namespace   string   // Filter by namespace (swarm scope)
	Limit       int
	Semantic    bool // Use vector search
	IncludeAll  bool // Bypass ExcludeTags filtering (show everything)
}

// ApplyDefaults sets default ExcludeTags if none are specified and IncludeAll is false.
func (o *SearchOptions) ApplyDefaults() {
	if !o.IncludeAll && o.ExcludeTags == nil {
		o.ExcludeTags = DefaultExcludeTags
	}
	if o.IncludeAll {
		o.ExcludeTags = nil
	}
}

// FilterMemories applies ExcludeTags filtering to a slice of memories.
// This is useful for post-search filtering where the store method doesn't
// natively support ExcludeTags (e.g., SearchMemories).
func FilterMemories(memories []*Memory, excludeTags []string) []*Memory {
	if len(excludeTags) == 0 {
		return memories
	}
	excludeSet := make(map[string]bool, len(excludeTags))
	for _, t := range excludeTags {
		excludeSet[t] = true
	}
	filtered := make([]*Memory, 0, len(memories))
	for _, m := range memories {
		excluded := false
		for _, tag := range m.Tags {
			if excludeSet[tag] {
				excluded = true
				break
			}
		}
		if !excluded {
			filtered = append(filtered, m)
		}
	}
	return filtered
}
