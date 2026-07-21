package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
)

// newSessionTestDB creates a project-shaped temp dir and returns its dbPath.
func newSessionTestDB(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	memDir := filepath.Join(root, ".aide", "memory")
	if err := os.MkdirAll(memDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	return filepath.Join(memDir, "memory.db")
}

// TestSessionEnd is the regression guard for the dead session-end write path:
// the session-end hooks spawn `aide session end --session=ID`, which for a
// long time did not exist as a subcommand and failed silently (detached spawn,
// stdio ignored). It asserts the command is dispatchable and performs
// teardown: session-end message, session-scoped state cleared, metrics
// recorded — while GLOBAL state survives, because other sessions sharing the
// store may still be live (the multi-instance clobber guard).
func TestSessionEnd(t *testing.T) {
	dbPath := newSessionTestDB(t)
	const sessionID = "sess-123"
	const otherSession = "sess-other"

	// Seed: session-scoped keys for the ending session, the same keys for a
	// concurrent session, and bare global spellings (sessionless writers).
	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatalf("NewBackend (seed): %v", err)
	}
	for _, key := range sessionStateKeys {
		if err := b.SetState(key, "x", sessionID); err != nil {
			t.Fatalf("seed session state %q: %v", key, err)
		}
		if err := b.SetState(key, "y", otherSession); err != nil {
			t.Fatalf("seed other-session state %q: %v", key, err)
		}
	}
	if err := b.SetState("mode", "eco", ""); err != nil {
		t.Fatalf("seed global state: %v", err)
	}
	b.Close()

	// Exercise through the real dispatch, exactly as the hook invokes it.
	if err := cmdSession(dbPath, []string{"end", "--session=" + sessionID, "--duration=45000"}); err != nil {
		t.Fatalf("session end: %v", err)
	}

	b2, err := NewBackend(dbPath)
	if err != nil {
		t.Fatalf("NewBackend (verify): %v", err)
	}
	defer b2.Close()

	for _, key := range sessionStateKeys {
		if st, err := b2.GetState(key, sessionID); err == nil && st != nil {
			t.Errorf("session-scoped state %q survived session end (value %q)", key, st.Value)
		}
		if st, err := b2.GetState(key, otherSession); err != nil || st == nil || st.Value != "y" {
			t.Errorf("concurrent session's state %q was clobbered (state=%v err=%v)", key, st, err)
		}
	}
	if st, err := b2.GetState("mode", ""); err != nil || st == nil || st.Value != "eco" {
		t.Errorf("global state was clobbered by session end (state=%v err=%v)", st, err)
	}

	if st, err := b2.GetState("last_session_end", ""); err != nil || st == nil || st.Value == "" {
		t.Errorf("last_session_end not recorded (state=%v err=%v)", st, err)
	}
	if st, err := b2.GetState("last_session_duration", ""); err != nil || st == nil || st.Value != "45000" {
		t.Errorf("last_session_duration not recorded (state=%v err=%v)", st, err)
	}

	msgs, err := b2.ListMessages("")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	found := false
	for _, m := range msgs {
		if m.From == "system" && strings.Contains(m.Content, "Session "+sessionID+" ended (45s)") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("session-end system message not found in %d messages", len(msgs))
	}
}

func TestSessionEndValidation(t *testing.T) {
	dbPath := newSessionTestDB(t)

	if err := sessionEnd(dbPath, nil); err == nil {
		t.Error("expected error without --session")
	}

	// A bad --duration must be reported but must NOT abort teardown — the
	// hook spawns detached with stderr discarded, so aborting would
	// reintroduce the silent dead-write-path for that input class.
	for _, bad := range []string{"abc", "-5", "1e+21", "Infinity"} {
		if err := sessionEnd(dbPath, []string{"--session=s1", "--duration=" + bad}); err == nil {
			t.Errorf("expected error for --duration=%s", bad)
		}
	}
	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	defer b.Close()
	if st, err := b.GetState("last_session_end", ""); err != nil || st == nil || st.Value == "" {
		t.Errorf("teardown did not run despite bad --duration (state=%v err=%v)", st, err)
	}
	if st, err := b.GetState("last_session_duration", ""); err == nil && st != nil {
		t.Errorf("bad --duration recorded as metric (value %q)", st.Value)
	}
}

