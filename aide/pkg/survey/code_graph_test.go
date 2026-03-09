package survey

import (
	"errors"
	"fmt"
	"testing"
)

// mockCodeGrapher implements CodeGrapher for testing.
type mockCodeGrapher struct {
	symbols    map[string][]SymbolHit    // query -> hits
	references map[string][]ReferenceHit // symbolName -> references
	fileRefs   map[string][]ReferenceHit // filePath -> references
	containing map[string]*SymbolHit     // "file:line" -> symbol

	findSymErr    error
	findRefErr    error
	fileRefsErr   error
	containingErr error
}

func newMockCodeGrapher() *mockCodeGrapher {
	return &mockCodeGrapher{
		symbols:    make(map[string][]SymbolHit),
		references: make(map[string][]ReferenceHit),
		fileRefs:   make(map[string][]ReferenceHit),
		containing: make(map[string]*SymbolHit),
	}
}

func (m *mockCodeGrapher) FindSymbols(query string, kind string, limit int) ([]SymbolHit, error) {
	if m.findSymErr != nil {
		return nil, m.findSymErr
	}
	hits, ok := m.symbols[query]
	if !ok {
		return nil, nil
	}
	if kind != "" {
		var filtered []SymbolHit
		for _, h := range hits {
			if h.Kind == kind {
				filtered = append(filtered, h)
			}
		}
		hits = filtered
	}
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	return hits, nil
}

func (m *mockCodeGrapher) FindReferences(symbolName string, kind string, limit int) ([]ReferenceHit, error) {
	if m.findRefErr != nil {
		return nil, m.findRefErr
	}
	refs, ok := m.references[symbolName]
	if !ok {
		return nil, nil
	}
	if kind != "" {
		var filtered []ReferenceHit
		for _, r := range refs {
			if r.Kind == kind {
				filtered = append(filtered, r)
			}
		}
		refs = filtered
	}
	if limit > 0 && len(refs) > limit {
		refs = refs[:limit]
	}
	return refs, nil
}

func (m *mockCodeGrapher) GetFileReferences(filePath string) ([]ReferenceHit, error) {
	if m.fileRefsErr != nil {
		return nil, m.fileRefsErr
	}
	refs, ok := m.fileRefs[filePath]
	if !ok {
		return nil, nil
	}
	return refs, nil
}

func (m *mockCodeGrapher) GetContainingSymbol(filePath string, line int) (*SymbolHit, error) {
	if m.containingErr != nil {
		return nil, m.containingErr
	}
	key := fmt.Sprintf("%s:%d", filePath, line)
	sym, ok := m.containing[key]
	if !ok {
		return nil, nil
	}
	return sym, nil
}

func TestBuildCallGraph_SymbolNotFound(t *testing.T) {
	cg := newMockCodeGrapher()
	_, err := BuildCallGraph(cg, "nonexistent", GraphOptions{})
	if err == nil {
		t.Fatal("expected error for missing symbol")
	}
}

func TestBuildCallGraph_SingleNode(t *testing.T) {
	cg := newMockCodeGrapher()
	cg.symbols["main"] = []SymbolHit{
		{Name: "main", Kind: "function", FilePath: "main.go", Line: 1, EndLine: 10, Language: "go"},
	}

	graph, err := BuildCallGraph(cg, "main", GraphOptions{})
	if err != nil {
		t.Fatalf("BuildCallGraph: %v", err)
	}
	if graph.Root != "main" {
		t.Errorf("expected root 'main', got %q", graph.Root)
	}
	if len(graph.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(graph.Edges))
	}
}

