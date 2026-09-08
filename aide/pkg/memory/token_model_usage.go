package memory

// ModelUsageCounter preserves reported zero separately from absent counters.
// These are host-reported usage counters, never text estimates or savings.
type ModelUsageCounter struct {
	Tokens       int64 `json:"tokens"`
	Observations int   `json:"observations"`
}
type ModelUsageSource struct {
	Host          string                        `json:"host"`
	Source        string                        `json:"source"`
	Model         string                        `json:"model,omitempty"`
	Provider      string                        `json:"provider,omitempty"`
	Observations  int                           `json:"observations"`
	SourceTimed   int                           `json:"source_timed"`
	ObservedTimed int                           `json:"observed_timed"`
	Counters      map[string]*ModelUsageCounter `json:"counters"`
}

// TokenModelUsage includes captured records only: session coverage is partial.
// Conflicting and invalid identities never contribute counters. Sources must
// remain separate; recorded usage does not establish cost, savings or quality.
type TokenModelUsage struct {
	Version      int                 `json:"version"`
	Observations int                 `json:"observations"`
	Conflicts    int                 `json:"conflicts"`
	Invalid      int                 `json:"invalid"`
	BySource     []*ModelUsageSource `json:"by_source"`
}
