package oculus

import "testing"

func TestDetectPipelines_LinearChain(t *testing.T) {
	sg := &SymbolGraph{
		Edges: []SymbolEdge{
			{SourceFQN: "pkg.Parse", TargetFQN: "pkg.Transform", Kind: "call", ParamTypes: []string{"*AST"}, ReturnTypes: []string{"*IR", "error"}},
			{SourceFQN: "pkg.Transform", TargetFQN: "pkg.Render", Kind: "call", ParamTypes: []string{"*Output"}, ReturnTypes: []string{"string"}},
		},
	}
	// Parse(string) -> (*AST, error)
	// Transform(*AST) -> (*IR, error)  -- *AST overlaps
	// Render(*IR) -> string            -- *IR overlaps
	// But we need the callee signatures in the edges.
	// Edge Parse→Transform: ParamTypes=Transform's params=[*AST], ReturnTypes=Transform's returns=[*IR, error]
	// Edge Transform→Render: ParamTypes=Render's params=[*IR], ReturnTypes=Render's returns=[string]
	// Parse's own signature comes from edges where Parse is the callee (none here — it's the root)
	// We need to add Parse's signature via calleeSig lookup.

	// Rebuild with proper callee-annotated edges:
	sg = &SymbolGraph{
		Edges: []SymbolEdge{
			// Parse calls Transform. Edge carries Transform's signature.
			{SourceFQN: "pkg.Parse", TargetFQN: "pkg.Transform", Kind: "call",
				ParamTypes: []string{"*AST"}, ReturnTypes: []string{"*IR", "error"}},
			// Transform calls Render. Edge carries Render's signature.
			{SourceFQN: "pkg.Transform", TargetFQN: "pkg.Render", Kind: "call",
				ParamTypes: []string{"*IR"}, ReturnTypes: []string{"string"}},
		},
	}

	report := DetectPipelines(sg, 2)
	if len(report.Pipelines) == 0 {
		t.Fatal("expected at least 1 pipeline")
	}
	p := report.Pipelines[0]
	if p.Length < 2 {
		t.Errorf("pipeline length = %d, want >= 2", p.Length)
	}
	t.Logf("Pipeline: %d steps, type chain: %v", p.Length, p.TypeChain)
}

func TestDetectPipelines_NoPipeline(t *testing.T) {
	sg := &SymbolGraph{
		Edges: []SymbolEdge{
			{SourceFQN: "a.Foo", TargetFQN: "b.Bar", Kind: "call"},
			{SourceFQN: "b.Bar", TargetFQN: "c.Baz", Kind: "call"},
		},
	}
	report := DetectPipelines(sg, 3)
	if len(report.Pipelines) != 0 {
		t.Errorf("expected 0 pipelines (no type data), got %d", len(report.Pipelines))
	}
}

func TestDetectPipelines_BranchBreaks(t *testing.T) {
	sg := &SymbolGraph{
		Edges: []SymbolEdge{
			{SourceFQN: "a.Start", TargetFQN: "b.Left", Kind: "call", ParamTypes: []string{"int"}, ReturnTypes: []string{"string"}},
			{SourceFQN: "a.Start", TargetFQN: "c.Right", Kind: "call", ParamTypes: []string{"int"}, ReturnTypes: []string{"string"}},
		},
	}
	report := DetectPipelines(sg, 2)
	// Fork at a.Start — no linear chain of length >= 2
	for _, p := range report.Pipelines {
		if p.Length >= 2 {
			t.Errorf("expected no pipeline of length >= 2 at fork, got length %d", p.Length)
		}
	}
}

func TestDetectPipelines_MinLength(t *testing.T) {
	sg := &SymbolGraph{
		Edges: []SymbolEdge{
			{SourceFQN: "a.A", TargetFQN: "b.B", Kind: "call",
				ParamTypes: []string{"int"}, ReturnTypes: []string{"string"}},
		},
	}
	report := DetectPipelines(sg, 3)
	if len(report.Pipelines) != 0 {
		t.Errorf("expected 0 pipelines (chain too short), got %d", len(report.Pipelines))
	}
}

func TestDetectPipelines_EmptyGraph(t *testing.T) {
	report := DetectPipelines(nil, 3)
	if report == nil {
		t.Fatal("expected non-nil report")
	}
	if len(report.Pipelines) != 0 {
		t.Errorf("expected 0 pipelines, got %d", len(report.Pipelines))
	}
}

func TestTypesOverlap(t *testing.T) {
	tests := []struct {
		name    string
		returns []string
		params  []string
		want    bool
	}{
		{"exact match", []string{"Config"}, []string{"Config"}, true},
		{"pointer to value", []string{"*Config"}, []string{"Config"}, true},
		{"no overlap", []string{"int"}, []string{"string"}, false},
		{"error excluded", []string{"error"}, []string{"error"}, false},
		{"error with real type", []string{"*Result", "error"}, []string{"*Result"}, true},
		{"empty returns", nil, []string{"int"}, false},
		{"empty params", []string{"int"}, nil, false},
	}
	for _, tt := range tests {
		_, got := typesOverlap(tt.returns, tt.params)
		if got != tt.want {
			t.Errorf("typesOverlap(%q, %q) = %v, want %v (%s)", tt.returns, tt.params, got, tt.want, tt.name)
		}
	}
}

func TestNormalizeType(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"int", "int"},
		{"*Config", "Config"},
		{"**Config", "*Config"},
		{"[]byte", "[]byte"},
	}
	for _, tt := range tests {
		if got := normalizeType(tt.input); got != tt.want {
			t.Errorf("normalizeType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
