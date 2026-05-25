package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
)

// SwarmAgentItem represents one registered agent surfaced via state.
type SwarmAgentItem struct {
	AgentID       string `json:"agent"`
	ParentSession string `json:"parent_session,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	Status        string `json:"status,omitempty"`
	Type          string `json:"type,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
	EndedAt       string `json:"ended_at,omitempty"`
	Halt          bool   `json:"halt,omitempty"`
	HaltReason    string `json:"halt_reason,omitempty"`
	Paused        bool   `json:"paused,omitempty"`
	Deadline      string `json:"deadline,omitempty"`
}

// staleAgentAge: a completed agent older than this is hidden by default.
// Read-time filter only — data stays in the state bucket for forensics.
// Override with ?include_stale=1 to see everything.
const staleAgentAge = 24 * time.Hour

// ListSwarmAgentsOutput is the response body for APIListSwarmAgents.
type ListSwarmAgentsOutput struct {
	Body struct {
		Agents []SwarmAgentItem `json:"agents"`
	}
}

// SwarmTaskUpdate is a single Task event from WatchTasks.
type SwarmTaskUpdate struct {
	ID              string `json:"id"`
	Title           string `json:"title"`
	Status          string `json:"status"`
	ClaimedBy       string `json:"claimed_by,omitempty"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	Worktree        string `json:"worktree,omitempty"`
	Result          string `json:"result,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
	ClaimedAt       string `json:"claimed_at,omitempty"`
	CompletedAt     string `json:"completed_at,omitempty"`
}

// SwarmMessageUpdate is a single Message event from WatchMessages.
type SwarmMessageUpdate struct {
	ID              uint64 `json:"id"`
	From            string `json:"from"`
	To              string `json:"to,omitempty"`
	Content         string `json:"content"`
	Type            string `json:"type,omitempty"`
	Priority        string `json:"priority,omitempty"`
	ParentSessionID string `json:"parent_session_id,omitempty"`
	CreatedAt       string `json:"created_at,omitempty"`
}

