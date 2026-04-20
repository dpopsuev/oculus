package oculus

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dpopsuev/oculus/v3/graph"
)

// Probe returns all vitals for a single symbol. Zero traversal.
func Probe(sg *SymbolGraph, symbol string) *ProbeResult {
	if sg == nil {
		return nil
	}

	idx := buildNodeIndex(sg)
	symbol = resolveSymbol(sg, symbol)
	sym, ok := idx[symbol]
	if !ok {
		return nil
	}

	fanIn := graph.FanIn(sg.Edges)
	fanOut := graph.FanOut(sg.Edges)
	fi := fanIn[symbol]
	fo := fanOut[symbol]

	inst := 0.0
	if fi+fo > 0 {
		inst = float64(fo) / float64(fi+fo)
	}

	var crossPkg int
	var boundaries []string
	outSet := make(map[string]bool)
	inSet := make(map[string]bool)
	for _, e := range sg.Edges {
		if e.SourceFQN == symbol {
			targetPkg := pkgOf(e.TargetFQN)
			if targetPkg != sym.Package {
				crossPkg++
			}
			outSet[e.TargetFQN] = true
			boundaries = appendUniq(boundaries, targetPkg)
		}
		if e.TargetFQN == symbol {
			inSet[e.SourceFQN] = true
		}
	}

	circuits := 0
	for out := range outSet {
		if inSet[out] {
			circuits++
		}
	}

	return &ProbeResult{
		FQN:         symbol,
		Package:     sym.Package,
		File:        sym.File,
		Line:        sym.Line,
		EndLine:     sym.EndLine,
		Kind:        sym.Kind,
		Exported:    sym.Exported,
		Params:      sym.ParamTypes,
		Returns:     sym.ReturnTypes,
		FanIn:       fi,
		FanOut:      fo,
		Instability: inst,
		CrossPkg:    crossPkg,
		Circuits:    circuits,
		Boundaries:  boundaries,
	}
}

// TraceScenario traces upstream to entry points and downstream to leaves.
func TraceScenario(sg *SymbolGraph, symbol string, maxDepth int, stress bool, topN int) *ScenarioResult {
	if sg == nil || maxDepth <= 0 {
		return nil
	}

	idx := buildNodeIndex(sg)
	symbol = resolveSymbol(sg, symbol)
	if _, ok := idx[symbol]; !ok {
		return nil
	}

	fwd := buildFwdAdj(sg.Edges)
	rev := buildRevAdj(sg.Edges)

	downstream := bfsDirected(symbol, fwd, maxDepth)
	upstream := bfsDirected(symbol, rev, maxDepth)

	allNodes := make(map[string]bool)
	allNodes[symbol] = true
	for fqn := range downstream {
		allNodes[fqn] = true
	}
	for fqn := range upstream {
		allNodes[fqn] = true
	}

	var downNodes []ScenarioNode
	for fqn, depth := range downstream {
		kind := "internal"
		if len(fwd[fqn]) == 0 {
			kind = "leaf"
		}
		n := ScenarioNode{FQN: fqn, Package: pkgOf(fqn), Depth: depth, Kind: kind}
		if stress {
			n.FanOut = len(fwd[fqn])
			n.DownstreamCount = countReachable(fqn, fwd)
		}
		downNodes = append(downNodes, n)
	}
	sort.Slice(downNodes, func(i, j int) bool { return downNodes[i].Depth < downNodes[j].Depth })
	if topN > 0 && len(downNodes) > topN {
		downNodes = downNodes[:topN]
	}

	var upNodes []ScenarioNode
	for fqn, depth := range upstream {
		kind := "internal"
		if len(rev[fqn]) == 0 {
			kind = "entry"
		}
		upNodes = append(upNodes, ScenarioNode{FQN: fqn, Package: pkgOf(fqn), Depth: -depth, Kind: kind})
	}
	sort.Slice(upNodes, func(i, j int) bool { return upNodes[i].Depth > upNodes[j].Depth })

	var edges []SymbolEdge
	for _, e := range sg.Edges {
		if allNodes[e.SourceFQN] && allNodes[e.TargetFQN] {
			edges = append(edges, e)
		}
	}

	return &ScenarioResult{
		Symbol:     symbol,
		Upstream:   upNodes,
		Downstream: downNodes,
		Edges:      edges,
	}
}

