package observe

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type captureSink struct {
	mu     sync.Mutex
	events []*Event
}

func (c *captureSink) Emit(e *Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}

func TestSpanLifecycleCapturesDurationAndAttrs(t *testing.T) {
	sink := &captureSink{}
	r := &Recorder{sink: sink}

	span := r.Start("AnalyzeDeadCode", KindSpan).
		Category("analyzer").
		Subtype("deadcode").
		Attr("symbols", "5648").
		Tokens(0).
		Saved(0)
	time.Sleep(2 * time.Millisecond)
	span.End()

	if len(sink.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(sink.events))
	}
	e := sink.events[0]
	if e.Name != "AnalyzeDeadCode" || e.Kind != KindSpan {
		t.Errorf("name/kind wrong: %+v", e)
	}
	if e.Category != "analyzer" || e.Subtype != "deadcode" {
		t.Errorf("category/subtype wrong: %+v", e)
	}
	if e.Attrs["symbols"] != "5648" {
		t.Errorf("attrs not captured: %+v", e.Attrs)
	}
	if e.DurationMs < 1 {
		t.Errorf("expected non-zero duration, got %d", e.DurationMs)
	}
	if e.ID == "" || e.Timestamp.IsZero() {
		t.Errorf("ID/Timestamp not set")
	}
}

func TestSpanWithoutSinkIsNoop(t *testing.T) {
	r := &Recorder{}
	span := r.Start("noop", KindSpan)
	span.End()
}

func TestErrorCapture(t *testing.T) {
	sink := &captureSink{}
	r := &Recorder{sink: sink}
	span := r.Start("op", KindSpan).Err(errors.New("boom"))
	span.End()
	if sink.events[0].Error != "boom" {
		t.Errorf("expected error captured, got %q", sink.events[0].Error)
	}
}

func TestStartCtxAndFromContextEnrichSingleSpan(t *testing.T) {
	sink := &captureSink{}
	r := &Recorder{sink: sink}
	ctx, span := r.StartCtx(context.Background(), "code_outline", KindToolCall)
	span.Category("consume").Subtype("outline")

	// Simulate a downstream handler enriching via the context.
	FromContext(ctx).Tokens(900).Saved(500).FilePath("foo.go")

	span.End()
	if len(sink.events) != 1 {
		t.Fatalf("expected 1 event from a single span, got %d", len(sink.events))
	}
	e := sink.events[0]
	if e.Tokens != 900 || e.TokensSaved != 500 || e.FilePath != "foo.go" {
		t.Errorf("handler enrichment did not reach the same span: %+v", e)
	}
}

func TestFromContextWithoutSpanIsNoop(t *testing.T) {
	// Calling setters on a context with no span must not panic.
	FromContext(context.Background()).Tokens(123).Saved(456).End()
	FromContext(nil).Attr("k", "v").End()
}

func TestRecordOneOffEvent(t *testing.T) {
	sink := &captureSink{}
	r := &Recorder{sink: sink}
	r.Record(&Event{Kind: KindSession, Name: "session-start"})
	if len(sink.events) != 1 || sink.events[0].Name != "session-start" {
		t.Errorf("record one-off failed: %+v", sink.events)
	}
	if sink.events[0].ID == "" || sink.events[0].Timestamp.IsZero() {
		t.Errorf("ID/Timestamp should be auto-populated")
	}
}
