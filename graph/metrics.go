package graph

import (
	"math/rand"
	"runtime"
	"sync"
)

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

// BetweennessCentrality computes betweenness centrality for all nodes using
// parallel BFS shortest paths (Brandes' algorithm). Each source node's BFS
// runs in its own goroutine — embarrassingly parallel since BFS reads shared
// adjacency and writes to local delta maps. Returns normalized scores (0-1).
func BetweennessCentrality[E Edge](edges []E) map[string]float64 {
	// Build undirected adjacency (read-only during parallel BFS).
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

	nodeList := make([]string, 0, len(nodes))
	for n := range nodes {
		nodeList = append(nodeList, n)
	}

	// For large graphs, sample source nodes instead of using all.
	// Full Brandes' is O(V×E) — at 2000 nodes it's 12s even parallelized.
	// Sampling 200 sources gives statistically accurate top-k rankings.
	const maxSources = 200
	sources := nodeList
	if len(sources) > maxSources {
		rng := rand.New(rand.NewSource(42)) // deterministic for reproducibility
		rng.Shuffle(len(sources), func(i, j int) { sources[i], sources[j] = sources[j], sources[i] })
		sources = sources[:maxSources]
	}

	// Parallel Brandes': each goroutine runs BFS from a subset of sources,
	// accumulates local scores, then we merge.
	workers := runtime.GOMAXPROCS(0)
	if workers > len(sources) {
		workers = len(sources)
	}
	if workers < 1 {
		workers = 1
	}

	// Partition source nodes across workers.
	partials := make([]map[string]float64, workers)
	var wg sync.WaitGroup

	chunkSize := (len(sources) + workers - 1) / workers
	for w := range workers {
		start := w * chunkSize
		end := start + chunkSize
		if end > len(sources) {
			end = len(sources)
		}
		if start >= end {
			continue
		}

		wg.Add(1)
		go func(workerID int, srcs []string) {
			defer wg.Done()
			local := make(map[string]float64, len(nodes))

			for _, src := range srcs {
				brandesBFS(src, nodeList, adj, local)
			}

			partials[workerID] = local
		}(w, sources[start:end])
	}
	wg.Wait()

	// Merge partial scores.
	centrality := make(map[string]float64, len(nodes))
	for _, partial := range partials {
		for k, v := range partial {
			centrality[k] += v
		}
	}

	// Normalize: scale by (V/sampled) to approximate full scores,
	// then by (n-1)*(n-2) for undirected graph normalization.
	n := float64(len(nodeList))
	sampled := float64(len(sources))
	if n > 2 && sampled > 0 {
		norm := (n - 1) * (n - 2) * (sampled / n)
		for k := range centrality {
			centrality[k] /= norm
		}
	}

	return centrality
}

// brandesBFS runs one iteration of Brandes' algorithm from a single source node.
// Reads adj (shared, read-only). Writes to local (per-goroutine, no lock needed).
func brandesBFS(src string, nodeList []string, adj AdjMap, local map[string]float64) {
	stack := make([]string, 0, len(nodeList))
	pred := make(map[string][]string, len(nodeList))
	sigma := make(map[string]float64, len(nodeList))
	dist := make(map[string]int, len(nodeList))
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

	delta := make(map[string]float64, len(stack))
	for i := len(stack) - 1; i >= 0; i-- {
		w := stack[i]
		for _, v := range pred[w] {
			delta[v] += (sigma[v] / sigma[w]) * (1 + delta[w])
		}
		if w != src {
			local[w] += delta[w]
		}
	}
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
