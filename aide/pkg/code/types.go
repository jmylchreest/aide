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

// LangExtensions maps file extensions to languages
var LangExtensions = map[string]string{
	// TypeScript/JavaScript
	".ts":  LangTypeScript,
	".tsx": LangTypeScript,
	".js":  LangJavaScript,
	".jsx": LangJavaScript,
	".mjs": LangJavaScript,
	".cjs": LangJavaScript,
	// Go
	".go": LangGo,
	// Python
	".py":  LangPython,
	".pyw": LangPython,
	".pyi": LangPython,
	// Rust
	".rs": LangRust,
	// Java
	".java": LangJava,
	// C/C++
	".c":   LangC,
	".h":   LangC,
	".cpp": LangCPP,
	".cc":  LangCPP,
	".cxx": LangCPP,
	".hpp": LangCPP,
	".hh":  LangCPP,
	".hxx": LangCPP,
	// C#
	".cs": LangCSharp,
	// Ruby
	".rb":   LangRuby,
	".rake": LangRuby,
	// PHP
	".php": LangPHP,
	// Swift
	".swift": LangSwift,
	// Kotlin
	".kt":  LangKotlin,
	".kts": LangKotlin,
	// Scala
	".scala": LangScala,
	".sc":    LangScala,
	// Elixir
	".ex":  LangElixir,
	".exs": LangElixir,
	// Lua
	".lua": LangLua,
	// Shell/Bash
	".sh":   LangBash,
	".bash": LangBash,
	".zsh":  LangBash,
	// SQL
	".sql": LangSQL,
	// Web
	".html": LangHTML,
	".htm":  LangHTML,
	".css":  LangCSS,
	".scss": LangCSS,
	".less": LangCSS,
	// Config
	".yaml": LangYAML,
	".yml":  LangYAML,
	".toml": LangTOML,
	".json": LangJSON,
	".hcl":  LangHCL,
	".tf":   LangHCL,
	// Proto
	".proto": LangProtobuf,
	// Docker
	"Dockerfile": LangDockerfile,
	// OCaml
	".ml":  LangOCaml,
	".mli": LangOCaml,
	// Elm
	".elm": LangElm,
	// Groovy
	".groovy": LangGroovy,
	".gradle": LangGroovy,
}

// LangFilenames maps known filenames (without extension) to languages.
var LangFilenames = map[string]string{
	"Makefile":       LangBash,
	"GNUmakefile":    LangBash,
	"Jenkinsfile":    LangGroovy,
	"Vagrantfile":    LangRuby,
	"Rakefile":       LangRuby,
	"Gemfile":        LangRuby,
	"BUILD":          LangPython, // Bazel
	"BUILD.bazel":    LangPython,
	"WORKSPACE":      LangPython, // Bazel
	"SConstruct":     LangPython,
	"SConscript":     LangPython,
	"CMakeLists.txt": LangBash, // Close enough for symbol extraction
}

// ShebangLangs maps shebang interpreter names to languages.
var ShebangLangs = map[string]string{
	"python":  LangPython,
	"python2": LangPython,
	"python3": LangPython,
	"ruby":    LangRuby,
	"bash":    LangBash,
	"sh":      LangBash,
	"zsh":     LangBash,
	"node":    LangJavaScript,
	"deno":    LangTypeScript,
	"bun":     LangTypeScript,
	"lua":     LangLua,
	"perl":    LangBash, // Best-effort
	"php":     LangPHP,
	"elixir":  LangElixir,
	"groovy":  LangGroovy,
	"swift":   LangSwift,
	"kotlin":  LangKotlin,
	"scala":   LangScala,
}

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
