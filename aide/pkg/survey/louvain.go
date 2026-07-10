// Package survey: louvain.go clusters the file graph into modules.
//
// Deterministic by construction, per the stored-id-determinism decision:
// no randomness anywhere, nodes iterated in sorted order during the local-
// move phase, ties broken by total order, community IDs assigned by size
// with sorted-member tiebreaks, and IDs remapped to the previous persisted
// assignment on re-run so an unchanged grouping keeps unchanged IDs.
package survey

import (
	"sort"
)

// ModuleGraph is an undirected weighted graph over file/unit ids.
type ModuleGraph struct {
	adj map[string]map[string]float64
}

func NewModuleGraph() *ModuleGraph {
	return &ModuleGraph{adj: make(map[string]map[string]float64)}
}

// AddNode ensures id exists even with no edges (isolates become their own
// single-member communities).
func (g *ModuleGraph) AddNode(id string) {
	if g.adj[id] == nil {
		g.adj[id] = make(map[string]float64)
	}
}

// AddEdge accumulates weight between a and b. Self-edges are ignored —
// intra-unit references are not coupling.
func (g *ModuleGraph) AddEdge(a, b string, w float64) {
	if a == b || w <= 0 {
		return
	}
	g.AddNode(a)
	g.AddNode(b)
	g.adj[a][b] += w
	g.adj[b][a] += w
}

// NodeCount returns the number of nodes.
func (g *ModuleGraph) NodeCount() int { return len(g.adj) }

// EdgeCount returns the number of distinct undirected edges.
func (g *ModuleGraph) EdgeCount() int {
	total := 0
	for _, nbrs := range g.adj {
		total += len(nbrs)
	}
	return total / 2
}

func (g *ModuleGraph) sortedNodes() []string {
	nodes := make([]string, 0, len(g.adj))
	for n := range g.adj {
		nodes = append(nodes, n)
	}
	sort.Strings(nodes)
	return nodes
}

// Cohesion is the ratio of intra-member edges to possible member pairs.
func (g *ModuleGraph) Cohesion(members []string) float64 {
	n := len(members)
	if n <= 1 {
		return 1.0
	}
	in := make(map[string]bool, n)
	for _, m := range members {
		in[m] = true
	}
	edges := 0
	for _, m := range members {
		for nb := range g.adj[m] {
			if in[nb] {
				edges++
			}
		}
	}
	return float64(edges/2) / (float64(n) * float64(n-1) / 2)
}

// Degree returns a node's weighted degree.
func (g *ModuleGraph) Degree(id string) float64 {
	total := 0.0
	for _, w := range g.adj[id] {
		total += w
	}
	return total
}

// ClusterOptions tunes community detection and its post-processing.
type ClusterOptions struct {
	Resolution             float64        // >1 more/smaller communities; 0 = 1.0
	HubExcludePercentile   float64        // degree percentile above which nodes are held out; 0 = 99.5, >=100 disables
	MaxCommunityFraction   float64        // communities larger than this fraction get re-split; 0 = 0.25
	MinSplitSize           int            // never split communities smaller than this; 0 = 10
	CohesionSplitThreshold float64        // re-split low-cohesion communities below this; 0 = 0.05
	CohesionSplitMinSize   int            // only cohesion-split at or above this size; 0 = 50
	PreviousAssignment     map[string]int // node -> community id from the previous run, for stable IDs
}

func (o *ClusterOptions) withDefaults() ClusterOptions {
	out := *o
	if out.Resolution <= 0 {
		out.Resolution = 1.0
	}
	if out.HubExcludePercentile <= 0 {
		out.HubExcludePercentile = 99.5
	}
	if out.MaxCommunityFraction <= 0 {
		out.MaxCommunityFraction = 0.25
	}
	if out.MinSplitSize <= 0 {
		out.MinSplitSize = 10
	}
	if out.CohesionSplitThreshold <= 0 {
		out.CohesionSplitThreshold = 0.05
	}
	if out.CohesionSplitMinSize <= 0 {
		out.CohesionSplitMinSize = 50
	}
	return out
}

