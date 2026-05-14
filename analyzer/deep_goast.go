package analyzer

import (
	"context"
	"io/fs"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	oculus "github.com/dpopsuev/oculus/v3"
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

// callRef pairs a callee name with the kind of edge that produced it.
type callRef struct {
	name string
	kind string // one of the oculus.CallEdge* constants
}

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

	// Build async-aware call refs: same keying as funcIndex but carries edge kind.
	callRefs := make(map[string][]callRef)
	for i := range funcs {
		for _, name := range funcs[i].Callees {
			callRefs[funcs[i].Name] = append(callRefs[funcs[i].Name], callRef{name: name, kind: oculus.CallEdgeSync})
		}
	}
	if refs, err2 := extractAsyncRefs(a.root); err2 == nil {
		for caller, rs := range refs {
			callRefs[caller] = append(callRefs[caller], rs...)
		}
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

		for _, ref := range callRefs[fn.Name] {
			// Channel operations target a variable, not a function.
			// Emit the edge with the channel var as a pseudo-node; don't recurse.
			if ref.kind == oculus.CallEdgeChanSend || ref.kind == oculus.CallEdgeChanRecv {
				chanKey := fn.Package + "." + ref.name
				nodeSet[chanKey] = oculus.Symbol{Name: ref.name, Package: fn.Package, Kind: "channel"}
				edges = append(edges, oculus.CallEdge{
					Caller:    fn.Name,
					Callee:    ref.name,
					CallerPkg: fn.Package,
					CalleePkg: fn.Package,
					Line:      fn.Line,
					File:      fn.File,
					Kind:      ref.kind,
				})
				continue
			}

			calleeFn, ok := funcIndex[ref.name]
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
				Kind:         ref.kind,
			})
			walk(ref.name, d+1)
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

// extractAsyncRefs walks all Go files under root and returns a map of
// caller name → []callRef for async constructs only:
//   - go f()          → kind: CallEdgeGoroutine
//   - ch <- v (send)  → kind: CallEdgeChanSend  (callee = channel var name)
//   - <-ch  (recv)    → kind: CallEdgeChanRecv  (callee = channel var name)
//
// Regular function calls are handled via Symbol.Callees; this supplements
// only the async seams that extractCallees misses.
func extractAsyncRefs(root string) (map[string][]callRef, error) {
	out := make(map[string][]callRef)

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, root, func(fi fs.FileInfo) bool {
		return !fi.IsDir() && strings.HasSuffix(fi.Name(), ".go") &&
			!strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		// Best-effort: walk subdirectories manually.
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return nil
			}
			collectAsyncFromFile(f, out)
			return nil
		})
		return out, nil
	}
	for _, pkg := range pkgs {
		for _, f := range pkg.Files {
			collectAsyncFromFile(f, out)
		}
	}
	return out, nil
}

// collectAsyncFromFile adds goroutine/channel callRefs from a single parsed file.
func collectAsyncFromFile(f *ast.File, out map[string][]callRef) {
	var currentFunc string

	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			if node.Name != nil {
				currentFunc = node.Name.Name
			}

		case *ast.GoStmt:
			// go f() or go func() { ... }()
			if name := callExprName(node.Call); name != "" && currentFunc != "" {
				out[currentFunc] = append(out[currentFunc], callRef{name: name, kind: oculus.CallEdgeGoroutine})
			}

		case *ast.SendStmt:
			// ch <- value: callee is the channel variable
			if name := identName(node.Chan); name != "" && currentFunc != "" {
				out[currentFunc] = append(out[currentFunc], callRef{name: name, kind: oculus.CallEdgeChanSend})
			}

		case *ast.UnaryExpr:
			// <-ch receive expression
			if node.Op == token.ARROW {
				if name := identName(node.X); name != "" && currentFunc != "" {
					out[currentFunc] = append(out[currentFunc], callRef{name: name, kind: oculus.CallEdgeChanRecv})
				}
			}
		}
		return true
	})
}

// callExprName returns the function name from a CallExpr, or "" if unresolvable.
func callExprName(call *ast.CallExpr) string {
	if call == nil {
		return ""
	}
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

// identName returns the name of an identifier expression, or "".
func identName(expr ast.Expr) string {
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}

// extractCallees walks a function body and returns all function names called.
func extractCallees(body *ast.BlockStmt) []string {
	seen := make(map[string]bool)
	var callees []string

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			var name string
			switch fn := node.Fun.(type) {
			case *ast.Ident:
				name = fn.Name
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			}
			if name != "" && !seen[name] {
				seen[name] = true
				callees = append(callees, name)
			}
		case *ast.CompositeLit:
			// Struct literal construction: Config{Name: "x"}
			var name string
			switch t := node.Type.(type) {
			case *ast.Ident:
				name = t.Name
			case *ast.SelectorExpr:
				name = t.Sel.Name
			}
			if name != "" && !seen[name] {
				seen[name] = true
				callees = append(callees, name)
			}
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
