package graph

// Cohesion computes the edge density within a group of nodes.
// Returns (actual edges) / (possible edges), where possible = n*(n-1)/2
// for an undirected view. Returns 1.0 for groups with fewer than 2 nodes.
func Cohesion(group []string, adj AdjMap) float64 {
	n := len(group)
	if n < 2 {
		return 1.0
	}
	possibleEdges := n * (n - 1) / 2
	groupSet := make(NodeSet, n)
	for _, s := range group {
		groupSet[s] = true
	}
	counted := make(NodeSet)
	actualEdges := 0
	for _, s := range group {
		for neighbor := range adj[s] {
			if !groupSet[neighbor] {
				continue
			}
			fwd := s + "|" + neighbor
			rev := neighbor + "|" + s
			if !counted[fwd] && !counted[rev] {
				counted[fwd] = true
				actualEdges++
			}
		}
	}
	return float64(actualEdges) / float64(possibleEdges)
}

// BetweennessCentrality computes approximate betweenness centrality for all
// nodes using BFS shortest paths. High-centrality nodes are choke points —
// many shortest paths pass through them. Returns normalized scores (0-1).
func BetweennessCentrality[E Edge](edges []E) map[string]float64 {
	// Build undirected adjacency.
	adj := make(AdjMap)
	nodes := make(NodeSet)
	for _, e := range edges {
		s, t := e.Source(), e.Target()
		nodes[s] = true
		nodes[t] = true
		if adj[s] == nil {
			adj[s] = make(map[string]bool)
		}
		if adj[t] == nil {
			adj[t] = make(map[string]bool)
		}
		adj[s][t] = true
		adj[t][s] = true
	}

	centrality := make(map[string]float64, len(nodes))
	nodeList := make([]string, 0, len(nodes))
	for n := range nodes {
		nodeList = append(nodeList, n)
	}

	// Brandes' algorithm: BFS from each node, accumulate dependency scores.
	for _, src := range nodeList {
		// BFS
		stack := make([]string, 0)
		pred := make(map[string][]string)
		sigma := make(map[string]float64) // number of shortest paths
		dist := make(map[string]int)
		for _, n := range nodeList {
			dist[n] = -1
		}
		sigma[src] = 1
		dist[src] = 0
		queue := []string{src}

		for len(queue) > 0 {
			v := queue[0]
			queue = queue[1:]
			stack = append(stack, v)
			for w := range adj[v] {
				if dist[w] < 0 {
					dist[w] = dist[v] + 1
					queue = append(queue, w)
				}
				if dist[w] == dist[v]+1 {
					sigma[w] += sigma[v]
					pred[w] = append(pred[w], v)
				}
			}
		}

		// Accumulate dependencies.
		delta := make(map[string]float64)
		for i := len(stack) - 1; i >= 0; i-- {
			w := stack[i]
			for _, v := range pred[w] {
				delta[v] += (sigma[v] / sigma[w]) * (1 + delta[w])
			}
			if w != src {
				centrality[w] += delta[w]
			}
		}
	}

	// Normalize by (n-1)*(n-2) for undirected graphs.
	n := float64(len(nodeList))
	if n > 2 {
		norm := (n - 1) * (n - 2)
		for k := range centrality {
			centrality[k] /= norm
		}
	}

	return centrality
}

// BFSGroup performs BFS from start through undirected adjacency, returning
// all reachable nodes as a group. Marks visited nodes in the provided set
// to avoid re-processing across multiple calls.
func BFSGroup(start string, adj AdjMap, visited NodeSet) []string {
	queue := []string{start}
	visited[start] = true
	var group []string
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		group = append(group, cur)
		for neighbor := range adj[cur] {
			if !visited[neighbor] {
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}
	return group
}
