package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// MemoryItem is the JSON representation of a memory entry.
type MemoryItem struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags"`
}

// CreateMemoryInput is the request body for creating a memory.
type CreateMemoryInput struct {
	Body struct {
		Category string   `json:"category" required:"true"`
		Content  string   `json:"content" required:"true"`
		Tags     []string `json:"tags,omitempty"`
	}
}

// DeleteMemoryInput is the request for deleting a memory.
type DeleteMemoryInput struct {
	Project string `path:"project"`
	ID      string `path:"id"`
}

// APICreateMemory creates a new memory entry.
func (h *Handler) APICreateMemory(ctx context.Context, input *struct {
	Project string `path:"project"`
	Body    struct {
		Category string   `json:"category" required:"true"`
		Content  string   `json:"content" required:"true"`
		Tags     []string `json:"tags,omitempty"`
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
	m := &memory.Memory{
		Category: memory.Category(input.Body.Category),
		Content:  input.Body.Content,
		Tags:     input.Body.Tags,
	}
	if err := s.AddMemory(m); err != nil {
		return nil, err
	}
	return nil, nil
}

// APIDeleteMemory deletes a memory entry by ID.
func (h *Handler) APIDeleteMemory(ctx context.Context, input *struct {
	Project string `path:"project"`
	ID      string `path:"id"`
}) (*struct{}, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	s := inst.Store()
	if s == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	if err := s.DeleteMemory(input.ID); err != nil {
		return nil, err
	}
	return nil, nil
}

// GetMemoryOutput is the response body for APIGetMemory.
type GetMemoryOutput struct {
	Body struct {
		Memory *MemoryItem `json:"memory"`
	}
}

// APIGetMemory returns a single memory by ID.
func (h *Handler) APIGetMemory(ctx context.Context, input *struct {
	Project string `path:"project"`
	ID      string `path:"id"`
}) (*GetMemoryOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	s := inst.Store()
	if s == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}
	m, err := s.GetMemory(input.ID)
	if err != nil {
		return nil, huma.Error404NotFound("memory not found")
	}
	out := &GetMemoryOutput{}
	out.Body.Memory = &MemoryItem{
		ID:       m.ID,
		Category: string(m.Category),
		Content:  m.Content,
		Tags:     m.Tags,
	}
	return out, nil
}

// ListMemoriesOutput is the response body for APIListMemories.
type ListMemoriesOutput struct {
	Body struct {
		Memories []MemoryItem `json:"memories"`
	}
}

// APIListMemories returns memories for an instance as JSON.
func (h *Handler) APIListMemories(ctx context.Context, input *struct {
	Project  string `path:"project"`
	Category string `query:"category"`
	Limit    int    `query:"limit" minimum:"1" maximum:"500" default:"50"`
}) (*ListMemoriesOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	s := inst.Store()
	if s == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	opts := memory.SearchOptions{
		Category: memory.Category(input.Category),
		Limit:    input.Limit,
	}
	memories, err := s.ListMemories(opts)
	if err != nil {
		return nil, err
	}

	out := &ListMemoriesOutput{}
	for _, m := range memories {
		out.Body.Memories = append(out.Body.Memories, MemoryItem{
			ID:       m.ID,
			Category: string(m.Category),
			Content:  m.Content,
			Tags:     m.Tags,
		})
	}
	return out, nil
}
