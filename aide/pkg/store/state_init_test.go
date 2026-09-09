package store

import (
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
