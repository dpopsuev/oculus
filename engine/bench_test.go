package engine

import (
	"context"
	"testing"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/internal/testfixture"
)

func benchRoot(b *testing.B) string {
	b.Helper()
	return testfixture.Repository(b, "go")
}

func BenchmarkEngine_ScanProject(b *testing.B) {
	root := benchRoot(b)
	eng := New(&mockStore{headSHA: "bench"}, []string{root})

	b.ResetTimer()
	for range b.N {
		result, err := eng.ScanProject(context.Background(), root, ScanOpts{Intent: "architecture"})
		if err != nil {
			b.Fatal(err)
		}
		if len(result.Report.Architecture.Services) == 0 {
			b.Fatal("no services")
		}
	}
}

func BenchmarkEngine_GetSymbolGraph(b *testing.B) {
	root := benchRoot(b)
	eng := New(&mockStore{headSHA: "bench"}, []string{root})

	b.ResetTimer()
	for range b.N {
		sg, err := eng.GetSymbolGraph(context.Background(), root)
		if err != nil {
			b.Fatal(err)
		}
		if len(sg.Nodes) == 0 {
			b.Fatal("no nodes")
		}
	}
}

func BenchmarkEngine_GetMesh(b *testing.B) {
	root := benchRoot(b)
	eng := New(&mockStore{headSHA: "bench"}, []string{root})

	b.ResetTimer()
	for range b.N {
		mesh, err := eng.GetMesh(context.Background(), root)
		if err != nil {
			b.Fatal(err)
		}
		if len(mesh.Nodes) == 0 {
			b.Fatal("no nodes")
		}
	}
}

func BenchmarkEngine_GetHexaValidation(b *testing.B) {
	root := benchRoot(b)
	eng := New(&mockStore{headSHA: "bench"}, []string{root})

	b.ResetTimer()
	for range b.N {
		report, err := eng.GetHexaValidation(context.Background(), root)
		if err != nil {
			b.Fatal(err)
		}
		if len(report.Classification) == 0 {
			b.Fatal("no classification")
		}
	}
}

// BenchmarkEngine_OverlayMesh benchmarks the full mesh pipeline:
// scan → symbol graph → build mesh → overlay.
func BenchmarkEngine_OverlayMesh(b *testing.B) {
	root := benchRoot(b)
	eng := New(&mockStore{headSHA: "bench"}, []string{root})

	b.ResetTimer()
	for range b.N {
		sg, err := eng.GetSymbolGraph(context.Background(), root)
		if err != nil {
			b.Fatal(err)
		}
		scanResult, err := eng.ScanProject(context.Background(), root, ScanOpts{Intent: "architecture"})
		if err != nil {
			b.Fatal(err)
		}
		var names []string
		for _, svc := range scanResult.Report.Architecture.Services {
			names = append(names, svc.Name)
		}
		mesh := oculus.BuildMesh(sg, names)

		hexaReport, _ := eng.GetHexaValidation(context.Background(), root)
		roles := make(map[string]string)
		if hexaReport != nil {
			for _, c := range hexaReport.Classification {
				roles[c.Name] = string(c.Role)
			}
		}
		mesh.OverlayMesh(roles)
		mesh.Circuits(0.3)

		if len(mesh.Nodes) == 0 {
			b.Fatal("no nodes")
		}
	}
}
