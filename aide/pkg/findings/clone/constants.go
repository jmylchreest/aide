package clone

// Default configuration values for the clone detection analyser.
// These are the single source of truth — referenced by detect.go defaults(),
// the CLI help text, the runner, and the effective-values display.
const (
	// DefaultWindowSize is the number of tokens in the sliding hash window.
	DefaultWindowSize = 50

	// DefaultMinCloneLines is the minimum source-line span for a clone to
	// be reported. Set to 20 to balance actionable signal (warning+ at 50+
	// lines) against info-level noise. Industry tools vary: PMD CPD ~8-15
	// via 100 tokens, SonarQube 10 lines.
	DefaultMinCloneLines = 20

	// DefaultMinMatchCount is the minimum number of matching hash windows
	// required per clone region. Regions with fewer matches are filtered.
	DefaultMinMatchCount = 2

	// DefaultMaxBucketSize caps how many locations a single hash may
	// appear in before it is considered boilerplate and excluded.
	// A value of 0 means unlimited (no cap).
	DefaultMaxBucketSize = 10

	// DefaultMinSimilarity is the minimum similarity ratio (0.0–1.0).
	// 0.0 disables the filter entirely.
	DefaultMinSimilarity = 0.0

	// DefaultSevWarningLines is the line-span threshold at which a clone
	// finding is promoted from info to warning severity.
	DefaultSevWarningLines = 50

	// DefaultSevCriticalLines is the line-span threshold at which a clone
	// finding is promoted from warning to critical severity.
	DefaultSevCriticalLines = 100

	// DefaultMinSeverity is the minimum severity level for clone findings
	// to be emitted. Findings below this threshold are silently dropped.
	// Set to "warning" so that info-level clones (mostly boilerplate/noise)
	// are not stored by default.
	DefaultMinSeverity = "warning"

	// DefaultLanguageIsolation restricts clone detection to same-language
	// file pairs.
	DefaultLanguageIsolation = true

	// MaxFileSize is the maximum file size (in bytes) processed by the
	// clone detector. Files larger than this are skipped.
	MaxFileSize = 512 * 1024
)
