//go:build !windows

package grammar

import (
	"fmt"
	"unsafe"

	"github.com/ebitengine/purego"
	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// closeLibrary releases a library handle obtained from openAndLoadLanguage.
func closeLibrary(handle uintptr) error {
	if handle == 0 {
		return nil
	}
	return purego.Dlclose(handle)
}

// openAndLoadLanguage opens a shared library and loads the tree-sitter
// Language from the given C symbol. On Unix systems this uses purego
// (dlopen / dlsym).
func openAndLoadLanguage(libPath, cSymbol string) (*tree_sitter.Language, uintptr, error) {
	handle, err := purego.Dlopen(libPath, purego.RTLD_LAZY)
	if err != nil {
		return nil, 0, fmt.Errorf("dlopen %s: %w", libPath, err)
	}

	// The function returns unsafe.Pointer directly to avoid the
	// uintptr → unsafe.Pointer conversion that go vet warns about.
	var langFn func() unsafe.Pointer
	purego.RegisterLibFunc(&langFn, handle, cSymbol)

	ptr := langFn()
	lang := tree_sitter.NewLanguage(ptr)
	if lang == nil {
		return nil, handle, fmt.Errorf("symbol %s returned nil language", cSymbol)
	}

	return lang, handle, nil
}
