package grpcapi

import (
	"errors"
	"os"
	"strings"
	"syscall"
)

// ErrSandboxDenied marks a socket connect that was blocked by the calling
// process's sandbox — e.g. Codex CLI sandboxed shell/hook execs deny
// connect(2) with EPERM via seccomp. Distinct from a dead daemon: the sandbox
// rejects the connect before the kernel checks for a listener, so the state
// of the process behind the socket is unknowable from inside the sandbox.
var ErrSandboxDenied = errors.New("socket connect denied by sandbox")

// SandboxNetworkDisabled reports whether the current process runs inside a
// sandbox known to block socket connections. Codex CLI sets
// CODEX_SANDBOX_NETWORK_DISABLED=1 in sandboxed shell and hook execs.
func SandboxNetworkDisabled() bool {
	return os.Getenv("CODEX_SANDBOX_NETWORK_DISABLED") != ""
}

// isConnectDenied reports whether err looks like a connect(2) rejected by a
// seccomp/landlock policy. gRPC wraps the dial error in a status error that
// doesn't always preserve the errno chain, so the errno check has a string
// fallback.
func isConnectDenied(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "operation not permitted") ||
		strings.Contains(msg, "permission denied")
}
