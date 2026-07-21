package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/subscription"
)

// cmdSync fetches every configured subscription (or just the named ones)
// and reports what each peer currently publishes. Per-subscription failures
// are reported and do not stop the rest.
func cmdSync(dbPath string, args []string) error {
	if hasFlag(args, "--help") || hasFlag(args, "-h") {
		printSyncUsage()
		return nil
	}
	timeout := 60 * time.Second
	if v := parseFlag(args, "--timeout="); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("invalid --timeout: %w", err)
		}
		timeout = d
	}
	var names []string
	for _, a := range args {
		if len(a) > 0 && a[0] != '-' {
			names = append(names, a)
		}
	}

	subs := config.Get().Subscriptions
	if len(subs) == 0 {
		fmt.Println("No subscriptions configured. Add them to .aide/config/aide.json:")
		fmt.Println(`  { "subscriptions": [ { "name": "platform-team", "url": "git@host:platform/context.git" } ] }`)
		return nil
	}

	root := projectRoot(dbPath)
	var failed []error
	for _, sub := range subs {
		if len(names) > 0 && !slices.Contains(names, sub.Name) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		shareRoot, err := subscription.Sync(ctx, root, sub)
		cancel()
		if err != nil {
			fmt.Printf("✗ %s: %v\n", sub.Name, err)
			failed = append(failed, err)
			continue
		}
		latest, err := subscription.ReadDecisions(shareRoot)
		if err != nil {
			fmt.Printf("✗ %s: synced but unreadable: %v\n", sub.Name, err)
			failed = append(failed, err)
			continue
		}
		fmt.Printf("✓ %s: %d decision(s) available (%s)\n", sub.Name, len(latest), shareRoot)
	}
	return errors.Join(failed...)
}

func printSyncUsage() {
	fmt.Println(`aide sync - Fetch subscribed peer context

Usage:
  aide sync [name...] [--timeout=DURATION]

Fetches each configured subscription (git repositories into
.aide/cache/remotes/<name>/; local paths are validated in place) and
reports the decisions each peer publishes. Peer decisions appear in
session context as a read-only layer (origin peer:<name>), are never
re-exported, and sync only decisions — memories never leave a project.

Promote a peer decision into this project with:
  aide decision adopt <topic> --from=<name>

Subscriptions live in .aide/config/aide.json:
  { "subscriptions": [
      { "name": "platform-team", "url": "git@host:platform/context.git", "branch": "main" },
      { "name": "proto-repo",    "path": "../protos" }
  ] }`)
}
