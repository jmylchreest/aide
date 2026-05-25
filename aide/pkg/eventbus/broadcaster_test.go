package eventbus

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestBroadcaster_BasicPubSub(t *testing.T) {
	b := New[int](16)
	ctx := t.Context()

	ch, unsub := b.Subscribe(ctx, nil)
	defer unsub()

	b.Publish(1)
	b.Publish(2)
	b.Publish(3)

	got := []int{<-ch, <-ch, <-ch}
	want := []int{1, 2, 3}
	for i, v := range want {
		if got[i] != v {
			t.Errorf("event %d: got %d, want %d", i, got[i], v)
		}
	}
}

func TestBroadcaster_Filter(t *testing.T) {
	b := New[int](16)
	ctx := t.Context()

	// Subscriber wants only even numbers.
	ch, unsub := b.Subscribe(ctx, func(n int) bool { return n%2 == 0 })
	defer unsub()

	for i := range 6 {
		b.Publish(i + 1)
	}

	want := []int{2, 4, 6}
	for _, w := range want {
		select {
		case got := <-ch:
			if got != w {
				t.Errorf("got %d, want %d", got, w)
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("timeout waiting for %d", w)
		}
	}
}

func TestBroadcaster_MultipleSubscribersIndependent(t *testing.T) {
	b := New[string](8)
	ctx := t.Context()

	ch1, unsub1 := b.Subscribe(ctx, nil)
	ch2, unsub2 := b.Subscribe(ctx, nil)
	defer unsub1()
	defer unsub2()

	b.Publish("hello")

	for i, ch := range []<-chan string{ch1, ch2} {
		select {
		case got := <-ch:
			if got != "hello" {
				t.Errorf("subscriber %d: got %q, want %q", i, got, "hello")
			}
		case <-time.After(100 * time.Millisecond):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}
}

func TestBroadcaster_SlowConsumerDropsOldest(t *testing.T) {
	const bufSize = 4
	b := New[int](bufSize)
	ctx := t.Context()

	// Subscribe but never receive — buffer fills, drops accumulate.
	_, unsub := b.Subscribe(ctx, nil)
	defer unsub()

	const total = bufSize * 4
	for i := range total {
		b.Publish(i)
	}

	stats := b.Stats()
	expectedDrops := uint64(total - bufSize)
	if stats.Dropped != expectedDrops {
		t.Errorf("Dropped: got %d, want %d", stats.Dropped, expectedDrops)
	}
}

func TestBroadcaster_ContextCancelClosesChannel(t *testing.T) {
	b := New[int](4)
	ctx, cancel := context.WithCancel(t.Context())

	ch, _ := b.Subscribe(ctx, nil)
	cancel()

	// After cancel, the broadcaster's goroutine closes ch. Allow time for the
	// scheduler to run the unsubscribe goroutine.
	select {
	case _, ok := <-ch:
		if ok {
			t.Errorf("expected channel closed, got value")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("channel not closed after context cancel")
	}

	// Sanity: subscriber removed.
	if got := b.Stats().Subscribers; got != 0 {
		t.Errorf("Subscribers after cancel: got %d, want 0", got)
	}
}

func TestBroadcaster_UnsubscribeIdempotent(t *testing.T) {
	b := New[int](4)
	_, unsub := b.Subscribe(context.Background(), nil)
	unsub()
	unsub() // must not panic
	if got := b.Stats().Subscribers; got != 0 {
		t.Errorf("Subscribers after unsub: got %d, want 0", got)
	}
}

func TestBroadcaster_ConcurrentPublishers(t *testing.T) {
	b := New[int](1024)
	ctx := t.Context()

	ch, unsub := b.Subscribe(ctx, nil)
	defer unsub()

	const writers = 8
	const perWriter = 100

	var wg sync.WaitGroup
	for w := range writers {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for i := range perWriter {
				b.Publish(base*perWriter + i)
			}
		}(w)
	}

	// Receive concurrently with publishers.
	var received atomic.Uint64
	go func() {
		for range ch {
			received.Add(1)
		}
	}()

	wg.Wait()
	// Give receiver time to drain.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) && received.Load() < writers*perWriter {
		time.Sleep(5 * time.Millisecond)
	}

	got := received.Load()
	if got != writers*perWriter {
		t.Errorf("received: got %d, want %d (with no drops)", got, writers*perWriter)
	}
}

func TestBroadcaster_DefaultBufferSize(t *testing.T) {
	b := New[int](0)
	if b.bufSize != 256 {
		t.Errorf("default bufSize: got %d, want 256", b.bufSize)
	}
	b = New[int](-1)
	if b.bufSize != 256 {
		t.Errorf("negative bufSize: got %d, want 256", b.bufSize)
	}
}
