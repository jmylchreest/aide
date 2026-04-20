package observe

import (
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
