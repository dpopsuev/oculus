package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"
)

// isWorkspaceURI checks if a file:// URI falls within the workspace root.
func isWorkspaceURI(uri, absRoot string) bool {
	path := strings.TrimPrefix(uri, "file://")
	return strings.HasPrefix(path, absRoot)
}

func init() {
	lspAvailable := func(root string, pool lsp.Pool) bool {
		if pool != nil {
			return true
		}
		detected := lang.DetectLanguage(root)
		cmd := lang.DefaultLSPServer(detected)
		if cmd == "" {
			return false
		}
		bin := strings.Fields(cmd)[0]
		_, err := exec.LookPath(bin)
		return err == nil
	}

	Register(lang.Unknown, 100, func(root string, pool lsp.Pool) oculus.DeepAnalyzer {
		if !lspAvailable(root, pool) {
			return nil
		}
		if pool != nil {
			return NewLSPDeepWithPool(root, pool)
		}
		return NewLSPDeep(root)
	}, func(root string, pool lsp.Pool) oculus.TypeAnalyzer {
		if !lspAvailable(root, pool) {
			return nil
		}
		if pool != nil {
			return &LSPAnalyzer{pool: pool}
		}
		return &LSPAnalyzer{}
	})
}

// LSPDeepAnalyzer uses a single gopls connection for all oculus.DeepAnalyzer
// methods. The connection is started lazily on first call and reused.
type LSPDeepAnalyzer struct {
	root    string
	timeout time.Duration
	pool    lsp.Pool
}

// NewLSPDeep creates a deep analyzer that will start gopls on first use.
func NewLSPDeep(root string) *LSPDeepAnalyzer {
	return &LSPDeepAnalyzer{root: root, timeout: 30 * time.Second}
}

// NewLSPDeepWithPool creates a deep analyzer backed by a connection pool.
func NewLSPDeepWithPool(root string, pool lsp.Pool) *LSPDeepAnalyzer {
	return &LSPDeepAnalyzer{root: root, timeout: 30 * time.Second, pool: pool}
}

func (a *LSPDeepAnalyzer) startConn(ctx context.Context) (*lspConn, func(), error) {
	analyzer := &LSPAnalyzer{Timeout: a.timeout, pool: a.pool}
	conn, cleanup, err := analyzer.startServer(a.root)
	if err != nil {
		return nil, nil, err
	}
	conn.ctx = ctx
	return conn, cleanup, nil
}

// oculus.CallGraph uses callHierarchy/outgoingCalls recursively from all
// exported functions (or a single entry if opts.Entry is set).
func (a *LSPDeepAnalyzer) CallGraph(ctx context.Context, _ string, opts oculus.CallGraphOpts) (*oculus.CallGraph, error) {
	conn, cleanup, err := a.startConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("lsp deep call graph: %w", err)
	}
	defer cleanup()

	depth := opts.Depth
	if depth <= 0 {
		depth = oculus.DefaultCallGraphDepth
	}

	absRoot, _ := filepath.Abs(a.root)

	// When entry is specified, the non-entry path (lspCallGraphRoots) that
	// normally opens files via documentSymbols is skipped. We must open files
	// explicitly so the LSP server indexes them before workspace/symbol queries.
	// documentSymbols is synchronous — it blocks until parsing is done.
	if opts.Entry != "" {
		for _, f := range findSrcFiles(a.root) {
			if ctx.Err() != nil {
				break
			}
			conn.documentSymbols(f, a.root)
		}
	}

	roots := lspCallGraphRoots(opts, conn, a.root)

	nodeSet := make(map[string]oculus.FuncNode)
	var edges []oculus.CallEdge
	visited := make(map[string]bool)
	sigCache := make(map[string]*[2][]string) // callee FQN → [paramTypes, returnTypes]

	for _, entry := range roots {
		if ctx.Err() != nil {
			break
		}
		item, err := conn.findCallHierarchyItem(a.root, entry)
		if err != nil || item == nil {
			continue
		}

		var walk func(it *callHierarchyItem, d int)
		walk = func(it *callHierarchyItem, d int) {
			if d > depth || visited[it.Name] {
				return
			}
			visited[it.Name] = true

			pkg := uriToPackage(it.URI, a.root)
			nodeSet[pkg+"."+it.Name] = oculus.FuncNode{
				Name: it.Name, Package: pkg, Line: it.Range.Start.Line + 1,
				File: uriToRelPath(it.URI, a.root), EndLine: it.Range.End.Line + 1,
			}

			outgoing, err := conn.Request("callHierarchy/outgoingCalls", map[string]any{"item": it})
			if err != nil {
				return
			}
			var outs []outgoingCallItem
			if json.Unmarshal(outgoing, &outs) != nil {
				return
			}
			for _, out := range outs {
				calleePkg := uriToPackage(out.To.URI, a.root)
				inWorkspace := isWorkspaceURI(out.To.URI, absRoot)
				if inWorkspace {
					nodeSet[calleePkg+"."+out.To.Name] = oculus.FuncNode{
						Name: out.To.Name, Package: calleePkg,
						Line: out.To.Range.Start.Line + 1,
						File: uriToRelPath(out.To.URI, a.root), EndLine: out.To.Range.End.Line + 1,
					}
				}
				// Extract callee signature via hover at callee's definition.
				calleeParams, calleeReturns := resolveCalleeTypes(
					conn, sigCache, out.To.Name, calleePkg,
					out.To.URI, out.To.Range.Start.Line, out.To.Range.Start.Character,
				)
				edges = append(edges, oculus.CallEdge{
					Caller:      it.Name,
					Callee:      out.To.Name,
					CallerPkg:   pkg,
					CalleePkg:   calleePkg,
					Line:        out.To.Range.Start.Line + 1,
					File:        uriToRelPath(it.URI, a.root),
					CrossPkg:    pkg != calleePkg,
					ParamTypes:  calleeParams,
					ReturnTypes: calleeReturns,
				})
				// Only recurse into workspace callees — external calls are
				// kept as leaf edges but we don't walk into stdlib internals.
				if inWorkspace {
					walk(&out.To, d+1)
				}
			}
		}
		walk(item, 0)
	}

	// Note: go/parser fallback enrichment is handled by the universal hook
	// in DeepFallbackAnalyzer.CallGraph — no need to call it here.

	nodes := make([]oculus.FuncNode, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}
	return &oculus.CallGraph{Nodes: nodes, Edges: edges, Layer: oculus.LayerLSP}, nil
}



