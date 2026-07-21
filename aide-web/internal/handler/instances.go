package handler

import (
	"context"
	"path/filepath"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jmylchreest/aide/aide-web/internal/instance"
	"github.com/jmylchreest/aide/aide/pkg/store"
	"github.com/jmylchreest/aide/aide/pkg/survey"
)

// InstanceInfo is the JSON representation of an instance.
type InstanceInfo struct {
	// Slug is the disambiguated routing identifier (ProjectName + short
	// project-root hash). Use it for links/keys; ProjectName is display-only
	// and may collide across repos that share a base name.
	Slug        string          `json:"slug"`
	ProjectRoot string          `json:"project_root"`
	ProjectName string          `json:"project_name"`
	SocketPath  string          `json:"socket_path"`
	Status      instance.Status `json:"status"`
	Version     string          `json:"version"`
	// Parents are anchor-chain ancestor roots, nearest first. Consumers
	// match them against other instances' ProjectRoot to build estate trees.
	Parents []string `json:"parents,omitempty"`
	// Subprojects are surveyed child scopes (nested VCS roots, submodules)
	// under this instance's root — the downward half of the estate map,
	// available whether or not any child has a live daemon.
	Subprojects []SubprojectInfo `json:"subprojects,omitempty"`
}

// SubprojectInfo is one surveyed child project scope.
type SubprojectInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"` // relative to the instance root
	Evidence string `json:"evidence,omitempty"`
	HasStore bool   `json:"has_store,omitempty"`
}

// instanceSubprojects lists an instance's surveyed child scopes: through
// the live survey adapter when connected, else straight from the bolt
// file so idle nodes keep their estate shape visible. Best-effort — an
// unreadable or unsurveyed store yields none.
func instanceSubprojects(inst *instance.Instance) []SubprojectInfo {
	var entries []*survey.Entry
	if ss := inst.SurveyStore(); ss != nil {
		entries, _ = ss.ListEntries(survey.SearchOptions{Kind: survey.KindSubproject, Limit: 100})
	} else if dbPath := inst.DBPath(); dbPath != "" {
		entries, _ = store.ReadSurveyEntriesRO(filepath.Join(filepath.Dir(dbPath), "survey"), survey.KindSubproject, 100)
	}
	subs := make([]SubprojectInfo, 0, len(entries))
	for _, e := range entries {
		subs = append(subs, SubprojectInfo{
			Name:     e.Name,
			Path:     e.FilePath,
			Evidence: e.Metadata["evidence"],
			HasStore: e.Metadata["has_aide_store"] == "true",
		})
	}
	return subs
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
			Slug:        inst.Slug(),
			ProjectRoot: inst.ProjectRoot(),
			ProjectName: inst.ProjectName(),
			SocketPath:  inst.SocketPath(),
			Status:      inst.Status(),
			Version:     inst.Version(),
			Parents:     inst.Parents(),
			Subprojects: instanceSubprojects(inst),
		})
	}
	return out, nil
}
