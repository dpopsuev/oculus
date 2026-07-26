package engine

import (
	"context"
	"testing"

	"github.com/dpopsuev/oculus/v3/analyzer"
	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/clinic"
	"github.com/dpopsuev/oculus/v3/internal/testfixture"
)

func scanSyntheticRepository(t *testing.T) (string, *arch.ContextReport) {
	t.Helper()
	root := testfixture.Repository(t, "go")
	report, err := arch.ScanAndBuild(context.Background(), root, arch.ScanOpts{
		ExcludeTests: true,
		ChurnDays:    30,
	})
	if err != nil {
		t.Fatalf("ScanAndBuild: %v", err)
	}
	if len(report.Architecture.Services) == 0 {
		t.Fatal("scan returned no components")
	}
	return root, report
}

func TestSyntheticRepository_RoleAwareScanIsBounded(t *testing.T) {
	root, report := scanSyntheticRepository(t)
	typeAnalyzer := analyzer.NewFallback(root, nil)
	classes, err := typeAnalyzer.Classes(context.Background(), root)
	if err != nil {
		t.Fatalf("Classes: %v", err)
	}
	implementations, err := typeAnalyzer.Implements(context.Background(), root)
	if err != nil {
		t.Fatalf("Implements: %v", err)
	}

	classification := clinic.ComputeHexaClassification(report.Architecture.Services, report.Architecture.Edges, classes)
	withoutRoles := clinic.ComputeSOLIDScan(report.Architecture.Services, report.Architecture.Edges, classes, implementations, classification, root, nil, nil)
	roles := clinic.ResolveRoles(classification, nil)
	withRoles := clinic.ComputeSOLIDScan(report.Architecture.Services, report.Architecture.Edges, classes, implementations, classification, root, roles, nil)
	if len(withRoles.Violations) > len(withoutRoles.Violations)+4 {
		t.Errorf("role-aware violations = %d, baseline = %d", len(withRoles.Violations), len(withoutRoles.Violations))
	}
}

func TestSyntheticRepository_HasNoImportCycles(t *testing.T) {
	_, report := scanSyntheticRepository(t)
	if len(report.Cycles) != 0 {
		t.Fatalf("synthetic fixture has %d import cycles", len(report.Cycles))
	}
}
