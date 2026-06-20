package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/ts"
)

func (a *TreeSitterAnalyzer) tsClasses(root string) ([]oculus.ClassInfo, error) {
	var classes []oculus.ClassInfo
	err := a.walkTSFiles(root, func(tree ts.Tree, src []byte, pkg, file string) {
		extractTSClasses(tree.RootNode(), src, pkg, file, &classes)
	})
	return classes, err
}

func extractTSClasses(root ts.Node, src []byte, pkg, file string, classes *[]oculus.ClassInfo) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)

		// Unwrap export_statement to find the actual declaration.
		if child.Type() == "export_statement" {
			extractTSClasses(child, src, pkg, file, classes)
			continue
		}

		switch child.Type() {
		case "class_declaration", "abstract_class_declaration":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)
			ci := oculus.ClassInfo{
				Name:     name,
				Package:  pkg,
				Kind:     "class",
				Exported: tsIsExported(child),
				File:     file,
				Line:     int(nameNode.StartPoint().Row) + 1,
				EndLine:  int(child.EndPoint().Row) + 1,
			}
			if body := child.ChildByFieldName("body"); body != nil {
				ci.Methods = extractTSMethods(body, src, file)
				ci.Fields = extractTSFields(body, src)
			}
			*classes = append(*classes, ci)

		case "interface_declaration":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)
			ci := oculus.ClassInfo{
				Name:     name,
				Package:  pkg,
				Kind:     kindInterface,
				Exported: tsIsExported(child),
				File:     file,
				Line:     int(nameNode.StartPoint().Row) + 1,
				EndLine:  int(child.EndPoint().Row) + 1,
			}
			if body := child.ChildByFieldName("body"); body != nil {
				ci.Methods = extractTSInterfaceMethods(body, src, file)
			}
			*classes = append(*classes, ci)
		}
	}
}

func (a *TreeSitterAnalyzer) tsImplements(root string) ([]oculus.ImplEdge, error) {
	var edges []oculus.ImplEdge
	err := a.walkTSFiles(root, func(tree ts.Tree, src []byte, pkg, file string) {
		extractTSImplements(tree.RootNode(), src, &edges)
	})
	return edges, err
}

func extractTSImplements(root ts.Node, src []byte, edges *[]oculus.ImplEdge) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)

		if child.Type() == "export_statement" {
			extractTSImplements(child, src, edges)
			continue
		}

		switch child.Type() {
		case "class_declaration", "abstract_class_declaration":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			className := nameNode.Content(src)
			for j := 0; j < int(child.ChildCount()); j++ {
				heritage := child.Child(j)
				if heritage.Type() != "class_heritage" {
					continue
				}
				for k := 0; k < int(heritage.ChildCount()); k++ {
					clause := heritage.Child(k)
					switch clause.Type() {
					case "extends_clause":
						for _, name := range tsHeritageNames(clause, src) {
							*edges = append(*edges, oculus.ImplEdge{
								From: className, To: name, Kind: "extends",
							})
						}
					case "implements_clause":
						for _, name := range tsHeritageNames(clause, src) {
							*edges = append(*edges, oculus.ImplEdge{
								From: className, To: name, Kind: "implements",
							})
						}
					}
				}
			}

		case "interface_declaration":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			ifaceName := nameNode.Content(src)
			for j := 0; j < int(child.ChildCount()); j++ {
				clause := child.Child(j)
				if clause.Type() != "extends_type_clause" {
					continue
				}
				for _, name := range tsHeritageNames(clause, src) {
					*edges = append(*edges, oculus.ImplEdge{
						From: ifaceName, To: name, Kind: "extends",
					})
				}
			}
		}
	}
}

// tsHeritageNames extracts type names from an extends/implements/extends_type clause.
func tsHeritageNames(clause ts.Node, src []byte) []string {
	var names []string
	for i := 0; i < int(clause.ChildCount()); i++ {
		child := clause.Child(i)
		switch child.Type() {
		case "identifier", "type_identifier":
			names = append(names, child.Content(src))
		case "generic_type":
			if nameNode := child.ChildByFieldName("name"); nameNode != nil {
				names = append(names, nameNode.Content(src))
			}
		case "nested_type_identifier":
			raw := child.Content(src)
			if idx := strings.LastIndex(raw, "."); idx >= 0 {
				raw = raw[idx+1:]
			}
			if raw != "" {
				names = append(names, raw)
			}
		}
	}
	return names
}

func extractTSMethods(body ts.Node, src []byte, file string) []oculus.MethodInfo {
	var methods []oculus.MethodInfo
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child.Type() != "method_definition" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nameNode.Content(src)
		params := child.ChildByFieldName("parameters")
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
			EndLine:   int(child.EndPoint().Row) + 1,
		})
	}
	return methods
}

func extractTSInterfaceMethods(body ts.Node, src []byte, file string) []oculus.MethodInfo {
	var methods []oculus.MethodInfo
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child.Type() != "method_signature" && child.Type() != "abstract_method_signature" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		name := nameNode.Content(src)
		params := child.ChildByFieldName("parameters")
		sig := name
		if params != nil {
			sig = name + params.Content(src)
		}
		methods = append(methods, oculus.MethodInfo{
			Name:      name,
			Signature: sig,
			Exported:  true,
			File:      file,
			Line:      int(nameNode.StartPoint().Row) + 1,
			EndLine:   int(child.EndPoint().Row) + 1,
		})
	}
	return methods
}

func extractTSFields(body ts.Node, src []byte) []oculus.FieldInfo {
	var fields []oculus.FieldInfo
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child.Type() != "public_field_definition" && child.Type() != "property_signature" {
			continue
		}
		nameNode := child.ChildByFieldName("name")
		if nameNode == nil {
			continue
		}
		typStr := ""
		if typeNode := child.ChildByFieldName("type"); typeNode != nil {
			typStr = typeNode.Content(src)
		}
		fields = append(fields, oculus.FieldInfo{
			Name:     nameNode.Content(src),
			Type:     typStr,
			Exported: !strings.HasPrefix(nameNode.Content(src), "_"),
			Line:     int(child.StartPoint().Row) + 1,
		})
	}
	return fields
}

// tsIsExported returns true for all top-level declarations. TypeScript module
// visibility is controlled by export keywords, but for code intelligence
// within a project all declarations are discoverable.
func tsIsExported(_ ts.Node) bool { return true }

// --- file parsing / caching ---

func (a *TreeSitterAnalyzer) walkTSFiles(root string, fn func(ts.Tree, []byte, string, string)) error {
	files, err := a.parseTSFiles(root)
	if err != nil {
		return err
	}
	for _, f := range files {
		fn(f.tree, f.src, f.pkg, f.file)
	}
	return nil
}

func (a *TreeSitterAnalyzer) parseTSFiles(root string) ([]parsedFile, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	if a.cachedTS != nil {
		if files, ok := a.cachedTS[absRoot]; ok {
			a.mu.Unlock()
			return files, nil
		}
	}
	a.mu.Unlock()

	parser := ts.NewParser()
	parser.SetLanguage(ts.TypeScript())

	var files []parsedFile
	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if lang.ShouldSkipTSDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext != extTS && ext != extTSX {
			return nil
		}
		// Skip .d.ts declaration files — they're type stubs, not source.
		if strings.HasSuffix(d.Name(), ".d.ts") {
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
	if a.cachedTS == nil {
		a.cachedTS = make(map[string][]parsedFile)
	}
	a.cachedTS[absRoot] = files
	a.mu.Unlock()

	return files, nil
}
