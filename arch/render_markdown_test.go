package arch

import (
	"fmt"
	"strings"
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/model"
)

func testReport() *oculus.ContextReport {
	return &oculus.ContextReport{
		ScanCore: oculus.ScanCore{
			Project:    &model.Project{Language: model.LangGo},
			ModulePath: "example.com/test",
			Scanner:    "packages",
			Architecture: oculus.ArchModel{
				Services: []oculus.ArchService{
					{Name: "engine", LOC: 8000, Churn: 15, Symbols: []model.Symbol{
						{Name: "ScanProject", Kind: model.SymbolFunction, Exported: true},
						{Name: "Engine", Kind: model.SymbolStruct, Exported: true},
						{Name: "Protocol", Kind: model.SymbolInterface, Exported: true},
						{Name: "helper", Kind: model.SymbolFunction, Exported: false},
					}},
					{Name: "graph", LOC: 1500, Symbols: []model.Symbol{
						{Name: "Build", Kind: model.SymbolFunction, Exported: true},
						{Name: "Node", Kind: model.SymbolStruct, Exported: true},
					}},
					{Name: "survey", LOC: 6000, Churn: 26, Symbols: []model.Symbol{
						{Name: "Scanner", Kind: model.SymbolInterface, Exported: true},
						{Name: "LSPScanner", Kind: model.SymbolStruct, Exported: true},
						{Name: "GoScanner", Kind: model.SymbolStruct, Exported: true},
					}},
					{Name: "model", LOC: 500, Symbols: []model.Symbol{
						{Name: "Symbol", Kind: model.SymbolStruct, Exported: true},
						{Name: "Project", Kind: model.SymbolStruct, Exported: true},
					}},
				},
				Edges: makeEdges(),
			},
		},
		GraphMetrics: oculus.GraphMetrics{
			HotSpots: []oculus.HotSpot{
				{Component: "survey", FanIn: 4, Churn: 26},
				{Component: "engine", FanIn: 0, Churn: 15},
			},
		},
	}
}

func makeEdges() []oculus.ArchEdge {
	edges := []oculus.ArchEdge{
		{From: "engine", To: "graph", Weight: 50},
		{From: "engine", To: "survey", Weight: 30},
		{From: "engine", To: "model", Weight: 20},
		{From: "survey", To: "model", Weight: 60},
		{From: "survey", To: "graph", Weight: 5},
		{From: "graph", To: "model", Weight: 10},
	}
	for i := 0; i < 25; i++ {
		edges = append(edges, oculus.ArchEdge{
			From: "engine", To: "engine", Weight: 1,
		})
	}
	return edges
}

func TestRenderMarkdown_HotSpotsBeforeEdges(t *testing.T) {
	report := testReport()
	md := RenderMarkdown(report)

	hotIdx := strings.Index(md, "## Hot Spots")
	edgeIdx := strings.Index(md, "## Dependencies")

	if hotIdx < 0 {
		t.Fatal("missing Hot Spots section")
	}
	if edgeIdx < 0 {
		t.Fatal("missing Dependencies section")
	}
	if hotIdx > edgeIdx {
		t.Errorf("Hot Spots (pos %d) should appear before Dependencies (pos %d)", hotIdx, edgeIdx)
	}
}

func TestRenderMarkdown_LanguageAndSurveyLabels(t *testing.T) {
	md := RenderMarkdown(testReport())
	if !strings.Contains(md, "Languages: go") {
		t.Fatalf("missing Languages label: %s", md[:200])
	}
	if !strings.Contains(md, "Survey: packages") {
		t.Fatalf("missing Survey label: %s", md[:200])
	}
	if strings.Contains(md, "go | Scanner:") {
		t.Fatal("legacy dual Scanner wording should be gone")
	}
	if !strings.Contains(md, "## Hot Spots") {
		t.Fatal("Hot Spots section must always be present")
	}
}

