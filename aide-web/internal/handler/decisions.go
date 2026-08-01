package handler

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// DecisionItem is the JSON representation of a decision. Details and
// References are carried because writes are append-only and injection takes the
// newest revision: a round-trip that dropped them would permanently destroy the
// body of a blueprint-imported decision, where nearly all the guidance lives.
type DecisionItem struct {
	Topic      string   `json:"topic"`
	Decision   string   `json:"decision"`
	Rationale  string   `json:"rationale"`
	Details    string   `json:"details,omitempty"`
	References []string `json:"references,omitempty"`
	DecidedBy  string   `json:"decided_by"`
	Precedence int      `json:"precedence,omitempty"`
	CreatedAt  string   `json:"created_at"`
}

// ListDecisionsOutput is the response body for APIListDecisions.
type ListDecisionsOutput struct {
	Body struct {
		Decisions []DecisionItem `json:"decisions"`
	}
}

// APIListDecisions returns decisions for an instance as JSON.
func (h *Handler) APIListDecisions(ctx context.Context, input *struct {
	Project string `path:"project"`
}) (*ListDecisionsOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	s := inst.Store()
	if s == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	decisions, err := s.ListDecisions()
	if err != nil {
		return nil, err
	}

	out := &ListDecisionsOutput{}
	for _, d := range decisions {
		out.Body.Decisions = append(out.Body.Decisions, DecisionItem{
			Topic:      d.Topic,
			Decision:   d.Decision,
			Rationale:  d.Rationale,
			Details:    d.Details,
			References: d.References,
			DecidedBy:  d.DecidedBy,
			Precedence: d.Precedence,
			CreatedAt:  d.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

// carryString resolves an optional string field: an explicit value wins,
// otherwise the previous revision's value is carried forward.
func carryString(requested *string, prev *memory.Decision, get func(*memory.Decision) string) string {
	if requested != nil {
		return *requested
	}
	if prev != nil {
		return get(prev)
	}
	return ""
}

// APICreateDecision creates or updates a decision.
//
// Every optional field is a pointer so an omitted field means "leave as it was"
// rather than "clear it". Writes are append-only and injection reads only the
// newest revision, so a partial update that sent zero values would permanently
// destroy whatever it failed to mention — which is how a form carrying only
// topic, decision, and rationale used to wipe the details of a
// blueprint-imported decision, where nearly all the guidance lives. Send an
// explicit empty value to clear a field.
func (h *Handler) APICreateDecision(ctx context.Context, input *struct {
	Project string `path:"project"`
	Body    struct {
		Topic      string    `json:"topic" required:"true"`
		Decision   string    `json:"decision" required:"true"`
		Rationale  *string   `json:"rationale,omitempty"`
		Details    *string   `json:"details,omitempty"`
		References *[]string `json:"references,omitempty"`
		DecidedBy  *string   `json:"decided_by,omitempty"`
		Precedence *int      `json:"precedence,omitempty"`
	}
}) (*struct{}, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	s := inst.Store()
	if s == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	// Previous revision, if any, supplies every field the request omitted.
	prev, err := s.GetDecision(input.Body.Topic)
	if err != nil {
		prev = nil
	}

	d := &memory.Decision{
		Topic:      input.Body.Topic,
		Decision:   input.Body.Decision,
		Rationale:  carryString(input.Body.Rationale, prev, func(p *memory.Decision) string { return p.Rationale }),
		Details:    carryString(input.Body.Details, prev, func(p *memory.Decision) string { return p.Details }),
		DecidedBy:  carryString(input.Body.DecidedBy, prev, func(p *memory.Decision) string { return p.DecidedBy }),
		Precedence: store.ResolvePrecedence(s, input.Body.Topic, input.Body.Precedence),
	}
	switch {
	case input.Body.References != nil:
		d.References = *input.Body.References
	case prev != nil:
		d.References = prev.References
	}

	if err := s.SetDecision(d); err != nil {
		return nil, err
	}
	return nil, nil
}

// APIDeleteDecision deletes a decision by topic.
func (h *Handler) APIDeleteDecision(ctx context.Context, input *struct {
	Project string `path:"project"`
	Topic   string `path:"topic"`
}) (*struct{}, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	s := inst.Store()
	if s == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	if _, err := s.DeleteDecision(input.Topic); err != nil {
		return nil, err
	}
	return nil, nil
}
