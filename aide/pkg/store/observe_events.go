package store

import (
	"encoding/json"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	bolt "go.etcd.io/bbolt"
)

// observeToTokenEvent maps an observe.Event into the legacy TokenEvent shape
// where it carries cost data the old token UI/CLI consumes. Returns nil for
// events that have no equivalent (e.g., generic spans, hook lifecycle).
func observeToTokenEvent(e *observe.Event) *memory.TokenEvent {
	var eventType string
	switch e.Kind {
	case observe.KindToolCall:
		switch e.Subtype {
		case "outline":
			eventType = memory.TokenEventOutlineUsed
		case "symbol":
			eventType = memory.TokenEventSymbolRead
		case "file":
			eventType = memory.TokenEventRead
		default:
			if e.Tokens == 0 && e.TokensSaved == 0 {
				return nil
			}
			return nil
		}
	case observe.KindInjection:
		eventType = memory.TokenEventContextInjected
	default:
		return nil
	}
	return &memory.TokenEvent{
		ID:          e.ID,
		SessionID:   e.SessionID,
		Timestamp:   e.Timestamp,
		EventType:   eventType,
		Tool:        e.Name,
		FilePath:    e.FilePath,
		Tokens:      e.Tokens,
		TokensSaved: e.TokensSaved,
	}
}

// AddObserveEvent persists one observe.Event. ID/Timestamp must be set by the
// caller (the Recorder always populates them).
func (s *BoltStore) AddObserveEvent(e *observe.Event) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketObserveEvents)
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		return b.Put([]byte(e.ID), data)
	})
}

// ObserveFilter narrows ListObserveEvents results.
type ObserveFilter struct {
	Kind      observe.Kind
	Name      string
	Category  string
	SessionID string
	Since     time.Time
	Until     time.Time
	Limit     int
}

// ListObserveEvents returns events newest-first. Zero-value filter fields are
// treated as "any". A non-positive Limit returns all matches.
func (s *BoltStore) ListObserveEvents(f ObserveFilter) ([]*observe.Event, error) {
	var out []*observe.Event
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketObserveEvents)
		c := b.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			var e observe.Event
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			if !f.Since.IsZero() && e.Timestamp.Before(f.Since) {
				break
			}
			if !f.Until.IsZero() && e.Timestamp.After(f.Until) {
				continue
			}
			if f.Kind != "" && e.Kind != f.Kind {
				continue
			}
			if f.Name != "" && e.Name != f.Name {
				continue
			}
			if f.Category != "" && e.Category != f.Category {
				continue
			}
			if f.SessionID != "" && e.SessionID != f.SessionID {
				continue
			}
			out = append(out, &e)
			if f.Limit > 0 && len(out) >= f.Limit {
				break
			}
		}
		return nil
	})
	return out, err
}

// CleanupObserveEvents removes events older than maxAge. Returns the count
// deleted. Run periodically to keep the bucket bounded.
func (s *BoltStore) CleanupObserveEvents(maxAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-maxAge)
	var keys [][]byte
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketObserveEvents)
		return b.ForEach(func(k, v []byte) error {
			var e observe.Event
			if err := json.Unmarshal(v, &e); err != nil {
				keys = append(keys, append([]byte{}, k...))
				return nil
			}
			if e.Timestamp.Before(cutoff) {
				keys = append(keys, append([]byte{}, k...))
			}
			return nil
		})
	})
	if err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, nil
	}
	count := 0
	err = s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketObserveEvents)
		for _, k := range keys {
			if err := b.Delete(k); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return count, err
}

// ObserveSink adapts BoltStore to observe.Sink so the package-level Recorder
// can persist events. Failures are intentionally swallowed — telemetry must
// never break the hot path.
type ObserveSink struct {
	store *BoltStore
}

func NewObserveSink(s *BoltStore) *ObserveSink { return &ObserveSink{store: s} }

func (s *ObserveSink) Emit(e *observe.Event) {
	_ = s.store.AddObserveEvent(e)
}
