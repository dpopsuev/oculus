package oculus

import "context"

// SymbolSource produces symbols and their relationships from source code.
// Each implementation uses a different strategy: LSP (remote server),
// GoAST (in-memory Go parsing), TreeSitter (universal parser).
//
// The generic SymbolPipeline uses SymbolSource to walk call graphs with
// bounded concurrency, timeout, and progress — decoupled from any specific
// symbol producer.
type SymbolSource interface {
	// Roots discovers entry-point symbols for graph traversal.
	// An empty query returns all exported functions; a specific name
	// returns matching symbols.
	Roots(ctx context.Context, query string) ([]SourceSymbol, error)

	// Children returns outgoing relationships from a symbol.
	// For call graphs, these are callees. For data flow, downstream nodes.
	Children(ctx context.Context, sym SourceSymbol) ([]SourceRelation, error)

	// Hover returns type/signature info for a symbol at its definition.
	Hover(ctx context.Context, sym SourceSymbol) (*SourceTypeInfo, error)
}

// SourceSymbol identifies a symbol discovered by a SymbolSource.
type SourceSymbol struct {
	Name    string `json:"name"`
	Package string `json:"package"`
	File    string `json:"file,omitempty"`
	Line    int    `json:"line,omitempty"`
	Col     int    `json:"col,omitempty"`
	EndLine int    `json:"end_line,omitempty"`
	Kind    int    `json:"kind,omitempty"` // LSP-compatible kind (12=function, 6=method)

	// Handle carries source-specific opaque data (e.g., callHierarchyItem
	// for LSP, *ast.FuncDecl for GoAST). The Pipeline passes it through
	// without inspecting it.
	Handle any `json:"-"`
}

// SourceRelation represents a directed relationship between two symbols.
type SourceRelation struct {
	Target      SourceSymbol `json:"target"`
	Kind        string       `json:"kind"` // "call", "data_store", "reference"
	InWorkspace bool         `json:"in_workspace"`
}

// SourceTypeInfo holds resolved type information for a symbol.
type SourceTypeInfo struct {
	ParamTypes  []string `json:"param_types,omitempty"`
	ReturnTypes []string `json:"return_types,omitempty"`
	Signature   string   `json:"signature,omitempty"`
}

// SourceFunc is the universal function descriptor that any language parser
// produces. FuncIndexSource wraps []SourceFunc into a SymbolSource —
// adding a language is just a parser function that returns []SourceFunc.
type SourceFunc struct {
	Name        string   `json:"name"`
	Package     string   `json:"package"`
	File        string   `json:"file"`
	Line        int      `json:"line"`
	EndLine     int      `json:"end_line"`
	ParamTypes  []string `json:"param_types,omitempty"`
	ReturnTypes []string `json:"return_types,omitempty"`
	Callees     []string `json:"callees,omitempty"`
	Exported    bool     `json:"exported"`
}