// resolveCalleeTypes extracts callee param/return types via textDocument/hover
// at the callee's definition position. Cached by callee FQN.
func resolveCalleeTypes(
	conn *lspConn, cache map[string]*[2][]string,
	calleeName, calleePkg, calleeURI string, defLine, defCol int,
) (paramTypes, returnTypes []string) {
	fqn := calleePkg + "." + calleeName
	if cached, ok := cache[fqn]; ok {
		if cached != nil {
			return cached[0], cached[1]
		}
		return nil, nil
	}

	defPath := strings.TrimPrefix(calleeURI, "file://")
	hover, err := conn.hoverAt(defPath, defLine, defCol)
	if err != nil || hover == "" {
		cache[fqn] = nil
		return nil, nil
	}

	sig := extractSignatureFromHover(hover)
	if sig == "" {
		cache[fqn] = nil
		return nil, nil
	}
	params, returns := parseSignatureTypes(sig)
	if len(params) == 0 && len(returns) == 0 {
		cache[fqn] = nil
		return nil, nil
	}
	cache[fqn] = &[2][]string{params, returns}
	return params, returns
}

// extractSignatureFromHover pulls a function signature from LSP hover markdown.
// Language-agnostic: handles Go (func), Python (def), TypeScript (function),
// Rust (fn/pub fn), and C/C++ (return_type name(...)).
func extractSignatureFromHover(hover string) string {
	lines := strings.Split(hover, "\n")
	inBlock := false
	blockLang := ""
	// First pass: look inside code blocks (most LSP servers use markdown fences).
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inBlock {
				inBlock = false
				blockLang = ""
				continue
			}
			inBlock = true
			blockLang = strings.TrimPrefix(trimmed, "```")
			continue
		}
		if !inBlock {
			continue
		}
		if sig := matchSignatureLine(trimmed, blockLang); sig != "" {
			return sig
		}
	}
	// Second pass: scan non-fenced lines (some servers return plain text).
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			continue
		}
		if sig := matchSignatureLine(trimmed, ""); sig != "" {
			return sig
		}
	}
	return ""
}

// matchSignatureLine detects a function signature in a single hover line.
func matchSignatureLine(line, blockLang string) string {
	switch {
	// Go
	case strings.HasPrefix(line, "func "), strings.HasPrefix(line, "func("):
		return line
	// Python: "def foo(...)" or pyright "(function) def foo(...)"
	case strings.HasPrefix(line, "def "):
		return line
	case strings.Contains(line, ") def "):
		if idx := strings.Index(line, "def "); idx >= 0 {
			return line[idx:]
		}
	// TypeScript/JavaScript
	case strings.HasPrefix(line, "function "):
		return line
	// Rust
	case strings.HasPrefix(line, "fn "), strings.HasPrefix(line, "pub fn "):
		return line
	// C/C++: detected by code fence language since there's no keyword prefix
	case (blockLang == "c" || blockLang == "cpp" || blockLang == "c++") && strings.Contains(line, "("):
		return line
	}
	return ""
}

