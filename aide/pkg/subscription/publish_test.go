package subscription

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	git "github.com/go-git/go-git/v5"
	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/contextshare"
	"github.com/jmylchreest/aide/aide/pkg/memory"
)

// writeTopic returns a write callback that publishes one decision (plus
// the manifest) into the share root — write-once, so re-application after
// a retry reset is a no-op.
func writeTopic(t *testing.T, topic, value string, at time.Time) func(string) error {
	t.Helper()
	return func(root string) error {
		d := &memory.Decision{Topic: topic, Decision: value, CreatedAt: at}
		p := contextshare.DecisionPath(root, d)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, contextshare.MarshalDecision(d), 0o644); err != nil {
			return err
		}
		return contextshare.WriteManifest(root, at)
	}
}

// TestPublishGit covers the full loop against a bare remote: the first
// publish bootstraps an empty repository, a second publisher's records
// survive alongside the first's, and a content-free re-publish ships
// nothing.
func TestPublishGit(t *testing.T) {
	remote := t.TempDir()
	if _, err := git.PlainInit(remote, true); err != nil {
		t.Fatal(err)
	}
	sub := config.SubscriptionConfig{Name: "team", URL: remote, Publish: true}
	now := time.Now()

	projA := t.TempDir()
	pushed, err := Publish(context.Background(), projA, sub, writeTopic(t, "from-a", "a-rule", now))
	if err != nil {
		t.Fatalf("first publish into empty remote: %v", err)
	}
	if !pushed {
		t.Error("first publish reported nothing shipped")
	}

	projB := t.TempDir()
	if pushed, err = Publish(context.Background(), projB, sub, writeTopic(t, "from-b", "b-rule", now)); err != nil || !pushed {
		t.Fatalf("second publisher: pushed=%v err=%v", pushed, err)
	}

	// A fresh consumer sees both publishers' decisions.
	reader := t.TempDir()
	root, err := Sync(context.Background(), reader, config.SubscriptionConfig{Name: "team", URL: remote})
	if err != nil {
		t.Fatal(err)
	}
	latest, err := ReadDecisions(root)
	if err != nil {
		t.Fatal(err)
	}
	if latest["from-a"] == nil || latest["from-b"] == nil {
		t.Fatalf("consumer sees %v, want both from-a and from-b", latest)
	}

	// Re-publishing identical content (only the manifest watermark moves)
	// must not commit churn.
	if pushed, err = Publish(context.Background(), projA, sub, writeTopic(t, "from-a", "a-rule", now)); err != nil {
		t.Fatal(err)
	} else if pushed {
		t.Error("watermark-only re-publish shipped a commit")
	}
}

// TestPublishPath: a path subscription applies the write in place.
func TestPublishPath(t *testing.T) {
	dir := t.TempDir()
	proj := t.TempDir()
	sub := config.SubscriptionConfig{Name: "local", Path: dir, Publish: true}
	if _, err := Publish(context.Background(), proj, sub, writeTopic(t, "t", "v", time.Now())); err != nil {
		t.Fatal(err)
	}
	latest, err := ReadDecisions(dir)
	if err != nil || latest["t"] == nil {
		t.Fatalf("path publish unreadable: %v %v", latest, err)
	}
}

// TestPublishRequiresFlag: Publish refuses subscriptions not marked
// publish-enabled.
func TestPublishRequiresFlag(t *testing.T) {
	sub := config.SubscriptionConfig{Name: "team", URL: t.TempDir()}
	if _, err := Publish(context.Background(), t.TempDir(), sub, func(string) error { return nil }); err == nil {
		t.Error("expected refusal for non-publish subscription")
	}
}
