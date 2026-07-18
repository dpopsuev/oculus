package arch

import (
	"strings"
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
)

func TestCrateLikeSparseWarning(t *testing.T) {
	r := &ContextReport{
		ScanCore: ScanCore{Scanner: "composite"},
	}
	r.Architecture.Services = []oculus.ArchService{
		{Name: "crate_a"},
		{Name: "crate_b"},
		{Name: "pkg/ts"},
	}
	r.Architecture.Edges = []oculus.ArchEdge{
		{From: "pkg/ts", To: "pkg/ts"},
	}
	w := CrateLikeSparseWarning(r)
	if w == "" || !strings.Contains(w, "crate-like") {
		t.Fatalf("expected crate-like warning, got %q", w)
	}
	r.Architecture.Edges = append(r.Architecture.Edges, oculus.ArchEdge{From: "crate_a", To: "crate_b"})
	if CrateLikeSparseWarning(r) != "" {
		t.Fatal("warning should clear once crates are linked")
	}
}
