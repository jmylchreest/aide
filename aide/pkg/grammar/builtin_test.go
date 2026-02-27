package grammar

import (
	"sort"
	"testing"
	"unsafe"
)

// expectedBuiltins lists the 10 core grammars that must be compiled in.
var expectedBuiltins = []string{
	"c", "cpp", "go", "java", "javascript", "python", "rust", "tsx", "typescript", "zig",
}

func TestNewBuiltinRegistryContainsAll(t *testing.T) {
	r := NewBuiltinRegistry()

	names := r.Names()
	sort.Strings(names)

	if len(names) != len(expectedBuiltins) {
		t.Fatalf("expected %d builtins, got %d: %v", len(expectedBuiltins), len(names), names)
	}

	for i, want := range expectedBuiltins {
		if names[i] != want {
			t.Errorf("Names()[%d] = %q; want %q", i, names[i], want)
		}
	}
}

func TestBuiltinRegistryHas(t *testing.T) {
	r := NewBuiltinRegistry()

	for _, name := range expectedBuiltins {
		if !r.Has(name) {
			t.Errorf("Has(%q) = false; want true", name)
		}
	}

	// Unknown grammars should not be present.
	for _, name := range []string{"ruby", "kotlin", "nonexistent"} {
		if r.Has(name) {
			t.Errorf("Has(%q) = true; want false (not a builtin)", name)
		}
	}
}

func TestBuiltinRegistryLoadAll(t *testing.T) {
	r := NewBuiltinRegistry()

	for _, name := range expectedBuiltins {
		t.Run(name, func(t *testing.T) {
			lang, err := r.Load(name)
			if err != nil {
				t.Fatalf("Load(%q): %v", name, err)
			}
			if lang == nil {
				t.Fatalf("Load(%q) returned nil Language", name)
			}
		})
	}
}

func TestBuiltinRegistryLoadCaching(t *testing.T) {
	r := NewBuiltinRegistry()

	lang1, err := r.Load("go")
	if err != nil {
		t.Fatal(err)
	}

	lang2, err := r.Load("go")
	if err != nil {
		t.Fatal(err)
	}

	// Same pointer should be returned on second call (double-check locking cache).
	if lang1 != lang2 {
		t.Error("Load should return the cached Language on second call")
	}
}

func TestBuiltinRegistryLoadNotFound(t *testing.T) {
	r := NewBuiltinRegistry()

	_, err := r.Load("ruby")
	if err == nil {
		t.Fatal("expected error loading non-builtin grammar")
	}

	if _, ok := err.(*GrammarNotFoundError); !ok {
		t.Errorf("error type = %T; want *GrammarNotFoundError", err)
	}
}

func TestBuiltinRegistryRegisterCustom(t *testing.T) {
	r := NewBuiltinRegistry()

	// Register a fake grammar provider that returns a non-nil pointer.
	// We just need to verify that Register adds it and Load calls the provider.
	called := false
	r.Register("testlang", func() unsafe.Pointer {
		called = true
		// Return a non-nil pointer so NewLanguage succeeds.
		// In real code this would point to a TSLanguage struct.
		dummy := uint64(0)
		return unsafe.Pointer(&dummy)
	})

	if !r.Has("testlang") {
		t.Error("Has(\"testlang\") should be true after Register")
	}

	// Loading should call the provider.
	lang, err := r.Load("testlang")
	if !called {
		t.Error("provider was not called during Load")
	}
	// The Language wraps a fake pointer, so it won't be usable for parsing,
	// but it should be non-nil and not error.
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if lang == nil {
		t.Error("expected non-nil Language from provider")
	}
}
