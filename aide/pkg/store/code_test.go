package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmylchreest/aide/aide/pkg/code"
	bolt "go.etcd.io/bbolt"
)

func setupTestCodeStore(t *testing.T) (*CodeStore, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "aide-code-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "index.db")
	searchPath := filepath.Join(tmpDir, "search.bleve")
	cs, err := NewCodeStore(dbPath, searchPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create code store: %v", err)
	}

	cleanup := func() {
		cs.Close()
		os.RemoveAll(tmpDir)
	}

	return cs, cleanup
}

// =============================================================================
// Interface Compliance
// =============================================================================

func TestCodeStoreImplementsCodeIndexStore(t *testing.T) {
	var _ CodeIndexStore = (*CodeStore)(nil)
}

// =============================================================================
// Symbol Operations
// =============================================================================

func TestSymbolOperations(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	t.Run("AddAndGetSymbol", func(t *testing.T) {
		sym := &code.Symbol{
			Name:      "getUser",
			Kind:      code.KindFunction,
			Signature: "func getUser(id string) (*User, error)",
			FilePath:  "pkg/auth/user.go",
			StartLine: 42,
			EndLine:   55,
			Language:  "go",
		}

		if err := cs.AddSymbol(sym); err != nil {
			t.Fatalf("AddSymbol failed: %v", err)
		}

		// ID should be auto-generated
		if sym.ID == "" {
			t.Fatal("expected ID to be auto-generated")
		}

		// CreatedAt should be auto-set
		if sym.CreatedAt.IsZero() {
			t.Fatal("expected CreatedAt to be auto-set")
		}

		// Retrieve by ID
		got, err := cs.GetSymbol(sym.ID)
		if err != nil {
			t.Fatalf("GetSymbol failed: %v", err)
		}

		if got.Name != "getUser" {
			t.Errorf("name = %q, want %q", got.Name, "getUser")
		}
		if got.Kind != code.KindFunction {
			t.Errorf("kind = %q, want %q", got.Kind, code.KindFunction)
		}
		if got.Signature != "func getUser(id string) (*User, error)" {
			t.Errorf("signature mismatch: %q", got.Signature)
		}
		if got.FilePath != "pkg/auth/user.go" {
			t.Errorf("filepath = %q, want %q", got.FilePath, "pkg/auth/user.go")
		}
		if got.StartLine != 42 {
			t.Errorf("start line = %d, want %d", got.StartLine, 42)
		}
		if got.Language != "go" {
			t.Errorf("language = %q, want %q", got.Language, "go")
		}
	})

	t.Run("GetNonexistentSymbol", func(t *testing.T) {
		_, err := cs.GetSymbol("nonexistent-id")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("DeleteSymbol", func(t *testing.T) {
		sym := &code.Symbol{
			Name:     "toDelete",
			Kind:     code.KindFunction,
			FilePath: "delete.go",
			Language: "go",
		}
		cs.AddSymbol(sym)

		if err := cs.DeleteSymbol(sym.ID); err != nil {
			t.Fatalf("DeleteSymbol failed: %v", err)
		}

		_, err := cs.GetSymbol(sym.ID)
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound after delete, got %v", err)
		}
	})

	t.Run("AddSymbolWithExplicitID", func(t *testing.T) {
		sym := &code.Symbol{
			ID:       "custom-id-123",
			Name:     "customFunc",
			Kind:     code.KindFunction,
			FilePath: "custom.go",
			Language: "go",
		}
		if err := cs.AddSymbol(sym); err != nil {
			t.Fatalf("AddSymbol with explicit ID failed: %v", err)
		}
		if sym.ID != "custom-id-123" {
			t.Errorf("expected ID to remain %q, got %q", "custom-id-123", sym.ID)
		}

		got, err := cs.GetSymbol("custom-id-123")
		if err != nil {
			t.Fatalf("GetSymbol with custom ID failed: %v", err)
		}
		if got.Name != "customFunc" {
			t.Errorf("name = %q, want %q", got.Name, "customFunc")
		}
	})
}

// =============================================================================
// Symbol Search
// =============================================================================

func TestSymbolSearch(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	// Add a set of symbols for searching
	symbols := []*code.Symbol{
		{Name: "getUser", Kind: code.KindFunction, Signature: "func getUser(id string) *User", FilePath: "user.go", Language: "go", StartLine: 10},
		{Name: "getUserById", Kind: code.KindFunction, Signature: "func getUserById(id int) *User", FilePath: "user.go", Language: "go", StartLine: 20},
		{Name: "createUser", Kind: code.KindFunction, Signature: "func createUser(name string) error", FilePath: "user.go", Language: "go", StartLine: 30},
		{Name: "User", Kind: code.KindType, Signature: "type User struct", FilePath: "types.go", Language: "go", StartLine: 5},
		{Name: "UserService", Kind: code.KindInterface, Signature: "interface UserService", FilePath: "service.ts", Language: "typescript", StartLine: 1},
		{Name: "handleRequest", Kind: code.KindMethod, Signature: "func (s *Server) handleRequest()", FilePath: "server.go", Language: "go", StartLine: 100},
	}

	for _, sym := range symbols {
		if err := cs.AddSymbol(sym); err != nil {
			t.Fatalf("AddSymbol(%s) failed: %v", sym.Name, err)
		}
	}

	// Give bleve a moment to index
	time.Sleep(50 * time.Millisecond)

	t.Run("SearchByName", func(t *testing.T) {
		results, err := cs.SearchSymbols("getUser", code.SearchOptions{})
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}
		if len(results) == 0 {
			t.Fatal("expected at least one result for 'getUser'")
		}

		// Should include both getUser and getUserById
		names := make(map[string]bool)
		for _, r := range results {
			names[r.Symbol.Name] = true
		}
		if !names["getUser"] {
			t.Error("expected 'getUser' in results")
		}
		if !names["getUserById"] {
			t.Error("expected 'getUserById' in results")
		}
	})

	t.Run("SearchWithKindFilter", func(t *testing.T) {
		results, err := cs.SearchSymbols("User", code.SearchOptions{Kind: code.KindInterface})
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}

		for _, r := range results {
			if r.Symbol.Kind != code.KindInterface {
				t.Errorf("expected kind %q, got %q for %q", code.KindInterface, r.Symbol.Kind, r.Symbol.Name)
			}
		}
	})

	t.Run("SearchWithLanguageFilter", func(t *testing.T) {
		results, err := cs.SearchSymbols("User", code.SearchOptions{Language: "typescript"})
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}

		for _, r := range results {
			if r.Symbol.Language != "typescript" {
				t.Errorf("expected lang %q, got %q", "typescript", r.Symbol.Language)
			}
		}
	})

	t.Run("SearchWithFileFilter", func(t *testing.T) {
		results, err := cs.SearchSymbols("User", code.SearchOptions{FilePath: "types.go"})
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}

		for _, r := range results {
			if r.Symbol.FilePath != "types.go" {
				t.Errorf("expected file %q, got %q", "types.go", r.Symbol.FilePath)
			}
		}
	})

	t.Run("SearchWithLimit", func(t *testing.T) {
		// Note: SearchSymbols requests limit*2 from bleve and applies filters post-search,
		// so the result count may exceed limit when no filters eliminate results.
		// Test that limit at least constrains the search scope.
		results, err := cs.SearchSymbols("user", code.SearchOptions{Limit: 1})
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}
		// With limit=1, bleve fetches 2 hits; after filtering we may get up to 2
		if len(results) == 0 {
			t.Error("expected at least 1 result")
		}
		if len(results) > 2 {
			t.Errorf("expected at most 2 results with limit=1 (limit*2 bleve window), got %d", len(results))
		}
	})

	t.Run("SearchNoResults", func(t *testing.T) {
		results, err := cs.SearchSymbols("zzzzNonexistent", code.SearchOptions{})
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("SearchResultsHaveScores", func(t *testing.T) {
		results, err := cs.SearchSymbols("getUser", code.SearchOptions{})
		if err != nil {
			t.Fatalf("SearchSymbols failed: %v", err)
		}
		for _, r := range results {
			if r.Score <= 0 {
				t.Errorf("expected positive score for %q, got %f", r.Symbol.Name, r.Score)
			}
		}
	})
}

// =============================================================================
// Reference Operations
// =============================================================================

func TestReferenceOperations(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	t.Run("AddAndSearchReferences", func(t *testing.T) {
		refs := []*code.Reference{
			{SymbolName: "getUser", Kind: code.RefKindCall, FilePath: "handler.go", Line: 15, Column: 8, Context: "user := getUser(id)"},
			{SymbolName: "getUser", Kind: code.RefKindCall, FilePath: "service.go", Line: 42, Column: 12, Context: "u, err := getUser(req.ID)"},
			{SymbolName: "getUser", Kind: code.RefKindTypeRef, FilePath: "test.go", Line: 10, Column: 5, Context: "// uses getUser"},
			{SymbolName: "createUser", Kind: code.RefKindCall, FilePath: "handler.go", Line: 30, Column: 8, Context: "err := createUser(name)"},
		}

		for _, ref := range refs {
			if err := cs.AddReference(ref); err != nil {
				t.Fatalf("AddReference failed: %v", err)
			}
			if ref.ID == "" {
				t.Fatal("expected ID to be auto-generated")
			}
		}

		// Search all references to getUser
		results, err := cs.SearchReferences(code.ReferenceSearchOptions{
			SymbolName: "getUser",
		})
		if err != nil {
			t.Fatalf("SearchReferences failed: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("expected 3 refs for getUser, got %d", len(results))
		}
	})

	t.Run("SearchReferencesWithKindFilter", func(t *testing.T) {
		results, err := cs.SearchReferences(code.ReferenceSearchOptions{
			SymbolName: "getUser",
			Kind:       code.RefKindCall,
		})
		if err != nil {
			t.Fatalf("SearchReferences failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 call refs, got %d", len(results))
		}
		for _, r := range results {
			if r.Kind != code.RefKindCall {
				t.Errorf("expected kind %q, got %q", code.RefKindCall, r.Kind)
			}
		}
	})

	t.Run("SearchReferencesWithFileFilter", func(t *testing.T) {
		results, err := cs.SearchReferences(code.ReferenceSearchOptions{
			SymbolName: "getUser",
			FilePath:   "handler.go",
		})
		if err != nil {
			t.Fatalf("SearchReferences failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 ref in handler.go, got %d", len(results))
		}
	})

	t.Run("SearchReferencesWithLimit", func(t *testing.T) {
		results, err := cs.SearchReferences(code.ReferenceSearchOptions{
			SymbolName: "getUser",
			Limit:      1,
		})
		if err != nil {
			t.Fatalf("SearchReferences failed: %v", err)
		}
		if len(results) != 1 {
			t.Errorf("expected 1 ref with limit=1, got %d", len(results))
		}
	})

	t.Run("SearchReferencesNoResults", func(t *testing.T) {
		results, err := cs.SearchReferences(code.ReferenceSearchOptions{
			SymbolName: "nonexistentSymbol",
		})
		if err != nil {
			t.Fatalf("SearchReferences failed: %v", err)
		}
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})

	t.Run("GetReference", func(t *testing.T) {
		ref := &code.Reference{
			SymbolName: "getReference",
			Kind:       code.RefKindCall,
			FilePath:   "ref.go",
			Line:       42,
			Column:     10,
			Context:    "x := getReference()",
		}
		if err := cs.AddReference(ref); err != nil {
			t.Fatalf("AddReference: %v", err)
		}

		got, err := cs.GetReference(ref.ID)
		if err != nil {
			t.Fatalf("GetReference: %v", err)
		}
		if got.SymbolName != "getReference" {
			t.Errorf("symbol name = %q, want %q", got.SymbolName, "getReference")
		}
		if got.Line != 42 {
			t.Errorf("line = %d, want 42", got.Line)
		}
		if got.Context != "x := getReference()" {
			t.Errorf("context = %q", got.Context)
		}
	})

	t.Run("GetReferenceNonexistent", func(t *testing.T) {
		_, err := cs.GetReference("nonexistent")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("ReferenceAutoGeneratesFields", func(t *testing.T) {
		ref := &code.Reference{
			SymbolName: "testFunc",
			Kind:       code.RefKindCall,
			FilePath:   "main.go",
			Line:       1,
		}
		if err := cs.AddReference(ref); err != nil {
			t.Fatalf("AddReference failed: %v", err)
		}
		if ref.ID == "" {
			t.Error("expected ID to be auto-generated")
		}
		if ref.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be auto-set")
		}
	})
}

// =============================================================================
// Clear File References
// =============================================================================

func TestClearFileReferences(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	// Add references from multiple files
	refs := []*code.Reference{
		{SymbolName: "foo", Kind: code.RefKindCall, FilePath: "a.go", Line: 1},
		{SymbolName: "foo", Kind: code.RefKindCall, FilePath: "b.go", Line: 2},
		{SymbolName: "foo", Kind: code.RefKindCall, FilePath: "a.go", Line: 3},
		{SymbolName: "bar", Kind: code.RefKindCall, FilePath: "a.go", Line: 4},
	}
	for _, ref := range refs {
		cs.AddReference(ref)
	}

	// Verify initial state
	allFoo, _ := cs.SearchReferences(code.ReferenceSearchOptions{SymbolName: "foo"})
	if len(allFoo) != 3 {
		t.Fatalf("expected 3 foo refs initially, got %d", len(allFoo))
	}

	// Clear references from a.go
	if err := cs.ClearFileReferences("a.go"); err != nil {
		t.Fatalf("ClearFileReferences failed: %v", err)
	}

	// foo should now only have 1 ref (from b.go)
	remaining, _ := cs.SearchReferences(code.ReferenceSearchOptions{SymbolName: "foo"})
	if len(remaining) != 1 {
		t.Errorf("expected 1 foo ref after clearing a.go, got %d", len(remaining))
	}
	if len(remaining) > 0 && remaining[0].FilePath != "b.go" {
		t.Errorf("expected remaining ref in b.go, got %s", remaining[0].FilePath)
	}

	// bar should have 0 refs (was only in a.go)
	barRefs, _ := cs.SearchReferences(code.ReferenceSearchOptions{SymbolName: "bar"})
	if len(barRefs) != 0 {
		t.Errorf("expected 0 bar refs after clearing a.go, got %d", len(barRefs))
	}
}

// =============================================================================
// File Info (Tracking)
// =============================================================================

func TestFileInfoOperations(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	t.Run("SetAndGetFileInfo", func(t *testing.T) {
		now := time.Now().Truncate(time.Millisecond) // truncate for JSON roundtrip
		info := &code.FileInfo{
			Path:      "pkg/auth/user.go",
			ModTime:   now,
			SymbolIDs: []string{"sym1", "sym2", "sym3"},
		}

		if err := cs.SetFileInfo(info); err != nil {
			t.Fatalf("SetFileInfo failed: %v", err)
		}

		got, err := cs.GetFileInfo("pkg/auth/user.go")
		if err != nil {
			t.Fatalf("GetFileInfo failed: %v", err)
		}

		if got.Path != "pkg/auth/user.go" {
			t.Errorf("path = %q, want %q", got.Path, "pkg/auth/user.go")
		}
		if len(got.SymbolIDs) != 3 {
			t.Errorf("expected 3 symbol IDs, got %d", len(got.SymbolIDs))
		}
	})

	t.Run("GetNonexistentFileInfo", func(t *testing.T) {
		_, err := cs.GetFileInfo("nonexistent.go")
		if err != ErrNotFound {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("OverwriteFileInfo", func(t *testing.T) {
		info1 := &code.FileInfo{Path: "update.go", SymbolIDs: []string{"a"}}
		cs.SetFileInfo(info1)

		info2 := &code.FileInfo{Path: "update.go", SymbolIDs: []string{"b", "c"}}
		cs.SetFileInfo(info2)

		got, _ := cs.GetFileInfo("update.go")
		if len(got.SymbolIDs) != 2 {
			t.Errorf("expected 2 symbol IDs after overwrite, got %d", len(got.SymbolIDs))
		}
	})
}

// =============================================================================
// GetFileSymbols
// =============================================================================

func TestGetFileSymbols(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	// Add symbols and track them in file info
	sym1 := &code.Symbol{Name: "funcA", Kind: code.KindFunction, FilePath: "main.go", Language: "go"}
	sym2 := &code.Symbol{Name: "funcB", Kind: code.KindFunction, FilePath: "main.go", Language: "go"}
	cs.AddSymbol(sym1)
	cs.AddSymbol(sym2)

	cs.SetFileInfo(&code.FileInfo{
		Path:      "main.go",
		SymbolIDs: []string{sym1.ID, sym2.ID},
	})

	symbols, err := cs.GetFileSymbols("main.go")
	if err != nil {
		t.Fatalf("GetFileSymbols failed: %v", err)
	}
	if len(symbols) != 2 {
		t.Errorf("expected 2 symbols, got %d", len(symbols))
	}

	names := map[string]bool{}
	for _, s := range symbols {
		names[s.Name] = true
	}
	if !names["funcA"] || !names["funcB"] {
		t.Error("expected both funcA and funcB in results")
	}
}

func TestGetFileSymbolsNonexistentFile(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	_, err := cs.GetFileSymbols("nope.go")
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// =============================================================================
// ClearFile
// =============================================================================

func TestClearFile(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	// Add symbols and register them with file info
	sym1 := &code.Symbol{Name: "clearMe", Kind: code.KindFunction, FilePath: "clear.go", Language: "go"}
	sym2 := &code.Symbol{Name: "clearMeToo", Kind: code.KindFunction, FilePath: "clear.go", Language: "go"}
	cs.AddSymbol(sym1)
	cs.AddSymbol(sym2)

	cs.SetFileInfo(&code.FileInfo{
		Path:      "clear.go",
		SymbolIDs: []string{sym1.ID, sym2.ID},
	})

	// Verify symbols exist
	_, err := cs.GetSymbol(sym1.ID)
	if err != nil {
		t.Fatalf("expected sym1 to exist before clear")
	}

	// Clear file
	if err := cs.ClearFile("clear.go"); err != nil {
		t.Fatalf("ClearFile failed: %v", err)
	}

	// Symbols should be gone
	_, err = cs.GetSymbol(sym1.ID)
	if err != ErrNotFound {
		t.Errorf("expected sym1 to be deleted, got %v", err)
	}
	_, err = cs.GetSymbol(sym2.ID)
	if err != ErrNotFound {
		t.Errorf("expected sym2 to be deleted, got %v", err)
	}

	// File info should be gone
	_, err = cs.GetFileInfo("clear.go")
	if err != ErrNotFound {
		t.Errorf("expected file info to be deleted, got %v", err)
	}
}

func TestClearFileNonexistent(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	// Should not error on nonexistent file
	err := cs.ClearFile("nonexistent.go")
	if err != nil {
		t.Errorf("ClearFile on nonexistent file should not error, got %v", err)
	}
}

// =============================================================================
// Stats
// =============================================================================

func TestStats(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	// Initially empty
	stats, err := cs.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.Symbols != 0 || stats.References != 0 || stats.Files != 0 {
		t.Errorf("expected all zeros, got symbols=%d refs=%d files=%d", stats.Symbols, stats.References, stats.Files)
	}

	// Add data
	sym := &code.Symbol{Name: "stat", Kind: code.KindFunction, FilePath: "stat.go", Language: "go"}
	cs.AddSymbol(sym)
	cs.SetFileInfo(&code.FileInfo{Path: "stat.go", SymbolIDs: []string{sym.ID}})
	cs.AddReference(&code.Reference{SymbolName: "stat", Kind: code.RefKindCall, FilePath: "other.go", Line: 1})

	stats, err = cs.Stats()
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.Symbols != 1 {
		t.Errorf("symbols = %d, want 1", stats.Symbols)
	}
	if stats.Files != 1 {
		t.Errorf("files = %d, want 1", stats.Files)
	}
	if stats.References != 1 {
		t.Errorf("references = %d, want 1", stats.References)
	}
}

// =============================================================================
// Clear (Full Reset)
// =============================================================================

func TestClear(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	// Add symbols, references, file info
	sym := &code.Symbol{Name: "clearAll", Kind: code.KindFunction, FilePath: "all.go", Language: "go"}
	cs.AddSymbol(sym)
	cs.SetFileInfo(&code.FileInfo{Path: "all.go", SymbolIDs: []string{sym.ID}})
	cs.AddReference(&code.Reference{SymbolName: "clearAll", Kind: code.RefKindCall, FilePath: "caller.go", Line: 1})

	// Clear everything
	if err := cs.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Verify all zeroed
	stats, err := cs.Stats()
	if err != nil {
		t.Fatalf("Stats after Clear failed: %v", err)
	}
	if stats.Symbols != 0 || stats.References != 0 || stats.Files != 0 {
		t.Errorf("expected all zeros after Clear, got symbols=%d refs=%d files=%d", stats.Symbols, stats.References, stats.Files)
	}

	// Verify symbol is gone from DB
	_, err = cs.GetSymbol(sym.ID)
	if err != ErrNotFound {
		t.Errorf("expected ErrNotFound after Clear, got %v", err)
	}

	// Verify search is also cleared (no results from bleve)
	time.Sleep(50 * time.Millisecond)
	results, err := cs.SearchSymbols("clearAll", code.SearchOptions{})
	if err != nil {
		t.Fatalf("SearchSymbols after Clear failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 search results after Clear, got %d", len(results))
	}
}

// =============================================================================
// Store Can Be Used After Clear (Reopen)
// =============================================================================

func TestUsableAfterClear(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	// Add, clear, then add again
	sym1 := &code.Symbol{Name: "before", Kind: code.KindFunction, FilePath: "a.go", Language: "go"}
	cs.AddSymbol(sym1)

	cs.Clear()

	sym2 := &code.Symbol{Name: "after", Kind: code.KindFunction, FilePath: "b.go", Language: "go"}
	if err := cs.AddSymbol(sym2); err != nil {
		t.Fatalf("AddSymbol after Clear failed: %v", err)
	}

	got, err := cs.GetSymbol(sym2.ID)
	if err != nil {
		t.Fatalf("GetSymbol after Clear+Add failed: %v", err)
	}
	if got.Name != "after" {
		t.Errorf("name = %q, want %q", got.Name, "after")
	}

	// Search should find new symbol
	time.Sleep(50 * time.Millisecond)
	results, err := cs.SearchSymbols("after", code.SearchOptions{})
	if err != nil {
		t.Fatalf("SearchSymbols failed: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected search to find 'after' symbol")
	}
}

// =============================================================================
// Code Search Mapping Rebuild
// =============================================================================

func TestCodeSearchMappingHashStored(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	// After NewCodeStore, the mapping hash should be stored in the meta bucket.
	var stored string
	cs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketCodeMeta)
		data := b.Get([]byte("search_mapping_hash"))
		if data != nil {
			stored = string(data)
		}
		return nil
	})

	if stored == "" {
		t.Fatal("expected search_mapping_hash to be stored")
	}

	// Should match current mapping.
	m, err := buildCodeIndexMapping()
	if err != nil {
		t.Fatalf("buildCodeIndexMapping: %v", err)
	}
	expected := MappingHash(m)
	if stored != expected {
		t.Errorf("hash mismatch: got %q, want %q", stored, expected)
	}
}

func TestCodeSearchMappingRebuild(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "aide-code-rebuild-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "index.db")
	searchPath := filepath.Join(tmpDir, "search.bleve")

	// Create store and add a symbol.
	cs, err := NewCodeStore(dbPath, searchPath)
	if err != nil {
		t.Fatalf("NewCodeStore: %v", err)
	}

	sym := &code.Symbol{
		Name:     "rebuildTest",
		Kind:     code.KindFunction,
		FilePath: "rebuild.go",
		Language: "go",
	}
	if err := cs.AddSymbol(sym); err != nil {
		cs.Close()
		t.Fatalf("AddSymbol: %v", err)
	}

	// Corrupt the stored hash to force a rebuild.
	cs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketCodeMeta)
		return b.Put([]byte("search_mapping_hash"), []byte("stale"))
	})
	cs.Close()

	// Reopen — ensureCodeSearchMapping should detect mismatch and rebuild.
	cs2, err := NewCodeStore(dbPath, searchPath)
	if err != nil {
		t.Fatalf("NewCodeStore reopen: %v", err)
	}
	defer cs2.Close()

	// Symbol should still be in BBolt.
	got, err := cs2.GetSymbol(sym.ID)
	if err != nil {
		t.Fatalf("GetSymbol after rebuild: %v", err)
	}
	if got.Name != "rebuildTest" {
		t.Errorf("name = %q, want %q", got.Name, "rebuildTest")
	}

	// Search should work after rebuild (re-indexed from bolt).
	time.Sleep(50 * time.Millisecond)
	results, err := cs2.SearchSymbols("rebuildTest", code.SearchOptions{})
	if err != nil {
		t.Fatalf("SearchSymbols after rebuild: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected search results after rebuild")
	}

	// Hash should be updated.
	var newHash string
	cs2.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(BucketCodeMeta)
		data := b.Get([]byte("search_mapping_hash"))
		if data != nil {
			newHash = string(data)
		}
		return nil
	})
	if newHash == "stale" {
		t.Error("expected hash to be updated after rebuild")
	}
}

// =============================================================================
// ListAllSymbols
// =============================================================================

func TestListAllSymbols(t *testing.T) {
	cs, cleanup := setupTestCodeStore(t)
	defer cleanup()

	for i := 0; i < 5; i++ {
		cs.AddSymbol(&code.Symbol{
			Name:     "sym" + string(rune('A'+i)),
			Kind:     code.KindFunction,
			FilePath: "list.go",
			Language: "go",
		})
	}

	all, err := cs.ListAllSymbols(0)
	if err != nil {
		t.Fatalf("ListAllSymbols failed: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("expected 5 symbols, got %d", len(all))
	}

	// Test with limit
	limited, err := cs.ListAllSymbols(3)
	if err != nil {
		t.Fatalf("ListAllSymbols with limit failed: %v", err)
	}
	if len(limited) != 3 {
		t.Errorf("expected 3 symbols with limit, got %d", len(limited))
	}
}
