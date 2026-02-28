// Package findings defines types for static analysis findings.
package findings

import "time"

// Severity levels for findings.
const (
	SevCritical = "critical"
	SevWarning  = "warning"
	SevInfo     = "info"
)

// SeverityRank returns a numeric rank for the given severity level:
// info=0, warning=1, critical=2. Unknown values return -1.
func SeverityRank(sev string) int {
	switch sev {
	case SevInfo:
		return 0
	case SevWarning:
		return 1
	case SevCritical:
		return 2
	default:
		return -1
	}
}

// Analyzer names.
const (
	AnalyzerComplexity = "complexity"
	AnalyzerCoupling   = "coupling"
	AnalyzerSecrets    = "secrets"
	AnalyzerClones     = "clones"
)

// Finding represents a single static analysis finding.
type Finding struct {
	ID        string            `json:"id"`                 // ULID
	Analyzer  string            `json:"analyzer"`           // "complexity", "coupling", "secrets", "clones"
	Severity  string            `json:"severity"`           // "critical", "warning", "info"
	Category  string            `json:"category,omitempty"` // Sub-category within analyzer
	FilePath  string            `json:"file"`               // Relative file path
	Line      int               `json:"line"`               // Start line (1-indexed)
	EndLine   int               `json:"endLine,omitempty"`  // End line (0 = single line)
	Title     string            `json:"title"`              // Short description
	Detail    string            `json:"detail,omitempty"`   // Extended explanation
	Metadata  map[string]string `json:"metadata,omitempty"` // Analyzer-specific data
	Accepted  bool              `json:"accepted,omitempty"` // Acknowledged/accepted by user
	CreatedAt time.Time         `json:"createdAt"`
}

// SearchOptions for filtering findings.
type SearchOptions struct {
	Analyzer        string // Filter by analyzer name
	Severity        string // Filter by severity
	FilePath        string // Filter by file path pattern (substring)
	Category        string // Filter by category
	Limit           int    // Max results (0 = default)
	IncludeAccepted bool   // Include accepted findings (default: hide them)
}

// Stats holds aggregate counts of findings.
type Stats struct {
	Total      int            `json:"total"`
	ByAnalyzer map[string]int `json:"byAnalyzer"`
	BySeverity map[string]int `json:"bySeverity"`
}

// SearchResult pairs a finding with its search relevance score.
type SearchResult struct {
	Finding *Finding
	Score   float64
}
