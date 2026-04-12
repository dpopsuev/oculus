package testkit

import (
	"context"
	"testing"

	"github.com/dpopsuev/oculus"
)

// StubDeepAnalyzer is a configurable test double for oculus.DeepAnalyzer.
// Set the fields to control what each method returns.
type StubDeepAnalyzer struct {
	CallGraphResult        *oculus.CallGraph
	CallGraphErr           error
	DataFlowResult         *oculus.DataFlow
	DataFlowErr            error
	StateMachinesResult    []oculus.StateMachine
	StateMachinesErr       error
}

func (s *StubDeepAnalyzer) CallGraph(_ context.Context, _ string, _ oculus.CallGraphOpts) (*oculus.CallGraph, error) {
	if s.CallGraphErr != nil {
		return nil, s.CallGraphErr
	}
	if s.CallGraphResult != nil {
		return s.CallGraphResult, nil
	}
	return SampleCallGraph(), nil
}

func (s *StubDeepAnalyzer) DataFlowTrace(_ context.Context, _, _ string, _ int) (*oculus.DataFlow, error) {
	if s.DataFlowErr != nil {
		return nil, s.DataFlowErr
	}
	if s.DataFlowResult != nil {
		return s.DataFlowResult, nil
	}
	return SampleDataFlow(), nil
}

func (s *StubDeepAnalyzer) DetectStateMachines(_ context.Context, _ string) ([]oculus.StateMachine, error) {
	if s.StateMachinesErr != nil {
		return nil, s.StateMachinesErr
	}
	if s.StateMachinesResult != nil {
		return s.StateMachinesResult, nil
	}
	return SampleStateMachines(), nil
}

// StubTypeAnalyzer is a configurable test double for oculus.TypeAnalyzer.
type StubTypeAnalyzer struct {
	ClassesResult    []oculus.ClassInfo
	ClassesErr       error
	ImplementsResult []oculus.ImplEdge
	ImplementsErr    error
	CallChainResult  []oculus.Call
	CallChainErr     error
	EntryPointsResult []oculus.EntryPoint
	EntryPointsErr    error
	FieldRefsResult  []oculus.FieldRef
	FieldRefsErr     error
	NestingResult    []oculus.NestingResult
	NestingErr       error
}

func (s *StubTypeAnalyzer) Classes(_ context.Context, _ string) ([]oculus.ClassInfo, error) {
	return s.ClassesResult, s.ClassesErr
}

func (s *StubTypeAnalyzer) Implements(_ context.Context, _ string) ([]oculus.ImplEdge, error) {
	return s.ImplementsResult, s.ImplementsErr
}

func (s *StubTypeAnalyzer) CallChain(_ context.Context, _, _ string, _ int) ([]oculus.Call, error) {
	if s.CallChainErr != nil {
		return nil, s.CallChainErr
	}
	if s.CallChainResult != nil {
		return s.CallChainResult, nil
	}
	return SampleCallChain(), nil
}

func (s *StubTypeAnalyzer) EntryPoints(_ context.Context, _ string) ([]oculus.EntryPoint, error) {
	if s.EntryPointsErr != nil {
		return nil, s.EntryPointsErr
	}
	if s.EntryPointsResult != nil {
		return s.EntryPointsResult, nil
	}
	return SampleEntryPoints(), nil
}

func (s *StubTypeAnalyzer) FieldRefs(_ context.Context, _ string) ([]oculus.FieldRef, error) {
	return s.FieldRefsResult, s.FieldRefsErr
}

func (s *StubTypeAnalyzer) NestingDepth(_ context.Context, _ string) ([]oculus.NestingResult, error) {
	return s.NestingResult, s.NestingErr
}

// --- Sample data factories ---

