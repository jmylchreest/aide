// Package instinct extracts behavioural patterns from the observe event
// stream and proposes them as durable memories the user can accept.
//
// Each pattern is an independent Parser. Adding a new pattern means adding
// a new file in this package — no edits to existing code required.
package instinct

import (
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

type Status string

const (
	StatusOpen     Status = "open"
	StatusAccepted Status = "accepted"
	StatusRejected Status = "rejected"
	StatusExpired  Status = "expired"
)

// Proposal is a candidate instinct emitted by a Parser. It carries enough
// evidence to be inspected even after the source observe events are pruned.
type Proposal struct {
	ID                string         `json:"id"`
	Shape             string         `json:"shape"`
	SessionID         string         `json:"session_id,omitempty"`
	ProposedAt        time.Time      `json:"proposed_at"`
	Summary           string         `json:"summary"`
	Evidence          Evidence       `json:"evidence,omitzero"`
	ProposedInstinct  ProposedMemory `json:"proposed_instinct,omitzero"`
	Status            Status         `json:"status"`
	RejectionCount    int            `json:"rejection_count,omitempty"`
	RejectionReason   string         `json:"rejection_reason,omitempty"`
	LastReproposalAt  time.Time      `json:"last_reproposal_at,omitempty"`
	AcceptedMemoryID  string         `json:"accepted_memory_id,omitempty"`
	ExpiresAt         time.Time      `json:"expires_at,omitempty"`
}

// Evidence captures the observe events that triggered a proposal. The
// snapshot is embedded so the proposal stays inspectable after observe
// retention expires the originals.
type Evidence struct {
	ObserveEventIDs []string         `json:"observe_event_ids,omitempty"`
	CrossSessionIDs []string         `json:"cross_session_ids,omitempty"`
	Snapshot        []*observe.Event `json:"snapshot,omitempty"`
}

// ProposedMemory is the shape an accepted proposal becomes when promoted
// into the memory store. Mirrors the writable fields of memory.Memory.
type ProposedMemory struct {
	Category string   `json:"category"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags,omitempty"`
	Priority float32  `json:"priority,omitempty"`
}

// ParserContext provides cross-cutting state a Parser may need beyond the
// per-session event slice. Detectors that operate only within a session
// can ignore it.
type ParserContext struct {
	Now              time.Time
	SessionID        string
	CrossSessionPast []*observe.Event
	// Classifications, when populated, give parsers LLM-grade intent labels
	// keyed by source observe-event ID. Parsers that can use these prefer
	// them over their marker/heuristic fallbacks.
	Classifications map[string]Classification
}

// Classification is the LLM's judgement about a single observe event,
// produced by the skill body and threaded into ParserContext via the
// `aide reflect run --classifications-json` flow.
type Classification struct {
	Intent     string  `json:"intent"`               // corrective | positive | neutral | request | other
	Confidence float32 `json:"confidence,omitempty"` // 0.0-1.0; 0 = unknown
	Reason     string  `json:"reason,omitempty"`
}

// Capabilities describes what a parser needs to function.
type Capabilities struct {
	// RequiresLLM, when true, means the parser only produces useful output
	// with LLM-provided classifications in ParserContext. Runner.Run in
	// deterministic mode skips parsers that require an LLM.
	RequiresLLM bool
}

// Parser is the contract every pattern detector implements. Detect runs over
// one session's events plus an optional cross-session window and returns
// zero or more candidate proposals. Pure function — no I/O.
type Parser interface {
	Name() string
	DefaultConfig() any
	Capabilities() Capabilities
	Detect(events []*observe.Event, cfg any, ctx ParserContext) []Proposal
}

// RunMode controls which parsers Runner.Run invokes.
type RunMode int

const (
	// RunDeterministic skips parsers that declare RequiresLLM. Used by the
	// Stop hook where no LLM is in the loop.
	RunDeterministic RunMode = iota
	// RunWithLLM invokes every enabled parser, threading any provided
	// Classifications through ParserContext. Used by the reflect skill
	// after the agent has done its judgement step.
	RunWithLLM
)
