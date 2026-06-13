package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/jmylchreest/aide/aide/pkg/config"
	"github.com/jmylchreest/aide/aide/pkg/store"
	bolt "go.etcd.io/bbolt"
)

// storeCompactPaths returns the bolt files backing every store, derived
// deterministically from the primary db path. The files need not exist —
// CompactClosedDB no-ops on a missing one.
func storeCompactPaths(dbPath string) []string {
	indexPath, _ := getCodeStorePaths(dbPath)
	return []string{
		dbPath, // primary: memories, decisions, state, tasks, messages, observe, ...
		indexPath,
		filepath.Join(getFindingsStorePath(dbPath), "findings.db"),
		filepath.Join(getSurveyStorePath(dbPath), "survey.db"),
	}
}

// compactStoresOnExit compacts every bolt store, reclaiming the free pages
// bbolt accumulates but never returns to the OS. It is meant to run as the LAST
// deferred action of a long-lived store owner (the daemon or the MCP server) —
// register it before any store is opened so every store's Close has already run
// by the time it executes.
//
// Gated by maintenance.compact_on_exit (default true). All output goes to
// stderr: the MCP server speaks JSON-RPC over stdout, so a stray stdout write
// would corrupt the protocol.
func compactStoresOnExit(dbPath string) {
	if !config.Get().Maintenance.CompactOnExit {
		return
	}
	for _, p := range storeCompactPaths(dbPath) {
		res, err := store.CompactClosedDB(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WARNING: compact %s: %v\n", filepath.Base(p), err)
			continue
		}
		if res.Compacted {
			fmt.Fprintf(os.Stderr, "compacted %s: reclaimed %s (%s -> %s)\n",
				filepath.Base(p), formatBytes(res.Reclaimed), formatBytes(res.Before), formatBytes(res.After))
		}
	}
}

// cmdMaintenance is the `aide maintenance` command group.
func cmdMaintenance(dbPath string, args []string) error {
	sub := ""
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "compact":
		return cmdMaintenanceCompact(dbPath, args[1:])
	case "", "-h", "--help", "help":
		printMaintenanceUsage()
		return nil
	default:
		return fmt.Errorf("unknown maintenance subcommand %q (try: compact)", sub)
	}
}

// cmdMaintenanceCompact compacts the bolt stores on demand. Unlike the
// automatic on-exit pass, it reports every store's outcome — including ones in
// use by a running aide process, which cannot be compacted while locked.
func cmdMaintenanceCompact(dbPath string, _ []string) error {
	fmt.Println("Compacting bolt stores...")
	var totalReclaimed int64
	var anyLocked bool

	for _, p := range storeCompactPaths(dbPath) {
		name := filepath.Base(p)
		res, err := store.CompactClosedDB(p)
		switch {
		case errors.Is(err, bolt.ErrTimeout):
			anyLocked = true
			fmt.Printf("  %-14s in use — skipped\n", name+":")
		case err != nil:
			fmt.Printf("  %-14s error: %v\n", name+":", err)
		case res.Compacted:
			totalReclaimed += res.Reclaimed
			fmt.Printf("  %-14s reclaimed %s (%s -> %s)\n",
				name+":", formatBytes(res.Reclaimed), formatBytes(res.Before), formatBytes(res.After))
		case res.Before == 0:
			fmt.Printf("  %-14s not present\n", name+":")
		default:
			fmt.Printf("  %-14s already compact (%s)\n", name+":", formatBytes(res.Before))
		}
	}

	fmt.Printf("\nTotal reclaimed: %s\n", formatBytes(totalReclaimed))
	if anyLocked {
		fmt.Println("\nFiles in use were skipped — a daemon or MCP server holds them open.")
		fmt.Println("They are compacted automatically when that process exits, unless")
		fmt.Println("maintenance.compact_on_exit=false. Stop it first to compact them now.")
	}
	return nil
}

func printMaintenanceUsage() {
	fmt.Println(`aide maintenance - On-disk upkeep of the bolt stores

Usage:
  aide maintenance compact    Rewrite each store to reclaim free pages

bbolt never returns freed pages to the OS, so a store file only shrinks when
rewritten. Compaction also runs automatically when the daemon or MCP server
exits; disable that with maintenance.compact_on_exit=false (or
AIDE_MAINTENANCE_COMPACT_ON_EXIT=0).

A store held open by a running aide process cannot be compacted on demand; stop
that process first, or rely on the automatic on-exit pass.`)
}
