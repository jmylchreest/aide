package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// newMemoryTestServer spins up an MCPServer backed by a real BoltStore in a
// temp dir so memory_add actually persists and is searchable.
func newMemoryTestServer(t *testing.T) (*MCPServer, func()) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.NewBoltStore(filepath.Join(dir, "memory.db"))
	if err != nil {
		t.Fatalf("failed to open bolt store: %v", err)
	}
	s := &MCPServer{store: st}
	return s, func() { st.Close() }
}

func TestHandleMemoryAdd_StoresAndReturnsID(t *testing.T) {
	s, cleanup := newMemoryTestServer(t)
	defer cleanup()

	result, _, err := s.handleMemoryAdd(context.Background(), nil, MemoryAddInput{
		Content:  "The tavern keeper always serves ale at room temperature",
		Category: string(memory.CategoryLearning),
		Tags:     []string{"preferences", "fantasy"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := extractText(result)
	if !strings.Contains(text, `"status":"stored"`) {
		t.Errorf("expected status stored in %q", text)
	}
	if !strings.Contains(text, `"category":"learning"`) {
		t.Errorf("expected category learning in %q", text)
	}

	// Verify it actually persisted and is searchable (read-after-write).
	search, _, err := s.handleMemorySearch(context.Background(), nil, MemorySearchInput{
		Query: "room temperature",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if !strings.Contains(extractText(search), "room temperature") {
		t.Errorf("search did not find added memory: %q", extractText(search))
	}
}

func TestHandleMemoryAdd_DefaultsToLearningCategory(t *testing.T) {
	s, cleanup := newMemoryTestServer(t)
	defer cleanup()

	result, _, err := s.handleMemoryAdd(context.Background(), nil, MemoryAddInput{
		Content: "Default category fact",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(extractText(result), `"category":"learning"`) {
		t.Errorf("expected default category learning in %q", extractText(result))
	}
}

func TestHandleMemoryAdd_EmptyContentIsError(t *testing.T) {
	s, cleanup := newMemoryTestServer(t)
	defer cleanup()

	result, _, err := s.handleMemoryAdd(context.Background(), nil, MemoryAddInput{
		Content: "   ",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "'content' is required")
}

func TestHandleMemoryAdd_UnknownCategoryIsError(t *testing.T) {
	s, cleanup := newMemoryTestServer(t)
	defer cleanup()

	result, _, err := s.handleMemoryAdd(context.Background(), nil, MemoryAddInput{
		Content:  "A fact with a bogus category",
		Category: "learnings",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "unknown category")
	assertIsError(t, result, `"learnings"`)

	// A malformed category must not be persisted — nothing should be searchable.
	search, _, err := s.handleMemorySearch(context.Background(), nil, MemorySearchInput{
		Query: "bogus category",
		Limit: 5,
	})
	if err != nil {
		t.Fatalf("search error: %v", err)
	}
	if strings.Contains(extractText(search), "bogus category") {
		t.Errorf("memory with invalid category should not have been stored: %q", extractText(search))
	}
}

func TestHandleMemoryAdd_ReservedCategoryIsError(t *testing.T) {
	s, cleanup := newMemoryTestServer(t)
	defer cleanup()

	result, _, err := s.handleMemoryAdd(context.Background(), nil, MemoryAddInput{
		Content:  "An instinct a caller shouldn't set",
		Category: string(memory.CategoryInstinct),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertIsError(t, result, "set by aide, not by callers")
}
