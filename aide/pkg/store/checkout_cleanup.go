package store

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/checkout"
)

const CheckoutGracePeriod = 7 * 24 * time.Hour

// PruneCheckouts requires ownership of the shared memory DB. closeOwner waits
// for active requests/watchers and closes all cache handles before removal.
// Git registrations (including locked/unmounted worktrees) always win over GC.
func PruneCheckouts(dbPath string, shared Store, now time.Time, pinned map[string]bool, closeOwner func(checkout.Info) error) (int, error) {
	base := filepath.Join(filepath.Dir(dbPath), "checkouts")
	entries, err := os.ReadDir(base)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		id, err := hex.DecodeString(e.Name())
		if err != nil || len(id) != 16 || !e.IsDir() || pinned[e.Name()] {
			continue
		}
		dir := filepath.Join(base, e.Name())
		data, err := os.ReadFile(filepath.Join(dir, "checkout.json"))
		if err != nil {
			continue
		}
		var c checkout.Info
		if json.Unmarshal(data, &c) != nil || c.ID != e.Name() || !filepath.IsAbs(c.Root) {
			continue
		}
		marker := filepath.Join(dir, "orphaned-at")
		missing := func(p string) bool { _, err := os.Lstat(p); return os.IsNotExist(err) }
		if !missing(c.Root) || c.GitDir == "" || !missing(c.GitDir) {
			_ = os.Remove(marker)
			continue
		}
		data, err = os.ReadFile(marker)
		if os.IsNotExist(err) {
			if err := os.WriteFile(marker, []byte(now.UTC().Format(time.RFC3339Nano)), 0600); err != nil {
				return removed, err
			}
			continue
		}
		if err != nil {
			continue
		}
		since, err := time.Parse(time.RFC3339Nano, string(data))
		if err != nil || now.Sub(since) < CheckoutGracePeriod {
			continue
		}
		if closeOwner != nil {
			if err := closeOwner(c); err != nil {
				return removed, err
			}
		}
		// Recheck after waiting for owners: registrations may have been restored.
		if !missing(c.Root) || !missing(c.GitDir) {
			_ = os.Remove(marker)
			continue
		}
		if err := archiveFindingsFile(filepath.Join(dir, "findings", "findings.db"), shared, c); err != nil {
			return removed, fmt.Errorf("preserve checkout %s acceptance: %w", c.ID, err)
		}
		if err := os.RemoveAll(dir); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}
