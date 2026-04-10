// Package oculus provides language-agnostic symbol resolution, call graph
// analysis, and type system inspection.
//
// Zero external domain dependencies — all types are self-contained.
package oculus

// ClassInfo describes a type declaration (struct, interface, class, trait).
type ClassInfo struct {
	Name     string       `json:"name"`
	Package  string       `json:"package"`
	Kind     string       `json:"kind"` // "struct", "interface", "class", "trait"
	Fields   []FieldInfo  `json:"fields,omitempty"`
	Methods  []MethodInfo `json:"methods,omitempty"`
	Exported bool         `json:"exported"`
	File     string       `json:"file,omitempty"`
	Line     int          `json:"line,omitempty"`
	EndLine  int          `json:"end_line,omitempty"`
}

// FieldInfo describes a single field within a type.
type FieldInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Exported bool   `json:"exported"`
	Tag      string `json:"tag,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// MethodInfo describes a method on a type.
type MethodInfo struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
	Exported  bool   `json:"exported"`
	File      string `json:"file,omitempty"`
	Line      int    `json:"line,omitempty"`
	EndLine   int    `json:"end_line,omitempty"`
}

// ImplEdge captures a type relationship (implements, extends, embeds).
type ImplEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // "implements", "extends", "embeds"
}

// FieldRef captures a struct field that references another declared type.
type FieldRef struct {
	Owner   string `json:"owner"`
	Field   string `json:"field"`
	RefType string `json:"ref_type"`
}

// Call represents a single call in a call chain.
type Call struct {
	Caller  string `json:"caller"`
	Callee  string `json:"callee"`
	Package string `json:"package"`
	Line    int    `json:"line,omitempty"`
	File    string `json:"file,omitempty"`
}

// EntryPoint represents a structurally significant entry function.
type EntryPoint struct {
	Name    string `json:"name"`
	Kind    string `json:"kind"` // "main", "http_handler", "cli_command", "test"
	Package string `json:"package"`
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	EndLine int    `json:"end_line,omitempty"`
}

// NestingResult holds the maximum nesting depth for a single function.
type NestingResult struct {
	Function string `json:"function"`
	Package  string `json:"package"`
	MaxDepth int    `json:"max_depth"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// CallGraphOpts configures call graph construction.
type CallGraphOpts struct {
	Entry        string // entry function name; empty = all exported
	Depth        int    // max recursion depth; 0 = default (10)
	ExportedOnly bool   // only include exported functions as roots
	Scope        string // limit to this package prefix

	// OnProgress is called after each root function is resolved.
	// Optional — nil means no progress notifications.
	OnProgress func(ProgressUpdate)
}

// ProgressUpdate reports incremental progress during call graph construction.
type ProgressUpdate struct {
	RootsResolved int    // roots walked so far
	RootsTotal    int    // total roots to walk
	NodesFound    int    // nodes discovered so far
	EdgesFound    int    // edges discovered so far
	Message       string // human-readable status
}

// CallEdge represents a single caller->callee edge in the call graph.
type CallEdge struct {
	Caller       string   `json:"caller"`
	Callee       string   `json:"callee"`
	CallerPkg    string   `json:"caller_pkg"`
	CalleePkg    string   `json:"callee_pkg"`
	Line         int      `json:"line,omitempty"`
	EndLine      int      `json:"end_line,omitempty"`
	File         string   `json:"file,omitempty"`
	ReceiverType string   `json:"receiver_type,omitempty"`
	CrossPkg     bool     `json:"cross_pkg,omitempty"`
	ParamTypes   []string `json:"param_types,omitempty"`
	ReturnTypes  []string `json:"return_types,omitempty"`
}

// Symbol is the canonical representation of any code symbol.
// Each scanner enriches the fields it knows about — no conversions at boundaries.
type Symbol struct {
	// Identity (set by any scanner)
	Name     string `json:"name"`
	Package  string `json:"package"`
	Kind     string `json:"kind,omitempty"` // "function", "struct", "interface", "method", "field"
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Col      int    `json:"col,omitempty"`
	EndLine  int    `json:"end_line,omitempty"`
	Exported bool   `json:"exported,omitempty"`

	// Type enrichment (GoAST, TreeSitter, LSP hover)
	ParamTypes   []string `json:"param_types,omitempty"`
	ReturnTypes  []string `json:"return_types,omitempty"`
	Signature    string   `json:"signature,omitempty"`
	ReceiverType string   `json:"receiver_type,omitempty"`

	// Structure enrichment (GoAST, TreeSitter, LSP callHierarchy)
	Callees []string `json:"callees,omitempty"`

	// Handle for source-specific opaque data (LSP callHierarchyItem, GoAST *ast.FuncDecl)
	Handle any `json:"-"`
}

// FQN returns the fully-qualified name: "package.Name".
func (s Symbol) FQN() string {
	if s.Package == "" {
		return s.Name
	}
	return s.Package + "." + s.Name
}

// FuncNode is an alias for Symbol — backward compatibility for call graph results.
type FuncNode = Symbol

// CallGraph is the result of call graph analysis.
type CallGraph struct {
	Nodes []FuncNode `json:"nodes"`
	Edges []CallEdge `json:"edges"`
	Layer string     `json:"layer,omitempty"`
}

