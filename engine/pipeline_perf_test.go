package engine

import (
	"context"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/analyzer"
	"github.com/dpopsuev/oculus/v3/internal/testfixture"
)

func TestPipelineStages_SyntheticRepository(t *testing.T) {
	root := testfixture.Repository(t, "go")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	deepAnalyzer := analyzer.CachedDeepFallback(root, nil)
	callGraph, err := deepAnalyzer.CallGraph(ctx, root, oculus.CallGraphOpts{Depth: oculus.DefaultCallGraphDepth})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	typeAnalyzer := analyzer.NewFallback(root, nil)
	classes, err := typeAnalyzer.Classes(ctx, root)
	if err != nil {
		t.Fatalf("Classes: %v", err)
	}
	implementations, err := typeAnalyzer.Implements(ctx, root)
	if err != nil {
		t.Fatalf("Implements: %v", err)
	}
	fieldReferences, err := typeAnalyzer.FieldRefs(ctx, root)
	if err != nil {
		t.Fatalf("FieldRefs: %v", err)
	}

	symbolGraph := oculus.MergeSymbolGraph(callGraph, classes, implementations, fieldReferences)
	report := oculus.DetectPipelines(symbolGraph, 2)
	if len(symbolGraph.Nodes) == 0 {
		t.Fatal("expected symbol nodes")
	}
	if report == nil || report.Summary == "" {
		t.Fatal("expected pipeline summary")
	}
}
