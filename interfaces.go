package oculus

import "context"

// --- ISP: Role-specific interfaces ---

// ClassAnalyzer extracts type declarations and implementation relationships.
type ClassAnalyzer interface {
	Classes(root string) ([]ClassInfo, error)
	Implements(root string) ([]ImplEdge, error)
}

// CallAnalyzer extracts call chains and entry points.
type CallAnalyzer interface {
	CallChain(root, entry string, depth int) ([]Call, error)
	EntryPoints(root string) ([]EntryPoint, error)
}

// MetricAnalyzer extracts structural metrics (nesting, field references).
type MetricAnalyzer interface {
	FieldRefs(root string) ([]FieldRef, error)
	NestingDepth(root string) ([]NestingResult, error)
}

// TypeAnalyzer extracts type-level structural metadata from source code.
// Composed of ClassAnalyzer + CallAnalyzer + MetricAnalyzer.
type TypeAnalyzer interface {
	ClassAnalyzer
	CallAnalyzer
	MetricAnalyzer
}

// DeepAnalyzer extracts cross-function, cross-package structural
// information for call graphs, data flow, and state machines.
type DeepAnalyzer interface {
	CallGraph(ctx context.Context, root string, opts CallGraphOpts) (*CallGraph, error)
	DataFlowTrace(ctx context.Context, root, entry string, depth int) (*DataFlow, error)
	DetectStateMachines(ctx context.Context, root string) ([]StateMachine, error)
}
