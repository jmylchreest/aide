package survey

import "fmt"

// DefaultGraphDepth is the maximum BFS depth when not specified.
const DefaultGraphDepth = 2

// DefaultGraphLimit caps the total number of nodes in the graph.
const DefaultGraphLimit = 50

// GraphOptions configures the call-graph traversal.
type GraphOptions struct {
	// MaxDepth limits BFS hops from the root (0 = DefaultGraphDepth).
	MaxDepth int
	// MaxNodes caps total graph nodes (0 = DefaultGraphLimit).
	MaxNodes int
	// Direction controls traversal: "both" (default), "callers", "callees".
	Direction string
}

// BuildCallGraph performs a BFS traversal starting from the given symbol name.
// It uses the CodeGrapher to discover call relationships at each hop.
func BuildCallGraph(cg CodeGrapher, symbolName string, opts GraphOptions) (*CallGraph, error) {
	if opts.MaxDepth <= 0 {
		opts.MaxDepth = DefaultGraphDepth
	}
	if opts.MaxNodes <= 0 {
		opts.MaxNodes = DefaultGraphLimit
	}
	if opts.Direction == "" {
		opts.Direction = "both"
	}

	// Resolve the root symbol.
	hits, err := cg.FindSymbols(symbolName, "", 10)
	if err != nil {
		return nil, fmt.Errorf("finding root symbol: %w", err)
	}
	if len(hits) == 0 {
		return nil, fmt.Errorf("symbol %q not found in code index", symbolName)
	}

	// Pick the best match (exact name match preferred, then first hit).
	root := hits[0]
	for _, h := range hits {
		if h.Name == symbolName {
			root = h
			break
		}
	}

	graph := &CallGraph{
		Root:  root.Name,
		Depth: opts.MaxDepth,
	}

	// Track visited nodes by "file:name" to avoid duplicates.
	visited := make(map[string]bool)
	nodeKey := func(name, filePath string) string {
		return filePath + ":" + name
	}

	addNode := func(h SymbolHit) bool {
		key := nodeKey(h.Name, h.FilePath)
		if visited[key] {
			return false
		}
		if len(graph.Nodes) >= opts.MaxNodes {
			return false
		}
		visited[key] = true
		graph.Nodes = append(graph.Nodes, GraphNode{
			Name:     h.Name,
			Kind:     h.Kind,
			FilePath: h.FilePath,
			Line:     h.Line,
			EndLine:  h.EndLine,
			Language: h.Language,
		})
		return true
	}

	// Seed root.
	addNode(root)

	// BFS queue: each item is (SymbolHit, currentDepth).
	type bfsItem struct {
		sym   SymbolHit
		depth int
	}
	queue := []bfsItem{{sym: root, depth: 0}}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= opts.MaxDepth {
			continue
		}

		// Find callees: references inside this symbol's body.
		if opts.Direction == "both" || opts.Direction == "callees" {
			callees, err := findCallees(cg, item.sym)
			if err == nil {
				for _, ce := range callees {
					graph.Edges = append(graph.Edges, GraphEdge{
						From:     item.sym.Name,
						To:       ce.ref.Symbol,
						Kind:     ce.ref.Kind,
						FilePath: ce.ref.FilePath,
						Line:     ce.ref.Line,
					})
					if ce.target != nil && addNode(*ce.target) {
						queue = append(queue, bfsItem{sym: *ce.target, depth: item.depth + 1})
					}
				}
			}
		}

		// Find callers: references TO this symbol from other symbols.
		if opts.Direction == "both" || opts.Direction == "callers" {
			callers, err := findCallers(cg, item.sym)
			if err == nil {
				for _, cl := range callers {
					graph.Edges = append(graph.Edges, GraphEdge{
						From:     cl.caller.Name,
						To:       item.sym.Name,
						Kind:     cl.ref.Kind,
						FilePath: cl.ref.FilePath,
						Line:     cl.ref.Line,
					})
					if addNode(cl.caller) {
						queue = append(queue, bfsItem{sym: cl.caller, depth: item.depth + 1})
					}
				}
			}
		}

		if len(graph.Nodes) >= opts.MaxNodes {
			break
		}
	}

	return graph, nil
}

// calleeResult pairs a reference with its resolved target symbol (if found).
type calleeResult struct {
	ref    ReferenceHit
	target *SymbolHit // nil if the definition could not be resolved
}

// findCallees returns the symbols called from within the given symbol's body.
func findCallees(cg CodeGrapher, sym SymbolHit) ([]calleeResult, error) {
	if sym.EndLine <= 0 {
		return nil, nil // Can't determine body range
	}

	refs, err := cg.GetFileReferences(sym.FilePath)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var results []calleeResult

	for _, ref := range refs {
		// Only include references within this symbol's line range.
		if ref.Line < sym.Line || ref.Line > sym.EndLine {
			continue
		}
		// Skip self-references and import references.
		if ref.Symbol == sym.Name || ref.Kind == "import" {
			continue
		}
		// Deduplicate by target symbol name.
		if seen[ref.Symbol] {
			continue
		}
		seen[ref.Symbol] = true

		// Try to resolve the target symbol definition.
		var target *SymbolHit
		hits, err := cg.FindSymbols(ref.Symbol, "", 5)
		if err == nil && len(hits) > 0 {
			// Pick exact name match.
			best := hits[0]
			for _, h := range hits {
				if h.Name == ref.Symbol {
					best = h
					break
				}
			}
			target = &best
		}

		results = append(results, calleeResult{ref: ref, target: target})
	}

	return results, nil
}

// callerResult pairs a calling symbol with the reference site.
type callerResult struct {
	caller SymbolHit
	ref    ReferenceHit
}

// findCallers returns the symbols that call the given symbol.
func findCallers(cg CodeGrapher, sym SymbolHit) ([]callerResult, error) {
	refs, err := cg.FindReferences(sym.Name, "call", 50)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool)
	var results []callerResult

	for _, ref := range refs {
		// Skip self-references.
		if ref.FilePath == sym.FilePath && ref.Line >= sym.Line && ref.Line <= sym.EndLine {
			continue
		}

		// Resolve the calling symbol via GetContainingSymbol.
		caller, err := cg.GetContainingSymbol(ref.FilePath, ref.Line)
		if err != nil || caller == nil {
			continue
		}

		// Deduplicate by caller.
		key := caller.FilePath + ":" + caller.Name
		if seen[key] {
			continue
		}
		seen[key] = true

		results = append(results, callerResult{caller: *caller, ref: ref})
	}

	return results, nil
}