// SampleCallGraph returns a minimal call graph for testing.
func SampleCallGraph() *oculus.CallGraph {
	return &oculus.CallGraph{
		Nodes: []oculus.Symbol{
			{Name: "main", Package: "cmd/app", Line: 10, File: "cmd/app/main.go", EndLine: 20},
			{Name: "Run", Package: "internal/core", Line: 15, File: "internal/core/core.go", EndLine: 45},
			{Name: "Get", Package: "internal/store", Line: 20, File: "internal/store/store.go", EndLine: 35},
		},
		Edges: []oculus.CallEdge{
			{Caller: "main", Callee: "Run", CallerPkg: "cmd/app", CalleePkg: "internal/core", CrossPkg: true, Line: 12, File: "cmd/app/main.go"},
			{Caller: "Run", Callee: "Get", CallerPkg: "internal/core", CalleePkg: "internal/store", CrossPkg: true, Line: 30, File: "internal/core/core.go"},
		},
		Layer: "stub",
	}
}

// SampleDataFlow returns a minimal data flow for testing.
func SampleDataFlow() *oculus.DataFlow {
	return &oculus.DataFlow{
		Nodes: []oculus.DataFlowNode{
			{Name: "main", Kind: "entry"},
			{Name: "HandleRequest", Kind: "process", Pkg: "internal/api"},
			{Name: "UserDB", Kind: "data_store"},
			{Name: "Client", Kind: "external"},
		},
		Edges: []oculus.DataFlowEdge{
			{From: "Client", To: "main", Label: "HTTP"},
			{From: "main", To: "HandleRequest"},
			{From: "HandleRequest", To: "UserDB", Label: "Query"},
		},
		Boundaries: []oculus.TrustBoundary{
			{Name: "Auth Zone", Nodes: []string{"HandleRequest", "UserDB"}},
		},
		Layer: "stub",
	}
}

// SampleStateMachines returns sample state machines for testing.
func SampleStateMachines() []oculus.StateMachine {
	return []oculus.StateMachine{
		{
			Name:    "OrderStatus",
			Package: "internal/order",
			States:  []string{"Pending", "Processing", "Shipped", "Delivered"},
			Transitions: []oculus.StateTransition{
				{From: "Pending", To: "Processing", Trigger: "confirm"},
				{From: "Processing", To: "Shipped", Trigger: "ship"},
				{From: "Shipped", To: "Delivered", Trigger: "deliver"},
			},
			Initial: "Pending",
		},
	}
}

// SampleCallChain returns a minimal call chain for sequence diagram testing.
func SampleCallChain() []oculus.Call {
	return []oculus.Call{
		{Caller: "main", Callee: "HandleRequest", Package: "cmd/app", Line: 10, File: "cmd/app/main.go"},
		{Caller: "HandleRequest", Callee: "GetUser", Package: "internal/api", Line: 25, File: "internal/api/handler.go"},
		{Caller: "GetUser", Callee: "QueryDB", Package: "internal/store", Line: 40, File: "internal/store/store.go"},
	}
}

// SampleEntryPoints returns sample entry points for testing.
func SampleEntryPoints() []oculus.EntryPoint {
	return []oculus.EntryPoint{
		{Name: "main", Kind: "main", Package: "cmd/app", File: "main.go", Line: 1, EndLine: 15},
	}
}

// SampleSymbolGraph returns a minimal symbol graph for testing.
func SampleSymbolGraph() *oculus.SymbolGraph {
	return &oculus.SymbolGraph{
		Nodes: []oculus.Symbol{
			{Name: "main", Package: "cmd/app", Kind: "function", File: "cmd/app/main.go", Line: 10, EndLine: 20, Exported: false},
			{Name: "Run", Package: "internal/core", Kind: "function", File: "internal/core/core.go", Line: 15, EndLine: 45, Exported: true},
			{Name: "Get", Package: "internal/store", Kind: "function", File: "internal/store/store.go", Line: 20, EndLine: 35, Exported: true},
			{Name: "Store", Package: "internal/store", Kind: "interface", File: "internal/store/store.go", Line: 5, EndLine: 12, Exported: true},
		},
		Edges: []oculus.SymbolEdge{
			{SourceFQN: "cmd/app.main", TargetFQN: "internal/core.Run", Kind: "call", File: "cmd/app/main.go", Line: 12},
			{SourceFQN: "internal/core.Run", TargetFQN: "internal/store.Get", Kind: "call", File: "internal/core/core.go", Line: 30},
			{SourceFQN: "internal/store.Get", TargetFQN: "internal/store.Store", Kind: "implements"},
		},
	}
}

