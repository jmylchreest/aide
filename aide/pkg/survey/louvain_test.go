package survey

import (
	"fmt"
	"reflect"
	"testing"
)

// clique wires every pair of nodes with the given weight.
func clique(g *ModuleGraph, weight float64, nodes ...string) {
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			g.AddEdge(nodes[i], nodes[j], weight)
		}
	}
}

func twoCliqueGraph() *ModuleGraph {
	g := NewModuleGraph()
	clique(g, 3, "a/1", "a/2", "a/3", "a/4")
	clique(g, 3, "b/1", "b/2", "b/3", "b/4")
	g.AddEdge("a/1", "b/1", 0.5) // weak bridge
	return g
}

func TestClusterSeparatesCliques(t *testing.T) {
	got := Cluster(twoCliqueGraph(), ClusterOptions{HubExcludePercentile: 100})
	if len(got) != 2 {
		t.Fatalf("expected 2 communities, got %d: %v", len(got), got)
	}
	for _, members := range got {
		prefix := members[0][:2]
		for _, m := range members {
			if m[:2] != prefix {
				t.Errorf("community mixes cliques: %v", members)
			}
		}
	}
}

func TestClusterDeterminism(t *testing.T) {
	first := Cluster(twoCliqueGraph(), ClusterOptions{})
	for run := 0; run < 5; run++ {
		again := Cluster(twoCliqueGraph(), ClusterOptions{})
		if !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d differs:\n%v\nvs\n%v", run, first, again)
		}
	}
}

func TestClusterIsolatesAndEmpty(t *testing.T) {
	if got := Cluster(NewModuleGraph(), ClusterOptions{}); len(got) != 0 {
		t.Errorf("empty graph: %v", got)
	}

	g := NewModuleGraph()
	g.AddNode("lonely.ts")
	clique(g, 1, "x/1", "x/2", "x/3")
	got := Cluster(g, ClusterOptions{HubExcludePercentile: 100})
	if len(got) != 2 {
		t.Fatalf("expected clique + isolate, got %v", got)
	}
	if !reflect.DeepEqual(got[0], []string{"x/1", "x/2", "x/3"}) {
		t.Errorf("largest community first: %v", got)
	}
	if !reflect.DeepEqual(got[1], []string{"lonely.ts"}) {
		t.Errorf("isolate becomes its own community: %v", got)
	}
}

func TestClusterHubExclusion(t *testing.T) {
	// Two cliques plus a logger-style hub wired to everything. Without
	// exclusion the hub can glue the cliques; with it, the hub is held out
	// and reattached to whichever side it is more strongly tied to.
	g := twoCliqueGraph()
	for _, n := range []string{"a/1", "a/2", "a/3", "a/4", "b/1", "b/2", "b/3"} {
		g.AddEdge("lib/logger", n, 3)
	}
	g.AddEdge("lib/logger", "a/1", 3) // extra pull toward the a-side

	got := Cluster(g, ClusterOptions{HubExcludePercentile: 85})
	if len(got) != 2 {
		t.Fatalf("expected 2 communities, got %v", got)
	}
	var loggerHome []string
	for _, members := range got {
		for _, m := range members {
			if m == "lib/logger" {
				loggerHome = members
			}
		}
	}
	if loggerHome == nil {
		t.Fatal("hub was dropped instead of reattached")
	}
	if loggerHome[0][:2] != "a/" {
		t.Errorf("hub reattached to %v, expected the a-side", loggerHome)
	}
}

func TestClusterOversizedSplit(t *testing.T) {
	// One weakly-bridged pair of cliques forced into a single community by
	// resolution would exceed the fraction cap; the split pass separates it.
	g := NewModuleGraph()
	clique(g, 1, "p/1", "p/2", "p/3", "p/4", "p/5", "p/6")
	clique(g, 1, "q/1", "q/2", "q/3", "q/4", "q/5", "q/6")
	g.AddEdge("p/1", "q/1", 4)

	got := Cluster(g, ClusterOptions{HubExcludePercentile: 100, MaxCommunityFraction: 0.4, MinSplitSize: 4})
	if len(got) < 2 {
		t.Fatalf("expected oversized community to split, got %v", got)
	}
}

func TestClusterRemapToPrevious(t *testing.T) {
	g := twoCliqueGraph()
	first := Cluster(g, ClusterOptions{HubExcludePercentile: 100})

	// Previous run had the b-clique as community 0 and a-clique as 7.
	previous := map[string]int{}
	for _, m := range first[0] {
		previous[m] = 7
	}
	for _, m := range first[1] {
		previous[m] = 0
	}
	remapped := Cluster(g, ClusterOptions{HubExcludePercentile: 100, PreviousAssignment: previous})
	if !reflect.DeepEqual(remapped[7], first[0]) || !reflect.DeepEqual(remapped[0], first[1]) {
		t.Errorf("IDs did not follow previous assignment:\nfirst: %v\nremapped: %v", first, remapped)
	}
}

func TestClusterScales(t *testing.T) {
	// 40 cliques of 10 nodes in a ring — sanity that multi-level
	// aggregation converges and every clique stays whole.
	g := NewModuleGraph()
	var firsts []string
	for c := 0; c < 40; c++ {
		var nodes []string
		for n := 0; n < 10; n++ {
			nodes = append(nodes, fmt.Sprintf("mod%02d/f%d", c, n))
		}
		clique(g, 2, nodes...)
		firsts = append(firsts, nodes[0])
	}
	for i := range firsts {
		g.AddEdge(firsts[i], firsts[(i+1)%len(firsts)], 0.5)
	}

	got := Cluster(g, ClusterOptions{HubExcludePercentile: 100})
	if len(got) != 40 {
		t.Fatalf("expected 40 communities, got %d", len(got))
	}
	for _, members := range got {
		if len(members) != 10 {
			t.Errorf("community size %d, want 10: %v", len(members), members)
		}
	}
}
