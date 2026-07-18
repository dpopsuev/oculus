package engine

import (
	"testing"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/model"
)

// typeUsages must union declaration sites with import-type consumers
// (CodeGraph imports-type), or cross-package TS type imports undercount.
func TestTypeUsages_IncludesImportedTypes(t *testing.T) {
	report := &arch.ContextReport{
		ScanCore: oculus.ScanCore{
			Architecture: oculus.ArchModel{
				Services: []oculus.ArchService{
					{
						Name: "packages/kernel/src",
						Symbols: []model.Symbol{
							{Name: "DiscussionRef", Kind: model.SymbolInterface, Exported: true},
						},
					},
					{
						Name:          "packages/foundry/src",
						ImportedTypes: []string{"DiscussionRef"},
					},
				},
			},
		},
	}

	r := typeUsages(report, "DiscussionRef")
	if len(r.Files) != 2 {
		t.Fatalf("files=%d, want 2 (decl + import type)\n%+v", len(r.Files), r.Files)
	}
	seen := map[string]bool{}
	for _, f := range r.Files {
		seen[f.Component] = true
	}
	if !seen["packages/kernel/src"] || !seen["packages/foundry/src"] {
		t.Fatalf("want kernel + foundry, got %v", seen)
	}
}
