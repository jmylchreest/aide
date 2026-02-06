package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

func setupTestServer(t *testing.T) (*Server, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "aide-memory-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := store.NewBoltStore(dbPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create store: %v", err)
	}

	srv := NewServer(s, ":0")

	cleanup := func() {
		s.Close()
		os.RemoveAll(tmpDir)
	}

	return srv, cleanup
}

func TestHealthEndpoint(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var result map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", result["status"])
	}
}

func TestMemoryEndpoints(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a memory
	mem := memory.Memory{
		ID:       "test-mem-1",
		Category: memory.CategoryLearning,
		Content:  "Test memory content",
	}

	body, _ := json.Marshal(mem)
	req := httptest.NewRequest(http.MethodPost, "/api/memories", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	// List memories
	req = httptest.NewRequest(http.MethodGet, "/api/memories", nil)
	w = httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var memories []memory.Memory
	if err := json.Unmarshal(w.Body.Bytes(), &memories); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(memories) != 1 {
		t.Errorf("expected 1 memory, got %d", len(memories))
	}

	// Search memories
	req = httptest.NewRequest(http.MethodGet, "/api/memories/search?q=test", nil)
	w = httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestTaskEndpoints(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a task
	task := memory.Task{
		ID:          "test-task-1",
		Title:       "Test Task",
		Description: "A test task",
		Status:      memory.TaskStatusPending,
	}

	body, _ := json.Marshal(task)
	req := httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	// Claim task
	claim := map[string]string{
		"taskId":  "test-task-1",
		"agentId": "agent-1",
	}
	body, _ = json.Marshal(claim)
	req = httptest.NewRequest(http.MethodPost, "/api/tasks/claim", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	// List tasks
	req = httptest.NewRequest(http.MethodGet, "/api/tasks", nil)
	w = httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestDecisionEndpoints(t *testing.T) {
	srv, cleanup := setupTestServer(t)
	defer cleanup()

	// Create a decision
	dec := memory.Decision{
		Topic:     "auth-strategy",
		Decision:  "JWT",
		Rationale: "Stateless and scalable",
	}

	body, _ := json.Marshal(dec)
	req := httptest.NewRequest(http.MethodPost, "/api/decisions", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	// Get decision
	req = httptest.NewRequest(http.MethodGet, "/api/decisions/auth-strategy", nil)
	w = httptest.NewRecorder()

	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var result memory.Decision
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if result.Decision != "JWT" {
		t.Errorf("expected decision 'JWT', got '%s'", result.Decision)
	}
}
