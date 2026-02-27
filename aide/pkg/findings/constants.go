package findings

import "time"

// Default configuration constants for the findings analysers.
// These are the single source of truth — referenced by each analyser's
// defaults, the Runner fallback logic, the CLI help text, and the MCP
// wiring. Keep them here so a value change is a one-line diff.
const (
	// -------------------------------------------------------------------------
	// Complexity analyser
	// -------------------------------------------------------------------------

	// DefaultComplexityThreshold is the minimum cyclomatic complexity to
	// report. Functions below this are not flagged. At threshold 15 only
	// genuinely complex functions surface; the critical boundary is 2×
	// threshold (30).
	DefaultComplexityThreshold = 15

	// -------------------------------------------------------------------------
	// Coupling analyser
	// -------------------------------------------------------------------------

	// DefaultFanOutThreshold is the minimum import count for a file to be
	// flagged as having excessive fan-out.
	DefaultFanOutThreshold = 15

	// DefaultFanInThreshold is the minimum number of dependents for a
	// project-local file to be flagged as a fragile dependency.
	DefaultFanInThreshold = 20

	// DefaultMaxCycleFindings caps the number of import-cycle findings
	// emitted per analysis run. Large codebases can have many cycles;
	// beyond this limit the signal-to-noise ratio drops.
	DefaultMaxCycleFindings = 50

	// -------------------------------------------------------------------------
	// Secrets analyser
	// -------------------------------------------------------------------------

	// DefaultSecretsMaxFileSize is the maximum file size (in bytes) the
	// secrets analyser will scan. Larger files are skipped.
	DefaultSecretsMaxFileSize int64 = 1 << 20 // 1 MiB

	// DefaultSnippetTruncateLen is the maximum character length of a
	// context line in a secrets finding before it is truncated.
	DefaultSnippetTruncateLen = 120

	// -------------------------------------------------------------------------
	// Runner defaults
	// -------------------------------------------------------------------------

	// DefaultCloneWindowSize mirrors clone.DefaultWindowSize for use by
	// the Runner, which cannot import pkg/findings/clone (import cycle).
	DefaultCloneWindowSize = 50

	// DefaultCloneMinLines mirrors clone.DefaultMinCloneLines for the
	// same reason.
	DefaultCloneMinLines = 20

	// DefaultRunnerSecretsMaxFileSize is the per-file size limit used by
	// the Runner's incremental secrets analyser. Larger than the
	// standalone default because the runner already filters by extension.
	DefaultRunnerSecretsMaxFileSize int64 = 10 << 20 // 10 MiB

	// DefaultRunnerConcurrency is the maximum number of concurrent
	// per-file analyser goroutines the Runner will launch.
	DefaultRunnerConcurrency = 16

	// DefaultRunnerStopTimeout is how long Runner.Stop() waits for
	// in-flight analysers to finish before giving up.
	DefaultRunnerStopTimeout = 30 * time.Second

	// -------------------------------------------------------------------------
	// Severity scaling
	// -------------------------------------------------------------------------

	// SeverityCriticalMultiplier is the factor applied to a threshold to
	// derive the critical-severity boundary. For example, with a
	// complexity threshold of 15, functions at 30+ are critical.
	SeverityCriticalMultiplier = 2

	// -------------------------------------------------------------------------
	// Search / list defaults
	// -------------------------------------------------------------------------

	// DefaultSearchLimit is the default result count for findings
	// full-text search (store, CLI, and MCP tool).
	DefaultSearchLimit = 20

	// DefaultListLimit is the default result count for findings list
	// queries (store, CLI, and MCP tool).
	DefaultListLimit = 100
)
