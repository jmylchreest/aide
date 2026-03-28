package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
)

// StateItem is the JSON representation of a state entry.
type StateItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Agent string `json:"agent,omitempty"`
}

// ListStateOutput is the response body for APIListState.
type ListStateOutput struct {
	Body struct {
		States []StateItem `json:"states"`
	}
}

// APIListState returns state entries for an instance as JSON.
func (h *Handler) APIListState(ctx context.Context, input *struct {
	Project string `path:"project"`
	Agent   string `query:"agent"`
}) (*ListStateOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	s := inst.Store()
	if s == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	states, err := s.ListState(input.Agent)
	if err != nil {
		return nil, err
	}

	out := &ListStateOutput{}
	for _, st := range states {
		out.Body.States = append(out.Body.States, StateItem{
			Key:   st.Key,
			Value: st.Value,
			Agent: st.Agent,
		})
	}
	return out, nil
}

// APIDeleteState deletes a state entry by key.
func (h *Handler) APIDeleteState(ctx context.Context, input *struct {
	Project string `path:"project"`
	Key     string `path:"key"`
}) (*struct{}, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	s := inst.Store()
	if s == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	if err := s.DeleteState(input.Key); err != nil {
		return nil, err
	}
	return nil, nil
}
