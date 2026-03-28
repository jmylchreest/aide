package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// TaskItem is the JSON representation of a task.
type TaskItem struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Status      string `json:"status"`
	ClaimedBy   string `json:"claimed_by,omitempty"`
	Result      string `json:"result,omitempty"`
}

// APICreateTask creates a new task.
func (h *Handler) APICreateTask(ctx context.Context, input *struct {
	Project string `path:"project"`
	Body    struct {
		Title       string `json:"title" required:"true"`
		Description string `json:"description,omitempty"`
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
	t := &memory.Task{
		Title:       input.Body.Title,
		Description: input.Body.Description,
		Status:      memory.TaskStatusPending,
	}
	if err := s.CreateTask(t); err != nil {
		return nil, err
	}
	return nil, nil
}

// APIDeleteTask deletes a task by ID.
func (h *Handler) APIDeleteTask(ctx context.Context, input *struct {
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
	if err := s.DeleteTask(input.ID); err != nil {
		return nil, err
	}
	return nil, nil
}

// ListTasksOutput is the response body for APIListTasks.
type ListTasksOutput struct {
	Body struct {
		Tasks []TaskItem `json:"tasks"`
	}
}

// APIListTasks returns tasks for an instance as JSON.
func (h *Handler) APIListTasks(ctx context.Context, input *struct {
	Project string `path:"project"`
	Status  string `query:"status"`
}) (*ListTasksOutput, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	s := inst.Store()
	if s == nil {
		return nil, huma.Error503ServiceUnavailable("instance not connected")
	}

	tasks, err := s.ListTasks(memory.TaskStatus(input.Status))
	if err != nil {
		return nil, err
	}

	out := &ListTasksOutput{}
	for _, t := range tasks {
		out.Body.Tasks = append(out.Body.Tasks, TaskItem{
			ID:          t.ID,
			Title:       t.Title,
			Description: t.Description,
			Status:      string(t.Status),
			ClaimedBy:   t.ClaimedBy,
			Result:      t.Result,
		})
	}
	return out, nil
}
