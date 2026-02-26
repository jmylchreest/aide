package grammar

import (
	"sync"
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"

	// Core compiled-in grammar bindings (9 languages).
	tree_sitter_zig "github.com/tree-sitter-grammars/tree-sitter-zig/bindings/go"
	tree_sitter_c "github.com/tree-sitter/tree-sitter-c/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_go "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tree_sitter_typescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// builtinGrammar holds a compiled-in grammar provider.
type builtinGrammar struct {
	name     string
	provider BuiltinProvider
}

// BuiltinRegistry manages the 9 core grammars compiled into the binary.
type BuiltinRegistry struct {
	mu       sync.RWMutex
	grammars map[string]*builtinGrammar
	loaded   map[string]*tree_sitter.Language
}

// NewBuiltinRegistry creates a new registry with all compiled-in grammars.
func NewBuiltinRegistry() *BuiltinRegistry {
	r := &BuiltinRegistry{
		grammars: make(map[string]*builtinGrammar),
		loaded:   make(map[string]*tree_sitter.Language),
	}
	registerBuiltins(r)
	return r
}

// Register adds a compiled-in grammar to the registry.
func (r *BuiltinRegistry) Register(name string, provider BuiltinProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.grammars[name] = &builtinGrammar{
		name:     name,
		provider: provider,
	}
}

// Load returns the Language for a built-in grammar.
func (r *BuiltinRegistry) Load(name string) (*tree_sitter.Language, error) {
	r.mu.RLock()
	if lang, ok := r.loaded[name]; ok {
		r.mu.RUnlock()
		return lang, nil
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check after acquiring write lock
	if lang, ok := r.loaded[name]; ok {
		return lang, nil
	}

	g, ok := r.grammars[name]
	if !ok {
		return nil, &ErrGrammarNotFound{Name: name}
	}

	ptr := g.provider()
	lang := tree_sitter.NewLanguage(ptr)
	if lang == nil {
		return nil, &ErrGrammarNotFound{Name: name}
	}
	r.loaded[name] = lang
	return lang, nil
}

// Has returns true if the grammar is compiled-in.
func (r *BuiltinRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.grammars[name]
	return ok
}

// Names returns the names of all compiled-in grammars.
func (r *BuiltinRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.grammars))
	for name := range r.grammars {
		names = append(names, name)
	}
	return names
}

// registerBuiltins wires up the 9 core grammars compiled into the binary.
// Each grammar Go binding exposes a function returning unsafe.Pointer.
func registerBuiltins(r *BuiltinRegistry) {
	r.Register("go", tree_sitter_go.Language)
	// TypeScript uses LanguageTypescript() not Language(), so wrap it.
	r.Register("typescript", func() unsafe.Pointer {
		return tree_sitter_typescript.LanguageTypescript()
	})
	r.Register("javascript", tree_sitter_javascript.Language)
	r.Register("python", tree_sitter_python.Language)
	r.Register("rust", tree_sitter_rust.Language)
	r.Register("java", tree_sitter_java.Language)
	r.Register("c", tree_sitter_c.Language)
	r.Register("cpp", tree_sitter_cpp.Language)
	r.Register("zig", tree_sitter_zig.Language)
}