func TestBuildCallGraph_Callees(t *testing.T) {
	cg := newMockCodeGrapher()

	// main calls helper
	cg.symbols["main"] = []SymbolHit{
		{Name: "main", Kind: "function", FilePath: "main.go", Line: 1, EndLine: 10, Language: "go"},
	}
	cg.symbols["helper"] = []SymbolHit{
		{Name: "helper", Kind: "function", FilePath: "util.go", Line: 5, EndLine: 15, Language: "go"},
	}

	// References in main.go (main's body calls helper at line 5)
	cg.fileRefs["main.go"] = []ReferenceHit{
		{Symbol: "helper", Kind: "call", FilePath: "main.go", Line: 5},
	}

	graph, err := BuildCallGraph(cg, "main", GraphOptions{Direction: "callees", MaxDepth: 1})
	if err != nil {
		t.Fatalf("BuildCallGraph: %v", err)
	}
	if len(graph.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(graph.Edges))
	}
	if len(graph.Edges) > 0 {
		e := graph.Edges[0]
		if e.From != "main" || e.To != "helper" {
			t.Errorf("expected edge main->helper, got %s->%s", e.From, e.To)
		}
	}
}

func TestBuildCallGraph_Callers(t *testing.T) {
	cg := newMockCodeGrapher()

	// helper is called by main
	cg.symbols["helper"] = []SymbolHit{
		{Name: "helper", Kind: "function", FilePath: "util.go", Line: 5, EndLine: 15, Language: "go"},
	}

	// References to helper (called from main.go line 8)
	cg.references["helper"] = []ReferenceHit{
		{Symbol: "helper", Kind: "call", FilePath: "main.go", Line: 8},
	}

	// GetContainingSymbol resolves main.go:8 -> main
	mainSym := &SymbolHit{Name: "main", Kind: "function", FilePath: "main.go", Line: 1, EndLine: 20, Language: "go"}
	cg.containing["main.go:8"] = mainSym

	graph, err := BuildCallGraph(cg, "helper", GraphOptions{Direction: "callers", MaxDepth: 1})
	if err != nil {
		t.Fatalf("BuildCallGraph: %v", err)
	}
	if len(graph.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(graph.Edges))
	}
	if len(graph.Edges) > 0 {
		e := graph.Edges[0]
		if e.From != "main" || e.To != "helper" {
			t.Errorf("expected edge main->helper, got %s->%s", e.From, e.To)
		}
	}
}

func TestBuildCallGraph_BothDirections(t *testing.T) {
	cg := newMockCodeGrapher()

	// A -> B -> C
	cg.symbols["B"] = []SymbolHit{
		{Name: "B", Kind: "function", FilePath: "b.go", Line: 1, EndLine: 10, Language: "go"},
	}
	cg.symbols["C"] = []SymbolHit{
		{Name: "C", Kind: "function", FilePath: "c.go", Line: 1, EndLine: 10, Language: "go"},
	}

	// B calls C (reference in B's body)
	cg.fileRefs["b.go"] = []ReferenceHit{
		{Symbol: "C", Kind: "call", FilePath: "b.go", Line: 5},
	}

	// A calls B (reference to B from a.go)
	cg.references["B"] = []ReferenceHit{
		{Symbol: "B", Kind: "call", FilePath: "a.go", Line: 3},
	}
	aSym := &SymbolHit{Name: "A", Kind: "function", FilePath: "a.go", Line: 1, EndLine: 10, Language: "go"}
	cg.containing["a.go:3"] = aSym

	graph, err := BuildCallGraph(cg, "B", GraphOptions{Direction: "both", MaxDepth: 1})
	if err != nil {
		t.Fatalf("BuildCallGraph: %v", err)
	}

	// Should have B, C (callee), A (caller) = 3 nodes
	if len(graph.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(graph.Nodes))
		for _, n := range graph.Nodes {
			t.Logf("  node: %s (%s)", n.Name, n.FilePath)
		}
	}
	// Should have B->C and A->B = 2 edges
	if len(graph.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(graph.Edges))
		for _, e := range graph.Edges {
			t.Logf("  edge: %s->%s", e.From, e.To)
		}
	}
}

