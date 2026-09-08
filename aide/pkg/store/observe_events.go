package store

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/eventbus"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	"github.com/oklog/ulid/v2"
	bolt "go.etcd.io/bbolt"
)

const (
	// attrStartLine / attrEndLine are well-known Attrs keys used to carry a
	// line range from the recorder (Read tool with offset/limit, code_read_symbol)
	// through observe events to the dashboard's clickable file viewer.
	attrStartLine = "start_line"
	attrEndLine   = "end_line"
)

// tokenEventToObserve translates a legacy TokenEvent into the observe.Event
// shape. Used by the one-shot migration that drains BucketTokenEvents into
// BucketObserveEvents at daemon startup.
func tokenEventToObserve(t *memory.TokenEvent) *observe.Event {
	var kind observe.Kind
	var category, subtype, name string
	switch t.EventType {
	case memory.TokenEventRead:
		kind, category, subtype, name = observe.KindToolCall, "consume", "file", t.Tool
	case memory.TokenEventOutlineUsed:
		kind, category, subtype, name = observe.KindToolCall, "consume", "outline", t.Tool
	case memory.TokenEventSymbolRead:
		kind, category, subtype, name = observe.KindToolCall, "consume", "symbol", t.Tool
	case memory.TokenEventReadAvoided:
		kind, category, subtype, name = observe.KindToolCall, "consume", "avoided", t.Tool
	case memory.TokenEventContextInjected:
		// For injections, the legacy Tool field carried the source label
		// (memory / decision / skill / enrichment). Preserve it as both
		// name and subtype.
		kind, category, subtype, name = observe.KindInjection, "inject", t.Tool, t.Tool
	default:
		return nil
	}
	ev := &observe.Event{
		ID:          t.ID,
		Timestamp:   t.Timestamp,
		Kind:        kind,
		Name:        name,
		Category:    category,
		Subtype:     subtype,
		FilePath:    t.FilePath,
		Tokens:      t.Tokens,
		TokensSaved: t.TokensSaved,
		SessionID:   t.SessionID,
	}
	if t.StartLine > 0 || t.EndLine > 0 {
		ev.Attrs = map[string]string{}
		if t.StartLine > 0 {
			ev.Attrs[attrStartLine] = strconv.Itoa(t.StartLine)
		}
		if t.EndLine > 0 {
			ev.Attrs[attrEndLine] = strconv.Itoa(t.EndLine)
		}
	}
	return ev
}

// observeToTokenEvent maps an observe.Event into the legacy TokenEvent shape
// where it carries cost data the old token UI/CLI consumes. Returns nil for
// events that have no equivalent (e.g., generic spans, hook lifecycle).
//
// EventType is category-aware: a tool_call's category becomes the event type
// (modify/execute/search/network/coordinate) so the dashboard's TYPE column
// distinguishes Edit ("modify") from Read ("read"). The well-known consume
// subtypes (file/outline/symbol/avoided) keep their legacy names so existing
// callers and the Stats aggregator continue to recognise them.
func observeToTokenEvent(e *observe.Event) *memory.TokenEvent {
	var eventType string
	switch e.Kind {
	case observe.KindHook:
		if e.Name != "output-transform" || e.Category != "transform" {
			return nil
		}
		eventType = "transformation"
	case observe.KindToolCall:
		switch {
		case e.Category == "consume" && e.Subtype == "outline":
			eventType = memory.TokenEventOutlineUsed
		case e.Category == "consume" && e.Subtype == "symbol":
			eventType = memory.TokenEventSymbolRead
		case e.Category == "consume" && e.Subtype == "avoided":
			eventType = memory.TokenEventReadAvoided
		case e.Category == "consume" && e.Subtype == "file":
			eventType = memory.TokenEventRead
		case e.Category != "":
			eventType = e.Category // modify, execute, search, network, coordinate
		default:
			return nil
		}
	case observe.KindInjection:
		eventType = memory.TokenEventContextInjected
	default:
		return nil
	}
	te := &memory.TokenEvent{
		Attrs:       e.Attrs,
		ID:          e.ID,
		SessionID:   e.SessionID,
		Timestamp:   e.Timestamp,
		EventType:   eventType,
		Tool:        e.Name,
		FilePath:    e.FilePath,
		Tokens:      e.Tokens,
		TokensSaved: e.TokensSaved,
	}
	if e.Attrs != nil {
		if v, ok := e.Attrs[attrStartLine]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				te.StartLine = n
			}
		}
		if v, ok := e.Attrs[attrEndLine]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				te.EndLine = n
			}
		}
	}
	return te
}

