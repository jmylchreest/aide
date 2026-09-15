package memory

// TokenWorkCounters accounts for recorded aide MCP service calls. Returned
// means no reported tool error, not task success or output quality. ElapsedMs
// sums measured wall time: calls may overlap and this is not CPU consumption.
// ReturnedText is the same server boundary present in ByStage, not extra text.
type TokenWorkCounters struct {
	Calls              int           `json:"calls"`
	Returned           int           `json:"returned"`
	ReportedErrors     int           `json:"reported_errors"`
	UnknownOutcomes    int           `json:"unknown_outcomes"`
	ElapsedMs          int64         `json:"elapsed_ms"`
	MeasuredDurations  int           `json:"measured_durations"`
	MissingDurations   int           `json:"missing_durations"`
	UnassignedSessions int           `json:"unassigned_sessions"`
	ReturnedText       TokenQuantity `json:"returned_text"`
	MissingPayload     int           `json:"missing_payload"`
}

// TokenWork does not infer avoided model work or include unobserved background
// activity. Nil means unavailable (for example, an older daemon); an empty
// versioned value means no eligible observations in the requested selection.
type TokenWork struct {
	Version int `json:"version"`
	TokenWorkCounters
	ByTool map[string]*TokenWorkCounters `json:"by_tool"`
}

func NewTokenWork() *TokenWork {
	return &TokenWork{Version: 1, ByTool: make(map[string]*TokenWorkCounters)}
}
