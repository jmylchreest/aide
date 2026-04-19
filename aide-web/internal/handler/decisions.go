package handler

import (
	"context"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// DecisionItem is the JSON representation of a decision.
type DecisionItem struct {
	Topic     string `json:"topic"`
	Decision  string `json:"decision"`
	Rationale string `json:"rationale"`
	DecidedBy string `json:"decided_by"`
	CreatedAt string `json:"created_at"`
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
			Topic:     d.Topic,
			Decision:  d.Decision,
			Rationale: d.Rationale,
			DecidedBy: d.DecidedBy,
			CreatedAt: d.CreatedAt.Format(time.RFC3339),
		})
	}
	return out, nil
}

// APICreateDecision creates or updates a decision.
func (h *Handler) APICreateDecision(ctx context.Context, input *struct {
	Project string `path:"project"`
	Body    struct {
		Topic     string `json:"topic" required:"true"`
		Decision  string `json:"decision" required:"true"`
		Rationale string `json:"rationale,omitempty"`
		DecidedBy string `json:"decided_by,omitempty"`
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
	d := &memory.Decision{
		Topic:     input.Body.Topic,
		Decision:  input.Body.Decision,
		Rationale: input.Body.Rationale,
		DecidedBy: input.Body.DecidedBy,
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
