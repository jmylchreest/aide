// Package survey defines types for codebase structural observations.
package survey

import "time"

// Kind classifies what a survey entry describes.
const (
	KindModule      = "module"       // Package/module/workspace member
	KindEntrypoint  = "entrypoint"   // main(), HTTP handler mount, CLI root, etc.
	KindDependency  = "dependency"   // External dependency or internal module relationship
	KindTechStack   = "tech_stack"   // Language, framework, build system detected
	KindChurn       = "churn"        // Git history hotspot (high-change file/dir)
	KindSubmodule   = "submodule"    // Git submodule reference
	KindWorkspace   = "workspace"    // Monorepo workspace root (npm, go, cargo, etc.)
	KindArchPattern = "arch_pattern" // Detected architectural pattern (MVC, hexagonal, etc.)
)

// Analyzer names — who produced this entry.
const (
	AnalyzerTopology    = "topology"    // Repo structure, build systems, workspaces
	AnalyzerEntrypoints = "entrypoints" // Entry point detection
	AnalyzerChurn       = "churn"       // Git history analysis
)

// Default result limits.
const (
	DefaultSearchLimit = 20
	DefaultListLimit   = 100
)

// Entry represents a single survey observation about the codebase.
type Entry struct {
	ID        string            `json:"id"`                 // ULID
	Analyzer  string            `json:"analyzer"`           // Who produced this: topology, entrypoints, churn
	Kind      string            `json:"kind"`               // What this describes: module, entrypoint, dependency, ...
	Name      string            `json:"name"`               // Human-readable label (e.g. "aide/pkg/store", "main.go:main()")
	FilePath  string            `json:"file,omitempty"`     // Relative file or directory path (empty for repo-level)
	Title     string            `json:"title"`              // Short summary
	Detail    string            `json:"detail,omitempty"`   // Extended explanation
	Metadata  map[string]string `json:"metadata,omitempty"` // Analyzer-specific data (e.g. "language":"go", "commits":"142")
	CreatedAt time.Time         `json:"createdAt"`
}

// SearchOptions for filtering survey entries.
type SearchOptions struct {
	Analyzer string // Filter by analyzer name
	Kind     string // Filter by kind
	FilePath string // Filter by file path pattern (substring)
	Limit    int    // Max results (0 = default)
}

// Stats holds aggregate counts of survey entries.
type Stats struct {
	Total      int            `json:"total"`
	ByAnalyzer map[string]int `json:"byAnalyzer"`
	ByKind     map[string]int `json:"byKind"`
}

// SearchResult pairs a survey entry with its search relevance score.
type SearchResult struct {
	Entry *Entry
	Score float64
}

// CodeSearcher abstracts the code index for the entrypoints analyzer.
// This avoids importing pkg/store (which imports pkg/survey) and breaking
// the import cycle.
type CodeSearcher interface {
	// FindSymbols searches for symbols matching the query string.
	// kind filters by symbol kind (empty = all). limit caps results (0 = default).
	FindSymbols(query string, kind string, limit int) ([]SymbolHit, error)

	// FindReferences searches for references to the given symbol name.
	// kind filters by reference kind (empty = all). limit caps results (0 = default).
	FindReferences(symbolName string, kind string, limit int) ([]ReferenceHit, error)
}

// SymbolHit is a simplified symbol result for the entrypoints analyzer.
type SymbolHit struct {
	Name     string
	Kind     string // function, method, class, interface, type
	FilePath string
	Line     int
	EndLine  int // End line of the symbol (0 if unknown)
	Language string
}

// ReferenceHit is a simplified reference result for the entrypoints analyzer.
type ReferenceHit struct {
	Symbol   string // Name of the referenced symbol
	Kind     string // call, type_ref, import
	FilePath string
	Line     int
}

// CodeGrapher extends CodeSearcher with methods needed for call-graph traversal.
// Kept separate so the entrypoints analyzer doesn't need these capabilities.
type CodeGrapher interface {
	CodeSearcher

	// GetFileReferences returns all references (call sites) in a file.
	GetFileReferences(filePath string) ([]ReferenceHit, error)

	// GetContainingSymbol returns the narrowest symbol that spans the given line.
	// Returns nil, nil if no symbol contains the line or the file is not indexed.
	GetContainingSymbol(filePath string, line int) (*SymbolHit, error)
}

// GraphNode represents a symbol in the call graph.
type GraphNode struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	FilePath string `json:"file"`
	Line     int    `json:"line"`
	EndLine  int    `json:"endLine,omitempty"`
	Language string `json:"lang,omitempty"`
}

// GraphEdge represents a call relationship between two symbols.
type GraphEdge struct {
	From     string `json:"from"` // Caller symbol name
	To       string `json:"to"`   // Callee symbol name
	Kind     string `json:"kind"` // call, type_ref
	FilePath string `json:"file"` // Where the reference occurs
	Line     int    `json:"line"` // Line of the reference
}

// CallGraph is the result of a BFS/DFS traversal over the code index.
type CallGraph struct {
	Root  string      `json:"root"`  // Starting symbol name
	Nodes []GraphNode `json:"nodes"` // All symbols in the graph
	Edges []GraphEdge `json:"edges"` // Call relationships
	Depth int         `json:"depth"` // Max depth reached
}
