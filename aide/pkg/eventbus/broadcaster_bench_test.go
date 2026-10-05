package eventbus

import (
	"context"
	"fmt"
	"runtime"
	"testing"
)

func BenchmarkBroadcasterSubscriptionChurn(b *testing.B) {
	bus := New[int](16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	baseline := runtime.NumGoroutine()
	b.ReportAllocs()
	for b.Loop() {
		for i := 0; i < 1000; i++ {
			_, unsub := bus.Subscribe(ctx, nil)
			unsub()
		}
	}
	runtime.Gosched()
	b.ReportMetric(float64(runtime.NumGoroutine()-baseline)/float64(b.N), "retained-goroutines/op")
}

func BenchmarkBroadcasterPublish(b *testing.B) {
	for _, count := range []int{1, 16, 128} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			bus := New[int](16)
			for i := 0; i < count; i++ {
				_, unsub := bus.Subscribe(context.Background(), nil)
				b.Cleanup(unsub)
			}
			b.ReportAllocs()
			for b.Loop() {
				bus.Publish(1)
			}
		})
	}
}
