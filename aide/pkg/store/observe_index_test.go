package store

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	bolt "go.etcd.io/bbolt"
)

func TestObserveTimeIndexMatchesLegacyScan(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 300; i++ {
		e := &observe.Event{ID: fmt.Sprintf("%04d", i), Timestamp: at.Add(time.Duration((i*37)%71) * time.Nanosecond), Kind: observe.KindToolCall, Name: fmt.Sprint(i % 3), Category: fmt.Sprint(i % 2), SessionID: fmt.Sprint(i % 5)}
		if i == 0 {
			e.Timestamp = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		if err := s.AddObserveEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	// Replacing an existing ID must remove its former timestamp entry.
	if err := s.AddObserveEvent(&observe.Event{ID: "0299", Timestamp: at.Add(-time.Hour), Kind: observe.KindHook}); err != nil {
		t.Fatal(err)
	}
	filters := make([]ObserveFilter, 0, 42)
	for _, limit := range []int{-1, 0, 1, 7, 200, 500} {
		filters = append(filters, ObserveFilter{Limit: limit}, ObserveFilter{Limit: limit, SessionID: "2"}, ObserveFilter{Limit: limit, Name: "1", Category: "0", Kind: observe.KindToolCall}, ObserveFilter{Limit: limit, Since: at.Add(20 * time.Nanosecond), Until: at.Add(30 * time.Nanosecond)}, ObserveFilter{Limit: limit, Until: at.Add(-2 * time.Hour)}, ObserveFilter{Limit: limit, Since: at.Add(time.Hour)}, ObserveFilter{Limit: limit, Since: at.Add(30 * time.Nanosecond), Until: at.Add(20 * time.Nanosecond)})
	}
	indexed := make([][]*observe.Event, len(filters))
	for i, f := range filters {
		indexed[i], err = s.ListObserveEvents(f)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := s.db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket(bucketObserveByTime) }); err != nil {
		t.Fatal(err)
	}
	for i, f := range filters {
		legacy, err := s.ListObserveEvents(f)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(indexed[i], legacy) {
			t.Fatalf("index differs from legacy scan for %+v", f)
		}
	}
}

func TestObserveTimeIndexUpgradeAndRetention(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.db")
	s, err := NewBoltStore(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	for _, e := range []*observe.Event{{ID: "old", Timestamp: at.Add(-48 * time.Hour)}, {ID: "new", Timestamp: at}} {
		if err := s.AddObserveEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	// Simulate the previous schema, including an unreadable legacy record.
	err = s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket(bucketObserveByTime); err != nil {
			return err
		}
		if err := tx.Bucket(BucketObserveEvents).Put([]byte("bad"), []byte("{")); err != nil {
			return err
		}
		return tx.Bucket(BucketMeta).Put([]byte("schema_version"), itob(1))
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = NewBoltStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	rows, err := s.ListObserveEvents(ObserveFilter{Limit: 1})
	if err != nil || len(rows) != 1 || rows[0].ID != "new" {
		t.Fatalf("upgrade rows=%v err=%v", rows, err)
	}
	if n, err := s.CleanupObserveEvents(24 * time.Hour); err != nil || n != 2 {
		t.Fatalf("cleanup=%d err=%v", n, err)
	}
	err = s.db.View(func(tx *bolt.Tx) error {
		idx := tx.Bucket(bucketObserveByTime)
		if idx == nil || idx.Stats().KeyN != 1 {
			t.Fatal("index not migrated or not pruned")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestObserveTimeIndexLegacyTokenMigration(t *testing.T) {
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	err = s.db.Update(func(tx *bolt.Tx) error {
		data, err := json.Marshal(map[string]any{"id": "legacy", "timestamp": time.Now(), "type": "read", "tool": "Read"})
		if err != nil {
			return err
		}
		return tx.Bucket(BucketTokenEvents).Put([]byte("legacy"), data)
	})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := s.MigrateTokenEventsToObserve(); err != nil || n != 1 {
		t.Fatalf("migration=%d err=%v", n, err)
	}
	rows, err := s.ListObserveEvents(ObserveFilter{Limit: 1})
	if err != nil || len(rows) != 1 || rows[0].ID != "legacy" {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
}
