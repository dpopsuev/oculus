package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"
	"golang.org/x/sync/errgroup"
)

const (
	lspIndexingRetries = 60
	lspIndexingBackoff = 3 * time.Second
)

// LSPConcurrency is the max number of concurrent call tree walks.
// Tunable via benchmark. Default 8 balances throughput vs LSP server load.
var LSPConcurrency = 8

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

// oculus.CallGraph uses a top-down call tree approach:
// 1. workspace/symbol to discover roots (1 call, not N file scans)
// 2. prepareCallHierarchy + outgoingCalls to walk the tree lazily
// 3. Semaphore-bounded goroutines for concurrent root processing
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

	// Step 1: Discover roots via workspace/symbol (1 call, not 549 file scans).
	// For a specific entry, just open that one file.
	roots, err := lspCallGraphRoots(opts, conn, a.root)
	if err != nil {
		return nil, fmt.Errorf("lsp call graph roots: %w", err)
	}

	// Step 2: Walk the call tree from roots with bounded concurrency.
	var mu sync.Mutex
	nodeSet := make(map[string]oculus.Symbol)
	var edges []oculus.CallEdge
	visited := make(map[string]bool)
	// sigCache is read and written from multiple goroutines; protect with sigMu.
	var sigMu sync.Mutex
	sigCache := make(map[string]*[2][]string)

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(LSPConcurrency)

	var walk func(it *callHierarchyItem, d int)
	walk = func(it *callHierarchyItem, d int) {
		if gCtx.Err() != nil {
			return
		}

		mu.Lock()
		if d > depth || visited[it.Name] {
			mu.Unlock()
			return
		}
		visited[it.Name] = true
		mu.Unlock()

		pkg := uriToPackage(it.URI, a.root)

		mu.Lock()
		nodeSet[pkg+"."+it.Name] = oculus.Symbol{
			Name: it.Name, Package: pkg, Line: it.Range.Start.Line + 1,
			File: uriToRelPath(it.URI, a.root), EndLine: it.Range.End.Line + 1,
		}
		mu.Unlock()

		outgoing, err := conn.request("callHierarchy/outgoingCalls", map[string]any{"item": it})
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

			calleeParams, calleeReturns := resolveCalleeTypes(
				conn, sigCache, &sigMu, out.To.Name, calleePkg,
				out.To.URI, out.To.Range.Start.Line, out.To.Range.Start.Character,
			)

			mu.Lock()
			if inWorkspace {
				nodeSet[calleePkg+"."+out.To.Name] = oculus.Symbol{
					Name: out.To.Name, Package: calleePkg,
					Line: out.To.Range.Start.Line + 1,
					File: uriToRelPath(out.To.URI, a.root), EndLine: out.To.Range.End.Line + 1,
				}
			}
			// Extract call site position from the first fromRange (if present).
			var siteLine, siteCol int
			if len(out.FromRanges) > 0 {
				siteLine = out.FromRanges[0].Start.Line + 1
				siteCol = out.FromRanges[0].Start.Character + 1
			}
			callerFile := uriToRelPath(it.URI, a.root)
			kind := asyncContextAt(a.root, callerFile, siteLine, siteCol)
			edges = append(edges, oculus.CallEdge{
				Caller:      it.Name,
				Callee:      out.To.Name,
				CallerPkg:   pkg,
				CalleePkg:   calleePkg,
				Line:        out.To.Range.Start.Line + 1,
				File:        callerFile,
				CrossPkg:    pkg != calleePkg,
				ParamTypes:  calleeParams,
				ReturnTypes: calleeReturns,
				Kind:        kind,
				SiteLine:    siteLine,
				SiteCol:     siteCol,
			})
			mu.Unlock()

			if inWorkspace {
				walk(&out.To, d+1)
			}
		}
	}

	for _, entry := range roots {
		if gCtx.Err() != nil {
			break
		}
		fileHint, lineHint := opts.EntryFile, opts.EntryLine
		if opts.Entry != "" && entry != opts.Entry {
			fileHint, lineHint = "", 0
		}
		item, err := conn.findCallHierarchyItem(a.root, entry, fileHint, lineHint, opts.Interactive)
		if err != nil || item == nil {
			continue
		}
		g.Go(func() error {
			walk(item, 0)
			return nil
		})
	}
	_ = g.Wait()

	// Note: go/parser fallback enrichment is handled by the universal hook
	// in DeepFallbackAnalyzer.CallGraph — no need to call it here.

	nodes := make([]oculus.Symbol, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}
	return &oculus.CallGraph{Nodes: nodes, Edges: edges, Layer: oculus.LayerLSP}, nil
}

