package store

import (
	"strconv"
	"sync"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func TestStateIncrementConcurrentAndIsolated(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()
	const workers = 48
	results := make(chan string, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			st, err := s.IncrementState("agent:one:iterations", "one")
			if err != nil {
				t.Error(err)
				return
			}
			results <- st.Value
		}()
	}
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for value := range results {
		if seen[value] {
			t.Errorf("duplicate increment %s", value)
		}
		seen[value] = true
	}
	got, err := s.GetState("agent:one:iterations")
	if err != nil || got.Value != strconv.Itoa(workers) || got.Agent != "one" || got.UpdatedAt.IsZero() {
		t.Fatalf("counter: %+v %v", got, err)
	}
	other, err := s.IncrementState("agent:two:iterations", "two")
	if err != nil || other.Value != "1" {
		t.Fatalf("other session: %+v %v", other, err)
	}
}

func TestStateIncrementPreservesInvalidAndOverflowValues(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()
	for _, value := range []string{"garbage", "", "-1", "9223372036854775807"} {
		if err := s.SetState(&memory.State{Key: "counter", Value: value}); err != nil {
			t.Fatal(err)
		}
		if got, err := s.IncrementState("counter", ""); err == nil || got != nil {
			t.Fatalf("invalid counter %q accepted: %+v %v", value, got, err)
		}
		got, err := s.GetState("counter")
		if err != nil || got.Value != value {
			t.Fatalf("invalid counter was overwritten: %+v %v", got, err)
		}
	}
}
