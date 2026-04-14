package analyzer

import (
	"context"
	"github.com/dpopsuev/oculus/v3"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"
)

// GoASTDeepAnalyzer uses go/ast for call graph, data flow, and state machine
// analysis. More accurate than regex, no external tools required.
type GoASTDeepAnalyzer struct {
	root string
}

func init() {
	Register(lang.Go, 90, func(root string, pool lsp.Pool) oculus.DeepAnalyzer {
		return NewGoASTDeep(root)
	}, nil)
}

// NewGoASTDeep creates a GoASTDeepAnalyzer for the given root directory.
// Returns nil if the root is not a Go project.
func NewGoASTDeep(root string) *GoASTDeepAnalyzer {
	if lang.DetectLanguage(root) != lang.Go {
		return nil
	}
	return &GoASTDeepAnalyzer{root: root}
}

// goFunc is an alias for Symbol — Go AST enriches Name, Package, File,
// Line, EndLine, ReceiverType, ParamTypes, ReturnTypes, Callees.
type goFunc = oculus.Symbol

func (a *GoASTDeepAnalyzer) CallGraph(ctx context.Context, _ string, opts oculus.CallGraphOpts) (*oculus.CallGraph, error) {
	depth := opts.Depth
	if depth <= 0 {
		depth = oculus.DefaultCallGraphDepth
	}

	funcs, err := a.parseFunctions(opts.Scope)
	if err != nil {
		return nil, err
	}

	// Build index by function name.
	funcIndex := make(map[string]*goFunc)
	for i := range funcs {
		funcIndex[funcs[i].Name] = &funcs[i]
	}

	// Determine root functions.
	var roots []string
	if opts.Entry != "" {
		roots = []string{opts.Entry}
	} else {
		for _, f := range funcs {
			if opts.Scope != "" && !strings.HasPrefix(f.Package, opts.Scope) {
				continue
			}
			if opts.ExportedOnly && !ast.IsExported(f.Name) {
				continue
			}
			if ast.IsExported(f.Name) {
				roots = append(roots, f.Name)
			}
		}
	}

	nodeSet := make(map[string]oculus.Symbol)
	var edges []oculus.CallEdge
	visited := make(map[string]bool)

	var walk func(name string, d int)
	walk = func(name string, d int) {
		if ctx.Err() != nil || d > depth || visited[name] {
			return
		}
		visited[name] = true

		fn, ok := funcIndex[name]
		if !ok {
			return
		}

		key := fn.Package + "." + fn.Name
		nodeSet[key] = oculus.Symbol{Name: fn.Name, Package: fn.Package, Line: fn.Line, File: fn.File, EndLine: fn.EndLine}

		for _, callee := range fn.Callees {
			calleeFn, ok := funcIndex[callee]
			if !ok {
				continue
			}
			calleeKey := calleeFn.Package + "." + calleeFn.Name
			nodeSet[calleeKey] = oculus.Symbol{Name: calleeFn.Name, Package: calleeFn.Package, Line: calleeFn.Line, File: calleeFn.File, EndLine: calleeFn.EndLine}
			edges = append(edges, oculus.CallEdge{
				Caller:       fn.Name,
				Callee:       calleeFn.Name,
				CallerPkg:    fn.Package,
				CalleePkg:    calleeFn.Package,
				Line:         fn.Line,
				File:         fn.File,
				ReceiverType: fn.ReceiverType,
				CrossPkg:     fn.Package != calleeFn.Package,
				ParamTypes:   calleeFn.ParamTypes,
				ReturnTypes:  calleeFn.ReturnTypes,
			})
			walk(callee, d+1)
		}
	}

	for _, root := range roots {
		walk(root, 0)
	}

	nodes := make([]oculus.Symbol, 0, len(nodeSet))
	for _, n := range nodeSet {
		nodes = append(nodes, n)
	}
	return &oculus.CallGraph{Nodes: nodes, Edges: edges, Layer: oculus.LayerGoAST}, nil
}

func (a *GoASTDeepAnalyzer) DataFlowTrace(ctx context.Context, _, entry string, maxDepth int) (*oculus.DataFlow, error) {
	if maxDepth <= 0 {
		maxDepth = oculus.DefaultDataFlowDepth
	}

	funcs, err := a.parseFunctions("") // DataFlowTrace needs full graph
	if err != nil {
		return nil, err
	}

	return dataFlowTrace(funcs, entry, maxDepth, oculus.LayerGoAST), nil
}

