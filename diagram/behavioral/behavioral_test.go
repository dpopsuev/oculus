package behavioral

import (
	"errors"
	"strings"
	"testing"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/diagram/core"
	"github.com/dpopsuev/oculus/testkit"
)

// --- Dataflow tests ---

func TestDataflow_Basic(t *testing.T) {
	in := core.Input{
		Root:         "/tmp",
		DeepAnalyzer: &testkit.StubDeepAnalyzer{},
	}
	out, err := Dataflow(in, core.Options{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "flowchart LR")
	assertContains(t, out, "HandleRequest")
	assertContains(t, out, "UserDB")
	assertContains(t, out, "Auth Zone")
	assertContains(t, out, "-->")
}

func TestDataflow_DefaultEntry(t *testing.T) {
	in := core.Input{
		Root:         "/tmp",
		DeepAnalyzer: &testkit.StubDeepAnalyzer{},
	}
	out, err := Dataflow(in, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "flowchart LR")
}

func TestDataflow_NodeShapes(t *testing.T) {
	in := core.Input{
		Root:         "/tmp",
		DeepAnalyzer: &testkit.StubDeepAnalyzer{},
	}
	out, err := Dataflow(in, core.Options{Entry: "main"})
	if err != nil {
		t.Fatal(err)
	}
	// Entry nodes get [[ ]] shape
	assertContains(t, out, "[[")
	// Data stores get [( )] shape
	assertContains(t, out, "[(")
	// External nodes get ([ ]) shape
	assertContains(t, out, "([")
}

func TestDataflow_WithTheme(t *testing.T) {
	theme := core.DefaultTheme().Resolve(core.ThemeNatural)
	in := core.Input{
		Root:          "/tmp",
		DeepAnalyzer:  &testkit.StubDeepAnalyzer{},
		ResolvedTheme: theme,
	}
	out, err := Dataflow(in, core.Options{Entry: "main"})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "init")
}

func TestDataflow_EdgeLabels(t *testing.T) {
	in := core.Input{
		Root:         "/tmp",
		DeepAnalyzer: &testkit.StubDeepAnalyzer{},
	}
	out, err := Dataflow(in, core.Options{Entry: "main"})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "Query")
	assertContains(t, out, "HTTP")
}

func TestDataflow_LayerComment(t *testing.T) {
	in := core.Input{
		Root:         "/tmp",
		DeepAnalyzer: &testkit.StubDeepAnalyzer{},
	}
	out, err := Dataflow(in, core.Options{Entry: "main"})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "layer: stub")
}

func TestDataflow_NoDeepAnalyzer(t *testing.T) {
	in := core.Input{Root: "/tmp"}
	_, err := Dataflow(in, core.Options{})
	if !errors.Is(err, core.ErrDeepAnalyzerRequired) {
		t.Errorf("got %v, want ErrDeepAnalyzerRequired", err)
	}
}