func TestBuildCallGraph_MaxDepth(t *testing.T) {
	cg := newMockCodeGrapher()

	// Chain: A -> B -> C -> D
	for _, name := range []string{"A", "B", "C", "D"} {
		cg.symbols[name] = []SymbolHit{
			{Name: name, Kind: "function", FilePath: name + ".go", Line: 1, EndLine: 10, Language: "go"},
		}
	}

	// A calls B, B calls C, C calls D
	cg.fileRefs["A.go"] = []ReferenceHit{{Symbol: "B", Kind: "call", FilePath: "A.go", Line: 5}}
	cg.fileRefs["B.go"] = []ReferenceHit{{Symbol: "C", Kind: "call", FilePath: "B.go", Line: 5}}
	cg.fileRefs["C.go"] = []ReferenceHit{{Symbol: "D", Kind: "call", FilePath: "C.go", Line: 5}}

	// Depth 1: A + B only
	graph, err := BuildCallGraph(cg, "A", GraphOptions{Direction: "callees", MaxDepth: 1})
	if err != nil {
		t.Fatalf("depth 1: %v", err)
	}
	if len(graph.Nodes) != 2 {
		t.Errorf("depth 1: expected 2 nodes, got %d", len(graph.Nodes))
	}

	// Depth 2: A + B + C
	graph, err = BuildCallGraph(cg, "A", GraphOptions{Direction: "callees", MaxDepth: 2})
	if err != nil {
		t.Fatalf("depth 2: %v", err)
	}
	if len(graph.Nodes) != 3 {
		t.Errorf("depth 2: expected 3 nodes, got %d", len(graph.Nodes))
	}

	// Depth 3: A + B + C + D
	graph, err = BuildCallGraph(cg, "A", GraphOptions{Direction: "callees", MaxDepth: 3})
	if err != nil {
		t.Fatalf("depth 3: %v", err)
	}
	if len(graph.Nodes) != 4 {
		t.Errorf("depth 3: expected 4 nodes, got %d", len(graph.Nodes))
	}
}

func TestBuildCallGraph_MaxNodes(t *testing.T) {
	cg := newMockCodeGrapher()

	// Create a fan-out: root calls many functions.
	cg.symbols["root"] = []SymbolHit{
		{Name: "root", Kind: "function", FilePath: "root.go", Line: 1, EndLine: 100, Language: "go"},
	}

	var fileRefs []ReferenceHit
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("fn%d", i)
		cg.symbols[name] = []SymbolHit{
			{Name: name, Kind: "function", FilePath: name + ".go", Line: 1, EndLine: 10, Language: "go"},
		}
		fileRefs = append(fileRefs, ReferenceHit{Symbol: name, Kind: "call", FilePath: "root.go", Line: i + 2})
	}
	cg.fileRefs["root.go"] = fileRefs

	graph, err := BuildCallGraph(cg, "root", GraphOptions{Direction: "callees", MaxNodes: 5})
	if err != nil {
		t.Fatalf("BuildCallGraph: %v", err)
	}
	if len(graph.Nodes) > 5 {
		t.Errorf("expected max 5 nodes, got %d", len(graph.Nodes))
	}
}

func TestBuildCallGraph_SkipsSelfReferences(t *testing.T) {
	cg := newMockCodeGrapher()

	cg.symbols["recurse"] = []SymbolHit{
		{Name: "recurse", Kind: "function", FilePath: "r.go", Line: 1, EndLine: 10, Language: "go"},
	}

	// recurse calls itself
	cg.fileRefs["r.go"] = []ReferenceHit{
		{Symbol: "recurse", Kind: "call", FilePath: "r.go", Line: 5},
	}

	graph, err := BuildCallGraph(cg, "recurse", GraphOptions{Direction: "callees", MaxDepth: 3})
	if err != nil {
		t.Fatalf("BuildCallGraph: %v", err)
	}
	// Should have only 1 node (self), no edges (self-references are skipped)
	if len(graph.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected 0 edges (self-ref skipped), got %d", len(graph.Edges))
	}
}