// resolveCalleeTypes extracts callee param/return types via textDocument/hover
// at the callee's definition position. Cached by callee FQN under mu.
func resolveCalleeTypes(
	conn *lspConn, cache map[string]*[2][]string, mu *sync.Mutex,
	calleeName, calleePkg, calleeURI string, defLine, defCol int,
) (paramTypes, returnTypes []string) {
	fqn := calleePkg + "." + calleeName
	mu.Lock()
	cached, hit := cache[fqn]
	mu.Unlock()
	if hit {
		if cached != nil {
			return cached[0], cached[1]
		}
		return nil, nil
	}

	defPath := strings.TrimPrefix(calleeURI, "file://")
	hover, err := conn.hoverAt(defPath, defLine, defCol)
	if err != nil || hover == "" {
		mu.Lock()
		cache[fqn] = nil
		mu.Unlock()
		return nil, nil
	}

	sig := extractSignatureFromHover(hover)
	if sig == "" {
		mu.Lock()
		cache[fqn] = nil
		mu.Unlock()
		return nil, nil
	}
	params, returns := parseSignatureTypes(sig)
	if len(params) == 0 && len(returns) == 0 {
		mu.Lock()
		cache[fqn] = nil
		mu.Unlock()
		return nil, nil
	}
	mu.Lock()
	cache[fqn] = &[2][]string{params, returns}
	mu.Unlock()
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

	item, err := conn.findCallHierarchyItem(a.root, entry, "", 0, false)
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

// lspCallGraphRoots discovers root functions via workspace/symbol (1 call)
// instead of scanning all files with documentSymbol (N calls).
func lspCallGraphRoots(opts oculus.CallGraphOpts, conn *lspConn, root string) ([]string, error) {
	if opts.Entry != "" {
		return []string{opts.Entry}, nil
	}
	if opts.ExportedOnly {
		return lspExportedRoots(conn, root)
	}
	// Default: all exported functions
	return lspExportedRoots(conn, root)
}

// lspExportedRoots uses workspace/symbol to find all exported functions.
// Retries with backoff if the server returns empty (indexing in progress).
// Falls back to documentSymbol when workspace/symbol errors or stays empty
// (for example TypeScript monorepos returning "No Project").
func lspExportedRoots(conn *lspConn, root string) ([]string, error) {
	detected := lang.DetectLanguage(root)
	allowEarlyFallback := detected == lang.TypeScript || detected == lang.JavaScript

	var symbols []workspaceSymbol
	var lastErr error
	primed := false

	for attempt := range lspIndexingRetries {
		if attempt > 0 {
			if err := indexingBackoff(conn.ctx, lspIndexingBackoff); err != nil {
				return nil, err
			}
		}
		result, err := conn.request("workspace/symbol", map[string]any{"query": "."})
		if err != nil {
			lastErr = err
			if errors.Is(err, lsp.ErrServerDead) {
				return nil, err
			}
			if !primed {
				primeWorkspace(conn, root)
				primed = true
				continue
			}
			// TypeScript/JavaScript often report "No Project" at repo root.
			// Don't spin for minutes when we can fall back to documentSymbol.
			if allowEarlyFallback || isNoProjectError(err) {
				break
			}
			continue
		}
		if err := json.Unmarshal(result, &symbols); err != nil {
			return nil, fmt.Errorf("workspace/symbol decode: %w", err)
		}
		if len(symbols) > 0 {
			break
		}
		if !primed {
			primeWorkspace(conn, root)
			primed = true
		}
		if allowEarlyFallback && attempt >= 2 {
			break
		}
		slog.LogAttrs(conn.ctx, slog.LevelInfo, "lsp: workspace/symbol empty, server may be indexing",
			slog.Int("attempt", attempt+1),
			slog.Int("max_retries", lspIndexingRetries),
			slog.String("raw_response", string(result)),
		)
	}
	if roots := exportedRootsFromWorkspaceSymbols(symbols); len(roots) > 0 {
		return roots, nil
	}

	roots := lspExportedRootsFromDocumentSymbols(conn, root)
	if len(roots) > 0 {
		if lastErr != nil {
			slog.LogAttrs(conn.ctx, slog.LevelWarn, "lsp: root discovery fallback to documentSymbol",
				slog.Any("workspace_symbol_error", lastErr),
				slog.Int("roots", len(roots)),
			)
		}
		return roots, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, nil
}

func exportedRootsFromWorkspaceSymbols(symbols []workspaceSymbol) []string {
	seen := make(map[string]bool)
	roots := make([]string, 0, len(symbols))
	for _, s := range symbols {
		if s.Kind != 12 && s.Kind != 6 { // function or method
			continue
		}
		name := s.Name
		if dot := strings.LastIndex(name, "."); dot >= 0 {
			name = name[dot+1:]
		}
		if !isExported(name) || seen[name] {
			continue
		}
		seen[name] = true
		roots = append(roots, name)
	}
	return roots
}

func lspExportedRootsFromDocumentSymbols(conn *lspConn, root string) []string {
	seen := make(map[string]bool)
	var roots []string
	for _, f := range findSrcFiles(root) {
		if conn.ctx != nil && conn.ctx.Err() != nil {
			break
		}
		syms, err := conn.documentSymbols(f, root)
		if err != nil {
			continue
		}
		for _, sym := range syms {
			if sym.Kind != 12 && sym.Kind != 6 {
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
			roots = append(roots, name)
		}
	}
	return roots
}

func primeWorkspace(conn *lspConn, root string) {
	for _, f := range findSrcFiles(root) {
		if conn.ctx != nil && conn.ctx.Err() != nil {
			return
		}
		conn.ensureOpen(f)
	}
}

func isNoProjectError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no project")
}

// indexingBackoff sleeps for d or until ctx is cancelled, whichever comes
// first. Returns ctx.Err() on cancellation, nil after a full sleep.
func indexingBackoff(ctx context.Context, d time.Duration) error {
	if ctx == nil {
		time.Sleep(d)
		return nil
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
