package main

import "time"

// Default configuration constants for cmd/aide.
// Centralised here so that CLI defaults, gRPC adapter fallbacks,
// and MCP wiring all reference the same values.
const (
	// -------------------------------------------------------------------------
	// gRPC / backend
	// -------------------------------------------------------------------------

	// DefaultPingTimeout is the deadline for the gRPC ping used to test
	// whether the MCP server socket is alive.
	DefaultPingTimeout = time.Second

	// -------------------------------------------------------------------------
	// Messages
	// -------------------------------------------------------------------------

	// DefaultMessageTTLSeconds is the default time-to-live for
	// inter-agent messages when no explicit TTL is provided.
	DefaultMessageTTLSeconds = 3600 // 1 hour

	// -------------------------------------------------------------------------
	// Sessions
	// -------------------------------------------------------------------------

	// DefaultSessionCleanupAge is the default maximum age for stale
	// agent-state entries during session init.
	DefaultSessionCleanupAge = 30 * time.Minute

	// DefaultSessionLimit is the default number of recent session
	// memories to fetch during session init.
	DefaultSessionLimit = 3

	// -------------------------------------------------------------------------
	// Memory list
	// -------------------------------------------------------------------------

	// DefaultMemoryListLimit is the default page size when listing
	// memories via the gRPC store adapter.
	DefaultMemoryListLimit = 50

	// -------------------------------------------------------------------------
	// Pprof debug server
	// -------------------------------------------------------------------------

	// DefaultPprofReadTimeout is the read timeout for the pprof HTTP
	// server.
	DefaultPprofReadTimeout = 30 * time.Second

	// DefaultPprofWriteTimeout is the write timeout for the pprof HTTP
	// server. Longer than read because profile collection can take time.
	DefaultPprofWriteTimeout = 60 * time.Second

	// DefaultPprofIdleTimeout is the idle (keep-alive) timeout for the
	// pprof HTTP server.
	DefaultPprofIdleTimeout = 120 * time.Second

	// DefaultPprofShutdownTimeout is how long to wait for in-flight
	// pprof requests during graceful shutdown.
	DefaultPprofShutdownTimeout = 2 * time.Second

	// -------------------------------------------------------------------------
	// MCP lazy-init polling
	// -------------------------------------------------------------------------

	// DefaultMCPPollInterval is the delay between polls when waiting
	// for a lazy-initialised subsystem (code store, watcher) to become
	// ready.
	DefaultMCPPollInterval = 100 * time.Millisecond

	// DefaultMCPPollCount is the maximum number of poll iterations
	// before giving up on a lazy-init subsystem.
	DefaultMCPPollCount = 50

	// -------------------------------------------------------------------------
	// MCP / CLI default limits
	// -------------------------------------------------------------------------

	// DefaultMemorySearchLimit is the default result count for the
	// memory_search MCP tool when the caller omits a limit.
	DefaultMemorySearchLimit = 10

	// DefaultCodeSearchLimit is the default result count for the
	// code_search MCP tool.
	DefaultCodeSearchLimit = 20

	// DefaultCodeRefsLimit is the default result count for the
	// code_references MCP tool.
	DefaultCodeRefsLimit = 50
)
