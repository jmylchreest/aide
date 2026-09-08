package memory

import "time"

type RetrievalSource struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}
type RetrievalStep struct {
	ID           string         `json:"id"`
	InvocationID string         `json:"invocation_id"`
	At           time.Time      `json:"at"`
	Tool         string         `json:"tool"`
	Status       string         `json:"status"`
	Target       string         `json:"target,omitempty"`
	Text         *TokenQuantity `json:"text,omitempty"`
}

// A retrieval episode is explicitly grouped by recorded context window. It is
// not a task boundary, a complete transcript, or proof of an avoided call.
type RetrievalWindow struct {
	Host           string             `json:"host"`
	SessionID      string             `json:"session_id"`
	ActorID        string             `json:"actor_id"`
	Epoch          string             `json:"epoch"`
	First          time.Time          `json:"first"`
	Last           time.Time          `json:"last"`
	Boundary       string             `json:"boundary"`
	Events         int                `json:"events"`
	Observed       TokenQuantity      `json:"observed"`
	Unattributed   TokenQuantity      `json:"unattributed"`
	Reference      TokenQuantity      `json:"reference"`
	Comparison     *TokenChange       `json:"comparison,omitempty"`
	FullReadEvents int                `json:"full_read_events"`
	SearchEvents   int                `json:"search_events"`
	FailedEvents   int                `json:"failed_events"`
	MissingPayload int                `json:"missing_payload"`
	Clipped        bool               `json:"clipped"`
	Issues         []string           `json:"issues"`
	Sources        []*RetrievalSource `json:"sources"`
	Steps          []*RetrievalStep   `json:"steps"`
	StepsLimited   bool               `json:"steps_limited"`
}
type TokenRetrievals struct {
	Windows          []*RetrievalWindow `json:"windows"`
	WindowsLimited   bool               `json:"windows_limited"`
	UnwindowedEvents int                `json:"unwindowed_events"`
}