// FindConvergence finds where N symbols' downstream call trees overlap.
func FindConvergence(sg *SymbolGraph, symbols []string, topN int) *ConvergenceResult {
	if sg == nil || len(symbols) < 2 {
		return &ConvergenceResult{Symbols: symbols}
	}

	for i, sym := range symbols {
		symbols[i] = resolveSymbol(sg, sym)
	}

	fwd := buildFwdAdj(sg.Edges)

	reachable := make([]map[string]bool, len(symbols))
	for i, sym := range symbols {
		reachable[i] = bfsAll(sym, fwd)
		reachable[i][sym] = true
	}

	counts := make(map[string]int)
	sources := make(map[string][]string)
	for i, sym := range symbols {
		for fqn := range reachable[i] {
			if fqn == sym {
				continue
			}
			counts[fqn]++
			sources[fqn] = append(sources[fqn], sym)
		}
	}

	var nodes []ConvergenceNode
	nodeSet := make(map[string]bool)
	for fqn, c := range counts {
		if c >= 2 {
			nodes = append(nodes, ConvergenceNode{FQN: fqn, Converges: c, Sources: sources[fqn]})
			nodeSet[fqn] = true
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Converges != nodes[j].Converges {
			return nodes[i].Converges > nodes[j].Converges
		}
		return nodes[i].FQN < nodes[j].FQN
	})
	if topN > 0 && len(nodes) > topN {
		nodes = nodes[:topN]
	}

	var edges []SymbolEdge
	for _, e := range sg.Edges {
		if nodeSet[e.SourceFQN] && nodeSet[e.TargetFQN] {
			edges = append(edges, e)
		}
	}

	return &ConvergenceResult{Symbols: symbols, Nodes: nodes, Edges: edges}
}

// Isolate removes a symbol and reports what disconnects.
func Isolate(sg *SymbolGraph, symbol string) *IsolateResult {
	if sg == nil {
		return nil
	}

	allNodes := make(map[string]bool, len(sg.Nodes))
	for _, n := range sg.Nodes {
		allNodes[n.FQN()] = true
	}

	beforeComps := componentsWith(sg.Edges, allNodes)

	var filtered []SymbolEdge
	for _, e := range sg.Edges {
		if e.SourceFQN != symbol && e.TargetFQN != symbol {
			filtered = append(filtered, e)
		}
	}
	remaining := make(map[string]bool, len(allNodes))
	for fqn := range allNodes {
		if fqn != symbol {
			remaining[fqn] = true
		}
	}
	afterComps := componentsWith(filtered, remaining)

	return &IsolateResult{
		Symbol:           symbol,
		ComponentsBefore: beforeComps,
		ComponentsAfter:  afterComps,
	}
}

func componentsWith(edges []SymbolEdge, nodes map[string]bool) int {
	adj := make(map[string]map[string]bool, len(nodes))
	for fqn := range nodes {
		adj[fqn] = make(map[string]bool)
	}
	for _, e := range edges {
		if nodes[e.SourceFQN] && nodes[e.TargetFQN] {
			adj[e.SourceFQN][e.TargetFQN] = true
			adj[e.TargetFQN][e.SourceFQN] = true
		}
	}

	visited := make(map[string]bool, len(nodes))
	count := 0
	for fqn := range nodes {
		if visited[fqn] {
			continue
		}
		count++
		queue := []string{fqn}
		visited[fqn] = true
		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			for nb := range adj[curr] {
				if !visited[nb] {
					visited[nb] = true
					queue = append(queue, nb)
				}
			}
		}
	}
	return count
}

// DetectEntryPoints finds all exported nodes with zero incoming call edges.
// Isolated nodes (no outgoing edges either) are excluded — they are islands,
// not entry points.
func DetectEntryPoints(sg *SymbolGraph) []string {
	if sg == nil {
		return nil
	}

	rev := make(map[string]bool)
	fwd := make(map[string]bool)
	for _, e := range sg.Edges {
		if e.Kind == "call" {
			rev[e.TargetFQN] = true
			fwd[e.SourceFQN] = true
		}
	}

	var entries []string
	for _, n := range sg.Nodes {
		fqn := n.FQN()
		if n.Exported && !rev[fqn] && fwd[fqn] {
			entries = append(entries, fqn)
		}
	}
	sort.Strings(entries)
	return entries
}

