package handler

import (
	"context"

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
