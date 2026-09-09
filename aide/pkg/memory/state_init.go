package memory

// StateInitializer optionally supports atomic create-if-absent state. It returns
// the persisted state and whether it was created. Existing values and timestamps
// must remain unchanged. Callers must not emulate this with Get followed by Set.
type StateInitializer interface {
	InitState(*State) (*State, bool, error)
}