// --- Assertion helpers ---

// AssertDataFlow verifies a DataFlow result has expected properties.
func AssertDataFlow(t *testing.T, flow *oculus.DataFlow, minNodes, minEdges int) {
	t.Helper()
	if flow == nil {
		t.Fatal("DataFlow is nil")
	}
	if len(flow.Nodes) < minNodes {
		t.Errorf("DataFlow: got %d nodes, want >= %d", len(flow.Nodes), minNodes)
	}
	if len(flow.Edges) < minEdges {
		t.Errorf("DataFlow: got %d edges, want >= %d", len(flow.Edges), minEdges)
	}
}

// AssertDataFlowHasNode verifies a DataFlow contains a node with the given name.
func AssertDataFlowHasNode(t *testing.T, flow *oculus.DataFlow, name, kind string) {
	t.Helper()
	for _, n := range flow.Nodes {
		if n.Name == name {
			if kind != "" && n.Kind != kind {
				t.Errorf("DataFlow node %q: got kind %q, want %q", name, n.Kind, kind)
			}
			return
		}
	}
	t.Errorf("DataFlow: node %q not found", name)
}

// AssertDataFlowHasEdge verifies a DataFlow contains an edge from->to.
func AssertDataFlowHasEdge(t *testing.T, flow *oculus.DataFlow, from, to string) {
	t.Helper()
	for _, e := range flow.Edges {
		if e.From == from && e.To == to {
			return
		}
	}
	t.Errorf("DataFlow: edge %s->%s not found", from, to)
}

// AssertDataFlowHasBoundary verifies a DataFlow has a trust boundary with the given name.
func AssertDataFlowHasBoundary(t *testing.T, flow *oculus.DataFlow, name string) {
	t.Helper()
	for _, b := range flow.Boundaries {
		if b.Name == name {
			return
		}
	}
	t.Errorf("DataFlow: boundary %q not found", name)
}

// AssertStateMachines verifies state machine results.
func AssertStateMachines(t *testing.T, machines []oculus.StateMachine, minCount int) {
	t.Helper()
	if len(machines) < minCount {
		t.Errorf("StateMachines: got %d, want >= %d", len(machines), minCount)
	}
}

// AssertStateMachineNamed verifies a state machine with the given name exists and has expected properties.
func AssertStateMachineNamed(t *testing.T, machines []oculus.StateMachine, name string, minStates, minTransitions int) {
	t.Helper()
	for _, m := range machines {
		if m.Name == name {
			if len(m.States) < minStates {
				t.Errorf("StateMachine %q: got %d states, want >= %d", name, len(m.States), minStates)
			}
			if len(m.Transitions) < minTransitions {
				t.Errorf("StateMachine %q: got %d transitions, want >= %d", name, len(m.Transitions), minTransitions)
			}
			return
		}
	}
	t.Errorf("StateMachine %q not found", name)
}

// AssertCallGraph verifies a CallGraph result has expected properties.
func AssertCallGraph(t *testing.T, cg *oculus.CallGraph, minNodes, minEdges int) {
	t.Helper()
	if cg == nil {
		t.Fatal("CallGraph is nil")
	}
	if len(cg.Nodes) < minNodes {
		t.Errorf("CallGraph: got %d nodes, want >= %d", len(cg.Nodes), minNodes)
	}
	if len(cg.Edges) < minEdges {
		t.Errorf("CallGraph: got %d edges, want >= %d", len(cg.Edges), minEdges)
	}
}
