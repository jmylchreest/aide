package store

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	bolt "go.etcd.io/bbolt"
)

// ListTokenEvents returns events from observe_events translated into the
// legacy TokenEvent shape. The legacy token_events bucket is migrated into
// observe_events at daemon startup so this is the single source of truth.
// Newest-first; zero-value since/until = no bound; limit <= 0 = all.
func (s *BoltStore) ListTokenEvents(sessionID string, limit int, since, until time.Time) ([]*memory.TokenEvent, error) {
	var events []*memory.TokenEvent
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketObserveEvents)
		c := b.Cursor()
		attribution := newWorkAttribution()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var oe observe.Event
			if json.Unmarshal(v, &oe) == nil {
				attribution.observe(&oe)
			}
		}
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var oe observe.Event
			if err := json.Unmarshal(v, &oe); err != nil {
				continue
			}
			oe = *attribution.project(&oe)
			if !since.IsZero() && oe.Timestamp.Before(since) {
				continue
			}
			if !until.IsZero() && oe.Timestamp.After(until) {
				continue
			}
			if sessionID != "" && oe.SessionID != sessionID {
				continue
			}
			te := observeToTokenEvent(&oe)
			if te == nil {
				continue
			}
			events = append(events, te)
		}
		return nil
	})
	sort.Slice(events, func(i, j int) bool {
		if events[i].Timestamp.Equal(events[j].Timestamp) {
			return events[i].ID > events[j].ID
		}
		return events[i].Timestamp.After(events[j].Timestamp)
	})
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	return events, err
}

// TokenStats computes aggregate statistics from token events.
// If sessionID is non-empty, only events from that session are included.
// Zero-value since/until are ignored (no bound).
func (s *BoltStore) TokenStats(sessionID string, since, until time.Time) (*memory.TokenStats, error) {
	stats := &memory.TokenStats{
		Accounting:   memory.NewTokenAccounting(),
		ByTool:       make(map[string]int),
		CallsByTool:  make(map[string]int),
		SavedByTool:  make(map[string]int),
		BySavingType: make(map[string]int),
		ByDelivery:   make(map[string]int),
	}
	sessions := make(map[string]bool)
	activity := newTokenActivity(since, until)
	retrievals := newTokenRetrievals(sessionID, since, until)
	stats.Accounting.Work = memory.NewTokenWork()
	usage := newModelUsage()

	tally := func(e *memory.TokenEvent) {
		if !since.IsZero() && e.Timestamp.Before(since) {
			return
		}
		if !until.IsZero() && e.Timestamp.After(until) {
			return
		}
		if sessionID != "" && e.SessionID != sessionID {
			return
		}

		stats.EventCount++
		if e.SessionID != "" {
			sessions[e.SessionID] = true
		}
		stats.Accounting.Add(e)
		if e.EventType == "transformation" {
			return
		}
		activity.add(e)

		// Every event with a tool counts as one call. Injection events
		// reuse Tool for the source name; the chart filters those out.
		if e.Tool != "" && e.EventType != memory.TokenEventContextInjected {
			stats.CallsByTool[e.Tool]++
			if e.TokensSaved > 0 {
				stats.SavedByTool[e.Tool] += e.TokensSaved
			}
		}

		switch e.EventType {
		case memory.TokenEventRead:
			stats.TotalRead += e.Tokens
			stats.ByTool[e.Tool] += e.Tokens
			stats.ReadCount++
		case memory.TokenEventOutlineUsed:
			stats.TotalRead += e.Tokens
			stats.TotalSaved += e.TokensSaved
			stats.ByTool[e.Tool] += e.Tokens
			stats.BySavingType["outline"] += e.TokensSaved
			stats.CodeToolCount++
		case memory.TokenEventSymbolRead:
			stats.TotalRead += e.Tokens
			stats.TotalSaved += e.TokensSaved
			stats.ByTool[e.Tool] += e.Tokens
			stats.BySavingType["symbol_read"] += e.TokensSaved
			stats.CodeToolCount++
		case memory.TokenEventReadAvoided:
			stats.TotalSaved += e.TokensSaved
			stats.BySavingType["read_avoided"] += e.TokensSaved
		case memory.TokenEventContextInjected:
			stats.TotalDelivered += e.Tokens
			if e.Tool != "" {
				stats.ByDelivery[e.Tool] += e.Tokens
			}
		case "modify", "write", "edit":
			if e.Attrs["accounting_version"] == "1" {
				if n, ok := memory.MeasuredBytes(e.Attrs, "argument_bytes"); ok {
					stats.TotalWritten += int(memory.EstimateTextTokens(n))
				}
				stats.TotalRead += e.Tokens
			} else {
				stats.TotalWritten += e.Tokens
			}
			stats.ByTool[e.Tool] += e.Tokens
		default:
			stats.TotalRead += e.Tokens
			stats.ByTool[e.Tool] += e.Tokens
		}
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketObserveEvents)
		c := b.Cursor()
		attribution := newWorkAttribution()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var oe observe.Event
			if json.Unmarshal(v, &oe) == nil {
				attribution.observe(&oe)
				usage.observe(&oe)
			}
		}
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var oe observe.Event
			if err := json.Unmarshal(v, &oe); err != nil {
				continue
			}
			projected := attribution.project(&oe)
			retrievals.add(projected)
			addTokenWork(stats.Accounting.Work, projected, sessionID, since, until)
			if te := observeToTokenEvent(projected); te != nil {
				tally(te)
			}
		}
		if len(retrievals.windows) == 0 {
			return nil
		}
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var oe observe.Event
			if json.Unmarshal(v, &oe) == nil {
				retrievals.context(&oe)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	modelUsage, usageSessions := usage.resultWithSessions(sessionID, since, until)
	for session := range usageSessions {
		sessions[session] = true
	}
	stats.Sessions = len(sessions)
	stats.Accounting.ModelUsage = modelUsage
	stats.Accounting.Activity = activity.result()
	stats.Accounting.Retrievals = retrievals.result()
	return stats, nil
}

// CleanupTokenEvents removes events older than maxAge. Returns the count of deleted events.
func (s *BoltStore) CleanupTokenEvents(maxAge time.Duration) (int, error) {
	if maxAge <= 0 {
		return 0, nil // 0 (or less) disables pruning: retain token events forever
	}
	cutoff := time.Now().Add(-maxAge)
	var keysToDelete [][]byte

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTokenEvents)
		return b.ForEach(func(k, v []byte) error {
			var e memory.TokenEvent
			if err := json.Unmarshal(v, &e); err != nil {
				// Delete malformed entries too
				keysToDelete = append(keysToDelete, append([]byte{}, k...))
				return nil
			}
			if e.Timestamp.Before(cutoff) {
				keysToDelete = append(keysToDelete, append([]byte{}, k...))
			}
			return nil
		})
	})
	if err != nil {
		return 0, err
	}

	if len(keysToDelete) == 0 {
		return 0, nil
	}

	var count int
	err = s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTokenEvents)
		for _, k := range keysToDelete {
			if err := b.Delete(k); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}