func TestRenderMarkdown_LanguagesFromInventoryField(t *testing.T) {
	r := testReport()
	r.Languages = []string{"rust", "typescript"}
	r.Scanner = "composite"
	r.Project.Language = model.LangUnknown
	md := RenderMarkdown(r)
	if !strings.Contains(md, "Languages: rust, typescript") {
		t.Fatalf("want inventory languages, got: %s", md[:240])
	}
}

func TestRenderMarkdown_HotSpotsNoneAboveThreshold(t *testing.T) {
	r := testReport()
	r.HotSpots = nil
	md := RenderMarkdown(r)
	if !strings.Contains(md, "None above threshold") {
		t.Fatalf("expected empty hot-spots explanation: %s", md)
	}
}

func TestRenderEdgeList_CapsAtMaxEdges(t *testing.T) {
	report := testReport()
	for i := 0; i < 25; i++ {
		src := fmt.Sprintf("pkg_%02d", i)
		report.Architecture.Edges = append(report.Architecture.Edges,
			oculus.ArchEdge{From: src, To: "model", Weight: 25 - i})
	}

	md := RenderEdgeList(report, "")

	depLines := 0
	for _, line := range strings.Split(strings.TrimSpace(md), "\n") {
		if strings.Contains(line, "depends on:") {
			depLines++
		}
	}

	if depLines > maxEdgesMarkdown {
		t.Errorf("adjacency list has %d source components, want at most %d", depLines, maxEdgesMarkdown)
	}

	if !strings.Contains(md, "more") {
		t.Error("expected overflow hint when source components are truncated")
	}
}

func TestRenderMarkdown_ShowsKeySymbols(t *testing.T) {
	report := testReport()
	md := RenderMarkdown(report)

	if !strings.Contains(md, "## Key Symbols") {
		t.Fatal("missing Key Symbols section")
	}
	if !strings.Contains(md, "interface Protocol") || !strings.Contains(md, "interface Scanner") {
		t.Error("expected interfaces to appear in key symbols")
	}
	if !strings.Contains(md, "func ScanProject") {
		t.Error("expected exported functions in key symbols")
	}
	if strings.Contains(md, "func helper") {
		t.Error("unexported symbols should not appear in key symbols")
	}
}

func TestRenderMarkdown_ProjectSummary(t *testing.T) {
	report := testReport()
	md := RenderMarkdown(report)

	if !strings.Contains(md, "go") {
		t.Error("expected language in summary")
	}
	headerIdx := strings.Index(md, "# example.com/test")
	hotIdx := strings.Index(md, "## Hot Spots")
	if headerIdx < 0 || hotIdx < 0 {
		t.Fatal("missing header or hot spots")
	}
	between := md[headerIdx:hotIdx]
	if !strings.Contains(between, "Components:") {
		t.Error("expected component count in summary line")
	}
}

func TestRenderEdgeList_AdjacencyFormat(t *testing.T) {
	report := testReport()
	md := RenderEdgeList(report, "")

	if strings.Contains(md, "->") {
		t.Error("expected natural language adjacency format, not arrow notation")
	}
	if !strings.Contains(md, "depends on:") {
		t.Error("expected 'depends on:' adjacency format")
	}
}

func TestRenderMarkdown_ShowsSignatures(t *testing.T) {
	report := testReport()
	for i, svc := range report.Architecture.Services {
		for j := range svc.Symbols {
			sym := &report.Architecture.Services[i].Symbols[j]
			if sym.Name == "ScanProject" {
				sym.Signature = "func ScanProject(root string) (*Result, error)"
			}
			if sym.Name == "Scanner" {
				sym.Signature = "interface Scanner { Scan(root string) (*Project, error) }"
			}
		}
	}

	md := RenderMarkdown(report)

	if !strings.Contains(md, "func ScanProject(root string) (*Result, error)") {
		t.Error("expected full signature for ScanProject")
	}
	if !strings.Contains(md, "interface Scanner { Scan(root string) (*Project, error) }") {
		t.Error("expected full signature for Scanner interface")
	}
}