func (a *GoASTDeepAnalyzer) DetectStateMachines(ctx context.Context, _ string) ([]oculus.StateMachine, error) {
	fset := token.NewFileSet()
	absRoot, _ := filepath.Abs(a.root)

	var machines []oculus.StateMachine

	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if lang.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != extGo || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = pkgRoot
		}

		for _, decl := range f.Decls {
			if sm := parseIotaConstGroup(decl, f, pkg); sm != nil {
				machines = append(machines, *sm)
			}
		}
		return nil
	})

	return machines, nil
}

// parseIotaConstGroup checks if a declaration is an iota-based const group
// and returns a oculus.StateMachine if so.
func parseIotaConstGroup(decl ast.Decl, f *ast.File, pkg string) *oculus.StateMachine {
	gd, ok := decl.(*ast.GenDecl)
	if !ok || gd.Tok != token.CONST || len(gd.Specs) < 3 {
		return nil
	}

	var typeName string
	var values []string
	hasIota := false

	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range vs.Names {
			values = append(values, name.Name)
		}
		if vs.Type != nil {
			if ident, ok := vs.Type.(*ast.Ident); ok {
				typeName = ident.Name
			}
		}
		for _, v := range vs.Values {
			if ident, ok := v.(*ast.Ident); ok && ident.Name == "iota" {
				hasIota = true
			}
		}
	}

	if !hasIota || len(values) < 3 {
		return nil
	}
	if typeName == "" {
		typeName = values[0] + "Type"
	}

	transitions := findASTSwitchTransitions(f, values)
	return &oculus.StateMachine{
		Name:        typeName,
		Package:     pkg,
		States:      values,
		Transitions: transitions,
		Initial:     values[0],
	}
}

func findASTSwitchTransitions(f *ast.File, states []string) []oculus.StateTransition {
	stateSet := make(map[string]bool, len(states))
	for _, s := range states {
		stateSet[s] = true
	}

	var transitions []oculus.StateTransition
	ast.Inspect(f, func(n ast.Node) bool {
		sw, ok := n.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		// Check cases for state references.
		for _, stmt := range sw.Body.List {
			cc, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range cc.List {
				if ident, ok := expr.(*ast.Ident); ok && stateSet[ident.Name] {
					// Look for assignments to the same type in the case body.
					for _, bs := range cc.Body {
						as, ok := bs.(*ast.AssignStmt)
						if !ok {
							continue
						}
						for _, rhs := range as.Rhs {
							if ri, ok := rhs.(*ast.Ident); ok && stateSet[ri.Name] && ri.Name != ident.Name {
								transitions = append(transitions, oculus.StateTransition{
									From: ident.Name,
									To:   ri.Name,
								})
							}
						}
					}
				}
			}
		}
		return true
	})
	return transitions
}

// parseFunctions walks the Go source tree and extracts all function declarations
// with their callees.
func (a *GoASTDeepAnalyzer) parseFunctions(scope string) ([]goFunc, error) {
	fset := token.NewFileSet()
	absRoot, err := filepath.Abs(a.root)
	if err != nil {
		return nil, err
	}

	var funcs []goFunc

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if lang.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != extGo || strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}

		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = pkgRoot
		}

		// Scope filter: skip files outside the requested scope prefix.
		if scope != "" && !strings.HasPrefix(pkg, scope) {
			return nil
		}

		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return nil
		}

		relFile := filepath.ToSlash(rel)
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			name := fd.Name.Name
			var recvType string
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				recvType = receiverTypeName(fd.Recv.List[0].Type)
				name = recvType + "." + name // index as ReceiverType.Method
			}
			callees := extractCallees(fd.Body)
			funcs = append(funcs, goFunc{
				Name:         name,
				Package:      pkg,
				File:         relFile,
				Line:         fset.Position(fd.Pos()).Line,
				EndLine:      fset.Position(fd.End()).Line,
				ReceiverType: recvType,
				ParamTypes:   extractFieldTypes(fd.Type.Params),
				ReturnTypes:  extractFieldTypes(fd.Type.Results),
				Callees:      callees,
			})
		}
		return nil
	})

	return funcs, err
}

