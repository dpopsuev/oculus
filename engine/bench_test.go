package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus"
)

// benchRoot returns the path to the oculus repo root for self-scan benchmarks.
func benchRoot(b *testing.B) string {
	b.Helper()
	// Engine tests run from engine/ — go up one level.
	dir, err := filepath.Abs("..")
	if err != nil {
		b.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		b.Skip("oculus root not found")
	}
	return dir
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
