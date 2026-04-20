package oculus_test

import (
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
)

func TestDiffSymbolGraphs_Added(t *testing.T) {
	before := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{{Name: "A", Package: "pkg"}},
	}
	after := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "A", Package: "pkg"},
			{Name: "B", Package: "pkg"},
		},
	}
	d := oculus.DiffSymbolGraphs(before, after)
	if d == nil {
		t.Fatal("expected non-nil SymbolDiff")
	}
	if len(d.Added) != 1 || d.Added[0] != "pkg.B" {
		t.Errorf("Added = %v, want [pkg.B]", d.Added)
	}
	if len(d.Removed) != 0 {
		t.Errorf("Removed = %v, want empty", d.Removed)
	}
	if len(d.Modified) != 0 {
		t.Errorf("Modified = %v, want empty", d.Modified)
	}
}

func TestDiffSymbolGraphs_Removed(t *testing.T) {
	before := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "A", Package: "pkg"},
			{Name: "B", Package: "pkg"},
		},
	}
	after := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{{Name: "A", Package: "pkg"}},
	}
	d := oculus.DiffSymbolGraphs(before, after)
	if d == nil {
		t.Fatal("expected non-nil SymbolDiff")
	}
	if len(d.Removed) != 1 || d.Removed[0] != "pkg.B" {
		t.Errorf("Removed = %v, want [pkg.B]", d.Removed)
	}
	if len(d.Added) != 0 {
		t.Errorf("Added = %v, want empty", d.Added)
	}
}

func TestDiffSymbolGraphs_Modified(t *testing.T) {
	before := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{{Name: "A", Package: "pkg", Signature: "func() error"}},
	}
	after := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{{Name: "A", Package: "pkg", Signature: "func(ctx context.Context) error"}},
	}
	d := oculus.DiffSymbolGraphs(before, after)
	if d == nil {
		t.Fatal("expected non-nil SymbolDiff")
	}
	if len(d.Modified) != 1 || d.Modified[0] != "pkg.A" {
		t.Errorf("Modified = %v, want [pkg.A]", d.Modified)
	}
}

func TestDiffSymbolGraphs_Identical(t *testing.T) {
	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "A", Package: "pkg", Signature: "func()"},
			{Name: "B", Package: "pkg", Signature: "func()"},
		},
	}
	d := oculus.DiffSymbolGraphs(sg, sg)
	if d == nil {
		t.Fatal("expected non-nil SymbolDiff")
	}
	if len(d.Added) != 0 {
		t.Errorf("Added = %v, want empty", d.Added)
	}
	if len(d.Removed) != 0 {
		t.Errorf("Removed = %v, want empty", d.Removed)
	}
	if len(d.Modified) != 0 {
		t.Errorf("Modified = %v, want empty", d.Modified)
	}
}
