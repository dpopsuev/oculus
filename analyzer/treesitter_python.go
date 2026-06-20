package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/ts"
)

func (a *TreeSitterAnalyzer) pythonClasses(root string) ([]oculus.ClassInfo, error) {
	var classes []oculus.ClassInfo
	err := a.walkPyFiles(root, func(tree ts.Tree, src []byte, pkg, file string) {
		rootNode := tree.RootNode()
		extractPythonClasses(rootNode, src, pkg, file, &classes)
	})
	return classes, err
}

func extractPythonClasses(root ts.Node, src []byte, pkg, file string, classes *[]oculus.ClassInfo) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "class_definition" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nameNode.Content(src)
		if name == "" {
			continue
		}
		ci := oculus.ClassInfo{
			Name:     name,
			Package:  pkg,
			Kind:     "class",
			Exported: !strings.HasPrefix(name, "_"),
			File:     file,
			Line:     int(nameNode.StartPoint().Row) + 1,
			EndLine:  int(child.EndPoint().Row) + 1,
		}
		if body := child.ChildByFieldName("body"); body != nil {
			ci.Methods = extractPythonMethods(body, src, file)
		}
		*classes = append(*classes, ci)
	}
}

func extractPythonMethods(body ts.Node, src []byte, file string) []oculus.MethodInfo {
	var methods []oculus.MethodInfo
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child.Type() != "function_definition" && child.Type() != "decorated_definition" {
			continue
		}
		fn := child
		if child.Type() == "decorated_definition" {
			fn = findDefinitionInDecorated(child)
			if fn == nil {
				continue
			}
		}
		nameNode := fn.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nameNode.Content(src)
		params := fn.ChildByFieldName("parameters")
		sig := name
		if params != nil {
			sig = name + params.Content(src)
		}
		methods = append(methods, oculus.MethodInfo{
			Name:      name,
			Signature: sig,
			Exported:  !strings.HasPrefix(name, "_"),
			File:      file,
			Line:      int(nameNode.StartPoint().Row) + 1,
			EndLine:   int(fn.EndPoint().Row) + 1,
		})
	}
	return methods
}

func findDefinitionInDecorated(node ts.Node) ts.Node {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "function_definition" || child.Type() == "async_function_definition" {
			return child
		}
	}
	return nil
}

func (a *TreeSitterAnalyzer) pythonImplements(root string) ([]oculus.ImplEdge, error) {
	var edges []oculus.ImplEdge
	err := a.walkPyFiles(root, func(tree ts.Tree, src []byte, pkg, file string) {
		rootNode := tree.RootNode()
		for i := 0; i < int(rootNode.ChildCount()); i++ {
			child := rootNode.Child(i)
			if child.Type() != "class_definition" {
				continue
			}
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			className := nameNode.Content(src)
			supers := child.ChildByFieldName("superclasses")
			if supers == nil {
				continue
			}
			for j := 0; j < int(supers.ChildCount()); j++ {
				arg := supers.Child(j)
				if arg.Type() != "identifier" {
					continue
				}
				parent := arg.Content(src)
				if parent == "object" || parent == "" {
					continue
				}
				edges = append(edges, oculus.ImplEdge{
					From: className,
					To:   parent,
					Kind: "extends",
				})
			}
		}
	})
	return edges, err
}

// --- file parsing / caching ---

func (a *TreeSitterAnalyzer) walkPyFiles(root string, fn func(ts.Tree, []byte, string, string)) error {
	files, err := a.parsePyFiles(root)
	if err != nil {
		return err
	}
	for _, f := range files {
		fn(f.tree, f.src, f.pkg, f.file)
	}
	return nil
}

func (a *TreeSitterAnalyzer) parsePyFiles(root string) ([]parsedFile, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	if a.cachedPy != nil {
		if files, ok := a.cachedPy[absRoot]; ok {
			a.mu.Unlock()
			return files, nil
		}
	}
	a.mu.Unlock()

	parser := ts.NewParser()
	parser.SetLanguage(ts.Python())

	var files []parsedFile
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if lang.ShouldSkipPythonDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".py") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		tree, parseErr := parser.Parse(src)
		if parseErr != nil {
			return nil
		}
		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = pkgRoot
		}
		files = append(files, parsedFile{tree: tree, src: src, pkg: pkg, file: filepath.ToSlash(rel)})
		return nil
	})
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	if a.cachedPy == nil {
		a.cachedPy = make(map[string][]parsedFile)
	}
	a.cachedPy[absRoot] = files
	a.mu.Unlock()

	return files, nil
}
