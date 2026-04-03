package handler

import (
	"context"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide-web/internal/instance"
)

// InstanceInfo is the JSON representation of an instance.
type InstanceInfo struct {
	ProjectRoot string          `json:"project_root"`
	ProjectName string          `json:"project_name"`
	SocketPath  string          `json:"socket_path"`
	Status      instance.Status `json:"status"`
	Version     string          `json:"version"`
}

// ListInstancesOutput is the response body for APIListInstances.
type ListInstancesOutput struct {
	Body struct {
		Instances []InstanceInfo `json:"instances"`
	}
}

// APIDeleteInstance removes a disconnected instance from the registry.
func (h *Handler) APIDeleteInstance(ctx context.Context, input *struct {
	Project string `path:"project"`
}) (*struct{}, error) {
	inst := h.findInstance(input.Project)
	if inst == nil {
		return nil, huma.Error404NotFound("instance not found")
	}
	if inst.Status() == instance.StatusConnected {
		return nil, huma.Error409Conflict("cannot remove a connected instance — stop it first")
	}
	if err := h.manager.RemoveInstance(inst.ProjectRoot()); err != nil {
		return nil, err
	}
	return nil, nil
}

// APIListInstances returns all known instances as JSON.
func (h *Handler) APIListInstances(ctx context.Context, input *struct{}) (*ListInstancesOutput, error) {
	out := &ListInstancesOutput{}
	for _, inst := range h.manager.Instances() {
		out.Body.Instances = append(out.Body.Instances, InstanceInfo{
			ProjectRoot: inst.ProjectRoot(),
			ProjectName: inst.ProjectName(),
			SocketPath:  inst.SocketPath(),
			Status:      inst.Status(),
			Version:     inst.Version(),
		})
	}
	return out, nil
}