// extractCallees walks a function body and returns all function names called.
func extractCallees(body *ast.BlockStmt) []string {
	seen := make(map[string]bool)
	var callees []string

	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		var name string
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			name = fn.Name
		case *ast.SelectorExpr:
			name = fn.Sel.Name
		}
		if name != "" && !seen[name] {
			seen[name] = true
			callees = append(callees, name)
		}
		return true
	})
	return callees
}

// receiverTypeName extracts the type name from a method receiver expression.
// Handles both value (*ast.Ident) and pointer (*ast.StarExpr → *ast.Ident) receivers.
func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			return "*" + id.Name
		}
	}
	return ""
}

// exprTypeName converts a Go AST type expression to a readable string.
func exprTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprTypeName(t.X)
	case *ast.SelectorExpr:
		return exprTypeName(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + exprTypeName(t.Elt)
	case *ast.MapType:
		return "map[" + exprTypeName(t.Key) + "]" + exprTypeName(t.Value)
	case *ast.Ellipsis:
		return "..." + exprTypeName(t.Elt)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	case *ast.ChanType:
		return "chan " + exprTypeName(t.Value)
	}
	return ""
}

// extractFieldTypes returns type names from an ast.FieldList (params or results).
func extractFieldTypes(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	var types []string
	for _, field := range fl.List {
		typeName := exprTypeName(field.Type)
		if typeName == "" {
			continue
		}
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for i := 0; i < count; i++ {
			types = append(types, typeName)
		}
	}
	return types
}

// EnrichCallEdgeTypes fills in ParamTypes/ReturnTypes on edges that lack them
// by parsing source files. Two strategies:
// 1. For edges with File+Line: parse that file, find FuncDecl at that line
// 2. For edges without File (e.g., Regex): scan all Go files for callee by name
func EnrichCallEdgeTypes(root string, edges []oculus.CallEdge) {
	// Strategy 1: edges with known callee location
	type fileLine struct {
		file string
		line int
	}
	edgesByLoc := make(map[fileLine][]int)
	// Strategy 2: edges needing name-based lookup
	edgesByName := make(map[string][]int) // callee name → edge indices

	for i, e := range edges {
		if len(e.ParamTypes) > 0 {
			continue
		}
		if e.File != "" && e.Line > 0 {
			fl := fileLine{e.File, e.Line}
			edgesByLoc[fl] = append(edgesByLoc[fl], i)
		} else {
			edgesByName[e.Callee] = append(edgesByName[e.Callee], i)
		}
	}

	if len(edgesByLoc) == 0 && len(edgesByName) == 0 {
		return
	}

	// Parse Go files needed for location-based lookups
	parsedFiles := make(map[string]*ast.File)
	fileSets := make(map[string]*token.FileSet)
	for fl := range edgesByLoc {
		if _, done := parsedFiles[fl.file]; done {
			continue
		}
		absPath := filepath.Join(root, fl.file)
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, absPath, nil, 0)
		if err != nil {
			continue
		}
		parsedFiles[fl.file] = f
		fileSets[fl.file] = fset
	}

	// Strategy 1: match by file + line
	for fl, indices := range edgesByLoc {
		f, ok := parsedFiles[fl.file]
		if !ok {
			continue
		}
		fset := fileSets[fl.file]
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Type == nil {
				continue
			}
			if fset.Position(fd.Pos()).Line != fl.line {
				continue
			}
			pt := extractFieldTypes(fd.Type.Params)
			rt := extractFieldTypes(fd.Type.Results)
			for _, idx := range indices {
				edges[idx].ParamTypes = pt
				edges[idx].ReturnTypes = rt
			}
			break
		}
	}

	// Strategy 2: scan all Go files for function by name
	if len(edgesByName) > 0 {
		absRoot, _ := filepath.Abs(root)
		_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				if d != nil && d.IsDir() {
					name := d.Name()
					if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return nil
			}
			for _, decl := range f.Decls {
				fd, ok := decl.(*ast.FuncDecl)
				if !ok || fd.Type == nil || fd.Name == nil {
					continue
				}
				indices, need := edgesByName[fd.Name.Name]
				if !need {
					continue
				}
				pt := extractFieldTypes(fd.Type.Params)
				rt := extractFieldTypes(fd.Type.Results)
				for _, idx := range indices {
					edges[idx].ParamTypes = pt
					edges[idx].ReturnTypes = rt
				}
				delete(edgesByName, fd.Name.Name)
				if len(edgesByName) == 0 {
					return filepath.SkipAll
				}
			}
			return nil
		})
	}
}
