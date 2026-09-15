// Package observe is the unified observability event store.
package observe

import (
	"context"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

type Kind string

const (
	KindToolCall  Kind = "tool_call"
	KindSpan      Kind = "span"
	KindHook      Kind = "hook"
	KindInjection Kind = "injection"
	KindSession   Kind = "session"
)

type Event struct {
	ID          string            `json:"id"`
	Timestamp   time.Time         `json:"ts"`
	Kind        Kind              `json:"kind"`
	Name        string            `json:"name"`
	Category    string            `json:"category,omitempty"`
	Subtype     string            `json:"subtype,omitempty"`
	DurationMs  int64             `json:"dur_ms,omitempty"`
	Tokens      int               `json:"tokens,omitempty"`
	TokensSaved int               `json:"saved,omitempty"`
	FilePath    string            `json:"file,omitempty"`
	Parent      string            `json:"parent,omitempty"`
	SessionID   string            `json:"session,omitempty"`
	Error       string            `json:"error,omitempty"`
	Attrs       map[string]string `json:"attrs,omitempty"`
}

// Sink persists or forwards events. The Recorder calls Emit asynchronously
// from the caller's perspective (a buffered channel) so instrumentation never
// blocks the hot path.
type Sink interface {
	Emit(e *Event)
}

// Recorder is the entry point. Configure once at process startup with a Sink,
// then construct Spans anywhere via the package-level Start function.
type Recorder struct {
	mu   sync.RWMutex
	sink Sink
}

var defaultRecorder = &Recorder{}

// SetDefault installs the process-wide sink. Pass nil to disable recording.
func SetDefault(s Sink) {
	defaultRecorder.mu.Lock()
	defaultRecorder.sink = s
	defaultRecorder.mu.Unlock()
}

// Default returns the process-wide recorder.
func Default() *Recorder { return defaultRecorder }

// Span captures a unit of work. Build it with Start, decorate with the With*
// methods, then call End to record. Safe to use even when no sink is set —
// End becomes a no-op.
type Span struct {
	event Event
	start time.Time
	rec   *Recorder
}

// Start opens a new Span. Use defer span.End() right after.
func (r *Recorder) Start(name string, kind Kind) *Span {
	now := time.Now()
	return &Span{
		event: Event{
			ID:        ulid.Make().String(),
			Timestamp: now,
			Kind:      kind,
			Name:      name,
		},
		start: now,
		rec:   r,
	}
}

// Start is a convenience using the default recorder.
func Start(name string, kind Kind) *Span { return defaultRecorder.Start(name, kind) }

type spanCtxKey struct{}

// StartCtx opens a Span and attaches it to the returned context so downstream
// callers can enrich it via FromContext without coordinating a skip list.
// The middleware calls End via defer; handlers only call setters.
func (r *Recorder) StartCtx(ctx context.Context, name string, kind Kind) (context.Context, *Span) {
	span := r.Start(name, kind)
	return context.WithValue(ctx, spanCtxKey{}, span), span
}

// StartCtx is a convenience using the default recorder.
func StartCtx(ctx context.Context, name string, kind Kind) (context.Context, *Span) {
	return defaultRecorder.StartCtx(ctx, name, kind)
}

// FromContext returns the active Span on the context, or a no-op Span when
// none is present. Always non-nil so handlers can enrich without checking.
func FromContext(ctx context.Context) *Span {
	if ctx == nil {
		return noopSpan()
	}
	if s, ok := ctx.Value(spanCtxKey{}).(*Span); ok {
		return s
	}
	return noopSpan()
}

func noopSpan() *Span {
	// Recorder with nil sink — End() is a no-op, setters are harmless.
	return &Span{rec: &Recorder{}, start: time.Now()}
}

func (s *Span) Category(v string) *Span { s.event.Category = v; return s }
func (s *Span) Subtype(v string) *Span  { s.event.Subtype = v; return s }
func (s *Span) FilePath(v string) *Span { s.event.FilePath = v; return s }
func (s *Span) Session(v string) *Span  { s.event.SessionID = v; return s }
func (s *Span) Parent(v string) *Span   { s.event.Parent = v; return s }
func (s *Span) Tokens(n int) *Span      { s.event.Tokens = n; return s }
func (s *Span) Saved(n int) *Span       { s.event.TokensSaved = n; return s }

// TokensIfUnset records n only when no handler has already set Tokens.
// Used by the MCP middleware to backfill output-size cost for read-side
// tools that don't compute their own (search/list/stats) without
// stepping on handlers that compute a richer figure (code_outline,
// code_read_symbol set tokens + savings explicitly).
func (s *Span) TokensIfUnset(n int) *Span {
	if s.event.Tokens == 0 {
		s.event.Tokens = n
	}
	return s
}

func (s *Span) Attr(key, value string) *Span {
	if s.event.Attrs == nil {
		s.event.Attrs = make(map[string]string, 4)
	}
	s.event.Attrs[key] = value
	return s
}

func (s *Span) Err(err error) *Span {
	if err != nil {
		s.event.Error = err.Error()
	}
	return s
}

// ID returns the span's event ID, useful for parent linking on child spans.
func (s *Span) ID() string { return s.event.ID }

// End closes the span and emits the event.
func (s *Span) End() {
	s.event.DurationMs = time.Since(s.start).Milliseconds()
	s.rec.mu.RLock()
	sink := s.rec.sink
	s.rec.mu.RUnlock()
	if sink == nil {
		return
	}
	ev := s.event
	sink.Emit(&ev)
}

// Record emits a one-off event without timing (e.g., session start).
func (r *Recorder) Record(e *Event) {
	if e.ID == "" {
		e.ID = ulid.Make().String()
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now()
	}
	r.mu.RLock()
	sink := r.sink
	r.mu.RUnlock()
	if sink == nil {
		return
	}
	sink.Emit(e)
}

// Record uses the default recorder.
func Record(e *Event) { defaultRecorder.Record(e) }
