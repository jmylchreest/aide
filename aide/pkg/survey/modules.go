// Package survey: modules.go discovers structural modules by clustering the
// file graph — resolved imports plus symbol references — with Louvain
// community detection. Directory layout shows where files live; the import
// graph shows what they belong to. The divergences are the signal.
package survey

import (
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/jmylchreest/aide/aide/pkg/importresolve"
)

// Edge weights and guards for the module graph.
const (
	// ImportEdgeWeight scores a resolved import — the strong, explicit signal.
	ImportEdgeWeight = 3.0
	// SymbolEdgeWeight scores a symbol reference to its defining file — the
	// weaker signal, capped so name collisions cannot dominate imports.
	SymbolEdgeWeight = 1.0
	// MaxSymbolDefiners drops symbol-reference edges for names defined in
	// more than this many files: too ambiguous to attribute.
	MaxSymbolDefiners = 3
	// MinModuleSize is the smallest community stored as a module entry;
	// smaller ones are counted as unclustered singletons.
	MinModuleSize = 2
)

// ModuleFile is one source file the analyzer clusters.
type ModuleFile struct {
	Path     string
	Language string
}

// ModulesSource abstracts the code index for the modules analyzer.
type ModulesSource interface {
	// ListSourceFiles returns every indexed source file with its language.
	ListSourceFiles() ([]ModuleFile, error)
	// FileReferences returns all references made in a file.
	FileReferences(filePath string) ([]ReferenceHit, error)
	// DefiningFiles returns the distinct files defining a symbol name,
	// capped by the implementation (MaxSymbolDefiners+1 is enough).
	DefiningFiles(symbolName string) ([]string, error)
}

// ModulesConfig configures a modules run.
type ModulesConfig struct {
	RootDir  string
	Source   ModulesSource
	Resolver *importresolve.Resolver // nil = constructed from RootDir
	Previous map[string]int          // file -> community id from the prior run
	Cluster  ClusterOptions
}

// ModulesResult is the outcome of a modules run.
type ModulesResult struct {
	Entries         []*Entry
	Files           int
	Communities     int // stored module entries
	Singletons      int // files left unclustered (below MinModuleSize)
	ImportsTotal    int // internal-looking plus external — everything seen
	ImportsResolved int
}

