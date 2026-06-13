// Package eventbus provides a generic typed broadcaster used to fan out
// in-process events to live subscribers. Each aide domain instantiates its
// own typed broadcaster wired into its write path.
//
// Telemetry-grade, not delivery-guaranteed: Publish is non-blocking per
// subscriber (oldest event dropped on buffer overflow, drop counter
// incremented). Consumers that need exactly-once must reconcile by
// querying the underlying store with a cursor.
package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
)

// Broadcaster fans out events of type T to live subscribers. Construct with New.
type Broadcaster[T any] struct {
	mu      sync.RWMutex
	subs    map[uint64]*subscription[T]
	nextID  uint64
	bufSize int
}

type subscription[T any] struct {
	ch      chan T
	filter  func(T) bool
	closed  atomic.Bool
	dropped atomic.Uint64
}

type Stats struct {
	Subscribers int
	Dropped     uint64
}

// New constructs a Broadcaster with the given per-subscriber buffer size.
// bufSize <= 0 falls back to 256.
func New[T any](bufSize int) *Broadcaster[T] {
	if bufSize <= 0 {
		bufSize = 256
	}
	return &Broadcaster[T]{
		subs:    make(map[uint64]*subscription[T]),
		bufSize: bufSize,
	}
}

// Subscribe registers a new subscriber and returns its channel plus an
// unsubscribe func. The channel is closed when ctx is cancelled or unsub
// is called (whichever comes first). unsub is idempotent.
//
// filter may be nil; when non-nil, Publish only forwards events for which
// it returns true.
func (b *Broadcaster[T]) Subscribe(ctx context.Context, filter func(T) bool) (<-chan T, func()) {
	sub := &subscription[T]{
		ch:     make(chan T, b.bufSize),
		filter: filter,
	}

	b.mu.Lock()
	id := b.nextID
	b.nextID++
	b.subs[id] = sub
	b.mu.Unlock()

	once := sync.Once{}
	unsub := func() {
		once.Do(func() {
			b.mu.Lock()
			delete(b.subs, id)
			b.mu.Unlock()
			if sub.closed.CompareAndSwap(false, true) {
				close(sub.ch)
			}
		})
	}

	if ctx != nil {
		go func() {
			<-ctx.Done()
			unsub()
		}()
	}

	return sub.ch, unsub
}

// Publish fans event out to all matching subscribers. Non-blocking per
// subscriber: when the buffer is full, the oldest buffered event is
// dropped and the drop counter is incremented. Closed subscriptions are
// skipped silently.
func (b *Broadcaster[T]) Publish(event T) {
	// Snapshot under read lock so Publish doesn't hold it across sends.
	b.mu.RLock()
	subs := make([]*subscription[T], 0, len(b.subs))
	for _, s := range b.subs {
		subs = append(subs, s)
	}
	b.mu.RUnlock()

	for _, s := range subs {
		if s.closed.Load() {
			continue
		}
		if s.filter != nil && !s.filter(event) {
			continue
		}
		for {
			select {
			case s.ch <- event:
				goto next
			default:
				select {
				case <-s.ch:
					s.dropped.Add(1)
				default:
				}
				if s.closed.Load() {
					goto next
				}
			}
		}
	next:
	}
}

// Stats returns live subscriber count and cumulative dropped-event total
// across all subscribers. Diagnostic; not for hot-path use.
func (b *Broadcaster[T]) Stats() Stats {
	b.mu.RLock()
	defer b.mu.RUnlock()
	var dropped uint64
	for _, s := range b.subs {
		dropped += s.dropped.Load()
	}
	return Stats{
		Subscribers: len(b.subs),
		Dropped:     dropped,
	}
}
