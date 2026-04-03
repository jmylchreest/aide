package server

import (
	"io/fs"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jmylchreest/aide/aide-web/internal/handler"
	"github.com/jmylchreest/aide/aide-web/internal/instance"
	"github.com/jmylchreest/aide/aide-web/internal/webdist"
)

// Server holds the HTTP server state.
type Server struct {
	router  chi.Router
	manager *instance.Manager
	dev     bool
}

// New creates a new aide-web server.
func New(mgr *instance.Manager, dev bool) (*Server, error) {
	s := &Server{
		manager: mgr,
		dev:     dev,
	}
	s.setupRouter()
	return s, nil
}

// Handler returns the http.Handler.
func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) setupRouter() {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Compress(5))

	h := handler.New(s.manager)

	// JSON API endpoints
	api := humachi.New(r, huma.DefaultConfig("aide-web", "1.0.0"))
	huma.Get(api, "/api/version", h.APIGetVersion)
	huma.Get(api, "/api/instances", h.APIListInstances)
	huma.Get(api, "/api/instances/{project}/status", h.APIGetStatus)
	huma.Get(api, "/api/instances/{project}/status/detailed", h.APIGetDetailedStatus)
	huma.Get(api, "/api/instances/{project}/memories", h.APIListMemories)
	huma.Get(api, "/api/instances/{project}/memories/{id}", h.APIGetMemory)
	huma.Post(api, "/api/instances/{project}/memories", h.APICreateMemory)
	huma.Delete(api, "/api/instances/{project}/memories/{id}", h.APIDeleteMemory)
	huma.Get(api, "/api/instances/{project}/decisions", h.APIListDecisions)
	huma.Post(api, "/api/instances/{project}/decisions", h.APICreateDecision)
	huma.Delete(api, "/api/instances/{project}/decisions/{topic}", h.APIDeleteDecision)
	huma.Get(api, "/api/instances/{project}/tasks", h.APIListTasks)
	huma.Post(api, "/api/instances/{project}/tasks", h.APICreateTask)
	huma.Delete(api, "/api/instances/{project}/tasks/{id}", h.APIDeleteTask)
	huma.Get(api, "/api/instances/{project}/messages", h.APIListMessages)
	huma.Get(api, "/api/instances/{project}/state", h.APIListState)
	huma.Delete(api, "/api/instances/{project}/state/{key}", h.APIDeleteState)
	huma.Get(api, "/api/instances/{project}/findings", h.APIListFindings)
	huma.Post(api, "/api/instances/{project}/findings/accept", h.APIAcceptFindings)
	huma.Get(api, "/api/instances/{project}/survey", h.APIListSurvey)
	huma.Get(api, "/api/instances/{project}/tokens/stats", h.APIGetTokenStats)
	huma.Get(api, "/api/instances/{project}/tokens/events", h.APIListTokenEvents)
	huma.Get(api, "/api/search", h.APISearch)
	huma.Post(api, "/api/instances/{project}/code/index", h.APIRunCodeIndex)
	huma.Get(api, "/api/instances/{project}/code/file", h.APIReadFile)

	// Legacy JSON endpoints used by the code search page
	r.Get("/instances/{project}/code/search.json", h.CodeSearchJSON)

	// Serve Astro SPA — static assets and SPA fallback
	if s.dev {
		// Dev mode: serve from disk (run `npm run build` in web/ first, or use Vite proxy)
		fileServer := http.FileServer(http.Dir("internal/webdist/build"))
		r.Handle("/*", spaHandler(fileServer, http.Dir("internal/webdist/build")))
	} else {
		// Production: serve from embedded FS
		buildFS, _ := fs.Sub(webdist.FS, "build")
		fileServer := http.FileServer(http.FS(buildFS))
		r.Handle("/*", spaHandler(fileServer, http.FS(buildFS)))
	}

	s.router = r
}

// spaHandler serves static files and falls back to index.html for SPA routes.
func spaHandler(fileServer http.Handler, fileSystem http.FileSystem) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Try to serve the exact file
		if path != "/" && !strings.HasPrefix(path, "/api/") {
			f, err := fileSystem.Open(path)
			if err == nil {
				f.Close()
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		// SPA fallback: serve index.html for all non-file routes
		r.URL.Path = "/"
		fileServer.ServeHTTP(w, r)
	}
}
