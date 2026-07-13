package survey_test

import (
	"os"
	"testing"

	"github.com/dpopsuev/oculus/v3/survey"
)

// TestRustScanSeeshellFileLevel is an optional regression against the Seeshell
// single-crate tree when present on the machine that filed the bug.
func TestRustScanSeeshellFileLevel(t *testing.T) {
	const root = "/home/dpopsuev/Projects/seeshell"
	if _, err := os.Stat(root); err != nil {
		t.Skip("Seeshell checkout not present")
	}
	sc := &survey.RustScanner{Granularity: survey.FileLevel}
	proj, err := sc.Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.Namespaces) < 10 {
		t.Fatalf("namespaces=%d want >=10", len(proj.Namespaces))
	}
	if len(proj.DependencyGraph.Edges) < 5 {
		t.Fatalf("edges=%d want >=5", len(proj.DependencyGraph.Edges))
	}
	t.Logf("namespaces=%d edges=%d", len(proj.Namespaces), len(proj.DependencyGraph.Edges))
}
