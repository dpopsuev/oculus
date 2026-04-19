package book

import (
	"sort"
	"strings"
)

const defaultTopK = 3

// Query finds relevant knowledge entries for the given keywords.
// 1. Jaccard keyword match → top-K entry nodes
// 2. BFS from entry nodes, hops steps through edges
// 3. Load content for returned nodes
// hops=0 returns only the keyword-matched entries.
func (g *BookGraph) Query(keywords []string, hops int) *BookResult {
	tokens := tokenize(keywords)
	if len(tokens) == 0 {
		return &BookResult{}
	}

	type scored struct {
		id    string
		score float64
	}

	var hits []scored
	for id, node := range g.Nodes {
		s := jaccardScore(tokens, node.Keywords)
		if s > 0 {
			hits = append(hits, scored{id, s})
		}
	}
	if len(hits) == 0 {
		return &BookResult{}
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].score > hits[j].score })
	if len(hits) > defaultTopK {
		hits = hits[:defaultTopK]
	}

	roots := make([]string, len(hits))
	for i, h := range hits {
		roots[i] = h.id
	}

	included := make(map[string]bool, len(roots))
	for _, r := range roots {
		included[r] = true
	}

	if hops > 0 {
		adj := g.buildAdj()
		frontier := make(map[string]bool)
		for _, r := range roots {
			frontier[r] = true
		}
		for hop := 0; hop < hops; hop++ {
			next := make(map[string]bool)
			for node := range frontier {
				for neighbor := range adj[node] {
					if !included[neighbor] {
						included[neighbor] = true
						next[neighbor] = true
					}
				}
			}
			frontier = next
		}
	}

	var entries []BookNode
	for id := range included {
		if node, ok := g.Nodes[id]; ok {
			if node.Content == "" && g.fsys != nil {
				g.LoadContent(id)
				node = g.Nodes[id]
			}
			entries = append(entries, node)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })

	var edges []BookEdge
	for _, e := range g.Edges {
		if included[e.From] && included[e.To] {
			edges = append(edges, e)
		}
	}

	return &BookResult{
		Entries: entries,
		Edges:   edges,
		Roots:   roots,
	}
}

// buildAdj creates an undirected adjacency map from edges.
func (g *BookGraph) buildAdj() map[string]map[string]bool {
	adj := make(map[string]map[string]bool, len(g.Nodes))
	for _, e := range g.Edges {
		if adj[e.From] == nil {
			adj[e.From] = make(map[string]bool)
		}
		if adj[e.To] == nil {
			adj[e.To] = make(map[string]bool)
		}
		adj[e.From][e.To] = true
		adj[e.To][e.From] = true
	}
	return adj
}

// tokenize lowercases and splits keywords into individual tokens.
func tokenize(keywords []string) []string {
	var tokens []string
	for _, kw := range keywords {
		for _, word := range strings.Fields(strings.ToLower(kw)) {
			word = strings.Trim(word, ".,;:!?()[]{}\"'")
			if len(word) > 1 {
				tokens = append(tokens, word)
			}
		}
	}
	return tokens
}

// jaccardScore returns the overlap between query tokens and entry keywords.
func jaccardScore(queryTokens, keywords []string) float64 {
	if len(queryTokens) == 0 || len(keywords) == 0 {
		return 0
	}
	kwSet := make(map[string]bool, len(keywords))
	for _, kw := range keywords {
		kwSet[strings.ToLower(kw)] = true
	}

	matches := 0
	for _, qt := range queryTokens {
		if kwSet[qt] {
			matches++
			continue
		}
		for kw := range kwSet {
			if strings.Contains(kw, qt) || strings.Contains(qt, kw) {
				matches++
				break
			}
		}
	}

	union := len(queryTokens) + len(kwSet) - matches
	if union == 0 {
		return 0
	}
	return float64(matches) / float64(union)
}
