package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/store"
)

// A tool_call recorded with --error lands with Error set on the stored event.
// That field is the signal the friction detector reads, so this guards the
// observe → friction data path end-to-end on the Go side.
func TestObserveRecord_SetsErrorField(t *testing.T) {
	isolateHome(t)
	dbPath := filepath.Join(t.TempDir(), ".aide", "memory", "memory.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}

	args := []string{
		"--kind=tool_call",
		"--name=Bash",
		"--category=execute",
		"--session=s1",
		"--attr=command=go test ./...",
		"--error=exit status 1: build failed",
	}
	if err := cmdObserveRecord(dbPath, args); err != nil {
		t.Fatalf("record: %v", err)
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		t.Fatalf("backend: %v", err)
	}
	defer backend.Close()

	events, err := backend.Store().ListObserveEvents(store.ObserveFilter{SessionID: "s1", Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if got := events[0].Error; got != "exit status 1: build failed" {
		t.Errorf("event.Error = %q, want the recorded error text", got)
	}
}
