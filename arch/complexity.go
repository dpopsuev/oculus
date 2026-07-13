package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ComplexityDisclaimer is returned with every complexity_hints response.
const ComplexityDisclaimer = "heuristic AST patterns only — not a proof; prefer profiling for truth"

// PackageComplexity is the algo-pattern signal for one component.
type PackageComplexity struct {
	Component      string   `json:"component"`
	MaxLoopNesting int      `json:"max_loop_nesting"`
	Patterns       []string `json:"patterns,omitempty"`
	ComplexityHint string   `json:"complexity_hint,omitempty"`
	FilesScanned   int      `json:"files_scanned"`
}

// AnalyzePackageComplexity walks .go files under root/component (or matching
// basename) and extracts nested-loop / recursion heuristics.
func AnalyzePackageComplexity(root, component string) PackageComplexity {
	out := PackageComplexity{Component: component}
	dir := resolveComponentDir(root, component)
	if dir == "" {
		return out
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return out
	}
	seen := map[string]bool{}
	var patterns []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			patterns = append(patterns, p)
		}
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			continue
		}
		out.FilesScanned++
		nest := maxLoopNesting(f)
		if nest > out.MaxLoopNesting {
			out.MaxLoopNesting = nest
		}
		if nest >= 3 {
			add("nested_loops")
		} else if nest == 2 {
			add("double_loop")
		}
		if hasRecursion(f) {
			add("recursion")
		}
		if hasSortCall(f) {
			add("sort_call")
		}
	}
	sort.Strings(patterns)
	out.Patterns = patterns
	out.ComplexityHint = hintFromPatterns(out.MaxLoopNesting, patterns)
	return out
}

func resolveComponentDir(root, component string) string {
	component = filepath.ToSlash(strings.Trim(component, "/"))
	if component == "" || component == "." {
		return root
	}
	cand := filepath.Join(root, filepath.FromSlash(component))
	if st, err := os.Stat(cand); err == nil && st.IsDir() {
		return cand
	}
	base := filepath.Base(component)
	var found string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if filepath.Base(path) == ".git" || filepath.Base(path) == "vendor" {
			return filepath.SkipDir
		}
		if d.Name() == base {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func maxLoopNesting(f *ast.File) int {
	maxDepth := 0
	var walk func(n ast.Node, depth int)
	walk = func(n ast.Node, depth int) {
		if n == nil {
			return
		}
		switch n.(type) {
		case *ast.ForStmt, *ast.RangeStmt:
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
		}
		switch x := n.(type) {
		case *ast.FuncDecl:
			if x.Body != nil {
				for _, s := range x.Body.List {
					walk(s, depth)
				}
			}
		case *ast.BlockStmt:
			for _, s := range x.List {
				walk(s, depth)
			}
		case *ast.IfStmt:
			walk(x.Body, depth)
			walk(x.Else, depth)
		case *ast.ForStmt:
			walk(x.Body, depth)
		case *ast.RangeStmt:
			walk(x.Body, depth)
		case *ast.SwitchStmt:
			walk(x.Body, depth)
		case *ast.TypeSwitchStmt:
			walk(x.Body, depth)
		case *ast.SelectStmt:
			walk(x.Body, depth)
		case *ast.CaseClause:
			for _, s := range x.Body {
				walk(s, depth)
			}
		case *ast.CommClause:
			for _, s := range x.Body {
				walk(s, depth)
			}
		case *ast.FuncLit:
			if x.Body != nil {
				walk(x.Body, depth)
			}
		}
	}
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok {
			walk(fd, 0)
		}
	}
	return maxDepth
}

func hasRecursion(f *ast.File) bool {
	for _, d := range f.Decls {
		fd, ok := d.(*ast.FuncDecl)
		if !ok || fd.Body == nil || fd.Name == nil {
			continue
		}
		name := fd.Name.Name
		found := false
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				if fun.Name == name {
					found = true
					return false
				}
			case *ast.SelectorExpr:
				if fun.Sel != nil && fun.Sel.Name == name {
					found = true
					return false
				}
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func hasSortCall(f *ast.File) bool {
	found := false
	ast.Inspect(f, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil {
			return true
		}
		if sel.Sel.Name == "Sort" || sel.Sel.Name == "Slice" || sel.Sel.Name == "SliceStable" {
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == "sort" {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func hintFromPatterns(nesting int, patterns []string) string {
	if nesting >= 3 {
		return "nested loops suggest polynomial-ish cost (heuristic)"
	}
	for _, p := range patterns {
		if p == "recursion" {
			return "recursion detected (depth/cost unknown — heuristic)"
		}
		if p == "sort_call" {
			return "sort usage suggests ~n log n region (heuristic)"
		}
	}
	if nesting == 2 {
		return "double loop suggests quadratic-ish cost (heuristic)"
	}
	return ""
}

// EnrichHotSpotsComplexity annotates top spots with package AST heuristics.
func EnrichHotSpotsComplexity(root string, spots []HotSpot, topN int) []HotSpot {
	if topN <= 0 {
		topN = 10
	}
	if len(spots) > topN {
		spots = spots[:topN]
	}
	out := make([]HotSpot, len(spots))
	copy(out, spots)
	for i := range out {
		pc := AnalyzePackageComplexity(root, out[i].Component)
		out[i].ComplexityHint = pc.ComplexityHint
		out[i].Patterns = pc.Patterns
	}
	return out
}
