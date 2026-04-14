package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus/v3"

	"github.com/dpopsuev/oculus/v3/ts"
)

// ParsedFile holds a pre-parsed source file with its AST, source bytes,
// package name, and relative path. Created once by BuildParsedProject
// and reused by all oculus.DeepAnalyzer queries without redundant I/O.
type ParsedFile struct {
	Tree    ts.Tree
	Source  []byte
	Package string
	RelPath string
}

// ParsedProject caches parsed ASTs for an entire Go repository.
// It enables "parse once, query many" — all oculus.DeepAnalyzer methods
// iterate over Files instead of re-walking the filesystem.
type ParsedProject struct {
	Root  string
	Files []ParsedFile
}

// dataFlowTrace is a shared implementation for DataFlowTrace.
// Uses Symbol directly — no intermediate conversion types.
func dataFlowTrace(funcs []oculus.Symbol, entry string, maxDepth int, layer string) *oculus.DataFlow {
	funcIndex := make(map[string]*oculus.Symbol, len(funcs))
	for i := range funcs {
		funcIndex[funcs[i].Name] = &funcs[i]
	}

	nodeMap := make(map[string]oculus.DataFlowNode)
	var edges []oculus.DataFlowEdge
	visited := make(map[string]bool)
	nodeMap[entry] = oculus.DataFlowNode{Name: entry, Kind: "entry"}

	var trace func(name string, d int)
	trace = func(name string, d int) {
		if d > maxDepth || visited[name] {
			return
		}
		visited[name] = true
		fn, ok := funcIndex[name]
		if !ok {
			return
		}
		for _, callee := range fn.Callees {
			if _, ok := funcIndex[callee]; !ok {
				continue
			}
			if _, exists := nodeMap[callee]; !exists {
				nodeMap[callee] = oculus.DataFlowNode{Name: callee, Kind: "process", Pkg: funcIndex[callee].Package}
			}
			edges = append(edges, oculus.DataFlowEdge{From: name, To: callee})
			trace(callee, d+1)
		}
	}
	trace(entry, 0)

	nodes := make([]oculus.DataFlowNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	return &oculus.DataFlow{Nodes: nodes, Edges: edges, Layer: layer}
}

// collectTreeSitterCalls walks a tree-sitter node tree collecting function call names.
// Shared between Python and TypeScript deep analyzers.
func collectTreeSitterCalls(node ts.Node, src []byte, callType, funcField string,
	nameExtractor func(fn ts.Node, src []byte) string,
	seen map[string]bool, callees *[]string,
) {
	if node.Type() == callType {
		fn := node.ChildByFieldName(funcField)
		if fn != nil {
			name := nameExtractor(fn, src)
			if name != "" && !seen[name] {
				seen[name] = true
				*callees = append(*callees, name)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		collectTreeSitterCalls(node.Child(i), src, callType, funcField, nameExtractor, seen, callees)
	}
}

// BuildParsedProject walks root once, reads and parses every non-test .go
// file, and returns a ParsedProject. This is the "Divide" step in D&C:
// one filesystem walk, N parallel parses.
func BuildParsedProject(root string) (*ParsedProject, error) {
	parser := ts.NewParser()
	parser.SetLanguage(ts.Go())

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	var files []ParsedFile
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == dirVendor || base == dirTestdata || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != extGo || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		tree, err := parser.Parse(src)
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.Dir(rel)
		if pkg == "." {
			pkg = pkgRoot
		}
		pkg = filepath.ToSlash(pkg)
		files = append(files, ParsedFile{
			Tree:    tree,
			Source:  src,
			Package: pkg,
			RelPath: rel,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &ParsedProject{Root: absRoot, Files: files}, nil
}
