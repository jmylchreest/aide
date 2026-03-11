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
		graph.Nodes = append(graph.Nodes, GraphNode(h))
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

	// processNeighbors adds edges and enqueues newly discovered nodes.
	processNeighbors := func(neighbors []neighbor, depth int) {
		for _, n := range neighbors {
			graph.Edges = append(graph.Edges, GraphEdge{
				From:     n.edgeFrom,
				To:       n.edgeTo,
				Kind:     n.ref.Kind,
				FilePath: n.ref.FilePath,
				Line:     n.ref.Line,
			})
			if n.node != nil && addNode(*n.node) {
				queue = append(queue, bfsItem{sym: *n.node, depth: depth + 1})
			}
		}
	}

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
				processNeighbors(toCalleeNeighbors(item.sym, callees), item.depth)
			}
		}

		// Find callers: references TO this symbol from other symbols.
		if opts.Direction == "both" || opts.Direction == "callers" {
			callers, err := findCallers(cg, item.sym)
			if err == nil {
				processNeighbors(toCallerNeighbors(item.sym, callers), item.depth)
			}
		}

		if len(graph.Nodes) >= opts.MaxNodes {
			break
		}
	}

	return graph, nil
}

// neighbor represents a discovered graph relationship (callee or caller).
type neighbor struct {
	edgeFrom string
	edgeTo   string
	ref      ReferenceHit
	node     *SymbolHit // the neighbor node to potentially add to BFS queue
}

// toCalleeNeighbors converts calleeResults into a uniform []neighbor slice.
func toCalleeNeighbors(sym SymbolHit, callees []calleeResult) []neighbor {
	out := make([]neighbor, 0, len(callees))
	for _, ce := range callees {
		out = append(out, neighbor{
			edgeFrom: sym.Name,
			edgeTo:   ce.ref.Symbol,
			ref:      ce.ref,
			node:     ce.target,
		})
	}
	return out
}

// toCallerNeighbors converts callerResults into a uniform []neighbor slice.
func toCallerNeighbors(sym SymbolHit, callers []callerResult) []neighbor {
	out := make([]neighbor, 0, len(callers))
	for _, cl := range callers {
		node := cl.caller // copy so we can take address
		out = append(out, neighbor{
			edgeFrom: cl.caller.Name,
			edgeTo:   sym.Name,
			ref:      cl.ref,
			node:     &node,
		})
	}
	return out
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
