package graph

import (
	"sort"

	"gonum.org/v1/gonum/graph/topo"
)

// CycleGroup is a strongly connected component with more than one node.
// Every package in the group can reach every other — they form an
// indivisible coupling knot that Johnson's simple-cycle enumeration
// inflates into potentially hundreds of reported cycles.
type CycleGroup []string

// StronglyConnectedComponents returns all SCCs with more than one node,
// ordered by size descending. Each group is sorted alphabetically.
// This is the actionable coupling metric: one SCC = one knot to untangle,
// regardless of how many simple cycles Johnson's algorithm finds inside it.
func StronglyConnectedComponents[E Edge](edges []E) []CycleGroup {
	if len(edges) == 0 {
		return nil
	}
	sg := fromEdges(edges)
	sccs := topo.TarjanSCC(sg.g)

	var groups []CycleGroup
	for _, scc := range sccs {
		if len(scc) <= 1 {
			continue
		}
		group := make(CycleGroup, len(scc))
		for i, n := range scc {
			group[i] = sg.nodeName(n.ID())
		}
		sort.Strings(group)
		groups = append(groups, group)
	}
	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i]) > len(groups[j])
	})
	return groups
}

// ConnectedComponents finds connected components in an undirected view of the
// directed graph (edges treated as bidirectional). Returns groups of nodes,
// each sorted alphabetically. Groups are sorted by size descending.
func ConnectedComponents[E Edge](edges []E) [][]string {
	if len(edges) == 0 {
		return nil
	}
	sg := fromEdges(edges)
	components := topo.ConnectedComponents(asUndirected(sg))

	groups := make([][]string, 0, len(components))
	for _, comp := range components {
		group := make([]string, len(comp))
		for i, n := range comp {
			group[i] = sg.nodeName(n.ID())
		}
		sort.Strings(group)
		groups = append(groups, group)
	}

	sort.Slice(groups, func(i, j int) bool {
		return len(groups[i]) > len(groups[j])
	})
	return groups
}