func TestBuildCallGraph_SkipsImports(t *testing.T) {
	cg := newMockCodeGrapher()

	cg.symbols["main"] = []SymbolHit{
		{Name: "main", Kind: "function", FilePath: "main.go", Line: 1, EndLine: 10, Language: "go"},
	}
	cg.symbols["fmt"] = []SymbolHit{
		{Name: "fmt", Kind: "function", FilePath: "fmt.go", Line: 1, EndLine: 5, Language: "go"},
	}

	// main.go has an import reference (should be skipped) and a call reference
	cg.fileRefs["main.go"] = []ReferenceHit{
		{Symbol: "fmt", Kind: "import", FilePath: "main.go", Line: 3},
		{Symbol: "helper", Kind: "call", FilePath: "main.go", Line: 5},
	}
	cg.symbols["helper"] = []SymbolHit{
		{Name: "helper", Kind: "function", FilePath: "h.go", Line: 1, EndLine: 10, Language: "go"},
	}

	graph, err := BuildCallGraph(cg, "main", GraphOptions{Direction: "callees", MaxDepth: 1})
	if err != nil {
		t.Fatalf("BuildCallGraph: %v", err)
	}
	if len(graph.Edges) != 1 {
		t.Errorf("expected 1 edge (import skipped), got %d", len(graph.Edges))
	}
	if len(graph.Edges) > 0 && graph.Edges[0].To != "helper" {
		t.Errorf("expected edge to 'helper', got %q", graph.Edges[0].To)
	}
}

// =============================================================================
// Error Propagation Tests
// =============================================================================

func TestBuildCallGraph_FindSymbolsError(t *testing.T) {
	cg := newMockCodeGrapher()
	cg.findSymErr = errors.New("symbol search failed")

	_, err := BuildCallGraph(cg, "main", GraphOptions{Direction: "callees"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "finding root symbol: symbol search failed" {
		t.Errorf("error = %q, want %q", err.Error(), "finding root symbol: symbol search failed")
	}
}

func TestBuildCallGraph_GetFileReferencesError_Resilient(t *testing.T) {
	// BuildCallGraph silently ignores errors from findCallees/findCallers
	// during BFS traversal — only the root symbol lookup is fatal.
	cg := newMockCodeGrapher()
	cg.symbols["main"] = []SymbolHit{
		{Name: "main", Kind: "function", FilePath: "main.go", Line: 1, EndLine: 10, Language: "go"},
	}
	cg.fileRefsErr = errors.New("file refs failed")

	graph, err := BuildCallGraph(cg, "main", GraphOptions{Direction: "callees"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Graph is returned with just the root node, no edges.
	if len(graph.Nodes) != 1 {
		t.Errorf("expected 1 node (root only), got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected 0 edges (error swallowed), got %d", len(graph.Edges))
	}
}

func TestBuildCallGraph_FindReferencesError_Resilient(t *testing.T) {
	// Callers direction: FindReferences error is silently ignored.
	cg := newMockCodeGrapher()
	cg.symbols["main"] = []SymbolHit{
		{Name: "main", Kind: "function", FilePath: "main.go", Line: 1, EndLine: 10, Language: "go"},
	}
	cg.findRefErr = errors.New("reference search failed")

	graph, err := BuildCallGraph(cg, "main", GraphOptions{Direction: "callers"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(graph.Nodes) != 1 {
		t.Errorf("expected 1 node (root only), got %d", len(graph.Nodes))
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected 0 edges (error swallowed), got %d", len(graph.Edges))
	}
}

func TestBuildCallGraph_GetContainingSymbolError_Resilient(t *testing.T) {
	// GetContainingSymbol error in findCallers is silently ignored.
	cg := newMockCodeGrapher()
	cg.symbols["main"] = []SymbolHit{
		{Name: "main", Kind: "function", FilePath: "main.go", Line: 1, EndLine: 10, Language: "go"},
	}
	cg.references["main"] = []ReferenceHit{
		{Symbol: "main", Kind: "call", FilePath: "caller.go", Line: 5},
	}
	cg.containingErr = errors.New("containing symbol lookup failed")

	graph, err := BuildCallGraph(cg, "main", GraphOptions{Direction: "callers"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The callers path errors during findCallers, so no callers are added.
	if len(graph.Nodes) != 1 {
		t.Errorf("expected 1 node (root only), got %d", len(graph.Nodes))
	}
}
