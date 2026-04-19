package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// TokenEventItem is the JSON representation of a token event.
type TokenEventItem struct {
	ID          string `json:"id"`
	SessionID   string `json:"session_id"`
	Timestamp   string `json:"timestamp"`
	EventType   string `json:"event_type"`
	Tool        string `json:"tool"`
	FilePath    string `json:"file_path"`
	Tokens      int    `json:"tokens"`
	TokensSaved int    `json:"tokens_saved"`
}

// TokenStatsItem is the JSON representation of token stats.
type TokenStatsItem struct {
	TotalRead      int            `json:"total_read"`
	TotalSaved     int            `json:"total_saved"`
	TotalWritten   int            `json:"total_written"`
	TotalDelivered int            `json:"total_delivered"`
	EventCount     int            `json:"event_count"`
	ByTool         map[string]int `json:"by_tool"`
	BySavingType   map[string]int `json:"by_saving_type"`
	ByDelivery     map[string]int `json:"by_delivery"`
	Sessions       int            `json:"sessions"`
	ReadCount      int            `json:"read_count"`
	CodeToolCount  int            `json:"code_tool_count"`
}

// ListTokenEventsOutput is the response body for APIListTokenEvents.
type ListTokenEventsOutput struct {
	Body struct {
		Events []TokenEventItem `json:"events"`
	}
}

// GetTokenStatsOutput is the response body for APIGetTokenStats.
type GetTokenStatsOutput struct {
	Body TokenStatsItem
}

// APIListTokenEvents returns token events for an instance.
func (h *Handler) APIListTokenEvents(ctx context.Context, input *struct {
	Project   string `path:"project"`
	SessionID string `query:"session"`
	Limit     int    `query:"limit" minimum:"1" maximum:"500" default:"100"`
	Since     string `query:"since" doc:"RFC3339 lower bound (inclusive)"`
	Until     string `query:"until" doc:"RFC3339 upper bound (inclusive)"`
}) (*ListTokenEventsOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	st := inst.Store()
	if st == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	since, until, err := parseTimeRange(input.Since, input.Until)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	events, err := st.ListTokenEvents(input.SessionID, input.Limit, since, until)
	if err != nil {
		return nil, err
	}

	out := &ListTokenEventsOutput{}
	for _, e := range events {
		out.Body.Events = append(out.Body.Events, TokenEventItem{
			ID:          e.ID,
			SessionID:   e.SessionID,
			Timestamp:   e.Timestamp.UTC().Format(time.RFC3339),
			EventType:   e.EventType,
			Tool:        e.Tool,
			FilePath:    e.FilePath,
			Tokens:      e.Tokens,
			TokensSaved: e.TokensSaved,
		})
	}
	return out, nil
}

// APIGetTokenStats returns aggregated token statistics for an instance.
func (h *Handler) APIGetTokenStats(ctx context.Context, input *struct {
	Project   string `path:"project"`
	SessionID string `query:"session"`
	Since     string `query:"since" doc:"RFC3339 lower bound (inclusive)"`
	Until     string `query:"until" doc:"RFC3339 upper bound (inclusive)"`
}) (*GetTokenStatsOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	st := inst.Store()
	if st == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	since, until, err := parseTimeRange(input.Since, input.Until)
	if err != nil {
		return nil, huma.Error400BadRequest(err.Error())
	}

	stats, err := st.TokenStats(input.SessionID, since, until)
	if err != nil {
		return nil, err
	}

	out := &GetTokenStatsOutput{}
	out.Body = TokenStatsItem{
		TotalRead:      stats.TotalRead,
		TotalSaved:     stats.TotalSaved,
		TotalWritten:   stats.TotalWritten,
		TotalDelivered: stats.TotalDelivered,
		EventCount:     stats.EventCount,
		ByTool:         stats.ByTool,
		BySavingType:   stats.BySavingType,
		ByDelivery:     stats.ByDelivery,
		Sessions:       stats.Sessions,
		ReadCount:      stats.ReadCount,
		CodeToolCount:  stats.CodeToolCount,
	}
	return out, nil
}

// parseTimeRange parses optional RFC3339 since/until strings into time.Time values.
// Empty strings result in zero-value times (no bound).
func parseTimeRange(sinceStr, untilStr string) (since, until time.Time, err error) {
	if sinceStr != "" {
		since, err = time.Parse(time.RFC3339Nano, sinceStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid since: %w", err)
		}
	}
	if untilStr != "" {
		until, err = time.Parse(time.RFC3339Nano, untilStr)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid until: %w", err)
		}
	}
	if !since.IsZero() && !until.IsZero() && since.After(until) {
		return time.Time{}, time.Time{}, fmt.Errorf("since must not be after until")
	}
	return since, until, nil
}