// Cluster partitions the graph into communities: {community id: sorted members}.
// IDs are stable: size-descending with sorted-member tiebreaks, then remapped
// to PreviousAssignment by greedy overlap when provided.
func Cluster(g *ModuleGraph, opts ClusterOptions) map[int][]string {
	o := opts.withDefaults()
	if g.NodeCount() == 0 {
		return map[int][]string{}
	}

	// Hub exclusion: hold out super-connected utility nodes so they cannot
	// pull unrelated subsystems into one community; reattach afterwards by
	// neighbour majority vote.
	hubs := hubNodes(g, o.HubExcludePercentile)
	communities := louvainOn(g, hubs, o.Resolution)

	// Oversized split: a community above the fraction cap gets a second
	// pass on its own subgraph.
	maxSize := int(float64(g.NodeCount()) * o.MaxCommunityFraction)
	if maxSize < o.MinSplitSize {
		maxSize = o.MinSplitSize
	}
	var split [][]string
	for _, members := range communities {
		if len(members) > maxSize {
			split = append(split, splitCommunity(g, members, o.Resolution)...)
		} else {
			split = append(split, members)
		}
	}

	// Low-cohesion re-split: large communities glued together by bridge
	// nodes fall apart on a scoped second pass.
	var final [][]string
	for _, members := range split {
		if len(members) >= o.CohesionSplitMinSize && g.Cohesion(members) < o.CohesionSplitThreshold {
			parts := splitCommunity(g, members, o.Resolution)
			if len(parts) > 1 {
				final = append(final, parts...)
				continue
			}
		}
		final = append(final, members)
	}

	// Reattach hubs by neighbour majority vote.
	final = reattachHubs(g, final, hubs)

	// Total-order community IDs: size descending, then sorted members.
	sort.Slice(final, func(i, j int) bool {
		if len(final[i]) != len(final[j]) {
			return len(final[i]) > len(final[j])
		}
		return lessStringSlices(final[i], final[j])
	})
	result := make(map[int][]string, len(final))
	for i, members := range final {
		sort.Strings(members)
		result[i] = members
	}

	if len(opts.PreviousAssignment) > 0 {
		result = remapToPrevious(result, opts.PreviousAssignment)
	}
	return result
}

func lessStringSlices(a, b []string) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// hubNodes returns nodes whose weighted degree exceeds the given percentile.
func hubNodes(g *ModuleGraph, percentile float64) map[string]bool {
	hubs := make(map[string]bool)
	if percentile >= 100 {
		return hubs
	}
	nodes := g.sortedNodes()
	degrees := make([]float64, 0, len(nodes))
	for _, n := range nodes {
		degrees = append(degrees, g.Degree(n))
	}
	sorted := append([]float64(nil), degrees...)
	sort.Float64s(sorted)
	idx := int(float64(len(sorted)) * percentile / 100)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	threshold := sorted[idx]
	for i, n := range nodes {
		if degrees[i] > threshold {
			hubs[n] = true
		}
	}
	return hubs
}

// louvainOn runs community detection on the graph minus excluded nodes.
// Isolates become single-member communities.
func louvainOn(g *ModuleGraph, exclude map[string]bool, resolution float64) [][]string {
	var nodes []string
	for _, n := range g.sortedNodes() {
		if !exclude[n] {
			nodes = append(nodes, n)
		}
	}
	idx := make(map[string]int, len(nodes))
	for i, n := range nodes {
		idx[n] = i
	}

	lv := &lvGraph{n: len(nodes), adj: make([]map[int]float64, len(nodes)), self: make([]float64, len(nodes))}
	for i, a := range nodes {
		lv.adj[i] = make(map[int]float64)
		for b, w := range g.adj[a] {
			if j, ok := idx[b]; ok {
				lv.adj[i][j] = w
			}
		}
	}
	lv.computeTotals()

	assignment := lv.partition(resolution)

	byComm := make(map[int][]string)
	for i, c := range assignment {
		byComm[c] = append(byComm[c], nodes[i])
	}
	commIDs := make([]int, 0, len(byComm))
	for c := range byComm {
		commIDs = append(commIDs, c)
	}
	sort.Ints(commIDs)
	out := make([][]string, 0, len(commIDs))
	for _, c := range commIDs {
		members := byComm[c]
		sort.Strings(members)
		out = append(out, members)
	}
	return out
}