// RunModules builds the file graph and clusters it into module entries.
func RunModules(cfg ModulesConfig) (*ModulesResult, error) {
	if cfg.Source == nil {
		return nil, fmt.Errorf("modules analyzer requires the code index")
	}
	resolver := cfg.Resolver
	if resolver == nil {
		resolver = importresolve.New(cfg.RootDir)
	}

	files, err := cfg.Source.ListSourceFiles()
	if err != nil {
		return nil, fmt.Errorf("list source files: %w", err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })

	node := testPairing(files)
	result := &ModulesResult{Files: len(files)}

	g := NewModuleGraph()
	definers := make(map[string][]string)
	lookupDefiners := func(sym string) []string {
		if cached, ok := definers[sym]; ok {
			return cached
		}
		found, derr := cfg.Source.DefiningFiles(sym)
		if derr != nil {
			found = nil
		}
		sort.Strings(found)
		definers[sym] = found
		return found
	}

	for _, f := range files {
		from := node(f.Path)
		g.AddNode(from)

		refs, rerr := cfg.Source.FileReferences(f.Path)
		if rerr != nil {
			continue
		}

		// Unique symbol names referenced by this file, for attribution and
		// symbol edges — per-name, not per-call-site, so a hot call in a
		// loop does not outweigh an import.
		symbolNames := make(map[string]bool)
		var imports []string
		for _, ref := range refs {
			if ref.Kind == "import" {
				imports = append(imports, ref.Symbol)
			} else {
				symbolNames[ref.Symbol] = true
			}
		}
		sortedSymbols := make([]string, 0, len(symbolNames))
		for s := range symbolNames {
			sortedSymbols = append(sortedSymbols, s)
		}
		sort.Strings(sortedSymbols)

		for _, imp := range imports {
			result.ImportsTotal++
			targets := resolver.ResolveFiles(f.Language, f.Path, imp)
			if len(targets) == 0 {
				continue
			}
			result.ImportsResolved++
			if len(targets) == 1 {
				g.AddEdge(from, node(targets[0]), ImportEdgeWeight)
				continue
			}
			// Multi-file target (Go package, C# namespace, JVM wildcard):
			// attribute the import to the member files whose symbols this
			// file actually references; fan out fractionally only when
			// attribution finds nothing. Whole-package cliques are what
			// community detection cannot cut.
			targetSet := make(map[string]bool, len(targets))
			for _, t := range targets {
				targetSet[t] = true
			}
			var attributed []string
			for _, sym := range sortedSymbols {
				for _, d := range lookupDefiners(sym) {
					if targetSet[d] {
						attributed = append(attributed, d)
					}
				}
			}
			if len(attributed) > 0 {
				seen := make(map[string]bool, len(attributed))
				for _, t := range attributed {
					if !seen[t] {
						seen[t] = true
						g.AddEdge(from, node(t), ImportEdgeWeight)
					}
				}
				continue
			}
			w := ImportEdgeWeight / float64(len(targets))
			for _, t := range targets {
				g.AddEdge(from, node(t), w)
			}
		}

		for _, sym := range sortedSymbols {
			found := lookupDefiners(sym)
			if len(found) == 0 || len(found) > MaxSymbolDefiners {
				continue
			}
			for _, d := range found {
				if d != f.Path {
					g.AddEdge(from, node(d), SymbolEdgeWeight)
				}
			}
		}
	}

	opts := cfg.Cluster
	opts.PreviousAssignment = cfg.Previous
	communities := Cluster(g, opts)

	result.Entries, result.Communities, result.Singletons = communityEntries(g, communities, result)
	AnnotateEstTokens(cfg.RootDir, result.Entries)
	return result, nil
}

// testPairing maps test files onto their subjects so a test and the code it
// tests are one node — they are one module by definition, and test↔subject
// edges otherwise dominate the graph.
func testPairing(files []ModuleFile) func(string) string {
	exists := make(map[string]bool, len(files))
	for _, f := range files {
		exists[f.Path] = true
	}
	pair := make(map[string]string)
	for _, f := range files {
		if subject := testSubject(f.Path); subject != "" && exists[subject] {
			pair[f.Path] = subject
		}
	}
	return func(p string) string {
		if s, ok := pair[p]; ok {
			return s
		}
		return p
	}
}

// testSubject returns the conventional subject file for a test file name,
// or "" when the name carries no test convention.
func testSubject(p string) string {
	dir, base := path.Split(p)
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return dir + strings.TrimSuffix(base, "_test.go") + ".go"
	case strings.HasSuffix(base, ".test.ts"), strings.HasSuffix(base, ".spec.ts"):
		return dir + base[:len(base)-len(".test.ts")] + ".ts"
	case strings.HasSuffix(base, ".test.tsx"), strings.HasSuffix(base, ".spec.tsx"):
		return dir + base[:len(base)-len(".test.tsx")] + ".tsx"
	case strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"):
		return dir + strings.TrimPrefix(base, "test_")
	}
	return ""
}

