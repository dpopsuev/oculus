package oculus

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/dpopsuev/oculus/graph"
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

		// Component node — upgrade to MeshComponent if package node already exists at same key.
		if existing, exists := nodes[comp]; !exists {
			nodes[comp] = MeshNode{Name: comp, Level: MeshComponent}
		} else if existing.Level < MeshComponent {
			existing.Level = MeshComponent
			nodes[comp] = existing
		}
		cn := nodes[comp]
		if sym.Package != comp {
			cn.Children = appendUnique(cn.Children, sym.Package)
		}
		nodes[comp] = cn
	}

	// Classify edge weights using component context.
	edges := make([]SymbolEdge, len(sg.Edges))
	for i, e := range sg.Edges {
		edges[i] = e
		if e.Weight == 0 {
			edges[i].Weight = ClassifyEdgeWeight(e.SourceFQN, e.TargetFQN, componentNames)
		}
	}

	return &Mesh{Nodes: nodes, Edges: edges}
}

// Aggregate collapses symbol-level edges to the target mesh level.
// Returns deduplicated edges with source/target resolved to the target level.
func (m *Mesh) Aggregate(level MeshLevel) []SymbolEdge {
	type edgeKey struct{ src, tgt string }
	agg := make(map[edgeKey]*SymbolEdge)
	order := make([]edgeKey, 0)

	for _, e := range m.Edges {
		src := m.resolveToLevel(e.SourceFQN, level)
		tgt := m.resolveToLevel(e.TargetFQN, level)
		if src == "" || tgt == "" || src == tgt {
			continue
		}
		ek := edgeKey{src, tgt}
		if existing, ok := agg[ek]; ok {
			existing.Weight += e.Weight
		} else {
			agg[ek] = &SymbolEdge{
				SourceFQN: src, TargetFQN: tgt, Kind: e.Kind, Weight: e.Weight,
			}
			order = append(order, ek)
		}
	}

	result := make([]SymbolEdge, 0, len(agg))
	for _, ek := range order {
		result = append(result, *agg[ek])
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

// WeightedNeighbor pairs an FQN with the weight of its connecting edge.
type WeightedNeighbor struct {
	FQN    string  `json:"fqn"`
	Weight float64 `json:"weight"`
}

// NeighborhoodWeighted returns neighbors within hops, sorted by edge weight descending.
func (m *Mesh) NeighborhoodWeighted(fqn string, hops int) []WeightedNeighbor {
	if hops <= 0 {
		return []WeightedNeighbor{{FQN: fqn}}
	}

	// Build adjacency with weights
	type neighbor struct {
		fqn    string
		weight float64
	}
	adj := make(map[string][]neighbor)
	for _, e := range m.Edges {
		adj[e.SourceFQN] = append(adj[e.SourceFQN], neighbor{e.TargetFQN, e.Weight})
		adj[e.TargetFQN] = append(adj[e.TargetFQN], neighbor{e.SourceFQN, e.Weight})
	}

	bestWeight := make(map[string]float64)
	bestWeight[fqn] = 0
	frontier := []string{fqn}
	for d := 0; d < hops && len(frontier) > 0; d++ {
		var next []string
		for _, n := range frontier {
			for _, nb := range adj[n] {
				if _, seen := bestWeight[nb.fqn]; !seen {
					bestWeight[nb.fqn] = nb.weight
					next = append(next, nb.fqn)
				} else if nb.weight > bestWeight[nb.fqn] {
					bestWeight[nb.fqn] = nb.weight
				}
			}
		}
		frontier = next
	}

	result := make([]WeightedNeighbor, 0, len(bestWeight))
	for f, w := range bestWeight {
		if f == fqn {
			continue
		}
		result = append(result, WeightedNeighbor{FQN: f, Weight: w})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Weight > result[j].Weight
	})
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
	return m.BoundariesMinWeight(0)
}

// BoundariesMinWeight returns boundary-crossing edges with weight >= minWeight.
func (m *Mesh) BoundariesMinWeight(minWeight float64) []SymbolEdge {
	var seams []SymbolEdge
	for _, e := range m.Edges {
		if e.Weight < minWeight {
			continue
		}
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

// OverlayMesh enriches MeshNodes with data from existing analysis passes.
// roles: component name → HEXA role string (from clinic/hexa classification).
// Stability, choke points, and trust zones are computed from mesh edges.
func (m *Mesh) OverlayMesh(roles map[string]string) {
	// Compute fan-in / fan-out per node from edges.
	fanIn := make(map[string]int)
	fanOut := make(map[string]int)
	for _, e := range m.Edges {
		fanOut[e.SourceFQN]++
		fanIn[e.TargetFQN]++
	}

	// Choke point overlay (betweenness centrality, parallelized).
	centrality := graph.BetweennessCentrality(m.Edges)

	for key, node := range m.Nodes {
		// HEXA role overlay (component level).
		if node.Level == MeshComponent {
			if role, ok := roles[key]; ok {
				node.Role = role
			}
		}

		// Stability overlay.
		fi := fanIn[key]
		fo := fanOut[key]
		if fi > 0 || fo > 0 {
			node.FanIn = fi
			node.FanOut = fo
			node.Instability = float64(fo) / float64(fi+fo)
		}

		// Choke score overlay.
		if score, ok := centrality[key]; ok && score > 0 {
			node.ChokeScore = score
		}

		m.Nodes[key] = node
	}
}

// Circuits returns groups of symbols with bidirectional edges (A→B and B→A).
// minWeight filters out low-weight edges. Returns circuit groups as [][]string.
func (m *Mesh) Circuits(minWeight float64) [][]string {
	// Build directed adjacency with weight filter.
	edges := make(map[string]map[string]bool)
	for _, e := range m.Edges {
		if e.Weight < minWeight {
			continue
		}
		if edges[e.SourceFQN] == nil {
			edges[e.SourceFQN] = make(map[string]bool)
		}
		edges[e.SourceFQN][e.TargetFQN] = true
	}

	// Find bidirectional pairs.
	seen := make(map[string]bool)
	var circuits [][]string
	circuitID := 1
	for a, targets := range edges {
		for b := range targets {
			if a >= b { // avoid duplicate pairs
				continue
			}
			if edges[b] != nil && edges[b][a] {
				key := a + "|" + b
				if seen[key] {
					continue
				}
				seen[key] = true
				circuits = append(circuits, []string{a, b})

				// Tag nodes with circuit ID.
				if na, ok := m.Nodes[a]; ok {
					na.CircuitID = circuitID
					m.Nodes[a] = na
				}
				if nb, ok := m.Nodes[b]; ok {
					nb.CircuitID = circuitID
					m.Nodes[b] = nb
				}
				circuitID++
			}
		}
	}
	return circuits
}

func appendUnique(slice []string, s string) []string {
	for _, v := range slice {
		if v == s {
			return slice
		}
	}
	return append(slice, s)
}