// SwarmStateUpdate is a single StateChange event from WatchState.
type SwarmStateUpdate struct {
	Key       string `json:"key"`
	Value     string `json:"value,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Change    string `json:"change"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

// APIListSwarmAgents walks the State bucket and groups per-agent fields
// into one row per agent, optionally filtered by parent_session. By default
// hides agents marked status=completed with endedAt older than staleAgentAge
// — set include_stale=1 to see all.
func (h *Handler) APIListSwarmAgents(ctx context.Context, input *struct {
	Project       string `path:"project"`
	ParentSession string `query:"parent_session" doc:"Filter to one swarm"`
	IncludeStale  bool   `query:"include_stale" doc:"Include completed agents whose endedAt is > 24h ago"`
}) (*ListSwarmAgentsOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	s := inst.Store()
	if s == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	states, err := s.ListState("")
	if err != nil {
		return nil, err
	}
	by := map[string]*SwarmAgentItem{}
	for _, st := range states {
		if st.Agent == "" {
			continue
		}
		// State.Key looks like "agent:<id>:<field>" — split out the field.
		parts := splitAgentKey(st.Key)
		if parts == nil {
			continue
		}
		field := parts[2]
		row, ok := by[st.Agent]
		if !ok {
			row = &SwarmAgentItem{AgentID: st.Agent}
			by[st.Agent] = row
		}
		switch field {
		case "parent_session":
			row.ParentSession = st.Value
		case "namespace":
			row.Namespace = st.Value
		case "status":
			row.Status = st.Value
		case "type":
			row.Type = st.Value
		case "startedAt":
			row.StartedAt = st.Value
		case "endedAt":
			row.EndedAt = st.Value
		case "halt":
			row.Halt = isTruthyValue(st.Value)
		case "halt_reason":
			row.HaltReason = st.Value
		case "paused":
			row.Paused = isTruthyValue(st.Value)
		case "deadline":
			row.Deadline = st.Value
		}
	}
	out := &ListSwarmAgentsOutput{}
	staleCutoff := time.Now().Add(-staleAgentAge)
	for _, r := range by {
		if input.ParentSession != "" && r.ParentSession != input.ParentSession {
			continue
		}
		if !input.IncludeStale && isStaleAgent(r, staleCutoff) {
			continue
		}
		out.Body.Agents = append(out.Body.Agents, *r)
	}
	// Chronological order, newest first by startedAt — most active agents
	// surface at the top. Missing/unparseable startedAt sinks to the bottom
	// (those are stale/malformed records anyway). AgentID is the tiebreaker.
	sort.Slice(out.Body.Agents, func(i, j int) bool {
		ti := parseAgentTime(out.Body.Agents[i].StartedAt)
		tj := parseAgentTime(out.Body.Agents[j].StartedAt)
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return out.Body.Agents[i].AgentID < out.Body.Agents[j].AgentID
	})
	return out, nil
}

func splitAgentKey(k string) []string {
	// "agent:<id>:<field>"
	parts := make([]string, 0, 3)
	start := 0
	for i := 0; i < len(k) && len(parts) < 2; i++ {
		if k[i] == ':' {
			parts = append(parts, k[start:i])
			start = i + 1
		}
	}
	if len(parts) != 2 || parts[0] != "agent" {
		return nil
	}
	parts = append(parts, k[start:])
	return parts
}

func isTruthyValue(v string) bool {
	switch v {
	case "1", "true", "on", "yes", "True", "TRUE":
		return true
	}
	return false
}

// parseAgentTime tolerates empty / malformed timestamps by returning the
// zero time (which sorts last under After()).
func parseAgentTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// isStaleAgent: completed agents whose endedAt is past the cutoff. If the
// agent has no endedAt but its startedAt is old AND it's not running, also
// treat as stale (catches sessions that crashed without firing SubagentStop).
func isStaleAgent(a *SwarmAgentItem, cutoff time.Time) bool {
	if a.Status == "completed" {
		if a.EndedAt != "" {
			if t, err := time.Parse(time.RFC3339, a.EndedAt); err == nil {
				return t.Before(cutoff)
			}
		}
		if a.StartedAt != "" {
			if t, err := time.Parse(time.RFC3339, a.StartedAt); err == nil {
				return t.Before(cutoff)
			}
		}
		return true
	}
	if a.Status != "running" && a.StartedAt != "" {
		if t, err := time.Parse(time.RFC3339, a.StartedAt); err == nil {
			return t.Before(cutoff)
		}
	}
	return false
}

// APIWatchSwarmTasks streams task updates filtered by parent_session.
func (h *Handler) APIWatchSwarmTasks(w http.ResponseWriter, r *http.Request) {
	project, _ := url.PathUnescape(chi.URLParam(r, "project"))
	inst := h.findInstance(project)
	if inst == nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	client := inst.Client()
	if client == nil {
		http.Error(w, "instance not connected", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	stream, err := client.Swarm.WatchTasks(r.Context(), &grpcapi.SwarmWatchTasksRequest{
		ParentSessionId: q.Get("parent_session"),
		Status:          q.Get("status"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = StreamSSE(w, r, func() (*SwarmTaskUpdate, error) {
		t, err := stream.Recv()
		if err != nil {
			return nil, err
		}
		return &SwarmTaskUpdate{
			ID:              t.Id,
			Title:           t.Title,
			Status:          t.Status,
			ClaimedBy:       t.ClaimedBy,
			ParentSessionID: t.ParentSessionId,
			Worktree:        t.Worktree,
			Result:          t.Result,
			CreatedAt:       formatRFC(t.CreatedAt.AsTime()),
			ClaimedAt:       formatRFC(t.ClaimedAt.AsTime()),
			CompletedAt:     formatRFC(t.CompletedAt.AsTime()),
		}, nil
	}, func(t *SwarmTaskUpdate) string {
		return t.ID
	})
}

// APIWatchSwarmMessages streams messages filtered by parent_session / agent.
func (h *Handler) APIWatchSwarmMessages(w http.ResponseWriter, r *http.Request) {
	project, _ := url.PathUnescape(chi.URLParam(r, "project"))
	inst := h.findInstance(project)
	if inst == nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	client := inst.Client()
	if client == nil {
		http.Error(w, "instance not connected", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	stream, err := client.Swarm.WatchMessages(r.Context(), &grpcapi.SwarmWatchMessagesRequest{
		ParentSessionId: q.Get("parent_session"),
		AgentId:         q.Get("agent"),
		Priority:        q.Get("priority"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = StreamSSE(w, r, func() (*SwarmMessageUpdate, error) {
		m, err := stream.Recv()
		if err != nil {
			return nil, err
		}
		return &SwarmMessageUpdate{
			ID:              m.Id,
			From:            m.From,
			To:              m.To,
			Content:         m.Content,
			Type:            m.Type,
			Priority:        m.Priority,
			ParentSessionID: m.ParentSessionId,
			CreatedAt:       formatRFC(m.CreatedAt.AsTime()),
		}, nil
	}, func(m *SwarmMessageUpdate) string {
		return fmt.Sprintf("%d", m.ID)
	})
}

// APIWatchSwarmState streams state changes filtered by agent / key prefix.
func (h *Handler) APIWatchSwarmState(w http.ResponseWriter, r *http.Request) {
	project, _ := url.PathUnescape(chi.URLParam(r, "project"))
	inst := h.findInstance(project)
	if inst == nil {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	client := inst.Client()
	if client == nil {
		http.Error(w, "instance not connected", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	stream, err := client.Swarm.WatchState(r.Context(), &grpcapi.SwarmWatchStateRequest{
		AgentId:   q.Get("agent"),
		KeyPrefix: q.Get("key_prefix"),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = StreamSSE(w, r, func() (*SwarmStateUpdate, error) {
		c, err := stream.Recv()
		if err != nil {
			return nil, err
		}
		if c.State == nil {
			return &SwarmStateUpdate{Change: c.Change}, nil
		}
		return &SwarmStateUpdate{
			Key:       c.State.Key,
			Value:     c.State.Value,
			Agent:     c.State.Agent,
			Change:    c.Change,
			UpdatedAt: formatRFC(c.State.UpdatedAt.AsTime()),
		}, nil
	}, func(u *SwarmStateUpdate) string {
		return u.Key
	})
}

// APIAgentControl writes halt/pause/resume/deadline via the State bucket so
// the orchestrator can act from the dashboard.
func (h *Handler) APIAgentControl(ctx context.Context, input *struct {
	Project string `path:"project"`
	Body    struct {
		AgentID  string `json:"agent" required:"true"`
		Action   string `json:"action" required:"true" doc:"halt|pause|resume|deadline"`
		Reason   string `json:"reason,omitempty"`
		Duration string `json:"duration,omitempty" doc:"For action=deadline (e.g. 30m, 2h)"`
	}
}) (*struct{}, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	client := inst.Client()
	if client == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	set := func(key, value string) error {
		_, err := client.State.Set(ctx, &grpcapi.StateSetRequest{
			Key:     key,
			Value:   value,
			AgentId: input.Body.AgentID,
		})
		return err
	}
	del := func(key string) error {
		_, err := client.State.Delete(ctx, &grpcapi.StateDeleteRequest{
			Key: fmt.Sprintf("agent:%s:%s", input.Body.AgentID, key),
		})
		return err
	}

	switch input.Body.Action {
	case "halt":
		reason := input.Body.Reason
		if reason == "" {
			reason = "halted by orchestrator"
		}
		if err := set("halt", "true"); err != nil {
			return nil, err
		}
		if err := set("halt_reason", reason); err != nil {
			return nil, err
		}
	case "pause":
		if err := set("paused", "true"); err != nil {
			return nil, err
		}
	case "resume":
		for _, k := range []string{"paused", "halt", "halt_reason"} {
			if err := del(k); err != nil {
				return nil, err
			}
		}
	case "deadline":
		if input.Body.Duration == "" {
			return nil, huma.Error400BadRequest("duration required for action=deadline")
		}
		if err := set("deadline", input.Body.Duration); err != nil {
			return nil, err
		}
	default:
		return nil, huma.Error400BadRequest("unknown action: " + input.Body.Action)
	}
	return nil, nil
}
