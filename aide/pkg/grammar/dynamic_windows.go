//go:build windows

package grammar

import (
	"fmt"
	"syscall"
	"unsafe"

	tree_sitter "github.com/tree-sitter/go-tree-sitter"
)

// openAndLoadLanguage opens a shared library (DLL) and loads the tree-sitter
// Language from the given C symbol. On Windows this uses syscall.LoadDLL
// and GetProcAddress.
func openAndLoadLanguage(libPath, cSymbol string) (*tree_sitter.Language, uintptr, error) {
	dll, err := syscall.LoadDLL(libPath)
	if err != nil {
		return nil, 0, fmt.Errorf("LoadDLL %s: %w", libPath, err)
	}

	proc, err := dll.FindProc(cSymbol)
	if err != nil {
		_ = dll.Release()
		return nil, 0, fmt.Errorf("FindProc %s in %s: %w", cSymbol, libPath, err)
	}

	// Call the zero-argument C function that returns a const TSLanguage*.
	ret, _, _ := proc.Call()
	if ret == 0 {
		_ = dll.Release()
		return nil, 0, fmt.Errorf("symbol %s returned NULL", cSymbol)
	}

	lang := tree_sitter.NewLanguage(unsafe.Pointer(ret)) //nolint:govet // ret is a C pointer, not a Go pointer
	if lang == nil {
		_ = dll.Release()
		return nil, 0, fmt.Errorf("symbol %s returned nil language", cSymbol)
	}

	// Store the DLL handle. We use the Handle field which is a uintptr.
	return lang, uintptr(dll.Handle), nil
}
