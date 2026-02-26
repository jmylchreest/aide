package store

import (
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// TestCLIAccessWhileMCPServerRunning simulates the real-world scenario:
//   - MCP server has the database open (long-lived connection via BoltStore)
//   - 50+ concurrent CLI commands try to open the SAME database file directly
//     (simulating the case where gRPC socket doesn't exist or gRPC connection fails)
//
// This reproduces the timeout issue when skills run multiple aide CLI commands
// concurrently and they all fall back to direct DB access, competing for the file lock.
func TestCLIAccessWhileMCPServerRunning(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "memory.db")

	// Step 1: Open database like the MCP server does (keep it open)
	mcpDB, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		t.Fatalf("failed to open MCP server DB: %v", err)
	}
	defer mcpDB.Close()

	// Initialize bucket
	err = mcpDB.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("test"))
		return err
	})
	if err != nil {
		t.Fatalf("failed to create bucket: %v", err)
	}

	// Step 2: Launch 50 concurrent "CLI commands"
	numCLICommands := 50
	var wg sync.WaitGroup

	var timeoutCount atomic.Int32
	var successCount atomic.Int32
	var maxDelay atomic.Int64 // in nanoseconds
	var totalDelay atomic.Int64

	t.Logf("Launching %d concurrent CLI commands while MCP server holds DB...", numCLICommands)

	for i := 0; i < numCLICommands; i++ {
		wg.Add(1)
		go func(cmdID int) {
			defer wg.Done()

			// Simulate "aide state set key-N value-N"
			start := time.Now()

			// Try to open the database (this competes with MCP server)
			cliDB, err := bolt.Open(dbPath, 0600, &bolt.Options{Timeout: 3 * time.Second})
			if err != nil {
				timeoutCount.Add(1)
				t.Logf("✗ CLI %d: timeout after %v", cmdID, time.Since(start))
				return
			}
			defer cliDB.Close()

			// Perform a write
			err = cliDB.Update(func(tx *bolt.Tx) error {
				bucket := tx.Bucket([]byte("test"))
				if bucket == nil {
					return fmt.Errorf("bucket not found")
				}
				return bucket.Put([]byte(fmt.Sprintf("key-%d", cmdID)), []byte(fmt.Sprintf("value-%d", cmdID)))
			})

			elapsed := time.Since(start)

			if err != nil {
				t.Logf("✗ CLI %d: write failed after %v: %v", cmdID, elapsed, err)
				timeoutCount.Add(1)
				return
			}

			// Track statistics
			successCount.Add(1)
			delayNs := elapsed.Nanoseconds()
			totalDelay.Add(delayNs)

			// Update max delay atomically
			for {
				currentMax := maxDelay.Load()
				if delayNs <= currentMax || maxDelay.CompareAndSwap(currentMax, delayNs) {
					break
				}
			}

			if elapsed > 1*time.Second {
				t.Logf("⚠ CLI %d: slow but succeeded in %v", cmdID, elapsed)
			}
		}(i)

		// Small stagger to increase contention
		time.Sleep(2 * time.Millisecond)
	}

	wg.Wait()

	// Report statistics
	successes := successCount.Load()
	timeouts := timeoutCount.Load()
	peak := time.Duration(maxDelay.Load())
	avg := time.Duration(0)
	if successes > 0 {
		avg = time.Duration(totalDelay.Load() / int64(successes))
	}

	t.Logf("\n=== Results ===")
	t.Logf("Total commands:  %d", numCLICommands)
	t.Logf("Succeeded:       %d (%.1f%%)", successes, float64(successes)/float64(numCLICommands)*100)
	t.Logf("Timed out:       %d (%.1f%%)", timeouts, float64(timeouts)/float64(numCLICommands)*100)
	t.Logf("Peak delay:      %v", peak)
	t.Logf("Average delay:   %v", avg)

	// Note: Some timeouts are expected in this stress test because:
	// 1. MCP server holds DB connection open continuously
	// 2. BBolt only allows one writer at a time
	// 3. All 50 CLI commands compete for the lock simultaneously
	//
	// In real usage, CLI commands should use gRPC when MCP server is running
	// (see cmd/aide/backend.go NewBackend). This test simulates the failure
	// case when gRPC socket doesn't exist or connection fails.
	if timeouts > int32(numCLICommands/2) {
		t.Logf("NOTE: %d%% timeout rate is expected when all CLI commands compete for DB lock",
			timeouts*100/int32(numCLICommands))
		t.Logf("In production, CLI should use gRPC when MCP server is running")
	}

	if peak > 2*time.Second {
		t.Logf("WARNING: Peak delay of %v suggests high contention", peak)
	}
}
