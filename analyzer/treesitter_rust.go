package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/ts"
)

func (a *TreeSitterAnalyzer) rustClasses(root string) ([]oculus.ClassInfo, error) {
	var classes []oculus.ClassInfo
	err := a.walkRustFiles(root, func(tree ts.Tree, src []byte, pkg, file string) {
		root := tree.RootNode()
		extractRustClasses(root, src, pkg, file, &classes)
	})
	return classes, err
}

func extractRustClasses(root ts.Node, src []byte, pkg, file string, classes *[]oculus.ClassInfo) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		switch child.Type() {
		case "struct_item":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)
			ci := oculus.ClassInfo{
				Name:     name,
				Package:  pkg,
				Kind:     kindStruct,
				Exported: rustIsPublic(child, src),
				File:     file,
				Line:     int(nameNode.StartPoint().Row) + 1,
				EndLine:  int(child.EndPoint().Row) + 1,
				Fields:   extractRustStructFields(child, src),
			}
			*classes = append(*classes, ci)

		case "enum_item":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)
			ci := oculus.ClassInfo{
				Name:     name,
				Package:  pkg,
				Kind:     kindStruct,
				Exported: rustIsPublic(child, src),
				File:     file,
				Line:     int(nameNode.StartPoint().Row) + 1,
				EndLine:  int(child.EndPoint().Row) + 1,
			}
			*classes = append(*classes, ci)

		case "trait_item":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)
			ci := oculus.ClassInfo{
				Name:     name,
				Package:  pkg,
				Kind:     "trait",
				Exported: rustIsPublic(child, src),
				File:     file,
				Line:     int(nameNode.StartPoint().Row) + 1,
				EndLine:  int(child.EndPoint().Row) + 1,
				Methods:  extractRustTraitMethods(child, src, file),
			}
			*classes = append(*classes, ci)

		case "impl_item":
			typeNode := child.ChildByFieldName("type")
			if typeNode == nil {
				continue
			}
			typeName := rustSimpleTypeName(typeNode.Content(src))
			if typeName == "" {
				continue
			}
			// Only attach methods for inherent impls (no trait).
			body := child.ChildByFieldName("body")
			if body == nil {
				continue
			}
			methods := extractRustImplMethods(body, src, file)
			if len(methods) == 0 {
				continue
			}
			attachRustMethods(classes, typeName, pkg, methods)
		}
	}
}

func (a *TreeSitterAnalyzer) rustImplements(root string) ([]oculus.ImplEdge, error) {
	var edges []oculus.ImplEdge
	err := a.walkRustFiles(root, func(tree ts.Tree, src []byte, pkg, file string) {
		rootNode := tree.RootNode()
		for i := 0; i < int(rootNode.ChildCount()); i++ {
			child := rootNode.Child(i)
			switch child.Type() {
			case "impl_item":
				traitNode := child.ChildByFieldName("trait")
				typeNode := child.ChildByFieldName("type")
				if traitNode == nil || typeNode == nil {
					continue
				}
				typeName := rustSimpleTypeName(typeNode.Content(src))
				traitName := rustSimpleTypeName(traitNode.Content(src))
				if typeName == "" || traitName == "" {
					continue
				}
				edges = append(edges, oculus.ImplEdge{
					From: typeName,
					To:   traitName,
					Kind: "implements",
				})

			case "trait_item":
				nameNode := child.ChildByFieldName("name")
				if nameNode == nil {
					continue
				}
				traitName := nameNode.Content(src)
				for j := 0; j < int(child.ChildCount()); j++ {
					c := child.Child(j)
					if c.Type() != "trait_bounds" {
						continue
					}
					for k := 0; k < int(c.ChildCount()); k++ {
						bound := c.Child(k)
						if !bound.IsNamed() {
							continue
						}
						superName := rustSimpleTypeName(bound.Content(src))
						if superName == "" || superName == traitName {
							continue
						}
						edges = append(edges, oculus.ImplEdge{
							From: traitName,
							To:   superName,
							Kind: "inherits",
						})
					}
				}
			}
		}
	})
	return edges, err
}

