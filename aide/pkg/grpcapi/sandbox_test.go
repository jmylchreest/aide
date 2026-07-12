package grpcapi

import (
	"errors"
	"fmt"
	"syscall"
	"testing"
)

func TestIsConnectDenied(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"wrapped EPERM", fmt.Errorf("dial unix: %w", syscall.EPERM), true},
		{"wrapped EACCES", fmt.Errorf("dial unix: %w", syscall.EACCES), true},
		// gRPC status errors flatten the chain to a string
		{"grpc dial string", errors.New(`rpc error: code = Unavailable desc = connection error: desc = "transport: Error while dialing: dial unix .aide/aide.sock: connect: operation not permitted"`), true},
		{"permission denied string", errors.New("connect: permission denied"), true},
		{"connection refused", fmt.Errorf("dial unix: %w", syscall.ECONNREFUSED), false},
		{"timeout", errors.New("context deadline exceeded"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isConnectDenied(tt.err); got != tt.want {
				t.Errorf("isConnectDenied(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestSandboxNetworkDisabled(t *testing.T) {
	t.Setenv("CODEX_SANDBOX_NETWORK_DISABLED", "")
	if SandboxNetworkDisabled() {
		t.Error("expected false with empty env var")
	}
	t.Setenv("CODEX_SANDBOX_NETWORK_DISABLED", "1")
	if !SandboxNetworkDisabled() {
		t.Error("expected true with env var set")
	}
}
