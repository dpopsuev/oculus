package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/analyzer"
)

// TestPipelineStages_Dogfood profiles each stage of the DetectPipelines path
// independently on the Oculus codebase. Identifies which stage is the bottleneck.
func TestPipelineStages_Dogfood(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pipeline perf test in -short mode")
	}

	root := oculusRoot(t)
	ctx := context.Background()

	// Stage 1: CallGraph via DeepFallback
	da := analyzer.CachedDeepFallback(root, nil)
	start := time.Now()
	cg, err := da.CallGraph(ctx, root, oculus.CallGraphOpts{Depth: oculus.DefaultCallGraphDepth})
	callGraphDur := time.Since(start)
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	t.Logf("Stage 1 — CallGraph:       %8v  (%d edges, %d nodes)", callGraphDur, len(cg.Edges), len(cg.Nodes))

	// Stage 2: Classes via TypeAnalyzer fallback
	fa := analyzer.NewFallback(root, nil)
	start = time.Now()
	classes, _ := fa.Classes(ctx, root)
	classesDur := time.Since(start)
	t.Logf("Stage 2 — Classes:         %8v  (%d classes)", classesDur, len(classes))

	// Stage 3: Implements
	start = time.Now()
	impls, _ := fa.Implements(ctx, root)
	implsDur := time.Since(start)
	t.Logf("Stage 3 — Implements:      %8v  (%d edges)", implsDur, len(impls))

	// Stage 4: FieldRefs
	start = time.Now()
	refs, _ := fa.FieldRefs(ctx, root)
	fieldRefsDur := time.Since(start)
	t.Logf("Stage 4 — FieldRefs:       %8v  (%d refs)", fieldRefsDur, len(refs))

	// Stage 5: MergeSymbolGraph
	start = time.Now()
	sg := oculus.MergeSymbolGraph(cg, classes, impls, refs)
	mergeDur := time.Since(start)
	t.Logf("Stage 5 — MergeSymbolGraph:%8v  (%d nodes, %d edges)", mergeDur, len(sg.Nodes), len(sg.Edges))

	// Stage 6: DetectPipelines
	start = time.Now()
	report := oculus.DetectPipelines(sg, 2)
	pipelinesDur := time.Since(start)
	t.Logf("Stage 6 — DetectPipelines: %8v  (%d pipelines)", pipelinesDur, len(report.Pipelines))

	// Summary
	total := callGraphDur + classesDur + implsDur + fieldRefsDur + mergeDur + pipelinesDur
	t.Logf("---")
	t.Logf("Total:                     %8v", total)
	t.Logf("Bottleneck: %s", bottleneck(
		stage{"CallGraph", callGraphDur},
		stage{"Classes", classesDur},
		stage{"Implements", implsDur},
		stage{"FieldRefs", fieldRefsDur},
		stage{"Merge", mergeDur},
		stage{"Pipelines", pipelinesDur},
	))
}

// TestPipelineStages_Origami profiles each stage on the Origami repo.
// Skipped if Origami is not present on disk.
func TestPipelineStages_Origami(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping Origami perf test in -short mode")
	}

	origamiRoot := filepath.Join(os.Getenv("HOME"), "Workspace", "origami")
	if _, err := os.Stat(filepath.Join(origamiRoot, "go.mod")); err != nil {
		t.Skip("Origami repo not found at ~/Workspace/origami")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Stage 1: CallGraph
	da := analyzer.CachedDeepFallback(origamiRoot, nil)
	start := time.Now()
	cg, err := da.CallGraph(ctx, origamiRoot, oculus.CallGraphOpts{Depth: oculus.DefaultCallGraphDepth})
	callGraphDur := time.Since(start)
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	t.Logf("Stage 1 — CallGraph:       %8v  (%d edges, %d nodes)", callGraphDur, len(cg.Edges), len(cg.Nodes))

	if ctx.Err() != nil {
		t.Fatalf("Context expired after CallGraph: %v", ctx.Err())
	}

	// Stage 2: Classes
	fa := analyzer.NewFallback(origamiRoot, nil)
	start = time.Now()
	classes, _ := fa.Classes(ctx, origamiRoot)
	classesDur := time.Since(start)
	t.Logf("Stage 2 — Classes:         %8v  (%d classes)", classesDur, len(classes))

	if ctx.Err() != nil {
		t.Fatalf("Context expired after Classes: %v", ctx.Err())
	}

	// Stage 3: Implements
	start = time.Now()
	impls, _ := fa.Implements(ctx, origamiRoot)
	implsDur := time.Since(start)
	t.Logf("Stage 3 — Implements:      %8v  (%d edges)", implsDur, len(impls))

	// Stage 4: FieldRefs
	start = time.Now()
	refs, _ := fa.FieldRefs(ctx, origamiRoot)
	fieldRefsDur := time.Since(start)
	t.Logf("Stage 4 — FieldRefs:       %8v  (%d refs)", fieldRefsDur, len(refs))

	// Stage 5: MergeSymbolGraph
	start = time.Now()
	sg := oculus.MergeSymbolGraph(cg, classes, impls, refs)
	mergeDur := time.Since(start)
	t.Logf("Stage 5 — MergeSymbolGraph:%8v  (%d nodes, %d edges)", mergeDur, len(sg.Nodes), len(sg.Edges))

	// Stage 6: DetectPipelines
	start = time.Now()
	report := oculus.DetectPipelines(sg, 3)
	pipelinesDur := time.Since(start)
	t.Logf("Stage 6 — DetectPipelines: %8v  (%d pipelines)", pipelinesDur, len(report.Pipelines))

	// Summary
	total := callGraphDur + classesDur + implsDur + fieldRefsDur + mergeDur + pipelinesDur
	t.Logf("---")
	t.Logf("Total:                     %8v", total)
	t.Logf("Bottleneck: %s", bottleneck(
		stage{"CallGraph", callGraphDur},
		stage{"Classes", classesDur},
		stage{"Implements", implsDur},
		stage{"FieldRefs", fieldRefsDur},
		stage{"Merge", mergeDur},
		stage{"Pipelines", pipelinesDur},
	))

	if total > 90*time.Second {
		t.Errorf("Total pipeline time %v exceeds 90s budget", total)
	}
}

type stage struct {
	name string
	dur  time.Duration
}

func bottleneck(stages ...stage) string {
	max := stages[0]
	for _, s := range stages[1:] {
		if s.dur > max.dur {
			max = s
		}
	}
	return max.name + " (" + max.dur.String() + ")"
}
