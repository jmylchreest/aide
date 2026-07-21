package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// cascadeFixture builds parent/child stores with real markers:
// parent decides "shared-topic" and "parent-only"; child decides
// "shared-topic" (override) and "child-only".
func cascadeFixture(t *testing.T) (parentRoot, childRoot string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parentRoot = filepath.Join(tmp, "parent")
	childRoot = filepath.Join(parentRoot, "child")
	for _, d := range []string{
		filepath.Join(parentRoot, ".git"),
		filepath.Join(parentRoot, ".aide", "memory"),
		filepath.Join(childRoot, ".git"),
		filepath.Join(childRoot, ".aide", "memory"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	ps, err := store.NewBoltStore(computeDBPath(parentRoot))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	mustSet := func(s *store.BoltStore, topic, value string) {
		t.Helper()
		if err := s.SetDecision(&memory.Decision{Topic: topic, Decision: value, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}
	mustSet(ps, "shared-topic", "parent-version")
	mustSet(ps, "parent-only", "estate-rule")
	ps.Close()

	cs, err := store.NewBoltStore(computeDBPath(childRoot))
	if err != nil {
		t.Fatal(err)
	}
	mustSet(cs, "shared-topic", "child-override")
	mustSet(cs, "child-only", "local-rule")
	cs.Close()

	t.Setenv("AIDE_PROJECT_ROOT", "")
	os.Unsetenv("AIDE_PROJECT_ROOT")
	os.Unsetenv("AIDE_CASCADE_DISABLED")
	return parentRoot, childRoot
}

func decisionByTopic(ds []SessionDecision, topic string) *SessionDecision {
	for i := range ds {
		if ds[i].Topic == topic {
			return &ds[i]
		}
	}
	return nil
}

// TestSessionCascadeDecisions pins nearest-wins with origin provenance:
// local topics shadow the parent's, parent-only topics cascade in with
// Origin set, read through the read-only ladder (no daemon running).
func TestSessionCascadeDecisions(t *testing.T) {
	parentRoot, childRoot := cascadeFixture(t)

	backend, err := NewBackend(computeDBPath(childRoot))
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	result := SessionInitResult{}
	sessionFetchContext(backend, "", 0, &result)

	if d := decisionByTopic(result.Decisions, "shared-topic"); d == nil || d.Value != "child-override" || d.Origin != "" {
		t.Errorf("shared-topic = %+v, want local child-override with no origin", d)
	}
	if d := decisionByTopic(result.Decisions, "child-only"); d == nil || d.Origin != "" {
		t.Errorf("child-only = %+v, want local with no origin", d)
	}
	d := decisionByTopic(result.Decisions, "parent-only")
	if d == nil || d.Value != "estate-rule" {
		t.Fatalf("parent-only = %+v, want cascaded estate-rule", d)
	}
	if d.Origin != parentRoot {
		t.Errorf("parent-only origin = %q, want %q", d.Origin, parentRoot)
	}
	if d.OriginName == "" {
		t.Error("parent-only origin_name empty")
	}
}

// TestSessionCascadeKillSwitch: AIDE_CASCADE_DISABLED=1 restores full
// isolation.
func TestSessionCascadeKillSwitch(t *testing.T) {
	_, childRoot := cascadeFixture(t)
	t.Setenv("AIDE_CASCADE_DISABLED", "1")

	backend, err := NewBackend(computeDBPath(childRoot))
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	result := SessionInitResult{}
	sessionFetchContext(backend, "", 0, &result)

	if d := decisionByTopic(result.Decisions, "parent-only"); d != nil {
		t.Errorf("cascade ran despite kill switch: %+v", d)
	}
}

// TestFetchAncestorDecisionsMissingStore: a parent without a store yields
// nothing, never an error surface.
func TestFetchAncestorDecisionsMissingStore(t *testing.T) {
	tmp := t.TempDir()
	if ds := fetchAncestorDecisions(tmp); len(ds) != 0 {
		t.Errorf("expected no decisions from storeless root, got %d", len(ds))
	}
}
