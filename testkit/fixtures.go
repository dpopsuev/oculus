package testkit

import oculus "github.com/dpopsuev/oculus/v3"

// FixtureGraph returns a test SymbolGraph with known topology:
//
//	A → B → C → D    (linear chain)
//	A → E → F        (fork from A)
//	G → B             (second caller of B)
//	D → E             (convergence: A and G both reach E via different paths)
//	H                 (isolated node, no edges)
func FixtureGraph() *oculus.SymbolGraph {
	return &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "A", Package: "pkg1", Kind: "function", Exported: true, File: "pkg1/a.go", Line: 10, EndLine: 20, ParamTypes: []string{"context.Context"}, ReturnTypes: []string{"error"}},
			{Name: "B", Package: "pkg1", Kind: "function", Exported: true, File: "pkg1/b.go", Line: 5, EndLine: 15},
			{Name: "C", Package: "pkg2", Kind: "function", Exported: true, File: "pkg2/c.go", Line: 1, EndLine: 10},
			{Name: "D", Package: "pkg2", Kind: "function", Exported: true, File: "pkg2/d.go", Line: 1, EndLine: 8},
			{Name: "E", Package: "pkg3", Kind: "function", Exported: true, File: "pkg3/e.go", Line: 1, EndLine: 12},
			{Name: "F", Package: "pkg3", Kind: "function", Exported: true, File: "pkg3/f.go", Line: 1, EndLine: 5},
			{Name: "G", Package: "pkg4", Kind: "function", Exported: true, File: "pkg4/g.go", Line: 1, EndLine: 6},
			{Name: "H", Package: "pkg5", Kind: "function", Exported: true, File: "pkg5/h.go", Line: 1, EndLine: 3},
		},
		Edges: []oculus.SymbolEdge{
			{SourceFQN: "pkg1.A", TargetFQN: "pkg1.B", Kind: "call", Weight: 1},
			{SourceFQN: "pkg1.B", TargetFQN: "pkg2.C", Kind: "call", Weight: 1},
			{SourceFQN: "pkg2.C", TargetFQN: "pkg2.D", Kind: "call", Weight: 1},
			{SourceFQN: "pkg1.A", TargetFQN: "pkg3.E", Kind: "call", Weight: 1},
			{SourceFQN: "pkg3.E", TargetFQN: "pkg3.F", Kind: "call", Weight: 1},
			{SourceFQN: "pkg4.G", TargetFQN: "pkg1.B", Kind: "call", Weight: 1},
			{SourceFQN: "pkg2.D", TargetFQN: "pkg3.E", Kind: "call", Weight: 1},
		},
	}
}
