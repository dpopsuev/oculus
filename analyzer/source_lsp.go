package analyzer

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus"
)

// LSPSymbolSource implements oculus.SymbolSource backed by an LSP server.
// It wraps the existing lspConn and exposes Roots/Children/Hover as
// source-agnostic operations for the SymbolPipeline.
type LSPSymbolSource struct {
	conn *lspConn
	root string
}

// NewLSPSymbolSource creates an LSP-backed SymbolSource.
func NewLSPSymbolSource(conn *lspConn, root string) *LSPSymbolSource {
	return &LSPSymbolSource{conn: conn, root: root}
}

// Verify interface compliance.
var _ oculus.SymbolSource = (*LSPSymbolSource)(nil)

func (s *LSPSymbolSource) Roots(ctx context.Context, query string) ([]oculus.Symbol, error) {
	if query != "" {
		// Single entry: find it via workspace/symbol + prepareCallHierarchy.
		item, err := s.conn.findCallHierarchyItem(s.root, query)
		if err != nil || item == nil {
			return nil, err
		}
		return []oculus.Symbol{s.itemToSymbol(item)}, nil
	}

	// All exported roots via workspace/symbol.
	result, err := s.conn.requestWith(ctx, "workspace/symbol", map[string]any{"query": ""})
	if err != nil {
		return nil, err
	}
	var symbols []workspaceSymbol
	if json.Unmarshal(result, &symbols) != nil {
		return nil, nil
	}

	seen := make(map[string]bool)
	var roots []oculus.Symbol
	for _, sym := range symbols {
		if sym.Kind != 12 && sym.Kind != 6 { // function or method
			continue
		}
		name := sym.Name
		if dot := strings.LastIndex(name, "."); dot >= 0 {
			name = name[dot+1:]
		}
		if !isExported(name) || seen[name] {
			continue
		}
		seen[name] = true

		// Prepare call hierarchy item for the Handle.
		item := s.conn.prepareCallHierarchyAt(
			sym.Location.URI,
			sym.Location.Range.Start.Line,
			sym.Location.Range.Start.Character,
		)
		roots = append(roots, oculus.Symbol{
			Name:    name,
			Package: uriToPackage(sym.Location.URI, s.root),
			File:    uriToRelPath(sym.Location.URI, s.root),
			Line:    sym.Location.Range.Start.Line + 1,
			Kind:    lspKindToString(sym.Kind),
			Handle:  item, // may be nil if prepareCallHierarchy fails
		})
	}
	return roots, nil
}

func (s *LSPSymbolSource) Children(ctx context.Context, sym oculus.Symbol) ([]oculus.SourceRelation, error) {
	item, ok := sym.Handle.(*callHierarchyItem)
	if !ok || item == nil {
		// No call hierarchy handle — try to resolve it.
		var err error
		item, err = s.conn.findCallHierarchyItem(s.root, sym.Name)
		if err != nil || item == nil {
			return nil, nil
		}
	}

	outgoing, err := s.conn.requestWith(ctx, "callHierarchy/outgoingCalls", map[string]any{"item": item})
	if err != nil {
		return nil, err
	}
	var outs []outgoingCallItem
	if json.Unmarshal(outgoing, &outs) != nil {
		return nil, nil
	}

	absRoot, _ := filepath.Abs(s.root)
	var rels []oculus.SourceRelation
	for _, out := range outs {
		inWorkspace := isWorkspaceURI(out.To.URI, absRoot)
		rels = append(rels, oculus.SourceRelation{
			Target:      s.itemToSymbol(&out.To),
			Kind:        "call",
			InWorkspace: inWorkspace,
		})
	}
	return rels, nil
}

func (s *LSPSymbolSource) Hover(ctx context.Context, sym oculus.Symbol) (*oculus.SourceTypeInfo, error) {
	file := sym.File
	if file == "" {
		return nil, nil
	}
	// Resolve absolute path for hover.
	if !filepath.IsAbs(file) {
		abs, _ := filepath.Abs(s.root)
		file = filepath.Join(abs, file)
	}

	line := sym.Line - 1 // Symbol uses 1-indexed, LSP uses 0-indexed
	if line < 0 {
		line = 0
	}

	hover, err := s.conn.hoverAtCtx(ctx, file, line, sym.Col)
	if err != nil || hover == "" {
		return nil, nil
	}

	sig := extractSignatureFromHover(hover)
	if sig == "" {
		return nil, nil
	}
	params, returns := parseSignatureTypes(sig)
	if len(params) == 0 && len(returns) == 0 {
		return nil, nil
	}
	return &oculus.SourceTypeInfo{
		ParamTypes:  params,
		ReturnTypes: returns,
		Signature:   sig,
	}, nil
}

func (s *LSPSymbolSource) itemToSymbol(item *callHierarchyItem) oculus.Symbol {
	return oculus.Symbol{
		Name:    item.Name,
		Package: uriToPackage(item.URI, s.root),
		File:    uriToRelPath(item.URI, s.root),
		Line:    item.Range.Start.Line + 1,
		Col:     item.Range.Start.Character,
		EndLine: item.Range.End.Line + 1,
		Kind:    lspKindToString(item.Kind),
		Handle:  item,
	}
}

// lspKindToString converts LSP SymbolKind int to domain string.
func lspKindToString(kind int) string {
	switch kind {
	case 5:
		return "class"
	case 6:
		return "method"
	case 11:
		return "interface"
	case 12:
		return "function"
	case 13:
		return "variable"
	case 23:
		return "struct"
	default:
		return "function"
	}
}