// splitCommunity reruns detection on a community's subgraph. Returns the
// original members as one community when no finer structure exists.
func splitCommunity(g *ModuleGraph, members []string, resolution float64) [][]string {
	in := make(map[string]bool, len(members))
	for _, m := range members {
		in[m] = true
	}
	sub := NewModuleGraph()
	for _, m := range members {
		sub.AddNode(m)
		for nb, w := range g.adj[m] {
			if in[nb] && m < nb {
				sub.AddEdge(m, nb, w)
			}
		}
	}
	parts := louvainOn(sub, nil, resolution)
	if len(parts) <= 1 {
		sorted := append([]string(nil), members...)
		sort.Strings(sorted)
		return [][]string{sorted}
	}
	return parts
}

// reattachHubs assigns each excluded hub to the community holding the
// majority of its edge weight; a hub with no placed neighbours becomes its
// own community.
func reattachHubs(g *ModuleGraph, communities [][]string, hubs map[string]bool) [][]string {
	if len(hubs) == 0 {
		return communities
	}
	nodeComm := make(map[string]int)
	for ci, members := range communities {
		for _, m := range members {
			nodeComm[m] = ci
		}
	}
	hubList := make([]string, 0, len(hubs))
	for h := range hubs {
		hubList = append(hubList, h)
	}
	sort.Strings(hubList)

	for _, hub := range hubList {
		votes := make(map[int]float64)
		for nb, w := range g.adj[hub] {
			if ci, ok := nodeComm[nb]; ok {
				votes[ci] += w
			}
		}
		best, bestW := -1, 0.0
		commIDs := make([]int, 0, len(votes))
		for ci := range votes {
			commIDs = append(commIDs, ci)
		}
		sort.Ints(commIDs)
		for _, ci := range commIDs {
			if votes[ci] > bestW {
				best, bestW = ci, votes[ci]
			}
		}
		if best >= 0 {
			communities[best] = append(communities[best], hub)
			nodeComm[hub] = best
		} else {
			communities = append(communities, []string{hub})
			nodeComm[hub] = len(communities) - 1
		}
	}
	return communities
}

// remapToPrevious relabels community IDs to maximise member overlap with a
// previous assignment (greedy one-to-one by overlap size), assigning fresh
// IDs to unmatched communities in deterministic order.
func remapToPrevious(communities map[int][]string, previous map[string]int) map[int][]string {
	type overlap struct{ count, oldID, newID int }
	var overlaps []overlap
	for newID := 0; newID < len(communities); newID++ {
		counts := make(map[int]int)
		for _, member := range communities[newID] {
			if oc, ok := previous[member]; ok {
				counts[oc]++
			}
		}
		for oc, c := range counts {
			overlaps = append(overlaps, overlap{count: c, oldID: oc, newID: newID})
		}
	}
	sort.Slice(overlaps, func(i, j int) bool {
		if overlaps[i].count != overlaps[j].count {
			return overlaps[i].count > overlaps[j].count
		}
		if overlaps[i].oldID != overlaps[j].oldID {
			return overlaps[i].oldID < overlaps[j].oldID
		}
		return overlaps[i].newID < overlaps[j].newID
	})

	newToFinal := make(map[int]int)
	usedOld := make(map[int]bool)
	for _, ov := range overlaps {
		if usedOld[ov.oldID] {
			continue
		}
		if _, done := newToFinal[ov.newID]; done {
			continue
		}
		newToFinal[ov.newID] = ov.oldID
		usedOld[ov.oldID] = true
	}

	nextID := 0
	for newID := 0; newID < len(communities); newID++ {
		if _, done := newToFinal[newID]; done {
			continue
		}
		for usedOld[nextID] {
			nextID++
		}
		newToFinal[newID] = nextID
		usedOld[nextID] = true
	}

	out := make(map[int][]string, len(communities))
	for newID, members := range communities {
		out[newToFinal[newID]] = members
	}
	return out
}

