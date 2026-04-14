package oculus_test

import (
	"testing"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/testkit"
)

var tiers = []testkit.ScaleTier{
	testkit.TierSmall, testkit.TierMedium, testkit.TierLarge, testkit.TierXL,
}

func BenchmarkBuildMesh(b *testing.B) {
	for _, tier := range tiers {
		sg := testkit.GenerateSymbolGraph(tier)
		names := testkit.GenerateComponentNames(tier)
		b.Run(tier.Name, func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(len(sg.Edges)), "edges")
			for b.Loop() {
				oculus.BuildMesh(sg, names)
			}
		})
	}
}

func BenchmarkMesh_Boundaries(b *testing.B) {
	for _, tier := range tiers {
		sg := testkit.GenerateSymbolGraph(tier)
		names := testkit.GenerateComponentNames(tier)
		mesh := oculus.BuildMesh(sg, names)
		b.Run(tier.Name, func(b *testing.B) {
			b.ReportAllocs()
			b.ReportMetric(float64(len(sg.Edges)), "edges")
			for b.Loop() {
				mesh.BoundariesMinWeight(0.5)
			}
		})
	}
}

func BenchmarkMesh_Aggregate(b *testing.B) {
	for _, tier := range tiers {
		sg := testkit.GenerateSymbolGraph(tier)
		names := testkit.GenerateComponentNames(tier)
		mesh := oculus.BuildMesh(sg, names)
		b.Run(tier.Name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				mesh.Aggregate(oculus.MeshComponent)
			}
		})
	}
}

func BenchmarkMergeSymbolGraph(b *testing.B) {
	for _, tier := range tiers {
		sg := testkit.GenerateSymbolGraph(tier)
		// Simulate merge from call graph
		cg := &oculus.CallGraph{Edges: make([]oculus.CallEdge, len(sg.Edges))}
		for i, e := range sg.Edges {
			cg.Edges[i] = oculus.CallEdge{Caller: e.SourceFQN, Callee: e.TargetFQN}
		}
		b.Run(tier.Name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				oculus.MergeSymbolGraph(cg, nil, nil, nil)
			}
		})
	}
}