// DataFlowTrace uses callHierarchy to trace data flow from an entry,
// detecting data stores via workspace/symbol heuristics.
func (a *LSPDeepAnalyzer) DataFlowTrace(ctx context.Context, _, entry string, maxDepth int) (*oculus.DataFlow, error) {
	conn, cleanup, err := a.startConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("lsp deep dataflow: %w", err)
	}
	defer cleanup()

	if maxDepth <= 0 {
		maxDepth = oculus.DefaultDataFlowDepth
	}

	nodeMap := make(map[string]oculus.DataFlowNode)
	var edges []oculus.DataFlowEdge
	visited := make(map[string]bool)

	nodeMap[entry] = oculus.DataFlowNode{Name: entry, Kind: "entry"}

	item, err := conn.findCallHierarchyItem(a.root, entry)
	if err != nil || item == nil {
		return &oculus.DataFlow{
			Nodes: []oculus.DataFlowNode{{Name: entry, Kind: "entry"}},
			Layer: oculus.LayerLSP,
		}, nil
	}

	var trace func(it *callHierarchyItem, d int)
	trace = func(it *callHierarchyItem, d int) {
		if d > maxDepth || visited[it.Name] {
			return
		}
		visited[it.Name] = true

		pkg := uriToPackage(it.URI, a.root)
		if _, exists := nodeMap[it.Name]; !exists {
			nodeMap[it.Name] = oculus.DataFlowNode{Name: it.Name, Kind: "process", Pkg: pkg}
		}

		outgoing, err := conn.Request("callHierarchy/outgoingCalls", map[string]any{"item": it})
		if err != nil {
			return
		}
		var outs []outgoingCallItem
		if json.Unmarshal(outgoing, &outs) != nil {
			return
		}
		for _, out := range outs {
			lc := strings.ToLower(out.To.Name)
			isStore := strings.Contains(lc, "query") || strings.Contains(lc, "exec") ||
				strings.Contains(lc, "readfile") || strings.Contains(lc, "writefile")

			calleePkg := uriToPackage(out.To.URI, a.root)
			if isStore {
				storeName := calleePkg + " Store"
				if _, exists := nodeMap[storeName]; !exists {
					nodeMap[storeName] = oculus.DataFlowNode{Name: storeName, Kind: "data_store", Pkg: calleePkg}
				}
				edges = append(edges, oculus.DataFlowEdge{From: it.Name, To: storeName, Label: out.To.Name})
			} else {
				if _, exists := nodeMap[out.To.Name]; !exists {
					nodeMap[out.To.Name] = oculus.DataFlowNode{Name: out.To.Name, Kind: "process", Pkg: calleePkg}
				}
				edges = append(edges, oculus.DataFlowEdge{From: it.Name, To: out.To.Name})
			}
			trace(&out.To, d+1)
		}
	}

	trace(item, 0)

	nodes := make([]oculus.DataFlowNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	return &oculus.DataFlow{Nodes: nodes, Edges: edges, Layer: oculus.LayerLSP}, nil
}

// DetectStateMachines uses documentSymbol to find const groups and
// then workspace/symbol + textDocument/references to find switch contexts.
func (a *LSPDeepAnalyzer) DetectStateMachines(ctx context.Context, _ string) ([]oculus.StateMachine, error) {
	conn, cleanup, err := a.startConn(ctx)
	if err != nil {
		return nil, fmt.Errorf("lsp deep state machines: %w", err)
	}
	defer cleanup()

	files := findSrcFiles(a.root)
	var machines []oculus.StateMachine
	seen := make(map[string]bool)

	for _, f := range files {
		syms, err := conn.documentSymbols(f, a.root)
		if err != nil {
			continue
		}

		// Look for const groups that might represent state enums
		for _, sym := range syms {
			if sym.Kind != 14 {
				continue
			}
			if len(sym.Children) < 2 {
				continue
			}

			typeName := sym.Name
			if seen[typeName] {
				continue
			}
			seen[typeName] = true

			var states []string
			for _, ch := range sym.Children {
				states = append(states, ch.Name)
			}

			initial := states[0]
			for _, s := range states {
				ls := strings.ToLower(s)
				if strings.Contains(ls, "initial") || strings.Contains(ls, "new") ||
					strings.Contains(ls, "start") || strings.Contains(ls, "idle") {
					initial = s
					break
				}
			}

			machines = append(machines, oculus.StateMachine{
				Name:    typeName,
				Package: uriToPackage(pathToURI(f), a.root),
				States:  states,
				Initial: initial,
			})
		}
	}

	return machines, nil
}

// lspCallGraphRoots determines the root functions for call graph analysis.
func lspCallGraphRoots(opts oculus.CallGraphOpts, conn *lspConn, root string) []string {
	if opts.Entry != "" {
		return []string{opts.Entry}
	}
	files := findSrcFiles(root)
	seen := make(map[string]bool)
	var roots []string
	for _, f := range files {
		if conn.ctx != nil && conn.ctx.Err() != nil {
			break
		}
		syms, err := conn.documentSymbols(f, root)
		if err != nil {
			continue
		}
		for _, sym := range syms {
			if !isExported(sym.Name) {
				continue
			}
			if sym.Kind != 12 && sym.Kind != 6 {
				continue
			}
			if !seen[sym.Name] {
				seen[sym.Name] = true
				roots = append(roots, sym.Name)
			}
		}
	}
	return roots
}
