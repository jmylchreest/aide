package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/store"
)

// TestObserveRecordBatch: JSON Lines on stdin land as events under one
// store open; malformed lines are skipped, not fatal.
func TestObserveRecordBatch(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "proj")
	if err := os.MkdirAll(filepath.Join(root, ".aide", "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	dbPath := computeDBPath(root)

	lines := strings.Join([]string{
		`{"kind":"injection","name":"session-start","category":"inject","subtype":"decision","tokens":42,"session":"s1","attrs":{"scope":"project"}}`,
		`{"kind":"injection","name":"session-start","subtype":"memory","session":"s1"}`,
		`not json at all`,
		`{"name":"missing-kind"}`,
		`{"kind":"session","name":"session-start","category":"lifecycle","session":"s1"}`,
	}, "\n")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin })
	if _, err := w.WriteString(lines); err != nil {
		t.Fatal(err)
	}
	w.Close()

	if err := cmdObserveRecordBatch(dbPath); err != nil {
		t.Fatalf("batch: %v", err)
	}

	st, err := store.NewBoltStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	events, err := st.ListObserveEvents(store.ObserveFilter{Limit: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("recorded %d events, want 3 (2 malformed skipped)", len(events))
	}
	found := false
	for _, e := range events {
		if e.Subtype == "decision" && e.Tokens == 42 && e.Attrs["scope"] == "project" {
			found = true
		}
	}
	if !found {
		t.Error("decision event with tokens/attrs not found")
	}
}