func TestDataflow_AnalyzerError(t *testing.T) {
	stub := &testkit.StubDeepAnalyzer{
		DataFlowErr: errors.New("analyzer down"),
	}
	in := core.Input{Root: "/tmp", DeepAnalyzer: stub}
	_, err := Dataflow(in, core.Options{Entry: "main"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- State tests ---

func TestState_Basic(t *testing.T) {
	in := core.Input{
		Root:         "/tmp",
		DeepAnalyzer: &testkit.StubDeepAnalyzer{},
	}
	out, err := State(in, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "stateDiagram-v2")
	assertContains(t, out, "Pending")
	assertContains(t, out, "Delivered")
	assertContains(t, out, "[*] --> Pending")
	assertContains(t, out, "confirm")
}

func TestState_WithTheme(t *testing.T) {
	theme := core.DefaultTheme().Resolve(core.ThemeNatural)
	in := core.Input{
		Root:          "/tmp",
		DeepAnalyzer:  &testkit.StubDeepAnalyzer{},
		ResolvedTheme: theme,
	}
	out, err := State(in, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "init")
}

func TestState_ScopeFilter(t *testing.T) {
	// Multiple machines — scope filters to matching ones
	machines := append(testkit.SampleStateMachines(), oculus.StateMachine{
		Name:    "TaskState",
		Package: "internal/task",
		States:  []string{"Open", "Closed"},
		Transitions: []oculus.StateTransition{
			{From: "Open", To: "Closed", Trigger: "close"},
		},
	})
	stub := &testkit.StubDeepAnalyzer{StateMachinesResult: machines}
	in := core.Input{Root: "/tmp", DeepAnalyzer: stub}
	out, err := State(in, core.Options{Scope: "order"})
	if err != nil {
		t.Fatal(err)
	}
	// Single machine after filter — no subgraph nesting, but transitions present
	assertContains(t, out, "Pending")
	assertContains(t, out, "confirm")
}

func TestState_ScopeNoMatch(t *testing.T) {
	in := core.Input{
		Root:         "/tmp",
		DeepAnalyzer: &testkit.StubDeepAnalyzer{},
	}
	// Non-matching scope still renders (falls back to all machines)
	out, err := State(in, core.Options{Scope: "nonexistent_pkg"})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "stateDiagram-v2")
}

func TestState_NoMachinesDetected(t *testing.T) {
	stub := &testkit.StubDeepAnalyzer{
		StateMachinesResult: []oculus.StateMachine{},
	}
	in := core.Input{Root: "/tmp", DeepAnalyzer: stub}
	out, err := State(in, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "No state machines detected")
}

func TestState_NoDeepAnalyzer(t *testing.T) {
	in := core.Input{Root: "/tmp"}
	_, err := State(in, core.Options{})
	if !errors.Is(err, core.ErrDeepAnalyzerRequired) {
		t.Errorf("got %v, want ErrDeepAnalyzerRequired", err)
	}
}

func TestState_AnalyzerError(t *testing.T) {
	stub := &testkit.StubDeepAnalyzer{
		StateMachinesErr: errors.New("detection failed"),
	}
	in := core.Input{Root: "/tmp", DeepAnalyzer: stub}
	_, err := State(in, core.Options{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestState_MultipleMachines(t *testing.T) {
	stub := &testkit.StubDeepAnalyzer{
		StateMachinesResult: append(testkit.SampleStateMachines(), testkit.SampleStateMachines()[0]),
	}
	stub.StateMachinesResult[1].Name = "TaskStatus"
	stub.StateMachinesResult[1].Package = "internal/task"
	in := core.Input{Root: "/tmp", DeepAnalyzer: stub}
	out, err := State(in, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// Multiple machines should produce subgraph nesting via "state"
	assertContains(t, out, "state")
	assertContains(t, out, "OrderStatus")
	assertContains(t, out, "TaskStatus")
}

// --- CallGraph tests ---

func TestCallGraph_Basic(t *testing.T) {
	in := core.Input{
		Root:         "/tmp",
		DeepAnalyzer: &testkit.StubDeepAnalyzer{},
	}
	out, err := CallGraph(in, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "flowchart TB")
	assertContains(t, out, "main")
	assertContains(t, out, "Run")
	assertContains(t, out, "-.->") // cross-package dotted edge
}

func TestCallGraph_WithTheme(t *testing.T) {
	theme := core.DefaultTheme().Resolve(core.ThemeNatural)
	in := core.Input{
		Root:          "/tmp",
		DeepAnalyzer:  &testkit.StubDeepAnalyzer{},
		ResolvedTheme: theme,
	}
	out, err := CallGraph(in, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "init")
}

func TestCallGraph_PackageSubgraphs(t *testing.T) {
	in := core.Input{
		Root:         "/tmp",
		DeepAnalyzer: &testkit.StubDeepAnalyzer{},
	}
	out, err := CallGraph(in, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "subgraph")
	assertContains(t, out, "end")
}

func TestCallGraph_NoDeepAnalyzer(t *testing.T) {
	in := core.Input{Root: "/tmp"}
	_, err := CallGraph(in, core.Options{})
	if !errors.Is(err, core.ErrDeepAnalyzerRequired) {
		t.Errorf("got %v, want ErrDeepAnalyzerRequired", err)
	}
}

func TestCallGraph_AnalyzerError(t *testing.T) {
	stub := &testkit.StubDeepAnalyzer{
		CallGraphErr: errors.New("callgraph failed"),
	}
	in := core.Input{Root: "/tmp", DeepAnalyzer: stub}
	_, err := CallGraph(in, core.Options{})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- Sequence tests ---

func TestSequence_Basic(t *testing.T) {
	in := core.Input{
		Root:     "/tmp",
		Analyzer: &testkit.StubTypeAnalyzer{},
	}
	out, err := Sequence(in, core.Options{Entry: "main"})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "sequenceDiagram")
	assertContains(t, out, "participant")
	assertContains(t, out, "->>")
}

func TestSequence_AutoEntry(t *testing.T) {
	in := core.Input{
		Root:     "/tmp",
		Analyzer: &testkit.StubTypeAnalyzer{},
	}
	out, err := Sequence(in, core.Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, out, "sequenceDiagram")
}

func TestSequence_NoAnalyzer(t *testing.T) {
	in := core.Input{Root: "/tmp"}
	_, err := Sequence(in, core.Options{Entry: "main"})
	if !errors.Is(err, core.ErrTypeAnalyzerRequired) {
		t.Errorf("got %v, want ErrTypeAnalyzerRequired", err)
	}
}

func TestSequence_NoCalls(t *testing.T) {
	stub := &testkit.StubTypeAnalyzer{
		CallChainResult: []oculus.Call{},
	}
	in := core.Input{Root: "/tmp", Analyzer: stub}
	_, err := Sequence(in, core.Options{Entry: "main"})
	if err == nil {
		t.Fatal("expected error for no calls")
	}
}

func TestSequence_NoEntryPoints(t *testing.T) {
	stub := &testkit.StubTypeAnalyzer{
		EntryPointsResult: []oculus.EntryPoint{},
		CallChainResult:   []oculus.Call{},
	}
	in := core.Input{Root: "/tmp", Analyzer: stub}
	_, err := Sequence(in, core.Options{})
	if err == nil {
		t.Fatal("expected error when no entry points")
	}
}

// --- sanitizeMermaid test ---

func TestSanitizeMermaid(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{`simple`, `simple`},
		{`with "quotes"`, `with 'quotes'`},
		{`with (parens)`, `with [parens]`},
		{`with {braces}`, `with [braces]`},
	}
	for _, tt := range tests {
		got := sanitizeMermaid(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeMermaid(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- helpers ---

func assertContains(t *testing.T, output, want string) {
	t.Helper()
	if !strings.Contains(output, want) {
		t.Errorf("output missing %q\noutput:\n%s", want, output)
	}
}
