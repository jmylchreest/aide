package eventbus

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestUnsubscribeReleasesLiveContextWaiters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := New[int](1)
	before := runtime.NumGoroutine()
	for i := 0; i < 100; i++ {
		_, unsub := bus.Subscribe(ctx, nil)
		unsub()
	}
	if delta := runtime.NumGoroutine() - before; delta > 5 {
		t.Fatalf("unsubscribing retained %d goroutines while parent context is live", delta)
	}
}

func TestUnsubscribeDuringPublishFilter(t *testing.T) {
	bus := New[int](1)
	entered, release := make(chan struct{}), make(chan struct{})
	ch, unsub := bus.Subscribe(t.Context(), func(int) bool { close(entered); <-release; return true })
	result := make(chan any, 1)
	go func() { defer func() { result <- recover() }(); bus.Publish(1) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("publisher did not enter filter")
	}
	unsub()
	close(release)
	select {
	case failure := <-result:
		if failure != nil {
			t.Fatalf("publishing to an unsubscribed snapshot panicked: %v", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("publisher did not finish")
	}
	if _, open := <-ch; open {
		t.Fatal("unsubscribe did not close channel")
	}
}
