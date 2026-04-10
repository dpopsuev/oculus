package oculus

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"golang.org/x/sync/errgroup"
)

// DefaultPipelineConcurrency is the max number of concurrent walks.
const DefaultPipelineConcurrency = 8

// SymbolPipeline walks a SymbolSource with bounded concurrency to produce
// CallGraph, DataFlow, and StateMachine results. It implements DeepAnalyzer.
//
// The pipeline owns the concurrency strategy (errgroup, timeout, progress).
// The SymbolSource owns symbol discovery and navigation.
// Downstream consumers (Engine, Mesh, Diagrams) use Pipeline output —
// they never know which source produced it.
type SymbolPipeline struct {
	Source      SymbolSource
	Root        string
	Concurrency int // max parallel walks; 0 = DefaultPipelineConcurrency
}

// Verify interface compliance at compile time.
var _ DeepAnalyzer = (*SymbolPipeline)(nil)

func (p *SymbolPipeline) concurrency() int {
	if p.Concurrency > 0 {
		return p.Concurrency
	}
	return DefaultPipelineConcurrency
}

func (p *SymbolPipeline) CallGraph(ctx context.Context, _ string, opts CallGraphOpts) (*CallGraph, error) {
	depth := opts.Depth
	if depth <= 0 {
		depth = DefaultCallGraphDepth
	}

	// Step 1: Discover roots.
	query := opts.Entry // empty = all exported
	roots, err := p.Source.Roots(ctx, query)
	if err != nil {
		return nil, err
	}

	// Step 2: Walk from roots with bounded concurrency.
	var mu sync.Mutex
	nodeSet := make(map[string]FuncNode)
	var edges []CallEdge
	visited := make(map[string]bool)
	sigCache := make(map[string]*SourceTypeInfo)

	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(p.concurrency())

	var walk func(sym SourceSymbol, d int)
	walk = func(sym SourceSymbol, d int) {
		if gCtx.Err() != nil {
			return
		}

		mu.Lock()
		if d > depth || visited[sym.Name] {
			mu.Unlock()
			return
		}
		visited[sym.Name] = true
		mu.Unlock()

		mu.Lock()
		nodeSet[sym.Package+"."+sym.Name] = FuncNode{
			Name:    sym.Name,
			Package: sym.Package,
			Line:    sym.Line,
			File:    sym.File,
			EndLine: sym.EndLine,
		}
		mu.Unlock()

		if d >= depth {
			return // at depth limit — node recorded but children not expanded
		}

		children, err := p.Source.Children(gCtx, sym)
		if err != nil {
			return
		}

		for _, rel := range children {
			callee := rel.Target

			// Resolve type info via Hover (cached).
			mu.Lock()
			typeInfo, cached := sigCache[callee.Package+"."+callee.Name]
			mu.Unlock()

			if !cached {
				ti, _ := p.Source.Hover(gCtx, callee)
				mu.Lock()
				sigCache[callee.Package+"."+callee.Name] = ti
				mu.Unlock()
				typeInfo = ti
			}

			var paramTypes, returnTypes []string
			if typeInfo != nil {
				paramTypes = typeInfo.ParamTypes
				returnTypes = typeInfo.ReturnTypes
			}

			mu.Lock()
			if rel.InWorkspace {
				nodeSet[callee.Package+"."+callee.Name] = FuncNode{
					Name:    callee.Name,
					Package: callee.Package,
					Line:    callee.Line,
					File:    callee.File,
					EndLine: callee.EndLine,
				}
			}
			edges = append(edges, CallEdge{
				Caller:      sym.Name,
				Callee:      callee.Name,
				CallerPkg:   sym.Package,
				CalleePkg:   callee.Package,
				Line:        callee.Line,
				File:        sym.File,
				CrossPkg:    sym.Package != callee.Package,
				ParamTypes:  paramTypes,
				ReturnTypes: returnTypes,
			})
			mu.Unlock()

			if rel.InWorkspace {
				walk(callee, d+1)
			}
		}
	}

	var rootsDone atomic.Int32
	total := len(roots)

	for _, root := range roots {
		if gCtx.Err() != nil {
			break
		}
		g.Go(func() error {
			walk(root, 0)
			done := int(rootsDone.Add(1))
			if opts.OnProgress != nil {
				mu.Lock()
				opts.OnProgress(ProgressUpdate{
					RootsResolved: done,
					RootsTotal:    total,
					NodesFound:    len(nodeSet),
					EdgesFound:    len(edges),
					Message:       fmt.Sprintf("%d/%d roots resolved", done, total),
				})
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()

	nodes := make([]FuncNode, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}
	return &CallGraph{Nodes: nodes, Edges: edges}, nil
}

func (p *SymbolPipeline) DataFlowTrace(ctx context.Context, _, entry string, maxDepth int) (*DataFlow, error) {
	if maxDepth <= 0 {
		maxDepth = DefaultDataFlowDepth
	}

	// Find the entry symbol.
	roots, err := p.Source.Roots(ctx, entry)
	if err != nil || len(roots) == 0 {
		return &DataFlow{
			Nodes: []DataFlowNode{{Name: entry, Kind: "entry"}},
		}, nil
	}

	entryRoot := roots[0]
	nodeMap := make(map[string]DataFlowNode)
	var edges []DataFlowEdge
	visited := make(map[string]bool)

	nodeMap[entry] = DataFlowNode{Name: entry, Kind: "entry", Pkg: entryRoot.Package}

	var trace func(sym SourceSymbol, d int)
	trace = func(sym SourceSymbol, d int) {
		if ctx.Err() != nil || d > maxDepth || visited[sym.Name] {
			return
		}
		visited[sym.Name] = true

		children, err := p.Source.Children(ctx, sym)
		if err != nil {
			return
		}

		for _, rel := range children {
			callee := rel.Target
			if rel.Kind == "data_store" {
				storeName := callee.Package + " Store"
				if _, exists := nodeMap[storeName]; !exists {
					nodeMap[storeName] = DataFlowNode{Name: storeName, Kind: "data_store", Pkg: callee.Package}
				}
				edges = append(edges, DataFlowEdge{From: sym.Name, To: storeName, Label: callee.Name})
			} else {
				if _, exists := nodeMap[callee.Name]; !exists {
					nodeMap[callee.Name] = DataFlowNode{Name: callee.Name, Kind: "process", Pkg: callee.Package}
				}
				edges = append(edges, DataFlowEdge{From: sym.Name, To: callee.Name})
			}
			trace(callee, d+1)
		}
	}

	trace(entryRoot, 0)

	nodes := make([]DataFlowNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	return &DataFlow{Nodes: nodes, Edges: edges}, nil
}

func (p *SymbolPipeline) DetectStateMachines(ctx context.Context, root string) ([]StateMachine, error) {
	// StateMachine detection requires documentSymbol-level inspection
	// (const groups, switch statements). Not all SymbolSources support this.
	// For now, return nil — individual sources implement this directly.
	return nil, nil
}
