package constraint

import (
	"fmt"
	"testing"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/arch"
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

func BenchmarkComputeInterfaceMetrics(b *testing.B) {
	classes := make([]oculus.ClassInfo, 20)
	var impls []oculus.ImplEdge
	for i := range 20 {
		name := fmt.Sprintf("Type_%d", i)
		kind := "struct"
		if i%3 == 0 {
			kind = "interface"
		}
		classes[i] = oculus.ClassInfo{Name: name, Kind: kind, Package: "pkg"}
		if i > 0 && i%3 != 0 {
			impls = append(impls, oculus.ImplEdge{From: name, To: fmt.Sprintf("Type_%d", (i/3)*3), Kind: "implements"})
		}
	}
	b.ResetTimer()
	for range b.N {
		ComputeInterfaceMetrics(classes, impls)
	}
}
