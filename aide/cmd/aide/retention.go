package main

import (
	"context"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/store"
)

// runCleanupLoop periodically prunes the buckets that don't self-clean:
// agent-specific state (cleared via SubagentStop but lingers when sessions
// crash) and observe events (high-volume, no per-write TTL). Messages
// already self-prune on read.
//
// logf must be stderr-backed: the only caller is the MCP primary, whose
// stdout is the JSON-RPC channel, and any stray print corrupts the protocol.
//
// Returns when ctx is cancelled. Errors are logged but never fatal — the
// primary should never die from a cleanup hiccup.
func runCleanupLoop(ctx context.Context, st store.Store, logf func(format string, args ...any)) {
	cfg := config.Get().Cleanup
	if !cfg.Enabled {
		logf("cleanup loop disabled (cleanup.enabled=false)\n")
		return
	}

	interval := cfg.IntervalDuration()
	stateAge := cfg.StateMaxAgeDuration()
	observeAge := cfg.ObserveMaxAgeDuration()
	taskAge := cfg.TaskMaxAgeDuration()
	tokenAge := cfg.TokenMaxAgeDuration()
	logf("cleanup loop: every %s — state>%s, observe>%s, done-tasks>%s, token-events>%s\n", interval, stateAge, observeAge, taskAge, tokenAge)

	// Stagger the first run by a few seconds so startup logs stay
	// readable. After that, tick on the configured interval.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		counts, errs := retentionSweepOnce(st, cfg)
		for _, name := range retentionBuckets {
			if err := errs[name]; err != nil {
				logf("cleanup: %s error: %v\n", name, err)
			} else if n := counts[name]; n > 0 {
				logf("cleanup: pruned %d %s\n", n, name)
			}
		}

		timer.Reset(interval)
	}
}

// retentionBuckets names the time-based buckets the retention sweep covers,
// in reporting order. Memories and decisions are deliberately absent — they
// are knowledge, not telemetry, and are never retention-pruned.
var retentionBuckets = []string{
	"stale state entries",
	"stale observe events",
	"expired messages",
	"completed tasks",
	"token events",
}

// retentionSweepOnce runs one retention pass across all time-based buckets,
// using the configured per-bucket max ages (default 90d; "0" disables a
// bucket). Shared by the MCP primary's cleanup loop and the direct-mode
// sweep at session init, so retention holds whether or not a primary is up.
func retentionSweepOnce(st store.Store, cfg config.CleanupConfig) (map[string]int, map[string]error) {
	counts := map[string]int{}
	errs := map[string]error{}
	run := func(name string, fn func() (int, error)) {
		if n, err := fn(); err != nil {
			errs[name] = err
		} else if n > 0 {
			counts[name] = n
		}
	}
	run("stale state entries", func() (int, error) { return st.CleanupStaleState(cfg.StateMaxAgeDuration()) })
	run("stale observe events", func() (int, error) { return st.CleanupObserveEvents(cfg.ObserveMaxAgeDuration()) })
	run("expired messages", func() (int, error) { return st.PruneMessages() })
	run("completed tasks", func() (int, error) { return st.PruneCompletedTasks(cfg.TaskMaxAgeDuration()) })
	run("token events", func() (int, error) { return st.CleanupTokenEvents(cfg.TokenMaxAgeDuration()) })
	return counts, errs
}
