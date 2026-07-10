package handler

import (
	"context"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	"github.com/jmylchreest/aide/aide-web/internal/instance"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// isUnimplemented reports whether err is a gRPC Unimplemented status — i.e. the
// connected daemon is too old to expose the requested service. Version skew
// between a newer aide-web and an older `aide` daemon surfaces this way; callers
// turn it into a clear "upgrade that instance" message instead of an opaque 500.
func isUnimplemented(err error) bool {
	return status.Code(err) == codes.Unimplemented
}

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

// findInstance resolves the {project} route param to an instance. The slug
// (ProjectName + short root hash) is the primary key and is always unique. A
// bare project name / base-dir name still resolves for convenience and old
// bookmarks — but ONLY when it matches exactly one instance, so two repos that
// share a base name never silently resolve to whichever was discovered first.
func (h *Handler) findInstance(project string) *instance.Instance {
	var nameMatches []*instance.Instance
	for _, inst := range h.manager.Instances() {
		if inst.Slug() == project {
			return inst
		}
		if inst.ProjectName() == project || filepath.Base(inst.ProjectRoot()) == project {
			nameMatches = append(nameMatches, inst)
		}
	}
	if len(nameMatches) == 1 {
		return nameMatches[0]
	}
	// Several instances share the name (e.g. a stale registration beside a
	// live one during daemon cycling): a single CONNECTED match still
	// resolves unambiguously. Two connected same-name repos stay ambiguous.
	var connected *instance.Instance
	connectedCount := 0
	for _, inst := range nameMatches {
		if inst.Status() == instance.StatusConnected {
			connected = inst
			connectedCount++
		}
	}
	if connectedCount == 1 {
		return connected
	}
	return nil
}
