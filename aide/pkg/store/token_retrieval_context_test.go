package store

import (
	"slices"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func TestRetrievalContextGapsRemainConservativeAcrossReset(t *testing.T) {
	at := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		mutate     func(*observe.Event)
		continuity string
		oldGap     bool
		newGap     bool
		otherGap   bool
		unwindowed int
	}{
		{
			name:   "delayed_unknown_epoch_after_reset",
			oldGap: true, newGap: true, unwindowed: 1,
		},
		{
			name: "late_known_epoch_gap_only_taints_named_epoch",
			mutate: func(e *observe.Event) {
				e.Attrs["context_epoch"] = "old"
				e.Attrs["context_status"] = "pending"
			},
			oldGap: true, unwindowed: 1,
		},
		{
			name: "server_result_is_not_a_host_context_gap",
			mutate: func(e *observe.Event) {
				e.Attrs["observation_stage"] = "server_result"
			},
		},
		{
			name:   "missing_actor_cannot_select_one_actor",
			mutate: func(e *observe.Event) { delete(e.Attrs, "actor_id") },
			oldGap: true, newGap: true, otherGap: true, unwindowed: 1,
		},
		{
			name:   "missing_session_remains_unassigned",
			mutate: func(e *observe.Event) { e.SessionID = "" },
			oldGap: true, newGap: true, unwindowed: 1,
		},
		{
			name:       "unknown_continuity_cannot_release_old_window",
			continuity: "unknown",
			oldGap:     true, newGap: true, unwindowed: 1,
		},
		{
			name:   "equal_boundary_timestamp_remains_ambiguous",
			mutate: func(e *observe.Event) { e.Timestamp = at.Add(time.Minute) },
			oldGap: true, newGap: true, unwindowed: 1,
		},
		{
			name:   "first_result_is_not_a_proven_window_start",
			mutate: func(e *observe.Event) { e.Timestamp = at.Add(-time.Minute) },
			oldGap: true, newGap: true, unwindowed: 1,
		},
	}
	orders := []struct {
		name    string
		indices []int
	}{
		{"chronological_storage", []int{0, 1, 2, 3}},
		{"reverse_storage", []int{3, 2, 1, 0}},
		{"gap_stored_between_windows", []int{0, 3, 1, 2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, order := range orders {
				t.Run(order.name, func(t *testing.T) {
					s := retrievalStore(t)
					old := referenceEvent("old-call", "old", at)
					next := referenceEvent("new-call", "new", at.Add(time.Minute))
					next.Attrs["context_continuity"] = "reset"
					if tc.continuity != "" {
						next.Attrs["context_continuity"] = tc.continuity
					}
					other := referenceEvent("other-call", "old", at)
					other.Attrs["actor_id"] = "other"
					// Host observations currently receive persistence timestamps.
					// This may be an old result whose context lookup failed before
					// the reset, but which finished recording after the new result.
					// A later timestamp alone cannot prove it belongs to "new".
					gap := retrievalEvent("gap-call", "", "unverified", "30", at.Add(2*time.Minute))
					gap.Attrs["context_status"] = "unknown"
					if tc.mutate != nil {
						tc.mutate(gap)
					}
					events := []*observe.Event{old, next, other, gap}
					for _, index := range order.indices {
						addRetrieval(t, s, events[index])
					}
					stats, err := s.TokenStats("", time.Time{}, time.Time{})
					if err != nil {
						t.Fatal(err)
					}
					report := stats.Accounting.Retrievals
					if len(report.Windows) != 3 || report.UnwindowedEvents != tc.unwindowed {
						t.Fatalf("unexpected window coverage: %+v", report)
					}
					for _, window := range report.Windows {
						wantGap := tc.oldGap
						if window.ActorID == "other" {
							wantGap = tc.otherGap
						} else if window.Epoch == "new" {
							wantGap = tc.newGap
						}
						if slices.Contains(window.Issues, "context_gap") != wantGap {
							t.Fatalf("wrong gap attribution for %s/%s: %v", window.ActorID, window.Epoch, window.Issues)
						}
						if (window.Comparison == nil) != wantGap {
							t.Fatalf("unexpected comparison eligibility for %s/%s: %+v", window.ActorID, window.Epoch, window)
						}
						// Unassigned results must not be borrowed into a window's
						// observed costs or reference while diagnosing the gap.
						if window.Events != 1 || window.Observed.Bytes != 60 || window.Reference.Bytes != 300 {
							t.Fatalf("unassigned result entered window totals: %+v", window)
						}
						if window.ActorID == "actor" && window.Epoch == "old" {
							wantBoundary := "reset_observed"
							if tc.continuity == "unknown" {
								wantBoundary = "continuity_unknown"
							}
							if window.Boundary != wantBoundary {
								t.Fatalf("unexpected boundary: got %s want %s", window.Boundary, wantBoundary)
							}
						}
					}
				})
			}
		})
	}
}
