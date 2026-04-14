package testkit

import (
	"fmt"
	"testing"

	"github.com/dpopsuev/oculus/v3"
)

func BenchmarkFullPipeline_ByScale(b *testing.B) {
	tiers := []ScaleTier{
		TierSmall,
		TierMedium,
		TierLarge,
		TierXL,
		TierK8s,
		TierKernel,
		TierChrome,
	}

	for _, tier := range tiers {
		sg := GenerateSymbolGraph(tier)
		names := GenerateComponentNames(tier)

		b.Run(fmt.Sprintf("%s_%dc_%de", tier.Name, tier.Components, len(sg.Edges)), func(b *testing.B) {
			for range b.N {
				mesh := oculus.BuildMesh(sg, names)
				mesh.OverlayMesh(nil)
				mesh.Circuits(0.3)
			}
		})
	}
}
