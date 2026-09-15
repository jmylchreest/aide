package store

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/observe"
)

func retrievalStore(t *testing.T) *BoltStore {
	t.Helper()
	s, err := NewBoltStore(filepath.Join(t.TempDir(), "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func retrievalEvent(call, epoch, status, bytes string, at time.Time) *observe.Event {
	return &observe.Event{Kind: observe.KindToolCall, Name: "Bash", Category: "execute", SessionID: "s", Timestamp: at, Attrs: map[string]string{
		"accounting_version": "1", "observation_stage": "host_result", "host": "host", "actor_id": "actor", "invocation_id": call, "context_status": "active", "context_epoch": epoch,
		"retrieval_status": status, "payload_bytes": bytes, "retrieval_target": "source.ts",
	}}
}
func referenceEvent(call, epoch string, at time.Time) *observe.Event {
	e := retrievalEvent(call, epoch, "referenced", "60", at)
	e.Name = "code_outline"
	e.Category = "consume"
	e.Attrs["source_verification"] = "server_receipt"
	e.Attrs["retrieval_id"] = call
	e.Attrs["source_references"] = fmt.Sprintf(`[{"file":"source.ts","sha256":"%s","bytes":300}]`, strings.Repeat("a", 64))
	return e
}
func addRetrieval(t *testing.T, s *BoltStore, e *observe.Event) {
	t.Helper()
	if err := s.AddObserveEvent(e); err != nil {
		t.Fatal(err)
	}
}

func TestRetrievalWindowsCountBaselineAndBatchCostOnce(t *testing.T) {
	s := retrievalStore(t)
	at := time.Now().UTC()
	outline := referenceEvent("outline", "w", at)
	outline.Attrs["source_references"] = fmt.Sprintf(`[{"file":"source.ts","sha256":"%s","bytes":300},{"file":"second.ts","sha256":"%s","bytes":600}]`, strings.Repeat("a", 64), strings.Repeat("b", 64))
	addRetrieval(t, s, outline)
	addRetrieval(t, s, outline) // replay
	symbol := referenceEvent("symbol", "w", at.Add(time.Second))
	symbol.Attrs["payload_bytes"] = "800"
	addRetrieval(t, s, symbol)
	fallback := retrievalEvent("full", "w", "full_file", "300", at.Add(2*time.Second))
	fallback.Attrs["source_verification"] = "current_file_match"
	fallback.Attrs["source_references"] = referenceEvent("", "", at).Attrs["source_references"]
	addRetrieval(t, s, fallback)
	stats, err := s.TokenStats("s", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	r := stats.Accounting.Retrievals
	if len(r.Windows) != 1 {
		t.Fatalf("windows: %+v", r)
	}
	w := r.Windows[0]
	if w.Reference.Bytes != 900 || w.Observed.Bytes != 1160 || w.Observed.Events != 3 || w.Comparison == nil || w.Comparison.DeltaBytes != -260 || w.FullReadEvents != 1 {
		t.Fatalf("inflated comparison: %+v", w)
	}
	if len(w.Sources) != 2 || len(w.Steps) != 3 || w.Steps[0].InvocationID != "outline" {
		t.Fatalf("lost sequence: %+v", w)
	}
	if stats.TotalSaved != 0 {
		t.Fatal("conditional comparison entered legacy savings")
	}
}

func TestRetrievalWindowsPreserveUnknownsAndFilterClipping(t *testing.T) {
	s := retrievalStore(t)
	at := time.Now().UTC()
	addRetrieval(t, s, referenceEvent("first", "w", at))
	addRetrieval(t, s, referenceEvent("later", "w", at.Add(time.Second)))
	stats, _ := s.TokenStats("s", at.Add(time.Second), time.Time{})
	w := stats.Accounting.Retrievals.Windows[0]
	if !w.Clipped || w.Comparison != nil || w.Observed.Events != 1 {
		t.Fatalf("clipped window claimed complete: %+v", w)
	}
	unknown := retrievalEvent("unknown", "w", "unclassified_shell", "45", at.Add(2*time.Second))
	addRetrieval(t, s, unknown)
	failed := retrievalEvent("failed", "w", "failed", "30", at.Add(3*time.Second))
	addRetrieval(t, s, failed)
	missing := retrievalEvent("missing", "w", "unverified", "", at.Add(4*time.Second))
	addRetrieval(t, s, missing)
	stats, _ = s.TokenStats("s", time.Time{}, time.Time{})
	w = stats.Accounting.Retrievals.Windows[0]
	if w.Comparison != nil || w.Unattributed.Bytes != 45 || w.Observed.Bytes != 150 || w.MissingPayload != 1 || w.FailedEvents != 1 {
		t.Fatalf("unknown evidence fabricated: %+v", w)
	}
}

func TestRetrievalWindowsSeparateEpochsActorsAndServerObservations(t *testing.T) {
	s := retrievalStore(t)
	at := time.Now().UTC()
	addRetrieval(t, s, referenceEvent("old", "old", at))
	next := referenceEvent("new", "new", at.Add(time.Second))
	next.Attrs["context_continuity"] = "reset"
	addRetrieval(t, s, next)
	actor := referenceEvent("actor", "old", at)
	actor.Attrs["actor_id"] = "other"
	addRetrieval(t, s, actor)
	server := referenceEvent("server", "old", at)
	server.Attrs["observation_stage"] = "server_result"
	addRetrieval(t, s, server)
	unknown := referenceEvent("unknown", "", at)
	addRetrieval(t, s, unknown)
	stats, _ := s.TokenStats("s", time.Time{}, time.Time{})
	r := stats.Accounting.Retrievals
	if len(r.Windows) != 3 || r.UnwindowedEvents != 1 {
		t.Fatalf("mixed identities: %+v", r)
	}
	for _, w := range r.Windows {
		if w.Observed.Events != 1 {
			t.Fatal("server duplicated host delivery")
		}
		if w.ActorID == "actor" && w.Epoch == "old" && w.Boundary != "reset_observed" {
			t.Fatalf("missing reset boundary: %+v", w)
		}
	}
}

func TestRetrievalWindowsDoNotDependOnRecentEventPage(t *testing.T) {
	s := retrievalStore(t)
	at := time.Now().UTC()
	for i := 0; i < 220; i++ {
		addRetrieval(t, s, referenceEvent(fmt.Sprint(i), "w", at.Add(time.Duration(i)*time.Second)))
	}
	stats, _ := s.TokenStats("s", time.Time{}, time.Time{})
	w := stats.Accounting.Retrievals.Windows[0]
	if w.Observed.Events != 220 || w.Reference.Bytes != 300 || len(w.Steps) != 64 || !w.StepsLimited || w.Comparison == nil || w.Comparison.DeltaBytes != 300-220*60 {
		t.Fatalf("page-limited comparison: %+v", w)
	}
}

func TestRetrievalWindowRejectsUnprovenRangeVersionAndContextGap(t *testing.T) {
	s := retrievalStore(t)
	at := time.Now().UTC()
	addRetrieval(t, s, referenceEvent("outline", "w", at))
	partial := retrievalEvent("range", "w", "range", "30", at.Add(time.Second))
	partial.Attrs["source_verification"] = "current_range_match"
	partial.Attrs["source_references"] = strings.ReplaceAll(referenceEvent("", "", at).Attrs["source_references"], strings.Repeat("a", 64), strings.Repeat("b", 64))
	addRetrieval(t, s, partial)
	stats, _ := s.TokenStats("s", time.Time{}, time.Time{})
	if stats.Accounting.Retrievals.Windows[0].Comparison != nil {
		t.Fatal("changed range minted an unproven full-file baseline")
	}
	s2 := retrievalStore(t)
	addRetrieval(t, s2, referenceEvent("outline", "w", at))
	gap := retrievalEvent("pending", "w", "pending", "30", at.Add(time.Second))
	gap.Attrs["context_status"] = "pending"
	addRetrieval(t, s2, gap)
	stats, _ = s2.TokenStats("s", time.Time{}, time.Time{})
	if stats.Accounting.Retrievals.Windows[0].Comparison != nil {
		t.Fatal("pending context output was silently omitted")
	}
}

func TestRetrievalWindowRejectsUnboundReferenceClaims(t *testing.T) {
	s := retrievalStore(t)
	at := time.Now().UTC()
	addRetrieval(t, s, referenceEvent("outline", "w", at))
	invalid := retrievalEvent("unbound", "w", "referenced", "30", at.Add(time.Second))
	addRetrieval(t, s, invalid)
	stats, _ := s.TokenStats("s", time.Time{}, time.Time{})
	if stats.Accounting.Retrievals.Windows[0].Comparison != nil {
		t.Fatal("unbound referenced status accepted")
	}
	full := retrievalEvent("bad-full", "w", "full_file", "42", at.Add(2*time.Second))
	full.Attrs["source_verification"] = "current_file_match"
	full.Attrs["source_references"] = referenceEvent("", "", at).Attrs["source_references"]
	addRetrieval(t, s, full)
	stats, _ = s.TokenStats("s", time.Time{}, time.Time{})
	if stats.Accounting.Retrievals.Windows[0].FullReadEvents != 0 {
		t.Fatal("inconsistent full-file evidence counted as a delivered file")
	}
}

func TestRetrievalWindowsBoundSelectionAndKeepFailedOutputCost(t *testing.T) {
	s := retrievalStore(t)
	at := time.Now().UTC()
	for i := 0; i < 67; i++ {
		addRetrieval(t, s, referenceEvent(fmt.Sprint(i), fmt.Sprint(i), at.Add(time.Duration(i)*time.Second)))
	}
	failure := retrievalEvent("failure", "66", "failed", "30", at.Add(68*time.Second))
	addRetrieval(t, s, failure)
	stats, _ := s.TokenStats("s", time.Time{}, time.Time{})
	r := stats.Accounting.Retrievals
	if !r.WindowsLimited || len(r.Windows) != 64 {
		t.Fatalf("unbounded windows: %+v", r)
	}
	for _, w := range r.Windows {
		if w.Epoch == "66" && (w.Comparison == nil || w.Comparison.AfterBytes != 90 || w.FailedEvents != 1) {
			t.Fatalf("failed result cost lost: %+v", w)
		}
	}
}

func TestRetrievalWindowsRecognizeOpenCodeRenderedSourceWithoutByteEquality(t *testing.T) {
	s := retrievalStore(t)
	at := time.Now().UTC()
	outline := referenceEvent("outline", "w", at)
	outline.Attrs["host"] = "opencode"
	addRetrieval(t, s, outline)
	full := retrievalEvent("read", "w", "full_file", "480", at.Add(time.Second))
	full.Name = "Read"
	full.Attrs["host"] = "opencode"
	full.Attrs["retrieval_method"] = "native_read"
	full.Attrs["source_verification"] = "current_rendered_file_match"
	full.Attrs["source_references"] = outline.Attrs["source_references"]
	addRetrieval(t, s, full)
	stats, err := s.TokenStats("s", time.Time{}, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	w := stats.Accounting.Retrievals.Windows[0]
	if w.FullReadEvents != 1 || w.Reference.Bytes != 300 || w.Observed.Bytes != 540 || w.Comparison == nil || w.Comparison.DeltaBytes != -240 {
		t.Fatalf("rendered source confused with delivered bytes: %+v", w)
	}
	if stats.TotalSaved != 0 {
		t.Fatal("rendered comparison became causal savings")
	}
}

func TestRetrievalWindowsRenderedRangesNeedMatchingFullBaseline(t *testing.T) {
	for _, baseline := range []string{"matching", "changed", "absent"} {
		t.Run(baseline, func(t *testing.T) {
			s := retrievalStore(t)
			at := time.Now().UTC()
			outline := referenceEvent("outline", "w", at)
			outline.Attrs["host"] = "opencode"
			if baseline != "absent" {
				addRetrieval(t, s, outline)
			}
			partial := retrievalEvent("read", "w", "range", "90", at.Add(time.Second))
			partial.Name = "Read"
			partial.Attrs["host"] = "opencode"
			partial.Attrs["retrieval_method"] = "native_read"
			partial.Attrs["source_verification"] = "current_rendered_range_match"
			partial.Attrs["source_references"] = outline.Attrs["source_references"]
			if baseline == "changed" {
				partial.Attrs["source_references"] = strings.ReplaceAll(partial.Attrs["source_references"], strings.Repeat("a", 64), strings.Repeat("b", 64))
			}
			addRetrieval(t, s, partial)
			stats, err := s.TokenStats("s", time.Time{}, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			w := stats.Accounting.Retrievals.Windows[0]
			if w.FullReadEvents != 0 || (w.Comparison != nil) != (baseline == "matching") {
				t.Fatalf("range promoted to full source or valid range rejected: %+v", w)
			}
			if baseline == "absent" && (len(w.Sources) != 0 || w.Reference.Bytes != 0) {
				t.Fatalf("range minted baseline: %+v", w)
			}
		})
	}
}

func TestRetrievalWindowsRejectRenderedLabelsOutsideNativeOpenCodeReads(t *testing.T) {
	for _, mismatch := range []string{"host", "tool", "method", "missing-payload", "failed", "range-label"} {
		t.Run(mismatch, func(t *testing.T) {
			s := retrievalStore(t)
			at := time.Now().UTC()
			full := retrievalEvent("read", "w", "full_file", "480", at)
			full.Name = "Read"
			full.Attrs["host"] = "opencode"
			full.Attrs["retrieval_method"] = "native_read"
			full.Attrs["source_verification"] = "current_rendered_file_match"
			full.Attrs["source_references"] = referenceEvent("", "", at).Attrs["source_references"]
			switch mismatch {
			case "host":
				full.Attrs["host"] = "other"
			case "tool":
				full.Name = "Bash"
			case "method":
				full.Attrs["retrieval_method"] = "shell_cat"
			case "missing-payload":
				delete(full.Attrs, "payload_bytes")
			case "failed":
				full.Error = "read failed"
			case "range-label":
				full.Attrs["source_verification"] = "current_rendered_range_match"
			}
			addRetrieval(t, s, full)
			stats, err := s.TokenStats("s", time.Time{}, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			w := stats.Accounting.Retrievals.Windows[0]
			if w.FullReadEvents != 0 || len(w.Sources) != 0 || w.Comparison != nil {
				t.Fatalf("unverified rendered label trusted: %+v", w)
			}
		})
	}
}
