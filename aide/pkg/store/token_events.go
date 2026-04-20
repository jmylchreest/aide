package store

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
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

// ListTokenEvents returns token events from BOTH the legacy token_events
// bucket and the new observe_events bucket (translated). Results merged in
// reverse chronological order (newest first). Zero-value since/until = no bound.
// Limit <= 0 returns all matches.
func (s *BoltStore) ListTokenEvents(sessionID string, limit int, since, until time.Time) ([]*memory.TokenEvent, error) {
	var events []*memory.TokenEvent
	keep := func(e *memory.TokenEvent) bool {
		if !since.IsZero() && e.Timestamp.Before(since) {
			return false
		}
		if !until.IsZero() && e.Timestamp.After(until) {
			return false
		}
		if sessionID != "" && e.SessionID != sessionID {
			return false
		}
		return true
	}

	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketTokenEvents)
		c := b.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var e memory.TokenEvent
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			if !since.IsZero() && e.Timestamp.Before(since) {
				break
			}
			if keep(&e) {
				events = append(events, &e)
			}
		}

		ob := tx.Bucket(BucketObserveEvents)
		oc := ob.Cursor()
		for k, v := oc.Last(); k != nil; k, v = oc.Prev() {
			var oe observe.Event
			if err := json.Unmarshal(v, &oe); err != nil {
				continue
			}
			if !since.IsZero() && oe.Timestamp.Before(since) {
				break
			}
			te := observeToTokenEvent(&oe)
			if te != nil && keep(te) {
				events = append(events, te)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].Timestamp.After(events[j].Timestamp)
	})
	if limit > 0 && len(events) > limit {
		events = events[:limit]
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
		b := tx.Bucket(BucketTokenEvents)
		c := b.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var e memory.TokenEvent
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			if !since.IsZero() && e.Timestamp.Before(since) {
				break
			}
			tally(&e)
		}

		ob := tx.Bucket(BucketObserveEvents)
		oc := ob.Cursor()
		for k, v := oc.Last(); k != nil; k, v = oc.Prev() {
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
