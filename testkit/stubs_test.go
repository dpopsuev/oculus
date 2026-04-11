package testkit

import (
	"context"
	"errors"
	"testing"

	"github.com/dpopsuev/oculus"
)

func TestStubDeepAnalyzer_Defaults(t *testing.T) {
	stub := &StubDeepAnalyzer{}

	cg, err := stub.CallGraph(context.Background(), ".", oculus.CallGraphOpts{})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	AssertCallGraph(t, cg, 3, 2)

	flow, err := stub.DataFlowTrace(context.Background(), ".", "main", 5)
	if err != nil {
		t.Fatalf("DataFlowTrace: %v", err)
	}
	AssertDataFlow(t, flow, 4, 3)
	AssertDataFlowHasNode(t, flow, "HandleRequest", "process")
	AssertDataFlowHasNode(t, flow, "UserDB", "data_store")
	AssertDataFlowHasEdge(t, flow, "HandleRequest", "UserDB")
	AssertDataFlowHasBoundary(t, flow, "Auth Zone")

	machines, err := stub.DetectStateMachines(context.Background(), ".")
	if err != nil {
		t.Fatalf("DetectStateMachines: %v", err)
	}
	AssertStateMachines(t, machines, 1)
	AssertStateMachineNamed(t, machines, "OrderStatus", 4, 3)
}

func TestStubDeepAnalyzer_CustomResults(t *testing.T) {
	custom := &oculus.DataFlow{
		Nodes: []oculus.DataFlowNode{{Name: "A", Kind: "process"}},
		Edges: []oculus.DataFlowEdge{{From: "A", To: "B"}},
	}
	stub := &StubDeepAnalyzer{DataFlowResult: custom}

	flow, err := stub.DataFlowTrace(context.Background(), ".", "x", 1)
	if err != nil {
		t.Fatalf("DataFlowTrace: %v", err)
	}
	if len(flow.Nodes) != 1 || flow.Nodes[0].Name != "A" {
		t.Errorf("expected custom result, got %v", flow.Nodes)
	}
}

func TestStubDeepAnalyzer_Errors(t *testing.T) {
	errTest := errors.New("test error")
	stub := &StubDeepAnalyzer{
		CallGraphErr:     errTest,
		DataFlowErr:      errTest,
		StateMachinesErr: errTest,
	}

	if _, err := stub.CallGraph(context.Background(), ".", oculus.CallGraphOpts{}); !errors.Is(err, errTest) {
		t.Errorf("CallGraph: got %v, want %v", err, errTest)
	}
	if _, err := stub.DataFlowTrace(context.Background(), ".", "x", 1); !errors.Is(err, errTest) {
		t.Errorf("DataFlowTrace: got %v, want %v", err, errTest)
	}
	if _, err := stub.DetectStateMachines(context.Background(), "."); !errors.Is(err, errTest) {
		t.Errorf("DetectStateMachines: got %v, want %v", err, errTest)
	}
}

func TestStubTypeAnalyzer_Defaults(t *testing.T) {
	stub := &StubTypeAnalyzer{}

	calls, err := stub.CallChain(".", "main", 5)
	if err != nil {
		t.Fatalf("CallChain: %v", err)
	}
	if len(calls) != 3 {
		t.Errorf("CallChain: got %d calls, want 3", len(calls))
	}

	eps, err := stub.EntryPoints(".")
	if err != nil {
		t.Fatalf("EntryPoints: %v", err)
	}
	if len(eps) != 1 || eps[0].Name != "main" {
		t.Errorf("EntryPoints: got %v, want [main]", eps)
	}
}

func TestSampleFactories(t *testing.T) {
	cg := SampleCallGraph()
	if cg.Layer != "stub" {
		t.Errorf("SampleCallGraph layer: got %q, want %q", cg.Layer, "stub")
	}
	// Verify location metadata on FuncNodes
	for _, n := range cg.Nodes {
		if n.File == "" {
			t.Errorf("Symbol %q: missing File", n.Name)
		}
		if n.EndLine == 0 {
			t.Errorf("Symbol %q: missing EndLine", n.Name)
		}
	}
	// Verify location metadata on CallEdges
	for _, e := range cg.Edges {
		if e.File == "" {
			t.Errorf("CallEdge %s->%s: missing File", e.Caller, e.Callee)
		}
		if e.Line == 0 {
			t.Errorf("CallEdge %s->%s: missing Line", e.Caller, e.Callee)
		}
	}

	flow := SampleDataFlow()
	if flow.Layer != "stub" {
		t.Errorf("SampleDataFlow layer: got %q, want %q", flow.Layer, "stub")
	}

	machines := SampleStateMachines()
	if machines[0].Initial != "Pending" {
		t.Errorf("SampleStateMachines initial: got %q, want %q", machines[0].Initial, "Pending")
	}

	// Verify location on call chain
	chain := SampleCallChain()
	for _, c := range chain {
		if c.File == "" {
			t.Errorf("Call %s->%s: missing File", c.Caller, c.Callee)
		}
	}

	// Verify location on entry points
	eps := SampleEntryPoints()
	if eps[0].EndLine == 0 {
		t.Errorf("EntryPoint %q: missing EndLine", eps[0].Name)
	}
}
