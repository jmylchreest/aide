package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/contextshare"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// peerFixture builds a project with a local decision on "shared-topic" and
// a path subscription to a peer tree publishing "shared-topic" (which the
// local decision must shadow) and "peer-only".
func peerFixture(t *testing.T) (projectRoot string) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	projectRoot = filepath.Join(tmp, "proj")
	peerDir := filepath.Join(tmp, "peer-tree")
	for _, d := range []string{
		filepath.Join(projectRoot, ".git"),
		filepath.Join(projectRoot, ".aide", "config"),
		filepath.Join(projectRoot, ".aide", "memory"),
		peerDir,
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	now := time.Now()
	writePeer := func(d *memory.Decision) {
		t.Helper()
		p := contextshare.DecisionPath(peerDir, d)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, contextshare.MarshalDecision(d), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writePeer(&memory.Decision{Topic: "shared-topic", Decision: "peer-version", CreatedAt: now})
	writePeer(&memory.Decision{Topic: "peer-only", Decision: "team-rule", DecidedBy: "platform-team", CreatedAt: now})
	if err := contextshare.WriteManifest(peerDir, now); err != nil {
		t.Fatal(err)
	}

	cfgJSON := fmt.Sprintf(`{"subscriptions":[{"name":"team","path":%q}]}`, peerDir)
	if err := os.WriteFile(filepath.Join(projectRoot, ".aide", "config", "aide.json"), []byte(cfgJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(projectRoot); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = config.Load("") })

	ps, err := store.NewBoltStore(computeDBPath(projectRoot))
	if err != nil {
		t.Fatal(err)
	}
	if err := ps.SetDecision(&memory.Decision{Topic: "shared-topic", Decision: "local-version", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	ps.Close()

	os.Unsetenv("AIDE_PROJECT_ROOT")
	os.Unsetenv("AIDE_CASCADE_DISABLED")
	return projectRoot
}

// TestSessionPeerDecisions pins the subscription layer: peer-only topics
// appear with peer origin, locally decided topics shadow the peer's.
func TestSessionPeerDecisions(t *testing.T) {
	root := peerFixture(t)
	backend, err := NewBackend(computeDBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	result := SessionInitResult{}
	sessionFetchContext(backend, "", 0, &result)

	if d := decisionByTopic(result.Decisions, "shared-topic"); d == nil || d.Value != "local-version" || d.Origin != "" {
		t.Errorf("shared-topic = %+v, want local-version with no origin", d)
	}
	d := decisionByTopic(result.Decisions, "peer-only")
	if d == nil || d.Value != "team-rule" {
		t.Fatalf("peer-only = %+v, want team-rule from peer", d)
	}
	if d.OriginKind != "peer" || d.OriginName != "team" || d.Origin != "peer:team" {
		t.Errorf("peer-only origin = %q/%q/%q, want peer:team/team/peer", d.Origin, d.OriginName, d.OriginKind)
	}
}

// TestSessionPeerKillSwitch: AIDE_CASCADE_DISABLED also silences the peer
// layer.
func TestSessionPeerKillSwitch(t *testing.T) {
	root := peerFixture(t)
	t.Setenv("AIDE_CASCADE_DISABLED", "1")

	backend, err := NewBackend(computeDBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()

	result := SessionInitResult{}
	sessionFetchContext(backend, "", 0, &result)
	if d := decisionByTopic(result.Decisions, "peer-only"); d != nil {
		t.Errorf("peer layer ran despite kill switch: %+v", d)
	}
}

// TestContextAdopt promotes a peer decision into the local store with
// adoption provenance.
func TestContextAdopt(t *testing.T) {
	root := peerFixture(t)
	dbPath := computeDBPath(root)

	if err := cmdContextAdopt(dbPath, []string{"peer-only", "--from=team"}); err != nil {
		t.Fatal(err)
	}

	backend, err := NewBackend(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer backend.Close()
	d, err := backend.GetDecision("peer-only")
	if err != nil || d == nil {
		t.Fatalf("adopted decision missing: %v", err)
	}
	if d.Decision != "team-rule" {
		t.Errorf("adopted value = %q, want team-rule", d.Decision)
	}
	if !strings.Contains(d.DecidedBy, "adopted from peer team") || !strings.Contains(d.DecidedBy, "platform-team") {
		t.Errorf("DecidedBy = %q, want adoption provenance with original author", d.DecidedBy)
	}

	if err := cmdContextAdopt(dbPath, []string{"no-such-topic"}); err == nil {
		t.Error("expected error adopting unknown topic")
	}
}