// DataFlowNode represents a participant in a data flow.
type DataFlowNode struct {
	Name string `json:"name"`
	Kind string `json:"kind"` // "process", "data_store", "external", "entry"
	Pkg  string `json:"package,omitempty"`
}

// DataFlowEdge represents data moving between nodes.
type DataFlowEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label,omitempty"`
}

// TrustBoundary defines a security boundary containing nodes.
type TrustBoundary struct {
	Name  string   `json:"name"`
	Nodes []string `json:"nodes"`
}

// DataFlow is the result of data flow analysis.
type DataFlow struct {
	Nodes      []DataFlowNode  `json:"nodes"`
	Edges      []DataFlowEdge  `json:"edges"`
	Boundaries []TrustBoundary `json:"boundaries,omitempty"`
	Layer      string          `json:"layer,omitempty"`
}

// StateTransition represents a transition between states.
type StateTransition struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Trigger string `json:"trigger,omitempty"`
}

// StateMachine represents a detected state machine pattern.
type StateMachine struct {
	Name        string            `json:"name"`
	Package     string            `json:"package"`
	States      []string          `json:"states"`
	Transitions []StateTransition `json:"transitions"`
	Initial     string            `json:"initial,omitempty"`
}

// Convention represents a detected coding convention.
type Convention struct {
	Category string   `json:"category"` // naming, structure, style
	Pattern  string   `json:"pattern"`
	Examples []string `json:"examples,omitempty"`
	Count    int      `json:"count"`
}

// ConventionReport holds the result of convention detection.
type ConventionReport struct {
	Conventions []Convention `json:"conventions"`
	Total       int          `json:"total"`
}

// SymbolNode is an alias for Symbol — backward compatibility for symbol graph results.
type SymbolNode = Symbol

// SymbolEdge represents a typed, directed relationship between two symbols.
// Satisfies graph.Edge via Source()/Target().
type SymbolEdge struct {
	SourceFQN   string   `json:"source"`
	TargetFQN   string   `json:"target"`
	Kind        string   `json:"kind"` // "call", "implements", "extends", "embeds", "field_ref"
	File        string   `json:"file,omitempty"`
	Line        int      `json:"line,omitempty"`
	EndLine     int      `json:"end_line,omitempty"`
	ParamTypes  []string `json:"param_types,omitempty"`
	ReturnTypes []string `json:"return_types,omitempty"`
	Weight      float64  `json:"weight,omitempty"`
}

// Source implements graph.Edge.
func (e SymbolEdge) Source() string { return e.SourceFQN }

// Target implements graph.Edge.
func (e SymbolEdge) Target() string { return e.TargetFQN }

// SymbolGraph is the unified symbol-level graph result.
type SymbolGraph struct {
	Nodes []SymbolNode `json:"nodes"`
	Edges []SymbolEdge `json:"edges"`
}

// PipelineStep represents a single function in a detected pipeline.
type PipelineStep struct {
	FQN         string   `json:"fqn"`
	ParamTypes  []string `json:"param_types,omitempty"`
	ReturnTypes []string `json:"return_types,omitempty"`
}

// Pipeline represents a detected data transformation pipeline:
// a linear call chain where each function's return types overlap
// with the next function's parameter types.
type Pipeline struct {
	Steps     []PipelineStep `json:"steps"`
	TypeChain []string       `json:"type_chain"`
	Length    int             `json:"length"`
}

// PipelineReport holds the result of pipeline detection.
type PipelineReport struct {
	Pipelines []Pipeline `json:"pipelines"`
	Summary   string     `json:"summary"`
}

// MeshLevel represents a zoom level in the hierarchical mesh.
type MeshLevel int

const (
	MeshSymbol    MeshLevel = iota // individual function/type
	MeshFile                       // source file
	MeshPackage                    // Go package / module namespace
	MeshComponent                  // architectural component (ArchService)
)

// MeshNode represents a node at a specific level in the mesh hierarchy.
// Overlay fields are enriched by OverlayMesh from existing analysis passes.
type MeshNode struct {
	Name     string    `json:"name"`
	Level    MeshLevel `json:"level"`
	Parent   string    `json:"parent,omitempty"`
	Children []string  `json:"children,omitempty"`

	// Overlay: HEXA role (from clinic/hexa classification)
	Role string `json:"role,omitempty"` // "domain", "adapter", "port", "infra", "entrypoint", "app"

	// Overlay: stability metrics (from graph.FanIn/FanOut)
	FanIn       int     `json:"fan_in,omitempty"`
	FanOut      int     `json:"fan_out,omitempty"`
	Instability float64 `json:"instability,omitempty"` // Ce/(Ca+Ce), 0=stable, 1=unstable

	// Overlay: trust zone (from HEXA violation boundaries)
	TrustZone string `json:"trust_zone,omitempty"`

	// Overlay: choke point score (betweenness centrality)
	ChokeScore float64 `json:"choke_score,omitempty"`

	// Overlay: circuit membership (bidirectional coupling group)
	CircuitID int `json:"circuit_id,omitempty"` // 0 = not in a circuit
}

// Mesh is a hierarchical view of the codebase: symbols nested in files,
// files in packages, packages in components. Edges overlay the hierarchy
// at symbol level and can be aggregated upward.
type Mesh struct {
	Nodes map[string]MeshNode `json:"nodes"`
	Edges []SymbolEdge        `json:"edges"`
}
