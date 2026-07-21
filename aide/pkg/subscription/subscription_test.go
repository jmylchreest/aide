package subscription

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/contextshare"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func writeDecision(t *testing.T, root string, d *memory.Decision) {
	t.Helper()
	p := contextshare.DecisionPath(root, d)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, contextshare.MarshalDecision(d), 0o644); err != nil {
		t.Fatal(err)
	}
}

// buildTree writes a share tree: topic-a in two versions, topic-b once,
// topic-c tombstoned after its decision.
func buildTree(t *testing.T, root string) {
	t.Helper()
	now := time.Now()
	writeDecision(t, root, &memory.Decision{Topic: "topic-a", Decision: "old", CreatedAt: now.Add(-2 * time.Hour)})
	writeDecision(t, root, &memory.Decision{Topic: "topic-a", Decision: "new", CreatedAt: now.Add(-time.Hour)})
	writeDecision(t, root, &memory.Decision{Topic: "topic-b", Decision: "keep", CreatedAt: now.Add(-time.Hour)})
	writeDecision(t, root, &memory.Decision{Topic: "topic-c", Decision: "dead", CreatedAt: now.Add(-2 * time.Hour)})
	tomb := &memory.Tombstone{Kind: memory.TombstoneKindDecisionTopic, ID: "topic-c", DeletedAt: now.Add(-time.Hour)}
	tp := contextshare.TombstonePath(root, tomb)
	if err := os.MkdirAll(filepath.Dir(tp), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tp, contextshare.MarshalTombstone(tomb), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := contextshare.WriteManifest(root, now); err != nil {
		t.Fatal(err)
	}
}

func TestReadDecisions(t *testing.T) {
	root := t.TempDir()
	buildTree(t, root)

	latest, err := ReadDecisions(root)
	if err != nil {
		t.Fatal(err)
	}
	if d := latest["topic-a"]; d == nil || d.Decision != "new" {
		t.Errorf("topic-a = %+v, want latest version 'new'", d)
	}
	if d := latest["topic-b"]; d == nil || d.Decision != "keep" {
		t.Errorf("topic-b = %+v, want 'keep'", d)
	}
	if d, ok := latest["topic-c"]; ok {
		t.Errorf("tombstoned topic-c survived: %+v", d)
	}
}

func TestSyncPathSubscription(t *testing.T) {
	project := t.TempDir()
	peer := filepath.Join(project, "..", filepath.Base(project)+"-peer")
	if err := os.MkdirAll(peer, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(peer) })
	buildTree(t, peer)

	sub := config.SubscriptionConfig{Name: "local-peer", Path: peer}
	root, err := Sync(context.Background(), project, sub)
	if err != nil {
		t.Fatal(err)
	}
	if latest, err := ReadDecisions(root); err != nil || latest["topic-b"] == nil {
		t.Fatalf("read after path sync: latest=%v err=%v", latest, err)
	}
}

func TestSyncRejectsBadConfig(t *testing.T) {
	project := t.TempDir()
	for _, sub := range []config.SubscriptionConfig{
		{Name: "../evil", Path: project},
		{Name: "ok"},
		{Name: "ok", Path: project, URL: "https://example.com/x.git"},
	} {
		if _, err := Sync(context.Background(), project, sub); err == nil {
			t.Errorf("expected rejection for %+v", sub)
		}
	}
}

func TestSyncGitSubscription(t *testing.T) {
	source := t.TempDir()
	// Records live under context/ to exercise share-root discovery.
	buildTree(t, filepath.Join(source, "context"))
	repo, err := git.PlainInit(source, false)
	if err != nil {
		t.Fatal(err)
	}
	commitAll := func(msg string) {
		t.Helper()
		wt, err := repo.Worktree()
		if err != nil {
			t.Fatal(err)
		}
		if err := wt.AddWithOptions(&git.AddOptions{All: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := wt.Commit(msg, &git.CommitOptions{
			Author: &object.Signature{Name: "test", Email: "test@example.com", When: time.Now()},
		}); err != nil {
			t.Fatal(err)
		}
	}
	commitAll("initial context")

	project := t.TempDir()
	sub := config.SubscriptionConfig{Name: "team", URL: source}
	root, err := Sync(context.Background(), project, sub)
	if err != nil {
		t.Fatal(err)
	}
	latest, err := ReadDecisions(root)
	if err != nil || latest["topic-b"] == nil {
		t.Fatalf("read after clone: latest=%v err=%v", latest, err)
	}

	// A new decision in the source arrives on re-sync.
	writeDecision(t, filepath.Join(source, "context"),
		&memory.Decision{Topic: "topic-d", Decision: "fresh", CreatedAt: time.Now()})
	commitAll("add topic-d")
	root, err = Sync(context.Background(), project, sub)
	if err != nil {
		t.Fatal(err)
	}
	latest, err = ReadDecisions(root)
	if err != nil || latest["topic-d"] == nil {
		t.Fatalf("read after pull: latest=%v err=%v", latest, err)
	}

	// CachedRoot serves without network; EnsureFresh with a fresh stamp
	// must not need the source at all.
	if _, err := CachedRoot(project, sub); err != nil {
		t.Errorf("CachedRoot after sync: %v", err)
	}
	if _, err := EnsureFresh(context.Background(), project, sub, time.Hour); err != nil {
		t.Errorf("EnsureFresh with warm cache: %v", err)
	}
}
