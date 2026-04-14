package hexa

import (
	"fmt"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
)

func benchData(n int) ([]arch.ArchService, []arch.ArchEdge) {
	services := make([]arch.ArchService, n)
	var edges []arch.ArchEdge
	for i := range n {
		name := fmt.Sprintf("component_%d", i)
		services[i] = arch.ArchService{Name: name, LOC: 100 + i*10}
		if i > 0 {
			edges = append(edges, arch.ArchEdge{From: name, To: fmt.Sprintf("component_%d", i-1)})
		}
	}
	return services, edges
}

func BenchmarkComputeHexaClassification(b *testing.B) {
	services, edges := benchData(35)
	b.ResetTimer()
	for range b.N {
		ComputeHexaClassification(services, edges, nil)
	}
}

func BenchmarkComputeHexaViolations(b *testing.B) {
	services, edges := benchData(35)
	b.ResetTimer()
	for range b.N {
		ComputeHexaViolations(services, edges, nil)
	}
}
