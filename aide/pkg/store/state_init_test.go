package store

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/memory"
	bolt "go.etcd.io/bbolt"
)

func TestStateInitConcurrentStartupAndReset(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()
	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan *memory.State, workers)
	created := make(chan bool, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			st, fresh, err := s.InitState(&memory.State{Key: "context", Value: fmt.Sprintf("startup-%d", i)})
			if err != nil {
				t.Errorf("init: %v", err)
				return
			}
			results <- st
			created <- fresh
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	close(created)
	winner, err := s.GetState("context")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for fresh := range created {
		if fresh {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("created %d states; want one", n)
	}
	for st := range results {
		if !sameInitializedState(st, winner) {
			t.Fatalf("startup did not return winner: %+v != %+v", st, winner)
		}
	}

	// Race replay against an explicit reset. In either serialization order the
	// reset must be the persisted winner; startup never overwrites present state.
	reset := &memory.State{Key: "context", Value: "reset", UpdatedAt: time.Now().Add(time.Minute)}
	start = make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if _, fresh, err := s.InitState(&memory.State{Key: "context", Value: "replayed"}); err != nil || fresh {
				t.Errorf("replay fresh=%v err=%v", fresh, err)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		if err := s.SetState(reset); err != nil {
			t.Error(err)
		}
	}()
	close(start)
	wg.Wait()
	st, fresh, err := s.InitState(&memory.State{Key: "context", Value: "late-startup"})
	if err != nil || fresh || st == nil || !sameInitializedState(st, reset) {
		t.Fatalf("reset overwritten: state=%+v fresh=%v err=%v", st, fresh, err)
	}
}

func TestStateInitPreservesMalformedAndEmptyExistingValues(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()
	for _, value := range []string{"", "malformed payload"} {
		existing := &memory.State{Key: "context", Value: value, UpdatedAt: time.Now().Add(-time.Hour)}
		if err := s.SetState(existing); err != nil {
			t.Fatal(err)
		}
		got, created, err := s.InitState(&memory.State{Key: "context", Value: "replacement"})
		if err != nil || created || got == nil || !sameInitializedState(got, existing) {
			t.Fatalf("existing state changed: %+v %v %v", got, created, err)
		}
	}
	if err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(BucketState).Put([]byte("corrupt"), []byte("invalid json")) }); err != nil {
		t.Fatal(err)
	}
	if got, created, err := s.InitState(&memory.State{Key: "corrupt", Value: "replacement"}); err == nil || created || got != nil {
		t.Fatalf("corrupt persisted record must fail closed: %+v %v %v", got, created, err)
	}
	if err := s.db.View(func(tx *bolt.Tx) error {
		if string(tx.Bucket(BucketState).Get([]byte("corrupt"))) != "invalid json" {
			t.Error("corrupt record overwritten")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func sameInitializedState(a, b *memory.State) bool {
	return a.Key == b.Key && a.Value == b.Value && a.Agent == b.Agent && a.UpdatedAt.Equal(b.UpdatedAt)
}

func TestStateInitBoundedValidatesAndPreservesExistingAtCapacity(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()
	existing := &memory.State{Key: "first", Agent: "queue", Value: "original", UpdatedAt: time.Now().Add(-time.Hour)}
	if err := s.SetState(existing); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		state    *memory.State
		capacity int
	}{
		{nil, 1}, {&memory.State{Agent: "queue"}, 1},
		{&memory.State{Key: " ", Agent: "queue"}, 1},
		{&memory.State{Key: "second"}, 1}, {&memory.State{Key: "second", Agent: " "}, 1},
		{&memory.State{Key: "first", Agent: "queue"}, 0},
		{&memory.State{Key: "second", Agent: "queue"}, -1},
		{&memory.State{Key: "second", Agent: "queue"}, memory.MaxStateAgentEntries + 1},
		{&memory.State{Key: "first", Agent: "other"}, 1},
	} {
		if got, created, err := s.InitStateBounded(tc.state, tc.capacity); err == nil || got != nil || created {
			t.Fatalf("invalid bounded init accepted: %+v %v %v", got, created, err)
		}
	}
	got, created, err := s.InitStateBounded(&memory.State{Key: "first", Agent: "queue", Value: "replacement"}, 1)
	if err != nil || created || got == nil || !sameInitializedState(got, existing) {
		t.Fatalf("replay changed existing at capacity: %+v %v %v", got, created, err)
	}
	if got, created, err := s.InitStateBounded(&memory.State{Key: "second", Agent: "queue"}, 1); !errors.Is(err, memory.ErrStateAgentLimit) || got != nil || created {
		t.Fatalf("capacity not enforced: %+v %v %v", got, created, err)
	}
	states, err := s.ListState("")
	if err != nil || len(states) != 1 || !sameInitializedState(states[0], existing) {
		t.Fatalf("rejected calls mutated states: %+v %v", states, err)
	}
}

func TestStateInitBoundedConcurrentNamespaces(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()
	if err := s.SetState(&memory.State{Key: "global", Value: "excluded from agent capacity"}); err != nil {
		t.Fatal(err)
	}
	const capacity = 8
	const workers = 64
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			agent := "queue"
			if i%2 != 0 {
				agent = "queue:child"
			}
			proposed := &memory.State{Key: fmt.Sprintf("%s:%d", agent, i), Agent: agent, Value: "new"}
			got, created, err := s.InitStateBounded(proposed, capacity)
			if err != nil {
				if !errors.Is(err, memory.ErrStateAgentLimit) || got != nil || created {
					t.Errorf("unexpected failure: %+v %v %v", got, created, err)
				}
				return
			}
			if !created || got == nil || got.Agent != agent || got.UpdatedAt.IsZero() {
				t.Errorf("invalid insertion: %+v %v", got, created)
			}
			if !proposed.UpdatedAt.IsZero() {
				t.Error("initializer mutated caller state")
			}
		}(i)
	}
	close(start)
	wg.Wait()
	for _, agent := range []string{"queue", "queue:child"} {
		states, err := s.ListState(agent)
		if err != nil || len(states) != capacity {
			t.Fatalf("agent %s capacity=%d want=%d err=%v", agent, len(states), capacity, err)
		}
	}
	// Another agent and global state do not consume this namespace's capacity.
	got, created, err := s.InitStateBounded(&memory.State{Key: "independent", Agent: "independent"}, 1)
	if err != nil || !created || got == nil {
		t.Fatalf("independent namespace blocked: %+v %v %v", got, created, err)
	}
}

