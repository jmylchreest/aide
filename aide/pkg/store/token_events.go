package store

import (
	"encoding/json"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/oklog/ulid/v2"
	bolt "go.etcd.io/bbolt"
)

// AddTokenEvent records a token event. Routes through the unified observe
// store — the legacy token_events bucket is no longer written. The
// translation is the inverse of observeToTokenEvent, so subsequent reads via
// ListTokenEvents/TokenStats see this entry unchanged.
func (s *BoltStore) AddTokenEvent(e *memory.TokenEvent) error {
	if e.ID == "" {
		e.ID = ulid.Make().String()
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	ev := tokenEventToObserve(e)
	if ev == nil {
		return nil
	}
	return s.AddObserveEvent(ev)
}

// ListTokenEvents returns events from observe_events translated into the
// legacy TokenEvent shape. The legacy token_events bucket is migrated into
// observe_events at daemon startup so this is the single source of truth.
// Newest-first; zero-value since/until = no bound; limit <= 0 = all.
func (s *BoltStore) ListTokenEvents(sessionID string, limit int, since, until time.Time) ([]*memory.TokenEvent, error) {
	var events []*memory.TokenEvent
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketObserveEvents)
		c := b.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var oe observe.Event
			if err := json.Unmarshal(v, &oe); err != nil {
				continue
			}
			if !since.IsZero() && oe.Timestamp.Before(since) {
				break
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
			if limit > 0 && len(events) >= limit {
				break
			}
		}
		return nil
	})
	return events, err
}

// TokenStats computes aggregate statistics from token events.
// If sessionID is non-empty, only events from that session are included.
// Zero-value since/until are ignored (no bound).
func (s *BoltStore) TokenStats(sessionID string, since, until time.Time) (*memory.TokenStats, error) {
	stats := &memory.TokenStats{
		ByTool:       make(map[string]int),
		BySavingType: make(map[string]int),
		ByDelivery:   make(map[string]int),
	}
	sessions := make(map[string]bool)

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
		sessions[e.SessionID] = true

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
		}
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketObserveEvents)
		c := b.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var oe observe.Event
			if err := json.Unmarshal(v, &oe); err != nil {
				continue
			}
			if !since.IsZero() && oe.Timestamp.Before(since) {
				break
			}
			if te := observeToTokenEvent(&oe); te != nil {
				tally(te)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	stats.Sessions = len(sessions)
	return stats, nil
}

// CleanupTokenEvents removes events older than maxAge. Returns the count of deleted events.
func (s *BoltStore) CleanupTokenEvents(maxAge time.Duration) (int, error) {
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
