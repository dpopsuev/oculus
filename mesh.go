package oculus

import (
	"path/filepath"
	"sort"
	"strings"
)

// BuildMesh constructs a hierarchical mesh from a SymbolGraph.
// Each symbol is placed in the hierarchy: symbol → file → package → component.
// Component names are matched against the provided service names.
func BuildMesh(sg *SymbolGraph, componentNames []string) *Mesh {
	if sg == nil {
		return &Mesh{Nodes: make(map[string]MeshNode)}
	}

	nodes := make(map[string]MeshNode)

	// Sort component names longest-first for greedy matching.
	sorted := make([]string, len(componentNames))
	copy(sorted, componentNames)
	sort.Slice(sorted, func(i, j int) bool { return len(sorted[i]) > len(sorted[j]) })

	resolveComponent := func(pkg string) string {
		for _, c := range sorted {
			if pkg == c || strings.HasPrefix(pkg, c+"/") {
				return c
			}
		}
		if pkg == "" {
			return "(root)"
		}
		return pkg
	}

	for _, sym := range sg.Nodes {
		fqn := sym.FQN()

		// Symbol node
		fileKey := sym.Package + "/" + filepath.Base(sym.File)
		if sym.File == "" {
			fileKey = sym.Package + "/(unknown)"
		}
		nodes[fqn] = MeshNode{Name: fqn, Level: MeshSymbol, Parent: fileKey}

		// File node
		if _, exists := nodes[fileKey]; !exists {
			nodes[fileKey] = MeshNode{Name: fileKey, Level: MeshFile, Parent: sym.Package}
		}
		fn := nodes[fileKey]
		fn.Children = appendUnique(fn.Children, fqn)
		nodes[fileKey] = fn

		// Package node
		comp := resolveComponent(sym.Package)
		if _, exists := nodes[sym.Package]; !exists {
			nodes[sym.Package] = MeshNode{Name: sym.Package, Level: MeshPackage, Parent: comp}
		}
		pn := nodes[sym.Package]
		pn.Children = appendUnique(pn.Children, fileKey)
		nodes[sym.Package] = pn

		// Component node
		if _, exists := nodes[comp]; !exists {
			nodes[comp] = MeshNode{Name: comp, Level: MeshComponent}
		}
		cn := nodes[comp]
		cn.Children = appendUnique(cn.Children, sym.Package)
		nodes[comp] = cn
	}

	return &Mesh{Nodes: nodes, Edges: sg.Edges}
}

// Aggregate collapses symbol-level edges to the target mesh level.
// Returns deduplicated edges with source/target resolved to the target level.
func (m *Mesh) Aggregate(level MeshLevel) []SymbolEdge {
	type edgeKey struct{ src, tgt string }
	seen := make(map[edgeKey]bool)
	var result []SymbolEdge

	for _, e := range m.Edges {
		src := m.resolveToLevel(e.SourceFQN, level)
		tgt := m.resolveToLevel(e.TargetFQN, level)
		if src == "" || tgt == "" || src == tgt {
			continue
		}
		ek := edgeKey{src, tgt}
		if seen[ek] {
			continue
		}
		seen[ek] = true
		result = append(result, SymbolEdge{
			SourceFQN: src, TargetFQN: tgt, Kind: e.Kind,
		})
	}
	return result
}

// Neighborhood returns FQNs within hops structural distance of the given FQN.
func (m *Mesh) Neighborhood(fqn string, hops int) []string {
	if hops <= 0 {
		return []string{fqn}
	}

	// Build adjacency from edges
	adj := make(map[string][]string)
	for _, e := range m.Edges {
		adj[e.SourceFQN] = append(adj[e.SourceFQN], e.TargetFQN)
		adj[e.TargetFQN] = append(adj[e.TargetFQN], e.SourceFQN)
	}

	visited := map[string]bool{fqn: true}
	frontier := []string{fqn}
	for d := 0; d < hops && len(frontier) > 0; d++ {
		var next []string
		for _, n := range frontier {
			for _, nb := range adj[n] {
				if !visited[nb] {
					visited[nb] = true
					next = append(next, nb)
				}
			}
		}
		frontier = next
	}

	result := make([]string, 0, len(visited))
	for n := range visited {
		result = append(result, n)
	}
	sort.Strings(result)
	return result
}

// Distance returns the shortest edge-hop count between two FQNs.
// Returns -1 if no path exists.
func (m *Mesh) Distance(from, to string) int {
	if from == to {
		return 0
	}
	adj := make(map[string][]string)
	for _, e := range m.Edges {
		adj[e.SourceFQN] = append(adj[e.SourceFQN], e.TargetFQN)
		adj[e.TargetFQN] = append(adj[e.TargetFQN], e.SourceFQN)
	}

	visited := map[string]bool{from: true}
	frontier := []string{from}
	dist := 0
	for len(frontier) > 0 {
		dist++
		var next []string
		for _, n := range frontier {
			for _, nb := range adj[n] {
				if nb == to {
					return dist
				}
				if !visited[nb] {
					visited[nb] = true
					next = append(next, nb)
				}
			}
		}
		frontier = next
	}
	return -1
}

// Boundaries returns edges that cross component boundaries — architectural seams.
func (m *Mesh) Boundaries() []SymbolEdge {
	var seams []SymbolEdge
	for _, e := range m.Edges {
		srcComp := m.resolveToLevel(e.SourceFQN, MeshComponent)
		tgtComp := m.resolveToLevel(e.TargetFQN, MeshComponent)
		if srcComp != "" && tgtComp != "" && srcComp != tgtComp {
			seams = append(seams, e)
		}
	}
	return seams
}

// resolveToLevel walks up the parent chain to find the ancestor at the given level.
func (m *Mesh) resolveToLevel(fqn string, level MeshLevel) string {
	current := fqn
	for i := 0; i < 10; i++ { // safety bound
		node, exists := m.Nodes[current]
		if !exists {
			return current // not in mesh — return as-is
		}
		if node.Level == level {
			return current
		}
		if node.Parent == "" {
			return current
		}
		current = node.Parent
	}
	return current
}

func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}
