// Package server provides HTTP API for aide.
package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// Server provides HTTP API for aide.
type Server struct {
	store store.Store
	addr  string
	mux   *http.ServeMux
}

// NewServer creates a new HTTP server.
func NewServer(st store.Store, addr string) *Server {
	s := &Server{
		store: st,
		addr:  addr,
		mux:   http.NewServeMux(),
	}
	s.setupRoutes()
	return s
}

// setupRoutes configures all HTTP routes.
func (s *Server) setupRoutes() {
	// Memory endpoints
	s.mux.HandleFunc("/api/memories", s.handleMemories)
	s.mux.HandleFunc("/api/memories/", s.handleMemory)
	s.mux.HandleFunc("/api/memories/search", s.handleSearch)

	// Task endpoints
	s.mux.HandleFunc("/api/tasks", s.handleTasks)
	s.mux.HandleFunc("/api/tasks/", s.handleTask)
	s.mux.HandleFunc("/api/tasks/claim", s.handleClaimTask)

	// Decision endpoints
	s.mux.HandleFunc("/api/decisions", s.handleDecisions)
	s.mux.HandleFunc("/api/decisions/", s.handleDecision)

	// Message endpoints
	s.mux.HandleFunc("/api/messages", s.handleMessages)

	// Health check
	s.mux.HandleFunc("/health", s.handleHealth)
}

// Start starts the HTTP server.
func (s *Server) Start() error {
	fmt.Printf("aide server listening on %s\n", s.addr)
	srv := &http.Server{
		Addr:         s.addr,
		Handler:      s.mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	return srv.ListenAndServe()
}

// MaxRequestBodySize limits request body size to 1MB.
const MaxRequestBodySize = 1 << 20 // 1MB

// limitRequestBody wraps the request body with a size limit.
func limitRequestBody(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, MaxRequestBodySize)
}

// Response helpers
func jsonResponse(w http.ResponseWriter, data interface{}, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("http: failed to encode response: %v", err)
	}
}

func errorResponse(w http.ResponseWriter, message string, status int) {
	jsonResponse(w, map[string]string{"error": message}, status)
}

// Health check
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, map[string]string{"status": "ok"}, http.StatusOK)
}

// Memory handlers
func (s *Server) handleMemories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		category := r.URL.Query().Get("category")
		opts := memory.SearchOptions{
			Category: memory.Category(category),
			Limit:    100,
		}
		memories, err := s.store.ListMemories(opts)
		if err != nil {
			errorResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, memories, http.StatusOK)

	case http.MethodPost:
		limitRequestBody(w, r)
		var mem memory.Memory
		if err := json.NewDecoder(r.Body).Decode(&mem); err != nil {
			errorResponse(w, "invalid JSON or request too large", http.StatusBadRequest)
			return
		}
		if err := s.store.AddMemory(&mem); err != nil {
			errorResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, mem, http.StatusCreated)

	default:
		errorResponse(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/memories/")
	if id == "" {
		errorResponse(w, "memory ID required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		mem, err := s.store.GetMemory(id)
		if err != nil {
			errorResponse(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonResponse(w, mem, http.StatusOK)

	case http.MethodDelete:
		if err := s.store.DeleteMemory(id); err != nil {
			errorResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, map[string]string{"deleted": id}, http.StatusOK)

	default:
		errorResponse(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorResponse(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		errorResponse(w, "query parameter 'q' required", http.StatusBadRequest)
		return
	}

	memories, err := s.store.SearchMemories(query, 20)
	if err != nil {
		errorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonResponse(w, memories, http.StatusOK)
}

// Task handlers
func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		status := memory.TaskStatus(r.URL.Query().Get("status"))
		tasks, err := s.store.ListTasks(status)
		if err != nil {
			errorResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, tasks, http.StatusOK)

	case http.MethodPost:
		limitRequestBody(w, r)
		var task memory.Task
		if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
			errorResponse(w, "invalid JSON or request too large", http.StatusBadRequest)
			return
		}
		if err := s.store.CreateTask(&task); err != nil {
			errorResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, task, http.StatusCreated)

	default:
		errorResponse(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTask(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	if id == "" || id == "claim" {
		return // handled by other routes
	}

	switch r.Method {
	case http.MethodGet:
		task, err := s.store.GetTask(id)
		if err != nil {
			errorResponse(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonResponse(w, task, http.StatusOK)

	case http.MethodPatch:
		limitRequestBody(w, r)
		var update struct {
			Status string `json:"status"`
			Result string `json:"result"`
		}
		if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
			errorResponse(w, "invalid JSON or request too large", http.StatusBadRequest)
			return
		}
		if update.Status == "done" || update.Status == "completed" {
			if err := s.store.CompleteTask(id, update.Result); err != nil {
				errorResponse(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
		task, _ := s.store.GetTask(id)
		jsonResponse(w, task, http.StatusOK)

	default:
		errorResponse(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleClaimTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorResponse(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limitRequestBody(w, r)
	var req struct {
		TaskID  string `json:"taskId"`
		AgentID string `json:"agentId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorResponse(w, "invalid JSON or request too large", http.StatusBadRequest)
		return
	}

	task, err := s.store.ClaimTask(req.TaskID, req.AgentID)
	if err != nil {
		errorResponse(w, err.Error(), http.StatusConflict)
		return
	}
	jsonResponse(w, task, http.StatusOK)
}

// Decision handlers
func (s *Server) handleDecisions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		decisions, err := s.store.ListDecisions()
		if err != nil {
			errorResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, decisions, http.StatusOK)

	case http.MethodPost:
		limitRequestBody(w, r)
		var dec memory.Decision
		if err := json.NewDecoder(r.Body).Decode(&dec); err != nil {
			errorResponse(w, "invalid JSON or request too large", http.StatusBadRequest)
			return
		}
		if err := s.store.SetDecision(&dec); err != nil {
			errorResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, dec, http.StatusCreated)

	default:
		errorResponse(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleDecision(w http.ResponseWriter, r *http.Request) {
	topic := strings.TrimPrefix(r.URL.Path, "/api/decisions/")
	if topic == "" {
		errorResponse(w, "topic required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		dec, err := s.store.GetDecision(topic)
		if err != nil {
			errorResponse(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonResponse(w, dec, http.StatusOK)

	default:
		errorResponse(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// Message handlers
func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		agentID := r.URL.Query().Get("agent")
		messages, err := s.store.GetMessages(agentID)
		if err != nil {
			errorResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, messages, http.StatusOK)

	case http.MethodPost:
		limitRequestBody(w, r)
		var msg memory.Message
		if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
			errorResponse(w, "invalid JSON or request too large", http.StatusBadRequest)
			return
		}
		if err := s.store.AddMessage(&msg); err != nil {
			errorResponse(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonResponse(w, msg, http.StatusCreated)

	default:
		errorResponse(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
