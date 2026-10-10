package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
	bolt "go.etcd.io/bbolt"
)

func BenchmarkObserveIndexWrites(b *testing.B) {
	for _, indexed := range []bool{false, true} {
		b.Run(fmt.Sprint(indexed), func(b *testing.B) {
			s, err := NewBoltStore(filepath.Join(b.TempDir(), "events.db"))
			if err != nil {
				b.Fatal(err)
			}
			defer s.Close()
			if !indexed {
				if err := s.db.Update(func(tx *bolt.Tx) error { return tx.DeleteBucket(bucketObserveByTime) }); err != nil {
					b.Fatal(err)
				}
			}
			at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			rows := make([]*observe.Event, 100)
			for i := range rows {
				rows[i] = &observe.Event{Kind: observe.KindToolCall, Name: "Read", SessionID: "session", Timestamp: at}
			}
			b.ReportAllocs()
			for b.Loop() {
				for _, e := range rows {
					e.ID = ""
				}
				if _, err := s.AddObserveEvents(rows); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