// communityEntries renders clustered communities as survey entries.
func communityEntries(g *ModuleGraph, communities map[int][]string, res *ModulesResult) ([]*Entry, int, int) {
	ids := make([]int, 0, len(communities))
	for id := range communities {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	usedLabels := make(map[string]int)
	var entries []*Entry
	singletons := 0
	for _, id := range ids {
		members := communities[id]
		if len(members) < MinModuleSize {
			singletons += len(members)
			continue
		}

		label := moduleLabel(g, members)
		if n := usedLabels[label]; n > 0 {
			usedLabels[label] = n + 1
			label = fmt.Sprintf("%s-%d", label, n+1)
		} else {
			usedLabels[label] = 1
		}

		hub := hubMember(g, members)
		cohesion := g.Cohesion(members)
		membersJSON, _ := json.Marshal(members)

		entries = append(entries, &Entry{
			Analyzer: AnalyzerModules,
			Kind:     KindModule,
			Name:     label,
			Title:    fmt.Sprintf("Module %s: %d files, hub %s", label, len(members), hub),
			Detail: fmt.Sprintf(
				"Structural module discovered by clustering the import/reference graph. %d files, cohesion %.2f, hub %s. Top dirs: %s.",
				len(members), cohesion, hub, strings.Join(topDirs(members, 3), ", ")),
			Metadata: map[string]string{
				"community_id":     strconv.Itoa(id),
				"size":             strconv.Itoa(len(members)),
				"cohesion":         fmt.Sprintf("%.3f", cohesion),
				"hub":              hub,
				"top_dirs":         strings.Join(topDirs(members, 3), ","),
				"members":          string(membersJSON),
				"imports_resolved": fmt.Sprintf("%d/%d", res.ImportsResolved, res.ImportsTotal),
			},
		})
	}
	return entries, len(entries), singletons
}

// moduleLabel names a community after the longest common directory prefix of
// its members, falling back to the hub file's stem when members share none.
func moduleLabel(g *ModuleGraph, members []string) string {
	prefix := commonDirPrefix(members)
	if prefix != "" && prefix != "." {
		return prefix
	}
	hub := hubMember(g, members)
	base := path.Base(hub)
	if i := strings.IndexByte(base, '.'); i > 0 {
		base = base[:i]
	}
	return base
}

func commonDirPrefix(members []string) string {
	if len(members) == 0 {
		return ""
	}
	parts := strings.Split(path.Dir(members[0]), "/")
	for _, m := range members[1:] {
		mp := strings.Split(path.Dir(m), "/")
		limit := len(parts)
		if len(mp) < limit {
			limit = len(mp)
		}
		k := 0
		for k < limit && parts[k] == mp[k] {
			k++
		}
		parts = parts[:k]
		if len(parts) == 0 {
			return ""
		}
	}
	return strings.Join(parts, "/")
}

// hubMember returns the member with the highest weighted degree, ties broken
// by name for determinism.
func hubMember(g *ModuleGraph, members []string) string {
	best, bestDeg := "", -1.0
	for _, m := range members {
		d := g.Degree(m)
		if d > bestDeg || (d == bestDeg && (best == "" || m < best)) {
			best, bestDeg = m, d
		}
	}
	return best
}

// topDirs returns the most common directories among members, most frequent
// first, ties alphabetical.
func topDirs(members []string, limit int) []string {
	counts := make(map[string]int)
	for _, m := range members {
		counts[path.Dir(m)]++
	}
	dirs := make([]string, 0, len(counts))
	for d := range counts {
		dirs = append(dirs, d)
	}
	sort.Slice(dirs, func(i, j int) bool {
		if counts[dirs[i]] != counts[dirs[j]] {
			return counts[dirs[i]] > counts[dirs[j]]
		}
		return dirs[i] < dirs[j]
	})
	if len(dirs) > limit {
		dirs = dirs[:limit]
	}
	return dirs
}

// PreviousAssignmentFromEntries reconstructs the file -> community map from
// a prior run's stored entries, for stable IDs and membership diffs. Entries
// carry their full member list precisely so no separate assignment store
// (with its own gRPC surface) is needed.
func PreviousAssignmentFromEntries(entries []*Entry) map[string]int {
	previous := make(map[string]int)
	for _, e := range entries {
		if e.Analyzer != AnalyzerModules {
			continue
		}
		id, err := strconv.Atoi(e.Metadata["community_id"])
		if err != nil {
			continue
		}
		var members []string
		if json.Unmarshal([]byte(e.Metadata["members"]), &members) != nil {
			continue
		}
		for _, m := range members {
			previous[m] = id
		}
	}
	return previous
}