// TestSessionEndRoundsSeconds pins the human-readable suffix to rounding
// (matching the original TS cleanupSession's Math.round), not truncation.
func TestSessionEndRoundsSeconds(t *testing.T) {
	dbPath := newSessionTestDB(t)

	if err := cmdSession(dbPath, []string{"end", "--session=r1", "--duration=45900"}); err != nil {
		t.Fatalf("session end: %v", err)
	}

	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	defer b.Close()
	msgs, err := b.ListMessages("")
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	found := false
	for _, m := range msgs {
		if strings.Contains(m.Content, "Session r1 ended (46s)") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected rounded '(46s)' suffix for 45900ms in %d messages", len(msgs))
	}
}

// TestSessionEndWithoutSeededState asserts teardown is clean on an empty
// store — deleting absent keys and clearing absent agent state must not fail
// (the hook fires for sessions that never wrote state).
func TestSessionEndWithoutSeededState(t *testing.T) {
	dbPath := newSessionTestDB(t)

	if err := cmdSession(dbPath, []string{"end", "--session=empty-sess"}); err != nil {
		t.Fatalf("session end on empty store: %v", err)
	}

	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	defer b.Close()
	if st, err := b.GetState("last_session_end", ""); err != nil || st == nil || st.Value == "" {
		t.Errorf("last_session_end not recorded (state=%v err=%v)", st, err)
	}
	// No --duration given: the metric must not be invented.
	if st, err := b.GetState("last_session_duration", ""); err == nil && st != nil {
		t.Errorf("last_session_duration recorded without --duration (value %q)", st.Value)
	}
}

// TestSessionEndDeletesAnchor: session end must remove the session's
// anchor cache entries from every candidate location (XDG runtime dir and
// home fallback), so only live sessions keep cache files.
func TestSessionEndDeletesAnchor(t *testing.T) {
	dbPath := newSessionTestDB(t)
	const sessionID = "anchor-sess"

	home := t.TempDir()
	xdg := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_RUNTIME_DIR", xdg)

	for _, dir := range []string{
		filepath.Join(xdg, "aide", "anchors"),
		filepath.Join(home, ".aide", "anchors"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, sessionID+".json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		// A different session's entry must survive.
		if err := os.WriteFile(filepath.Join(dir, "other-sess.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	if err := cmdSession(dbPath, []string{"end", "--session=" + sessionID}); err != nil {
		t.Fatalf("session end: %v", err)
	}

	for _, dir := range []string{
		filepath.Join(xdg, "aide", "anchors"),
		filepath.Join(home, ".aide", "anchors"),
	} {
		if _, err := os.Stat(filepath.Join(dir, sessionID+".json")); err == nil {
			t.Errorf("anchor entry survived session end in %s", dir)
		}
		if _, err := os.Stat(filepath.Join(dir, "other-sess.json")); err != nil {
			t.Errorf("other session's anchor entry was deleted in %s", dir)
		}
	}
}

// TestRetentionSweep asserts the direct-mode sweep runs, stamps its
// rate-limit key, and skips within the minimum interval.
func TestRetentionSweep(t *testing.T) {
	dbPath := newSessionTestDB(t)
	config.Set(&config.Config{Cleanup: config.CleanupConfig{Enabled: true}})
	t.Cleanup(func() { config.Set(&config.Config{}) })

	b, err := NewBackend(dbPath)
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	defer b.Close()

	// First run must stamp the rate-limit key.
	_ = b.RetentionSweep()
	st, err := b.GetState(lastRetentionSweepKey, "")
	if err != nil || st == nil || st.Value == "" {
		t.Fatalf("retention sweep did not stamp %s (state=%v err=%v)", lastRetentionSweepKey, st, err)
	}

	// Backdate inside the window: sweep must skip and leave the stamp alone.
	within := time.Now().UTC().Add(-30 * time.Minute).Format(time.RFC3339)
	if err := b.SetState(lastRetentionSweepKey, within, ""); err != nil {
		t.Fatalf("backdate stamp: %v", err)
	}
	_ = b.RetentionSweep()
	if st, _ := b.GetState(lastRetentionSweepKey, ""); st == nil || st.Value != within {
		t.Errorf("sweep ran inside the rate-limit window (stamp %v)", st)
	}

	// Backdate beyond the window: sweep must run and re-stamp.
	past := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if err := b.SetState(lastRetentionSweepKey, past, ""); err != nil {
		t.Fatalf("backdate stamp: %v", err)
	}
	_ = b.RetentionSweep()
	if st, _ := b.GetState(lastRetentionSweepKey, ""); st == nil || st.Value == past {
		t.Errorf("sweep did not run after the rate-limit window elapsed (stamp %v)", st)
	}
}
