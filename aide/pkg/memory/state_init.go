package memory

import "errors"

// StateInitializer optionally supports atomic create-if-absent state. It returns
// the persisted state and whether it was created. Existing values and timestamps
// must remain unchanged. Callers must not emulate this with Get followed by Set.
type StateInitializer interface {
	InitState(*State) (*State, bool, error)
}

// StateIncrementer atomically increments a nonnegative decimal counter. Missing
// state starts at one; malformed or overflowing counters must remain unchanged.
// The key is canonical, including its agent prefix when agent is nonempty.
type StateIncrementer interface {
	IncrementState(key, agent string) (*State, error)
}

// MaxStateAgentEntries bounds the caller-selected capacity for atomic state
// initialization. It is not a limit on existing InitState or SetState calls.
const MaxStateAgentEntries = 4096

// ErrStateAgentLimit means a new key would exceed the selected agent capacity.
var ErrStateAgentLimit = errors.New("agent state entry limit reached")

// BoundedStateInitializer optionally supports atomic create-if-absent state
// with a per-agent entry limit. The state must have a nonempty key and agent,
// and maxAgentEntries must be between 1 and MaxStateAgentEntries. Existing
// same-agent keys are returned unchanged, even at capacity. New insertions must
// count and enforce the exact agent namespace within the same transaction;
// callers must not emulate this with ListState followed by InitState.
type BoundedStateInitializer interface {
	InitStateBounded(*State, int) (*State, bool, error)
}
