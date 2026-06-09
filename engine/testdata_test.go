package engine

import (
	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/graph"
	"github.com/dpopsuev/oculus/v3/model"
	"github.com/dpopsuev/oculus/v3/port"
)

// testReport returns a minimal but complete ContextReport for unit testing.
// 4 services, 4 edges, hot spots, import depth — enough to exercise
// all pure analysis paths without real filesystem scans.
func testReport() *arch.ContextReport {
	return &arch.ContextReport{
		ScanCore: arch.ScanCore{
			ModulePath:     "github.com/example/project",
			Scanner:        "test",
			SuggestedDepth: 1,
			Architecture: arch.ArchModel{
				Title: "project",
				Services: []arch.ArchService{
					{Name: "cmd/app", Package: "github.com/example/project/cmd/app", LOC: 100, Churn: 2, Language: model.LangGo, Symbols: model.SymbolsFromNames("main")},
					{Name: "internal/core", Package: "github.com/example/project/internal/core", LOC: 500, Churn: 20, Language: model.LangGo, Symbols: model.SymbolsFromNames("Run", "Config", "Init")},
					{Name: "internal/store", Package: "github.com/example/project/internal/store", LOC: 300, Churn: 8, Language: model.LangGo, Symbols: model.SymbolsFromNames("DB", "Get", "Put")},
					{Name: "pkg/logger", Package: "github.com/example/project/pkg/logger", LOC: 80, Churn: 1, Language: model.LangGo, Symbols: model.SymbolsFromNames("Log")},
				},
				Edges: []arch.ArchEdge{
					{From: "cmd/app", To: "internal/core", Weight: 1, CallSites: 3, LOCSurface: 10},
					{From: "internal/core", To: "internal/store", Weight: 1, CallSites: 5, LOCSurface: 15},
					{From: "internal/core", To: "pkg/logger", Weight: 1, CallSites: 2, LOCSurface: 3},
					{From: "internal/store", To: "pkg/logger", Weight: 1, CallSites: 1, LOCSurface: 2},
				},
			},
		},
		GraphMetrics: arch.GraphMetrics{
			HotSpots: []arch.HotSpot{
				{Component: "internal/core", FanIn: 1, Churn: 20},
				{Component: "internal/store", FanIn: 2, Churn: 8},
			},
			ImportDepth: graph.DepthMap{
				"cmd/app":        2,
				"internal/core":  1,
				"internal/store": 0,
				"pkg/logger":     0,
			},
			FanIn: graph.CountMap{
				"internal/core":  1,
				"internal/store": 1,
				"pkg/logger":     2,
			},
			FanOut: graph.CountMap{
				"cmd/app":        1,
				"internal/core":  2,
				"internal/store": 1,
			},
		},
	}
}

// testReportWithCycles returns a report containing a dependency cycle.
func testReportWithCycles() *arch.ContextReport {
	r := testReport()
	r.Architecture.Edges = append(r.Architecture.Edges,
		arch.ArchEdge{From: "internal/store", To: "internal/core", Weight: 1, CallSites: 1},
	)
	r.Cycles = []graph.Cycle{{"internal/core", "internal/store", "internal/core"}}
	return r
}

// testDesiredState returns a minimal desired architecture state.
func testDesiredState() *port.DesiredState {
	return &port.DesiredState{
		Layers: []string{"pkg/logger", "internal/store", "internal/core", "cmd/app"},
		Constraints: []port.HealthConstraint{
			{Component: "internal/core", MaxFanIn: 5, MaxChurn: 30},
		},
	}
}

// testReportWithFileGranularity returns a report where the "agents" directory
// is represented as file-level services (as if file_granularity=true was used).
// This simulates what a file-granularity scan of a Python project returns.
func testReportWithFileGranularity() *arch.ContextReport {
	return &arch.ContextReport{
		ScanCore: arch.ScanCore{
			ModulePath: "github.com/example/project",
			Scanner:    "test",
			Architecture: arch.ArchModel{
				Title: "project",
				Services: []arch.ArchService{
					{Name: "agents/turn_buffer.py", LOC: 120, Language: model.LangPython, Symbols: model.SymbolsFromNames("TurnBuffer", "flush")},
					{Name: "agents/renderer.py", LOC: 200, Language: model.LangPython, Symbols: model.SymbolsFromNames("StreamRenderer", "render")},
					{Name: "agents/widgets.py", LOC: 80, Language: model.LangPython, Symbols: model.SymbolsFromNames("Widget", "draw")},
					{Name: "core/engine.py", LOC: 300, Language: model.LangPython, Symbols: model.SymbolsFromNames("Engine", "run")},
				},
				Edges: []arch.ArchEdge{
					{From: "agents/turn_buffer.py", To: "agents/renderer.py", Weight: 1, CallSites: 3},
					{From: "agents/renderer.py", To: "agents/widgets.py", Weight: 1, CallSites: 5},
					{From: "core/engine.py", To: "agents/turn_buffer.py", Weight: 1, CallSites: 2},
				},
			},
		},
	}
}
