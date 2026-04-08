package testkit

import (
	"errors"
	"testing"

	"github.com/dpopsuev/oculus"
)

func TestStubDeepAnalyzer_Defaults(t *testing.T) {
	stub := &StubDeepAnalyzer{}

	cg, err := stub.CallGraph(".", oculus.CallGraphOpts{})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	AssertCallGraph(t, cg, 3, 2)

	flow, err := stub.DataFlowTrace(".", "main", 5)
	if err != nil {
		t.Fatalf("DataFlowTrace: %v", err)
	}
	AssertDataFlow(t, flow, 4, 3)
	AssertDataFlowHasNode(t, flow, "HandleRequest", "process")
	AssertDataFlowHasNode(t, flow, "UserDB", "data_store")
	AssertDataFlowHasEdge(t, flow, "HandleRequest", "UserDB")
	AssertDataFlowHasBoundary(t, flow, "Auth Zone")

	machines, err := stub.DetectStateMachines(".")
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

	flow, err := stub.DataFlowTrace(".", "x", 1)
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

	if _, err := stub.CallGraph(".", oculus.CallGraphOpts{}); !errors.Is(err, errTest) {
		t.Errorf("CallGraph: got %v, want %v", err, errTest)
	}
	if _, err := stub.DataFlowTrace(".", "x", 1); !errors.Is(err, errTest) {
		t.Errorf("DataFlowTrace: got %v, want %v", err, errTest)
	}
	if _, err := stub.DetectStateMachines("."); !errors.Is(err, errTest) {
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

	flow := SampleDataFlow()
	if flow.Layer != "stub" {
		t.Errorf("SampleDataFlow layer: got %q, want %q", flow.Layer, "stub")
	}

	machines := SampleStateMachines()
	if machines[0].Initial != "Pending" {
		t.Errorf("SampleStateMachines initial: got %q, want %q", machines[0].Initial, "Pending")
	}
}