// FindIslands identifies symbols unreachable from the given entry points.
func FindIslands(sg *SymbolGraph, entryPoints []string) *IslandResult {
	if sg == nil {
		return nil
	}

	if len(entryPoints) == 0 {
		entryPoints = DetectEntryPoints(sg)
	}

	allNodes := make(map[string]bool, len(sg.Nodes))
	for _, n := range sg.Nodes {
		allNodes[n.FQN()] = true
	}

	fwd := buildFwdAdj(sg.Edges)

	reachable := make(map[string]bool)
	for _, entry := range entryPoints {
		for fqn := range bfsAll(entry, fwd) {
			reachable[fqn] = true
		}
	}

	var unreachable []string
	for fqn := range allNodes {
		if !reachable[fqn] {
			unreachable = append(unreachable, fqn)
		}
	}
	sort.Strings(unreachable)

	return &IslandResult{
		EntryPoints: entryPoints,
		Reachable:   len(reachable),
		Total:       len(allNodes),
		Unreachable: unreachable,
	}
}

// --- helpers ---

func buildNodeIndex(sg *SymbolGraph) map[string]Symbol {
	idx := make(map[string]Symbol, len(sg.Nodes))
	for _, n := range sg.Nodes {
		idx[n.FQN()] = n
	}
	return idx
}

func buildFwdAdj(edges []SymbolEdge) map[string][]string {
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.SourceFQN] = append(adj[e.SourceFQN], e.TargetFQN)
	}
	return adj
}

func buildRevAdj(edges []SymbolEdge) map[string][]string {
	adj := make(map[string][]string)
	for _, e := range edges {
		adj[e.TargetFQN] = append(adj[e.TargetFQN], e.SourceFQN)
	}
	return adj
}

func bfsDirected(start string, adj map[string][]string, maxDepth int) map[string]int {
	visited := make(map[string]int)
	frontier := []string{start}
	depth := 0
	for len(frontier) > 0 && depth < maxDepth {
		depth++
		var next []string
		for _, n := range frontier {
			for _, nb := range adj[n] {
				if _, seen := visited[nb]; !seen && nb != start {
					visited[nb] = depth
					next = append(next, nb)
				}
			}
		}
		frontier = next
	}
	return visited
}

func bfsAll(start string, adj map[string][]string) map[string]bool {
	visited := map[string]bool{start: true}
	frontier := []string{start}
	for len(frontier) > 0 {
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
	return visited
}

func countReachable(start string, adj map[string][]string) int {
	return len(bfsAll(start, adj)) - 1
}

func pkgOf(fqn string) string {
	if i := strings.LastIndex(fqn, "."); i >= 0 {
		return fqn[:i]
	}
	return fqn
}

func appendUniq(s []string, v string) []string {
	for _, x := range s {
		if x == v {
			return s
		}
	}
	return append(s, v)
}

// resolveSymbol resolves a partial symbol name to a full FQN.
// Tries exact match first, then suffix match, then substring match.
// Returns "" if no match is found.
func resolveSymbol(sg *SymbolGraph, partial string) string {
	if sg == nil || partial == "" {
		return partial
	}

	// 1. Exact FQN match.
	for _, n := range sg.Nodes {
		if n.FQN() == partial {
			return partial
		}
	}

	// 2. Suffix match (partial = "ScanAndBuild" matches "arch.ScanAndBuild").
	for _, n := range sg.Nodes {
		fqn := n.FQN()
		if strings.HasSuffix(fqn, "."+partial) {
			return fqn
		}
	}

	// 3. Substring match.
	for _, n := range sg.Nodes {
		fqn := n.FQN()
		if strings.Contains(fqn, partial) {
			return fqn
		}
	}

	return ""
}

// ExplainEdge reads the source file and extracts lines around the edge location.
func ExplainEdge(rootPath string, edge SymbolEdge, contextLines int) string {
	if edge.File == "" || edge.Line <= 0 {
		return ""
	}

	fpath := filepath.Join(rootPath, edge.File)
	f, err := os.Open(fpath)
	if err != nil {
		return ""
	}
	defer f.Close()

	startLine := edge.Line - contextLines
	if startLine < 1 {
		startLine = 1
	}
	endLine := edge.Line + contextLines

	scanner := bufio.NewScanner(f)
	lineNum := 0
	var buf strings.Builder
	for scanner.Scan() {
		lineNum++
		if lineNum > endLine {
			break
		}
		if lineNum >= startLine {
			fmt.Fprintf(&buf, "%d\t%s\n", lineNum, scanner.Text())
		}
	}
	return buf.String()
}
