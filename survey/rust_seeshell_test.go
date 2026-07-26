package survey_test

import (
	"testing"

	"github.com/dpopsuev/oculus/v3/internal/testfixture"
	"github.com/dpopsuev/oculus/v3/survey"
)

func TestRustScanRepositoryFixture_FileLevel(t *testing.T) {
	root := testfixture.Repository(t, "rust")
	scanner := &survey.RustScanner{Granularity: survey.FileLevel}
	project, err := scanner.Scan(root)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(project.Namespaces) < 2 {
		t.Fatalf("namespaces=%d, want at least 2", len(project.Namespaces))
	}
	if len(project.DependencyGraph.Edges) == 0 {
		t.Fatal("expected dependency edges")
	}
}
