package store

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/eventbus"
	"github.com/jmylchreest/aide/aide/pkg/observe"
	bolt "go.etcd.io/bbolt"
)

func batchTestStore(t *testing.T) *BoltStore {
	t.Helper()
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestObserveBatchMutationOutcomes(t *testing.T) {
	s := batchTestStore(t)
	at := time.Now().UTC()
	row := func(at time.Time, n string) *observe.Event {
		return usageEvent("response", at, map[string]string{"input_tokens": n})
	}
	rows := []*observe.Event{row(at, "10"), row(at.Add(time.Second), "10"), row(at.Add(-time.Second), "10"), row(at, "20")}
	changed, err := s.AddObserveEvents(rows)
	if err != nil || !reflect.DeepEqual(changed, []bool{true, false, true, true}) {
		t.Fatalf("outcomes %v: %v", changed, err)
	}
	if rows[0].ID != rows[1].ID || rows[0].ID != rows[2].ID || rows[0].ID == rows[3].ID {
		t.Fatal("lost canonical or conflicting identities")
	}
	stored, err := s.ListObserveEvents(ObserveFilter{})
	if err != nil || len(stored) != 2 {
		t.Fatalf("stored %d: %v", len(stored), err)
	}
	replay := row(at.Add(time.Second), "10")
	changed, err = s.AddObserveEvents([]*observe.Event{replay})
	if err != nil || changed[0] || !replay.Timestamp.Equal(at.Add(-time.Second)) {
		t.Fatalf("replay %+v %v: %v", replay, changed, err)
	}
}

func TestObserveBatchRollbackAndBounds(t *testing.T) {
	s := batchTestStore(t)
	for _, bad := range [][]*observe.Event{{nil}, make([]*observe.Event, MaxObserveBatchEvents+1)} {
		if _, err := s.AddObserveEvents(bad); err == nil {
			t.Fatal("invalid batch accepted")
		}
	}
	first := &observe.Event{Kind: observe.KindHook, Tokens: 99, Attrs: map[string]string{"accounting_version": "1", "payload_bytes": "invalid"}}
	before := map[string]string{"accounting_version": "1", "payload_bytes": "invalid"}
	// The first write succeeds within the transaction; the second exceeds Bolt's key limit.
	if _, err := s.AddObserveEvents([]*observe.Event{first, {ID: strings.Repeat("x", bolt.MaxKeySize+1)}}); err == nil {
		t.Fatal("expected transaction failure")
	}
	if first.ID != "" || !first.Timestamp.IsZero() || first.Tokens != 99 || !reflect.DeepEqual(first.Attrs, before) {
		t.Fatalf("rollback mutated caller: %+v", first)
	}
	stored, err := s.ListObserveEvents(ObserveFilter{})
	if err != nil || len(stored) != 0 {
		t.Fatalf("partial transaction persisted: %d %v", len(stored), err)
	}
	changed, err := s.AddObserveEvents(nil)
	if err != nil || len(changed) != 0 {
		t.Fatalf("empty batch: %v %v", changed, err)
	}
	rows := make([]*observe.Event, MaxObserveBatchEvents)
	for i := range rows {
		rows[i] = &observe.Event{Kind: observe.KindHook, Name: "Stop"}
	}
	changed, err = s.AddObserveEvents(rows)
	if err != nil || len(changed) != MaxObserveBatchEvents {
		t.Fatalf("maximum batch: %d %v", len(changed), err)
	}
	for i, mutation := range changed {
		if !mutation || rows[i].ID == "" || rows[i].Timestamp.IsZero() {
			t.Fatalf("uncommitted row %d", i)
		}
	}
}

func TestObserveBatchConcurrentReplayAndOrdinaryHooks(t *testing.T) {
	s := batchTestStore(t)
	at := time.Now().UTC()
	var inserted atomic.Int32
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			changed, err := s.AddObserveEvents([]*observe.Event{usageEvent("same", at, map[string]string{"input_tokens": "10"})})
			if err != nil {
				t.Error(err)
				return
			}
			if changed[0] {
				inserted.Add(1)
			}
		}()
	}
	wg.Wait()
	if inserted.Load() != 1 {
		t.Fatalf("concurrent mutations = %d", inserted.Load())
	}
	rows := []*observe.Event{{Kind: observe.KindHook, Name: "Stop"}, {Kind: observe.KindHook, Name: "Stop"}}
	changed, err := s.AddObserveEvents(rows)
	if err != nil || !reflect.DeepEqual(changed, []bool{true, true}) || rows[0].ID == rows[1].ID {
		t.Fatalf("ordinary calls collapsed: %v %v", changed, err)
	}
}

func TestObserveSinkPublishesOnlyMutations(t *testing.T) {
	s := batchTestStore(t)
	bus := eventbus.New[*observe.Event](8)
	ch, unsubscribe := bus.Subscribe(context.Background(), nil)
	defer unsubscribe()
	sink := NewObserveSink(s)
	sink.SetBus(bus)
	at := time.Now().UTC()
	for range 3 {
		sink.Emit(usageEvent("same", at, map[string]string{"input_tokens": "10"}))
	}
	if len(ch) != 1 {
		t.Fatalf("broadcast %d unchanged retries", len(ch))
	}
	sink.Emit(usageEvent("same", at.Add(-time.Second), map[string]string{"input_tokens": "10"}))
	sink.Emit(usageEvent("same", at, map[string]string{"input_tokens": "20"}))
	if len(ch) != 3 {
		t.Fatalf("lost changed evidence: %d", len(ch))
	}
}

func TestObserveBatchExplicitIDAndCanonicalCopy(t *testing.T) {
	s := batchTestStore(t)
	e := &observe.Event{ID: "explicit", Kind: observe.KindHook, Name: "Stop", Timestamp: time.Now().UTC()}
	for i, want := range []bool{true, false} {
		changed, err := s.AddObserveEvents([]*observe.Event{e})
		if err != nil || changed[0] != want {
			t.Fatalf("explicit retry %d: %v %v", i, changed, err)
		}
	}
	e.Name = "Start"
	changed, err := s.AddObserveEvents([]*observe.Event{e})
	if err != nil || !changed[0] {
		t.Fatalf("explicit change: %v %v", changed, err)
	}
	first := usageEvent("canonical", time.Now().UTC(), map[string]string{"input_tokens": "10"})
	if err := s.AddObserveEvent(first); err != nil {
		t.Fatal(err)
	}
	retry := usageEvent("canonical", first.Timestamp, map[string]string{"input_tokens": "10", "retry_only": "extra"})
	changed, err = s.AddObserveEvents([]*observe.Event{retry})
	if err != nil || changed[0] || !reflect.DeepEqual(retry, first) {
		t.Fatalf("noncanonical copy: %+v %v %v", retry, changed, err)
	}
}
