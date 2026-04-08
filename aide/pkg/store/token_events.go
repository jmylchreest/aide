package store

import (
	"encoding/json"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/oklog/ulid/v2"
	bolt "go.etcd.io/bbolt"
)

// AddTokenEvent records a token event. If ID is empty, a ULID is generated.
func (s *BoltStore) AddTokenEvent(e *memory.TokenEvent) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTokenEvents)
		if e.ID == "" {
			e.ID = ulid.Make().String()
		}
		if e.Timestamp.IsZero() {
			e.Timestamp = time.Now()
		}
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		return b.Put([]byte(e.ID), data)
	})
}

// ListTokenEvents returns token events, optionally filtered by session ID and time range.
// Results are returned in reverse chronological order (newest first).
// If limit <= 0, all matching events are returned.
// Zero-value since/until are ignored (no bound).
func (s *BoltStore) ListTokenEvents(sessionID string, limit int, since, until time.Time) ([]*memory.TokenEvent, error) {
	var events []*memory.TokenEvent

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTokenEvents)
		c := b.Cursor()

		// ULIDs are time-ordered, so iterate in reverse for newest-first.
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var e memory.TokenEvent
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			if !since.IsZero() && e.Timestamp.Before(since) {
				break // ULIDs are time-ordered; no older events will match
			}
			if !until.IsZero() && e.Timestamp.After(until) {
				continue
			}
			if sessionID != "" && e.SessionID != sessionID {
				continue
			}
			events = append(events, &e)
			if limit > 0 && len(events) >= limit {
				break
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return events, nil
}

// TokenStats computes aggregate statistics from token events.
// If sessionID is non-empty, only events from that session are included.
// Zero-value since/until are ignored (no bound).
func (s *BoltStore) TokenStats(sessionID string, since, until time.Time) (*memory.TokenStats, error) {
	stats := &memory.TokenStats{
		ByTool:       make(map[string]int),
		BySavingType: make(map[string]int),
	}
	sessions := make(map[string]bool)

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTokenEvents)
		c := b.Cursor()

		// Iterate newest-first so we can break early on since bound.
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var e memory.TokenEvent
			if err := json.Unmarshal(v, &e); err != nil {
				continue // skip malformed
			}
			if !since.IsZero() && e.Timestamp.Before(since) {
				break // ULIDs are time-ordered; everything older is out of range
			}
			if !until.IsZero() && e.Timestamp.After(until) {
				continue
			}
			if sessionID != "" && e.SessionID != sessionID {
				continue
			}

			stats.EventCount++
			sessions[e.SessionID] = true

			switch e.EventType {
			case memory.TokenEventRead:
				stats.TotalRead += e.Tokens
				stats.ByTool[e.Tool] += e.Tokens
			case memory.TokenEventOutlineUsed:
				stats.TotalRead += e.Tokens
				stats.TotalSaved += e.TokensSaved
				stats.ByTool[e.Tool] += e.Tokens
				stats.BySavingType["outline"] += e.TokensSaved
			case memory.TokenEventReadAvoided:
				stats.TotalSaved += e.TokensSaved
				stats.BySavingType["read_avoided"] += e.TokensSaved
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
