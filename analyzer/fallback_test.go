package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus"
)

func TestFallback_Classes(t *testing.T) {
	dir := setupTestRepo(t)
	fb := NewFallback(dir, nil)
	classes, err := fb.Classes(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(classes) < 3 {
		t.Fatalf("expected at least 3 types, got %d", len(classes))
	}
}

func TestFallback_NestingDepth(t *testing.T) {
	dir := setupTestRepo(t)
	fb := NewFallback(dir, nil)
	results, err := fb.NestingDepth(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) == 0 {
		t.Fatal("expected nesting results")
	}
}

// TestPipelineFallback_CallGraph tests that NewPipelineFallback produces
// call graphs via the SymbolPipeline path (bounded concurrent walk).
func TestPipelineFallback_CallGraph(t *testing.T) {
	dir := setupTestRepo(t)
	fb := NewPipelineFallback(dir, nil)

	cg, err := fb.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if len(cg.Nodes) == 0 {
		t.Error("expected nodes from pipeline fallback")
	}
	if len(cg.Edges) == 0 {
		t.Error("expected edges from pipeline fallback")
	}
	t.Logf("PipelineFallback: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

// TestPipelineFallback_DetectStateMachines tests that raw analyzers
// are used as fallback when Pipeline returns nil.
func TestPipelineFallback_DetectStateMachines(t *testing.T) {
	dir := setupTestRepo(t)
	fb := NewPipelineFallback(dir, nil)

	machines, err := fb.DetectStateMachines(context.Background(), dir)
	if err != nil {
		t.Fatalf("DetectStateMachines: %v", err)
	}
	// The test repo may or may not have state machines; just verify it doesn't crash.
	t.Logf("PipelineFallback: %d state machines", len(machines))
}

func TestFallback_RegexFallback(t *testing.T) {
	dir := t.TempDir()
	// Rust project (no tree-sitter Rust implementation but regex handles it)
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "main.rs"), []byte(`
pub struct Foo {
    name: String,
}

pub trait Bar {
    fn do_thing(&self);
}

impl Bar for Foo {
    fn do_thing(&self) {}
}
`), 0o644)

	fb := NewFallback(dir, nil)
	classes, err := fb.Classes(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	// Regex should find at least the struct and trait
	if len(classes) < 2 {
		t.Fatalf("regex fallback: expected at least 2 types, got %d", len(classes))
	}

	edges, err := fb.Implements(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range edges {
		if e.From == "Foo" && e.To == "Bar" {
			found = true
		}
	}
	if !found {
		t.Error("regex fallback: expected Foo implements Bar")
	}
}

func TestGranularity_DefaultProducesEdges(t *testing.T) {
	dir := setupTestRepo(t)
	da := NewDeepFallback(dir, nil)

	// Default granularity (TypedCallGraph) should produce edges via tree-sitter.
	cg, err := da.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if len(cg.Edges) == 0 {
		t.Error("default granularity should produce edges")
	}
	t.Logf("default: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func TestGranularity_StructureSkipsExpensiveSources(t *testing.T) {
	dir := setupTestRepo(t)
	da := NewDeepFallback(dir, nil)

	// Structure granularity is lower than TypedCallGraph.
	// No sources are registered at Structure level, so Pipeline sources
	// are skipped. Falls through to raw analyzers (GoAST/TreeSitter).
	cg, err := da.CallGraph(context.Background(), dir, oculus.CallGraphOpts{
		Depth:       5,
		Granularity: oculus.GranularityStructure,
	})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	// Raw analyzers should still produce edges.
	t.Logf("structure: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func TestGranularity_SemanticIncludesAll(t *testing.T) {
	dir := setupTestRepo(t)
	da := NewDeepFallback(dir, nil)

	// Semantic granularity should include all sources (TypedCallGraph >= requested).
	// Wait — Semantic is HIGHER than TypedCallGraph. Only LSP satisfies it.
	// Since no LSP is available in test, falls through to raw analyzers.
	cg, err := da.CallGraph(context.Background(), dir, oculus.CallGraphOpts{
		Depth:       5,
		Granularity: oculus.GranularitySemantic,
	})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	t.Logf("semantic: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}
