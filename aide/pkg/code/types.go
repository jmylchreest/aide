// Package code provides code indexing and symbol extraction.
package code

import "time"

// Symbol represents a code symbol (function, method, class, etc.)
type Symbol struct {
	ID            string    `json:"id"`                   // ULID
	Name          string    `json:"name"`                 // Symbol name (e.g., "getUser")
	Kind          string    `json:"kind"`                 // function, method, class, interface, type
	Signature     string    `json:"signature"`            // Full signature (e.g., "async getUser(id: string): Promise<User>")
	DocComment    string    `json:"doc,omitempty"`        // Leading doc comment
	FilePath      string    `json:"file"`                 // Relative file path
	StartLine     int       `json:"start"`                // Line number (1-indexed)
	EndLine       int       `json:"end"`                  // End line number
	BodyStartLine int       `json:"bodyStart,omitempty"`  // Body start line (1-indexed, 0 if no body)
	BodyEndLine   int       `json:"bodyEnd,omitempty"`    // Body end line (1-indexed, 0 if no body)
	Complexity    int       `json:"complexity,omitempty"` // Cyclomatic complexity (0 = not computed)
	Language      string    `json:"lang"`                 // typescript, javascript, go, python
	CreatedAt     time.Time `json:"createdAt"`
}

// Reference represents a usage/call site of a symbol.
type Reference struct {
	ID         string    `json:"id"`            // ULID
	SymbolName string    `json:"symbol"`        // Name of the referenced symbol (e.g., "getUser")
	Kind       string    `json:"kind"`          // call, type_ref, import
	FilePath   string    `json:"file"`          // File where the reference occurs
	Line       int       `json:"line"`          // Line number (1-indexed)
	Column     int       `json:"col"`           // Column number (0-indexed)
	Context    string    `json:"ctx,omitempty"` // Surrounding code context
	Language   string    `json:"lang"`          // Language of the file
	CreatedAt  time.Time `json:"createdAt"`
}

// ReferenceKind constants
const (
	RefKindCall    = "call"     // Function/method call
	RefKindTypeRef = "type_ref" // Type reference
	RefKindImport  = "import"   // Import statement
)

// SymbolKind constants
const (
	KindFunction  = "function"
	KindMethod    = "method"
	KindClass     = "class"
	KindInterface = "interface"
	KindType      = "type"
	KindVariable  = "variable"
	KindConstant  = "constant"
)

// FileInfo tracks indexed files for incremental updates
type FileInfo struct {
	Path      string    `json:"path"`
	ModTime   time.Time `json:"modTime"`
	SymbolIDs []string  `json:"symbols"` // Symbol IDs in this file
}

// Language constants
const (
	LangTypeScript = "typescript"
	LangJavaScript = "javascript"
	LangGo         = "go"
	LangPython     = "python"
	LangRust       = "rust"
	LangJava       = "java"
	LangC          = "c"
	LangCPP        = "cpp"
	LangCSharp     = "csharp"
	LangRuby       = "ruby"
	LangPHP        = "php"
	LangSwift      = "swift"
	LangKotlin     = "kotlin"
	LangScala      = "scala"
	LangElixir     = "elixir"
	LangLua        = "lua"
	LangBash       = "bash"
	LangSQL        = "sql"
	LangHTML       = "html"
	LangCSS        = "css"
	LangYAML       = "yaml"
	LangTOML       = "toml"
	LangJSON       = "json"
	LangProtobuf   = "protobuf"
	LangHCL        = "hcl"
	LangDockerfile = "dockerfile"
	LangOCaml      = "ocaml"
	LangElm        = "elm"
	LangGroovy     = "groovy"
)

// SearchOptions for filtering symbol searches
type SearchOptions struct {
	Kind     string // Filter by symbol kind
	Language string // Filter by language
	FilePath string // Filter by file path pattern
	Limit    int    // Max results (0 = default)
}

// IndexStats contains indexing statistics
type IndexStats struct {
	Files      int `json:"files"`
	Symbols    int `json:"symbols"`
	References int `json:"references"`
}

// ReferenceSearchOptions for filtering reference searches
type ReferenceSearchOptions struct {
	SymbolName string // Filter by symbol name (required)
	Kind       string // Filter by reference kind (call, type_ref, import)
	FilePath   string // Filter by file path pattern
	Limit      int    // Max results (0 = default)
}

// WatcherConfig contains file watcher configuration
type WatcherConfig struct {
	Enabled       bool          // Enable file watching
	Paths         []string      // Paths to watch (empty = cwd)
	DebounceDelay time.Duration // Delay before reindexing (default 30s)
}

// DefaultDebounceDelay is the default delay before reindexing after file changes.
// Set high (30s) to handle Claude's rapid multi-file edit patterns.
const DefaultDebounceDelay = 30 * time.Second