// =============================================================================
// Core Louvain on a compact index graph
// =============================================================================

type lvGraph struct {
	n    int
	adj  []map[int]float64 // neighbour -> weight, no self entries
	self []float64         // self-loop weight (appears when aggregating)
	deg  []float64         // weighted degree incl. 2*self
	m2   float64           // 2 * total edge weight
}

func (lv *lvGraph) computeTotals() {
	lv.deg = make([]float64, lv.n)
	lv.m2 = 0
	for i := 0; i < lv.n; i++ {
		d := 2 * lv.self[i]
		for _, w := range lv.adj[i] {
			d += w
		}
		lv.deg[i] = d
		lv.m2 += d
	}
}

// partition runs multi-level Louvain and returns a community index per node.
func (lv *lvGraph) partition(resolution float64) []int {
	assignment := make([]int, lv.n)
	for i := range assignment {
		assignment[i] = i
	}
	cur := lv
	for {
		comm, moved := cur.localMove(resolution)
		next, mapping := cur.aggregate(comm)
		for i := range assignment {
			assignment[i] = mapping[comm[assignment[i]]]
		}
		if !moved || next.n == cur.n {
			break
		}
		cur = next
	}
	return assignment
}

// localMove greedily reassigns nodes to neighbouring communities while
// modularity improves. Nodes are visited in index order (callers construct
// indices from sorted ids) and gain ties break toward the smaller community
// index, so the outcome is deterministic.
func (lv *lvGraph) localMove(resolution float64) (comm []int, moved bool) {
	comm = make([]int, lv.n)
	commTot := make([]float64, lv.n)
	for i := 0; i < lv.n; i++ {
		comm[i] = i
		commTot[i] = lv.deg[i]
	}
	if lv.m2 == 0 {
		return comm, false
	}

	for {
		anyMove := false
		for i := 0; i < lv.n; i++ {
			old := comm[i]
			commTot[old] -= lv.deg[i]

			// Weight from i into each neighbouring community.
			wTo := make(map[int]float64)
			for nb, w := range lv.adj[i] {
				wTo[comm[nb]] += w
			}

			candidates := make([]int, 0, len(wTo)+1)
			for c := range wTo {
				candidates = append(candidates, c)
			}
			sort.Ints(candidates)

			// Strictly better gain wins; a tie within epsilon keeps the
			// earlier (smaller-index) winner, which candidates' sorted
			// order guarantees — deterministic without a random tiebreak.
			best, bestGain := old, wTo[old]-resolution*lv.deg[i]*commTot[old]/lv.m2
			for _, c := range candidates {
				if c == old {
					continue
				}
				gain := wTo[c] - resolution*lv.deg[i]*commTot[c]/lv.m2
				if gain > bestGain+1e-12 {
					best, bestGain = c, gain
				}
			}

			comm[i] = best
			commTot[best] += lv.deg[i]
			if best != old {
				anyMove = true
				moved = true
			}
		}
		if !anyMove {
			break
		}
	}
	return comm, moved
}

// aggregate collapses communities into supernodes, keeping internal weight
// as self-loops. Returns the new graph and old-community -> new-index map.
func (lv *lvGraph) aggregate(comm []int) (*lvGraph, map[int]int) {
	mapping := make(map[int]int)
	next := 0
	for i := 0; i < lv.n; i++ { // index order keeps supernode numbering deterministic
		if _, ok := mapping[comm[i]]; !ok {
			mapping[comm[i]] = next
			next++
		}
	}

	out := &lvGraph{n: next, adj: make([]map[int]float64, next), self: make([]float64, next)}
	for i := 0; i < next; i++ {
		out.adj[i] = make(map[int]float64)
	}
	for i := 0; i < lv.n; i++ {
		ci := mapping[comm[i]]
		out.self[ci] += lv.self[i]
		for nb, w := range lv.adj[i] {
			cj := mapping[comm[nb]]
			if ci == cj {
				out.self[ci] += w / 2 // each internal edge visited from both ends
			} else {
				out.adj[ci][cj] += w // visited once per direction; adj stays symmetric
			}
		}
	}
	out.computeTotals()
	return out, mapping
}