func TestStateInitBoundedFailsClosedOnMalformedStoredEntries(t *testing.T) {
	for _, raw := range []string{"invalid json", "null", `{}`, `{"key":"different","agent":"queue"}`, `{"key":"corrupt","agent":5}`} {
		t.Run(raw, func(t *testing.T) {
			s, cleanup := setupTestDB(t)
			defer cleanup()
			existing := &memory.State{Key: "existing", Agent: "queue", Value: "keep", UpdatedAt: time.Now().Add(-time.Hour)}
			if err := s.SetState(existing); err != nil {
				t.Fatal(err)
			}
			if err := s.db.Update(func(tx *bolt.Tx) error { return tx.Bucket(BucketState).Put([]byte("corrupt"), []byte(raw)) }); err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"corrupt", "new"} {
				if got, created, err := s.InitStateBounded(&memory.State{Key: key, Agent: "queue"}, 3); err == nil || created || got != nil {
					t.Fatalf("corruption ignored: %+v %v %v", got, created, err)
				}
			}
			got, created, err := s.InitStateBounded(&memory.State{Key: "existing", Agent: "queue", Value: "replace"}, 1)
			if err != nil || created || !sameInitializedState(got, existing) {
				t.Fatalf("unrelated corrupt record broke idempotent lookup: %+v %v %v", got, created, err)
			}
			if err := s.db.View(func(tx *bolt.Tx) error {
				b := tx.Bucket(BucketState)
				if string(b.Get([]byte("corrupt"))) != raw || b.Get([]byte("new")) != nil {
					t.Error("failed transaction changed stored records")
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestCombinedStoreDelegatesBoundedStateInit(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()
	combined := &CombinedStore{bolt: s}
	var initializer memory.BoundedStateInitializer = combined
	got, created, err := initializer.InitStateBounded(&memory.State{Key: "k", Agent: "queue"}, 1)
	if err != nil || !created || got == nil {
		t.Fatalf("combined bounded initialization: %+v %v %v", got, created, err)
	}
	if _, _, err := initializer.InitStateBounded(&memory.State{Key: "other", Agent: "queue"}, 1); !errors.Is(err, memory.ErrStateAgentLimit) {
		t.Fatalf("combined store bypassed bound: %v", err)
	}
}

func TestStateInitBoundedConcurrentSameKeyPreservesWinner(t *testing.T) {
	s, cleanup := setupTestDB(t)
	defer cleanup()
	const workers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan *memory.State, workers)
	created := make(chan bool, workers)
	timestamp := time.Now().Add(-time.Hour)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			got, fresh, err := s.InitStateBounded(&memory.State{Key: "same", Agent: "queue", Value: fmt.Sprint(i), UpdatedAt: timestamp}, 1)
			if err != nil {
				t.Errorf("same-key init at capacity: %v", err)
				return
			}
			results <- got
			created <- fresh
		}(i)
	}
	close(start)
	wg.Wait()
	close(results)
	close(created)
	winner, err := s.GetState("same")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for fresh := range created {
		if fresh {
			count++
		}
	}
	if count != 1 || !winner.UpdatedAt.Equal(timestamp) {
		t.Fatalf("winner count=%d state=%+v", count, winner)
	}
	for got := range results {
		if !sameInitializedState(got, winner) {
			t.Fatalf("inconsistent bounded replay: %+v != %+v", got, winner)
		}
	}
}
