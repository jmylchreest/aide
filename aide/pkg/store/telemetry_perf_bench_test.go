package store

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func BenchmarkTelemetryHistory(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		b.Run(fmt.Sprint(n), func(b *testing.B) {
			s, err := NewBoltStore(filepath.Join(b.TempDir(), "events.db"))
			if err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() { s.Close() })
			at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
			for i := 0; i < n; i += 100 {
				rows := make([]*observe.Event, 100)
				for j := range rows {
					rows[j] = &observe.Event{Timestamp: at.Add(time.Duration(i+j) * time.Second), Kind: observe.KindToolCall, Name: "Read", Category: "consume", Subtype: "file", SessionID: "session", Attrs: map[string]string{"accounting_version": "1", "observation_stage": "server_result", "payload_bytes": "1000"}}
				}
				if _, err := s.AddObserveEvents(rows); err != nil {
					b.Fatal(err)
				}
			}
			b.Run("page200", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					rows, err := s.ListObserveEvents(ObserveFilter{Limit: 200})
					if err != nil || len(rows) != 200 {
						b.Fatal(err)
					}
				}
			})
			b.Run("stats", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					stats, err := s.TokenStats("session", time.Time{}, time.Time{})
					if err != nil || stats == nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