func extractRustStructFields(structNode ts.Node, src []byte) []oculus.FieldInfo {
	var fields []oculus.FieldInfo
	for i := 0; i < int(structNode.ChildCount()); i++ {
		child := structNode.Child(i)
		if child.Type() != "field_declaration_list" {
			continue
		}
		for j := 0; j < int(child.ChildCount()); j++ {
			field := child.Child(j)
			if field.Type() != "field_declaration" {
				continue
			}
			nameNode := field.ChildByFieldName("name")
			typeNode := field.ChildByFieldName("type")
			if nameNode == nil || typeNode == nil {
				continue
			}
			fields = append(fields, oculus.FieldInfo{
				Name:     nameNode.Content(src),
				Type:     typeNode.Content(src),
				Exported: rustFieldIsPublic(field, src),
				Line:     int(field.StartPoint().Row) + 1,
			})
		}
		break
	}
	return fields
}

func extractRustTraitMethods(traitNode ts.Node, src []byte, file string) []oculus.MethodInfo {
	var methods []oculus.MethodInfo
	body := traitNode.ChildByFieldName("body")
	if body == nil {
		return nil
	}
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child.Type() != "function_signature_item" && child.Type() != "function_item" {
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

func extractRustImplMethods(body ts.Node, src []byte, file string) []oculus.MethodInfo {
	var methods []oculus.MethodInfo
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child.Type() != "function_item" {
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
			Exported:  rustIsPublic(child, src),
			File:      file,
			Line:      int(nameNode.StartPoint().Row) + 1,
			EndLine:   int(child.EndPoint().Row) + 1,
		})
	}
	return methods
}

func attachRustMethods(classes *[]oculus.ClassInfo, typeName, pkg string, methods []oculus.MethodInfo) {
	for k := range *classes {
		if (*classes)[k].Name == typeName && (*classes)[k].Package == pkg {
			(*classes)[k].Methods = append((*classes)[k].Methods, methods...)
			return
		}
	}
}

// rustIsPublic checks if a Rust item has a visibility_modifier child containing "pub".
func rustIsPublic(node ts.Node, src []byte) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "visibility_modifier" {
			return strings.Contains(child.Content(src), "pub")
		}
		if child.IsNamed() {
			break
		}
	}
	return false
}

func rustFieldIsPublic(field ts.Node, src []byte) bool {
	for i := 0; i < int(field.ChildCount()); i++ {
		child := field.Child(i)
		if child.Type() == "visibility_modifier" {
			return strings.Contains(child.Content(src), "pub")
		}
	}
	return false
}

// rustSimpleTypeName extracts the last path segment from a potentially qualified type.
func rustSimpleTypeName(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.LastIndex(raw, "::"); idx >= 0 {
		raw = raw[idx+2:]
	}
	// Strip generic args: Foo<Bar> → Foo
	if idx := strings.Index(raw, "<"); idx > 0 {
		raw = raw[:idx]
	}
	return raw
}

// --- file parsing / caching ---

func (a *TreeSitterAnalyzer) walkRustFiles(root string, fn func(ts.Tree, []byte, string, string)) error {
	files, err := a.parseRustFiles(root)
	if err != nil {
		return err
	}
	for _, f := range files {
		fn(f.tree, f.src, f.pkg, f.file)
	}
	return nil
}

func (a *TreeSitterAnalyzer) parseRustFiles(root string) ([]parsedFile, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	a.mu.Lock()
	if a.cachedRust != nil {
		if files, ok := a.cachedRust[absRoot]; ok {
			a.mu.Unlock()
			return files, nil
		}
	}
	a.mu.Unlock()

	parser := ts.NewParser()
	parser.SetLanguage(ts.Rust())

	var files []parsedFile
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
		if filepath.Ext(d.Name()) != extRust {
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
	if a.cachedRust == nil {
		a.cachedRust = make(map[string][]parsedFile)
	}
	a.cachedRust[absRoot] = files
	a.mu.Unlock()

	return files, nil
}
