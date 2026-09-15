package main

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/memory"
)

func TestStateInitCLIJSONDirectAndDaemon(t *testing.T) {
	for _, daemon := range []bool{false, true} {
		name := "direct"
		if daemon {
			name = "daemon"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "memory.db")
			if daemon {
				path = startDaemonForTest(t)
			}
			b, err := NewBackend(path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { b.Close() })
			if b.UsingGRPC() != daemon {
				t.Fatalf("gRPC mode = %v", b.UsingGRPC())
			}
			for _, agent := range []string{"", "worker-1"} {
				args := []string{"context", "startup", "--json"}
				if agent != "" {
					args = append(args, "--agent="+agent)
				}
				read := func() memory.State {
					var state memory.State
					out := captureStdout(t, func() {
						if err := stateInit(b, args); err != nil {
							t.Error(err)
						}
					})
					if err := json.Unmarshal([]byte(out), &state); err != nil {
						t.Fatalf("decode %q: %v", out, err)
					}
					return state
				}
				initial := read()
				key := "context"
				if agent != "" {
					key = "agent:" + agent + ":context"
				}
				if initial.Key != key || initial.Agent != agent || initial.Value != "startup" || initial.UpdatedAt.IsZero() {
					t.Fatalf("unexpected initial state %+v", initial)
				}
				if err := b.SetState("context", "reset", agent); err != nil {
					t.Fatal(err)
				}
				expected, err := b.GetState("context", agent)
				if err != nil {
					t.Fatal(err)
				}
				got := read()
				if got.Value != expected.Value || got.Key != expected.Key || got.Agent != expected.Agent || !got.UpdatedAt.Equal(expected.UpdatedAt) {
					t.Fatalf("replay lost reset: %+v != %+v", got, expected)
				}
			}
		})
	}
}
