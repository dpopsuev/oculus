package analyzer

import (
	"context"
	"github.com/dpopsuev/oculus"
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"
)

const LayerGoTools = "gotools"

func init() {
	Register(lang.Go, 85, func(root string, pool lsp.Pool) oculus.DeepAnalyzer {
		if os.Getenv("LOCUS_CALLGRAPH_BACKEND") != "gotools" {
			return nil
		}
		return NewGoToolsDeep(root)
	}, nil)
}

// GoToolsDeepAnalyzer uses golang.org/x/tools/go/callgraph for precise
// call graph analysis via Class Hierarchy Analysis (CHA). More accurate
// than AST walking for interface dispatch and method sets. Slower due to
// full type-checking.
type GoToolsDeepAnalyzer struct {
	root string
}

// NewGoToolsDeep creates a GoToolsDeepAnalyzer. Returns nil if not a Go project.
func NewGoToolsDeep(root string) *GoToolsDeepAnalyzer {
	if lang.DetectLanguage(root) != lang.Go {
		return nil
	}
	return &GoToolsDeepAnalyzer{root: root}
}

func (a *GoToolsDeepAnalyzer) CallGraph(ctx context.Context, _ string, opts oculus.CallGraphOpts) (*oculus.CallGraph, error) {
	absRoot, err := filepath.Abs(a.root)
	if err != nil {
		return nil, err
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps,
		Dir: absRoot,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("gotools: load packages: %w", err)
	}

	// Build SSA for all packages.
	prog, created := ssautil.AllPackages(pkgs, ssa.InstantiateGenerics)
	prog.Build()

	// Run CHA (Class Hierarchy Analysis) — fast, doesn't need entry points.
	cg := cha.CallGraph(prog)

	// Convert gonum call graph to Locus format.
	nodeSet := make(map[string]oculus.FuncNode)
	var edges []oculus.CallEdge

	for fn, node := range cg.Nodes {
		if fn == nil || fn.Synthetic != "" {
			continue
		}
		pos := prog.Fset.Position(fn.Pos())
		callerPkg := funcPackage(fn, absRoot, created)

		if opts.Scope != "" && !strings.HasPrefix(callerPkg, opts.Scope) {
			continue
		}
		if opts.ExportedOnly && !token.IsExported(fn.Name()) {
			continue
		}

		callerKey := callerPkg + "." + fn.Name()
		nodeSet[callerKey] = oculus.FuncNode{
			Name:    fn.Name(),
			Package: callerPkg,
			Line:    pos.Line,
		}

		for _, edge := range node.Out {
			callee := edge.Callee.Func
			if callee == nil || callee.Synthetic != "" {
				continue
			}
			calleePkg := funcPackage(callee, absRoot, created)
			calleeKey := calleePkg + "." + callee.Name()
			calleePos := prog.Fset.Position(callee.Pos())

			nodeSet[calleeKey] = oculus.FuncNode{
				Name:    callee.Name(),
				Package: calleePkg,
				Line:    calleePos.Line,
			}
			edges = append(edges, oculus.CallEdge{
				Caller:    fn.Name(),
				Callee:    callee.Name(),
				CallerPkg: callerPkg,
				CalleePkg: calleePkg,
				Line:      pos.Line,
				CrossPkg:  callerPkg != calleePkg,
			})
		}
	}

	nodes := make([]oculus.FuncNode, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}
	return &oculus.CallGraph{Nodes: nodes, Edges: edges, Layer: LayerGoTools}, nil
}

func (a *GoToolsDeepAnalyzer) DataFlowTrace(ctx context.Context, root, entry string, maxDepth int) (*oculus.DataFlow, error) {
	// Delegate to GoAST for data flow — CHA doesn't provide data flow info.
	goast := NewGoASTDeep(a.root)
	if goast == nil {
		return &oculus.DataFlow{Layer: LayerGoTools}, nil
	}
	return goast.DataFlowTrace(ctx, root, entry, maxDepth)
}

func (a *GoToolsDeepAnalyzer) DetectStateMachines(ctx context.Context, root string) ([]oculus.StateMachine, error) {
	// Delegate to GoAST — state machines are AST pattern matching, not call graph.
	goast := NewGoASTDeep(a.root)
	if goast == nil {
		return nil, nil
	}
	return goast.DetectStateMachines(ctx, root)
}

// funcPackage derives the Locus-style relative package path for an SSA function.
func funcPackage(fn *ssa.Function, absRoot string, _ []*ssa.Package) string {
	if fn.Package() == nil {
		return pkgRoot
	}

	// Try to derive relative path from the function's file position.
	rel := funcRelDir(fn, absRoot)
	if rel != "" {
		return rel
	}

	// Fallback: use the last path component of the package path.
	pkgPath := fn.Package().Pkg.Path()
	parts := strings.Split(pkgPath, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return pkgRoot
}

// funcRelDir derives a relative directory from a function's source position.
func funcRelDir(fn *ssa.Function, absRoot string) string {
	for _, m := range fn.Package().Members {
		pos := fn.Prog.Fset.Position(m.Pos())
		if pos.Filename == "" {
			continue
		}
		dir := filepath.Dir(pos.Filename)
		if rel, err := filepath.Rel(absRoot, dir); err == nil {
			return filepath.ToSlash(rel)
		}
	}
	return ""
}
