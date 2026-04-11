package graph

import (
	"fmt"
	"testing"
)

func generateEdges(n int) []testEdge {
	edges := make([]testEdge, 0, n*2)
	for i := range n {
		edges = append(edges, testEdge{
			from: fmt.Sprintf("node_%d", i),
			to:   fmt.Sprintf("node_%d", (i+1)%n),
		})
		if i > 0 {
			edges = append(edges, testEdge{
				from: fmt.Sprintf("node_%d", i),
				to:   fmt.Sprintf("node_%d", i/2), // tree-like shortcuts
			})
		}
	}
	return edges
}

func BenchmarkFanIn(b *testing.B) {
	for _, n := range []int{100, 500, 2000} {
		edges := generateEdges(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for range b.N {
				FanIn(edges)
			}
		})
	}
}

func BenchmarkFanOut(b *testing.B) {
	for _, n := range []int{100, 500, 2000} {
		edges := generateEdges(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for range b.N {
				FanOut(edges)
			}
		})
	}
}

func BenchmarkConnectedComponents(b *testing.B) {
	for _, n := range []int{100, 500, 2000} {
		edges := generateEdges(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for range b.N {
				ConnectedComponents(edges)
			}
		})
	}
}

func BenchmarkBetweennessCentrality(b *testing.B) {
	for _, n := range []int{50, 100, 500} {
		edges := generateEdges(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			for range b.N {
				BetweennessCentrality(edges)
			}
		})
	}
}
