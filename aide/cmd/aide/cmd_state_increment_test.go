package main

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func TestStateIncrementCLIAndConcurrentDaemon(t *testing.T) {
	for _, daemon := range []bool{false, true} {
		name := "direct"
		if daemon {
			name = "daemon"
		}
		t.Run(name, func(t *testing.T) {
			db := filepath.Join(t.TempDir(), "memory.db")
			if daemon {
				db = startDaemonForTest(t)
			}
			b, err := NewBackend(db)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			for _, agent := range []string{"", "session-one", "session-two"} {
				args := []string{"iterations", "--json"}
				if agent != "" {
					args = append(args, "--agent="+agent)
				}
				out := captureStdout(t, func() {
					if err := stateIncrement(b, args); err != nil {
						t.Error(err)
					}
				})
				var got memory.State
				if err := json.Unmarshal([]byte(out), &got); err != nil {
					t.Fatal(err)
				}
				if got.Value != "1" || got.Agent != agent {
					t.Fatalf("initial counter: %+v", got)
				}
			}
			const workers = 32
			var wg sync.WaitGroup
			for i := 0; i < workers; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					if _, err := b.IncrementState("iterations", "session-one"); err != nil {
						t.Error(err)
					}
				}()
			}
			wg.Wait()
			got, err := b.GetState("iterations", "session-one")
			if err != nil || got.Value != strconv.Itoa(workers+1) {
				t.Fatalf("counter lost increments: %+v %v", got, err)
			}
			other, err := b.GetState("iterations", "session-two")
			if err != nil || other.Value != "1" {
				t.Fatalf("session isolation: %+v %v", other, err)
			}
		})
	}
}
