package store

import (
	"strconv"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
)

// addTokenWork reads the original event before token projection loses errors
// and timings. Host observations never add a second service-call count.
func addTokenWork(w *memory.TokenWork, e *observe.Event, sessionID string, since, until time.Time) {
	if e.Kind != observe.KindToolCall || e.Attrs["accounting_version"] != "1" || e.Attrs["observation_stage"] != "server_result" {
		return
	}
	if (!since.IsZero() && e.Timestamp.Before(since)) || (!until.IsZero() && e.Timestamp.After(until)) || (sessionID != "" && e.SessionID != sessionID) {
		return
	}
	group := w.ByTool[e.Name]
	if group == nil {
		group = &memory.TokenWorkCounters{}
		w.ByTool[e.Name] = group
	}
	outcome := "unknown"
	if e.Attrs["work_version"] == "1" {
		outcome = e.Attrs["work_outcome"]
	}
	// Explicit errors remain evidence even on observations predating the work
	// contract, and override contradictory returned/unknown attributes.
	if e.Error != "" {
		outcome = "reported_error"
	}
	elapsed, err := strconv.ParseInt(e.Attrs["work_elapsed_ms"], 10, 64)
	hasDuration := e.Attrs["work_version"] == "1" && err == nil && elapsed >= 0 && elapsed <= 1<<53-1
	bytes, hasBytes := memory.MeasuredBytes(e.Attrs, "payload_bytes")
	for _, c := range []*memory.TokenWorkCounters{&w.TokenWorkCounters, group} {
		c.Calls++
		switch outcome {
		case "returned":
			c.Returned++
		case "reported_error":
			c.ReportedErrors++
		default:
			c.UnknownOutcomes++
		}
		if hasDuration {
			c.ElapsedMs += elapsed
			c.MeasuredDurations++
		} else {
			c.MissingDurations++
		}
		if e.SessionID == "" {
			c.UnassignedSessions++
		}
		if hasBytes {
			c.ReturnedText.Bytes += bytes
			c.ReturnedText.EstimatedTokens += memory.EstimateTextTokens(bytes)
			c.ReturnedText.Events++
		} else {
			c.MissingPayload++
		}
	}
}
