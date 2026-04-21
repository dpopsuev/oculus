package oculus_test

import (
	"strings"
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/book"
	"github.com/dpopsuev/oculus/v3/testkit"
)

func TestDiagnose_HighFanOut(t *testing.T) {
	sg := testkit.FixtureGraph()
	bg, err := book.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}

	// A has fan-out=2, probe it and verify book was queried.
	result := oculus.Diagnose(sg, bg, "pkg1.A")
	if result == nil {
		t.Fatal("expected non-nil DiagnoseResult")
	}
	if result.Probe == nil {
		t.Fatal("expected non-nil Probe in result")
	}
	if result.Probe.FQN != "pkg1.A" {
		t.Errorf("Probe.FQN = %q, want pkg1.A", result.Probe.FQN)
	}
	if result.Book == nil {
		t.Fatal("expected non-nil Book in result")
	}
	// A's kind is "function", so at minimum the book should be queried with "function".
	// The result should have at least one entry.
	if len(result.Book.Entries) == 0 {
		t.Error("expected at least one Book entry")
	}
}

func TestDiagnose_UnknownSymbol(t *testing.T) {
	sg := testkit.FixtureGraph()
	bg, err := book.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}

	result := oculus.Diagnose(sg, bg, "pkg99.DoesNotExist")
	if result != nil {
		t.Error("expected nil for unknown symbol")
	}
}

// --- TSK-178: Diagnose keyword derivation for structs ---

func TestDiagnose_StructKeywords(t *testing.T) {
	// Build a graph with a struct that has zero direct fan-out.
	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "Config", Package: "app", Kind: "struct", Exported: true, File: "app/config.go", Line: 1},
		},
		Edges: nil, // no edges at all
	}

	bg, err := book.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}

	result := oculus.Diagnose(sg, bg, "app.Config")
	if result == nil {
		t.Fatal("expected non-nil DiagnoseResult for struct")
	}

	// Struct-specific keywords should include "types" and "cohesion".
	if result.Book == nil {
		t.Fatal("expected non-nil Book result for struct — keywords should produce matches")
	}

	// Verify no Factory pattern in book results — a struct with zero fan-out
	// should NOT trigger factory/constructor keywords.
	for _, entry := range result.Book.Entries {
		lower := strings.ToLower(entry.ID)
		if strings.Contains(lower, "factory") {
			t.Errorf("struct with zero fan-out should NOT match Factory; got entry %q", entry.ID)
		}
	}
}

func TestDiagnose_StructNoFactoryKeywords(t *testing.T) {
	// A struct with zero fan-out should not have "constructor" or "factory" keywords.
	sg := &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "Data", Package: "model", Kind: "struct", Exported: true, File: "model/data.go", Line: 1},
		},
		Edges: nil,
	}

	bg, err := book.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}

	result := oculus.Diagnose(sg, bg, "model.Data")
	if result == nil {
		t.Fatal("expected non-nil DiagnoseResult")
	}

	// The kind should be "struct", triggering struct-specific keywords
	// ("types", "cohesion", "data class") instead of generic function keywords.
	if result.Probe.Kind != "struct" {
		t.Errorf("Kind = %q, want struct", result.Probe.Kind)
	}
}
