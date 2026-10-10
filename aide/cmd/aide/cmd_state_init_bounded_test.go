package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jmylchreest/aide/aide/pkg/grpcapi"
	"github.com/jmylchreest/aide/aide/pkg/memory"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func boundedTestBackend(t *testing.T, daemon bool) *Backend {
	t.Helper()
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
		t.Fatalf("gRPC mode = %v, want %v", b.UsingGRPC(), daemon)
	}
	return b
}

func boundedCLIResult(t *testing.T, b *Backend, args []string) (string, error) {
	t.Helper()
	var callErr error
	output := captureStdout(t, func() { callErr = stateInitBounded(b, args) })
	return output, callErr
}

func TestStateInitBoundedCLIDirectAndDaemon(t *testing.T) {
	for _, daemon := range []bool{false, true} {
		t.Run(fmt.Sprintf("daemon=%v", daemon), func(t *testing.T) {
			b := boundedTestBackend(t, daemon)
			read := func(key, value, agent string) memory.State {
				t.Helper()
				output, err := boundedCLIResult(t, b, []string{key, value, "--agent=" + agent, "--max-agent-entries=1", "--json"})
				if err != nil {
					t.Fatal(err)
				}
				var state memory.State
				if err := json.Unmarshal([]byte(output), &state); err != nil {
					t.Fatalf("decode %q: %v", output, err)
				}
				return state
			}
			first := read("item", "initial", "queue")
			if first.Key != "agent:queue:item" || first.Agent != "queue" || first.Value != "initial" || first.UpdatedAt.IsZero() {
				t.Fatalf("unexpected state: %+v", first)
			}
			replay := read("item", "replacement", "queue")
			if replay.Key != first.Key || replay.Value != first.Value || replay.Agent != first.Agent || !replay.UpdatedAt.Equal(first.UpdatedAt) {
				t.Fatalf("replay mutated state: %+v != %+v", replay, first)
			}
			output, err := boundedCLIResult(t, b, []string{"second", "overflow", "--agent=queue", "--max-agent-entries=1", "--json"})
			if err == nil || output != "" {
				t.Fatalf("capacity accepted: %q %v", output, err)
			}
			if daemon && status.Code(err) != codes.ResourceExhausted {
				t.Fatalf("daemon cap code=%v err=%v", status.Code(err), err)
			}
			if !daemon && !errors.Is(err, memory.ErrStateAgentLimit) {
				t.Fatalf("direct cap did not retain sentinel: %v", err)
			}
			other := read("item", "independent", "queue:child")
			if other.Key != "agent:queue:child:item" || other.Agent != "queue:child" {
				t.Fatalf("namespace lost: %+v", other)
			}
			states, err := b.ListState("")
			if err != nil || len(states) != 2 {
				t.Fatalf("wrong persisted state count: %+v %v", states, err)
			}
		})
	}
}

func TestStateInitBoundedCLIInvalidLimitsDoNotMutate(t *testing.T) {
	for _, daemon := range []bool{false, true} {
		t.Run(fmt.Sprintf("daemon=%v", daemon), func(t *testing.T) {
			b := boundedTestBackend(t, daemon)
			invalid := make([][]string, 0, 12)
			invalid = append(invalid, [][]string{
				{}, {"key"}, {"key", "value", "--agent=queue", "--json"},
				{"key", "value", "--max-agent-entries=1", "--json"},
				{"", "value", "--agent=queue", "--max-agent-entries=1", "--json"},
			}...)
			for _, limit := range []string{"", "0", "-1", "1.5", "invalid", fmt.Sprint(memory.MaxStateAgentEntries + 1), "18446744073709551616"} {
				invalid = append(invalid, []string{"key", "value", "--agent=queue", "--max-agent-entries=" + limit, "--json"})
			}
			for _, args := range invalid {
				output, err := boundedCLIResult(t, b, args)
				if err == nil || output != "" {
					t.Errorf("invalid args accepted: %q output=%q err=%v", args, output, err)
				}
			}
			states, err := b.ListState("")
			if err != nil || len(states) != 0 {
				t.Fatalf("invalid CLI calls mutated state: %+v %v", states, err)
			}
		})
	}
}

// This client supplies exactly the error returned by a pre-InitBounded daemon.
// The separate gRPC suite exercises a real old service descriptor over bufconn.
type unsupportedBoundedStateClient struct {
	grpcapi.StateServiceClient
	inits int
	sets  int
}

func (s *unsupportedBoundedStateClient) InitBounded(context.Context, *grpcapi.StateBoundedInitRequest, ...grpc.CallOption) (*grpcapi.StateSetResponse, error) {
	return nil, status.Error(codes.Unimplemented, "unknown method InitBounded")
}
func (s *unsupportedBoundedStateClient) Init(context.Context, *grpcapi.StateSetRequest, ...grpc.CallOption) (*grpcapi.StateSetResponse, error) {
	s.inits++
	return &grpcapi.StateSetResponse{}, nil
}
func (s *unsupportedBoundedStateClient) Set(context.Context, *grpcapi.StateSetRequest, ...grpc.CallOption) (*grpcapi.StateSetResponse, error) {
	s.sets++
	return &grpcapi.StateSetResponse{}, nil
}

func TestStateInitBoundedBackendPropagatesUnimplementedWithoutFallback(t *testing.T) {
	legacy := &unsupportedBoundedStateClient{}
	b := &Backend{useGRPC: true, grpcClient: &grpcapi.Client{State: legacy}}
	output, err := boundedCLIResult(t, b, []string{"item", "value", "--agent=queue", "--max-agent-entries=1", "--json"})
	if status.Code(err) != codes.Unimplemented || output != "" {
		t.Fatalf("unsupported bounded RPC reported success: %q %v", output, err)
	}
	if legacy.inits != 0 || legacy.sets != 0 {
		t.Fatalf("backend used unsafe fallback: Init=%d Set=%d", legacy.inits, legacy.sets)
	}
}