// AddObserveEvent persists one observe.Event. Populates ID and Timestamp if
// the caller left them empty (e.g., external RecordEvent RPC, observe record
// CLI) — the in-process Recorder always sets them, but defending here keeps
// the bolt layer from rejecting the write with "key required".
func (s *BoltStore) AddObserveEvent(e *observe.Event) error {
	// New measurements use one estimator, independently of legacy handler estimates.
	if e.Attrs["accounting_version"] == "1" {
		e.Tokens = 0
		e.Attrs["token_estimator"] = memory.TextEstimator
		for _, key := range []string{"payload_bytes", "argument_bytes"} {
			if _, ok := memory.MeasuredBytes(e.Attrs, key); !ok {
				delete(e.Attrs, key)
			}
		}
		if n, ok := memory.MeasuredBytes(e.Attrs, "payload_bytes"); ok {
			e.Tokens = int(memory.EstimateTextTokens(n))
		}
	}

	if e.ID == "" {
		e.ID = ulid.Make().String()
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketObserveEvents)
		// Only explicit origin identity deduplicates; equal commands are valid calls.
		if memory.HasObservationIdentity(e.SessionID, e.Attrs) {
			origin := []string{e.SessionID, e.Attrs["host"], e.Attrs["actor_id"], e.Attrs["invocation_id"], e.Attrs["observation_stage"]}
			if claim, ok := workHostClaim(e); ok {
				// Equal receipt retries still deduplicate. Contradictory receipts
				// must survive so attribution cannot silently select the first.
				origin = append(origin, "aide/work:1", claim.id, claim.tool, claim.hash)
			}
			identity, err := json.Marshal(origin)
			if err != nil {
				return err
			}
			key := []byte(fmt.Sprintf("%x", sha256.Sum256(identity)))
			origins, err := tx.CreateBucketIfNotExists([]byte("observe_origins"))
			if err != nil {
				return err
			}
			if id := origins.Get(key); id != nil {
				if data := b.Get(id); data != nil {
					// Keep the first observation and its timestamp stable on retries.
					return json.Unmarshal(data, e)
				}
			}
			if err := origins.Put(key, []byte(e.ID)); err != nil {
				return err
			}
		}
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
	if maxAge <= 0 {
		return 0, nil // 0 (or less) disables pruning: retain events forever
	}
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
		// The identity index follows event retention rather than growing forever.
		if origins := tx.Bucket([]byte("observe_origins")); origins != nil {
			c := origins.Cursor()
			for key, id := c.First(); key != nil; key, id = c.Next() {
				if b.Get(id) == nil {
					if err := c.Delete(); err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	return count, err
}

// metaKeyTokensMigrated is set in BucketMeta once MigrateTokenEventsToObserve
// completes successfully so subsequent daemon starts skip the migration.
const metaKeyTokensMigrated = "schema.tokens_migrated_to_observe"

// MigrateTokenEventsToObserve drains every entry from the legacy
// BucketTokenEvents into BucketObserveEvents (translated) and clears the
// legacy bucket. Idempotent: subsequent calls are no-ops once the meta flag
// is set. Atomic per-batch within a single bbolt transaction so a crash mid-
// migration leaves a recoverable state.
//
// Returns the number of events migrated on this call.
func (s *BoltStore) MigrateTokenEventsToObserve() (int, error) {
	if v, _ := s.GetMeta(metaKeyTokensMigrated); v == "1" {
		return 0, nil
	}
	migrated := 0
	err := s.db.Update(func(tx *bolt.Tx) error {
		legacy := tx.Bucket(BucketTokenEvents)
		obs := tx.Bucket(BucketObserveEvents)
		if legacy == nil || obs == nil {
			return nil
		}
		var keys [][]byte
		err := legacy.ForEach(func(k, v []byte) error {
			var t memory.TokenEvent
			if err := json.Unmarshal(v, &t); err != nil {
				keys = append(keys, append([]byte{}, k...))
				return nil
			}
			ev := tokenEventToObserve(&t)
			if ev == nil {
				keys = append(keys, append([]byte{}, k...))
				return nil
			}
			data, err := json.Marshal(ev)
			if err != nil {
				return err
			}
			if err := obs.Put([]byte(ev.ID), data); err != nil {
				return err
			}
			keys = append(keys, append([]byte{}, k...))
			migrated++
			return nil
		})
		if err != nil {
			return err
		}
		for _, k := range keys {
			if err := legacy.Delete(k); err != nil {
				return err
			}
		}
		meta := tx.Bucket(BucketMeta)
		if meta != nil {
			return meta.Put([]byte(metaKeyTokensMigrated), []byte("1"))
		}
		return nil
	})
	return migrated, err
}

// ObserveSink adapts BoltStore to observe.Sink so the package-level Recorder
// can persist events. Failures are intentionally swallowed — telemetry must
// never break the hot path.
type ObserveSink struct {
	store *BoltStore
	bus   *eventbus.Broadcaster[*observe.Event]
}

func NewObserveSink(s *BoltStore) *ObserveSink { return &ObserveSink{store: s} }

func (s *ObserveSink) SetBus(b *eventbus.Broadcaster[*observe.Event]) {
	s.bus = b
}

func (s *ObserveSink) Emit(e *observe.Event) {
	if err := s.store.AddObserveEvent(e); err != nil {
		return
	}
	if s.bus != nil {
		s.bus.Publish(e)
	}
}
