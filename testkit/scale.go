package testkit

import (
	"fmt"
	"math/rand"

	"github.com/dpopsuev/oculus"
)

// ScaleTier defines a fixture complexity level for benchmarks.
type ScaleTier struct {
	Name       string
	Components int
	Edges      int
}

// Standard tiers for benchmark scaling analysis.
var (
	TierSmall  = ScaleTier{Name: "small", Components: 10, Edges: 20}
	TierMedium = ScaleTier{Name: "medium", Components: 50, Edges: 150}
	TierLarge  = ScaleTier{Name: "large", Components: 200, Edges: 1000}
	TierXL     = ScaleTier{Name: "xl", Components: 500, Edges: 5000}
)

const symbolsPerComponent = 5

// GenerateComponentNames returns component name strings for the tier.
func GenerateComponentNames(tier ScaleTier) []string {
	names := make([]string, tier.Components)
	for i := range names {
		names[i] = fmt.Sprintf("pkg/comp_%d", i)
	}
	return names
}

// GenerateSymbolGraph creates a synthetic SymbolGraph with the given tier's
// complexity. Each component has ~5 symbols. Edges connect symbols across
// components with power-law distribution (some nodes are hubs).
func GenerateSymbolGraph(tier ScaleTier) *oculus.SymbolGraph {
	rng := rand.New(rand.NewSource(42)) // deterministic for reproducible benchmarks

	components := GenerateComponentNames(tier)
	totalSymbols := tier.Components * symbolsPerComponent

	// Generate nodes
	nodes := make([]oculus.Symbol, 0, totalSymbols)
	symbolFQNs := make([]string, 0, totalSymbols)
	for _, comp := range components {
		for j := 0; j < symbolsPerComponent; j++ {
			name := fmt.Sprintf("Func_%d", j)
			fqn := comp + "." + name
			kind := "function"
			if j == 0 {
				kind = "struct"
			}
			nodes = append(nodes, oculus.Symbol{
				Name:     name,
				Package:  comp,
				Kind:     kind,
				Exported: j < 3, // first 3 exported
			})
			symbolFQNs = append(symbolFQNs, fqn)
		}
	}

	// Generate edges with power-law distribution
	// ~30% cross-component, ~70% same-component
	edges := make([]oculus.SymbolEdge, 0, tier.Edges)
	seen := make(map[[2]string]bool)
	for len(edges) < tier.Edges {
		srcIdx := rng.Intn(totalSymbols)
		var tgtIdx int
		if rng.Float64() < 0.3 {
			// Cross-component: pick any symbol
			tgtIdx = rng.Intn(totalSymbols)
		} else {
			// Same-component: pick within same component
			compIdx := srcIdx / symbolsPerComponent
			base := compIdx * symbolsPerComponent
			tgtIdx = base + rng.Intn(symbolsPerComponent)
		}
		if srcIdx == tgtIdx {
			continue
		}
		key := [2]string{symbolFQNs[srcIdx], symbolFQNs[tgtIdx]}
		if seen[key] {
			continue
		}
		seen[key] = true
		edges = append(edges, oculus.SymbolEdge{
			SourceFQN: symbolFQNs[srcIdx],
			TargetFQN: symbolFQNs[tgtIdx],
			Kind:      "call",
			Weight:    1.0,
		})
	}

	return &oculus.SymbolGraph{Nodes: nodes, Edges: edges}
}
