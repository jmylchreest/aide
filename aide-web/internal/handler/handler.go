package handler

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/aide/aide-web/internal/instance"
)

// Build-time version injected via ldflags:
//
//	-X github.com/jmylchreest/aide/aide-web/internal/handler.Version=x.y.z
var Version = "0.0.0"

// Handler provides HTTP handlers for aide-web API endpoints.
type Handler struct {
	manager *instance.Manager
}

// New creates a new Handler.
func New(mgr *instance.Manager) *Handler {
	return &Handler{manager: mgr}
}

// getInstance extracts the project URL param and looks up the instance.
func (h *Handler) getInstance(r *http.Request) *instance.Instance {
	project, _ := url.PathUnescape(chi.URLParam(r, "project"))
	return h.findInstance(project)
}

// VersionOutput is the response body for APIGetVersion.
type VersionOutput struct {
	Body struct {
		Version string `json:"version"`
	}
}

// APIGetVersion returns the aide-web server version.
func (h *Handler) APIGetVersion(ctx context.Context, input *struct{}) (*VersionOutput, error) {
	out := &VersionOutput{}
	out.Body.Version = Version
	return out, nil
}

// findInstance looks up an instance by project name or base directory name.
func (h *Handler) findInstance(project string) *instance.Instance {
	for _, inst := range h.manager.Instances() {
		if inst.ProjectName() == project || filepath.Base(inst.ProjectRoot()) == project {
			return inst
		}
	}
	return nil
}
