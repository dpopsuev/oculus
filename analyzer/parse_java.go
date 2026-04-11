package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"

	"github.com/dpopsuev/oculus/ts"
)

func init() {
	RegisterSource(lang.Java, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
		if lang.DetectLanguage(root) != lang.Java {
			return nil
		}
		funcs := ParseJavaFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs)
	})
}

// ParseJavaFunctions parses .java files via tree-sitter.
func ParseJavaFunctions(root string) []oculus.Symbol {
	parser := ts.NewParser()
	parser.SetLanguage(ts.Java())

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil
	}

	var funcs []oculus.Symbol

	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == "target" || base == "build" || base == ".gradle" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".java" {
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
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = pkgRoot
		}

		extractJavaFuncs(tree.RootNode(), src, pkg, filepath.ToSlash(rel), &funcs)
		return nil
	})
	return funcs
}

func extractJavaFuncs(root ts.Node, src []byte, pkg, file string, funcs *[]oculus.Symbol) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		switch child.Type() {
		case "method_declaration", "constructor_declaration":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)

			var paramTypes []string
			if params := child.ChildByFieldName("parameters"); params != nil {
				paramTypes = extractJavaParamTypes(params, src)
			}

			var returnTypes []string
			if retType := child.ChildByFieldName("type"); retType != nil {
				rt := retType.Content(src)
				if rt != "" && rt != "void" {
					returnTypes = []string{rt}
				}
			}

			var callees []string
			if body := child.ChildByFieldName("body"); body != nil {
				callees = extractJavaCallees(body, src)
			}

			// Java: public methods are exported
			exported := true
			text := child.Content(src)
			if strings.Contains(text[:min(len(text), 30)], "private") {
				exported = false
			}

			*funcs = append(*funcs, oculus.Symbol{
				Name: name, Package: pkg, File: file,
				Line: int(child.StartPoint().Row) + 1, EndLine: int(child.EndPoint().Row) + 1,
				ParamTypes: paramTypes, ReturnTypes: returnTypes,
				Callees: callees, Exported: exported,
			})

		case "class_declaration", "interface_declaration", "enum_declaration":
			if body := child.ChildByFieldName("body"); body != nil {
				extractJavaFuncs(body, src, pkg, file, funcs)
			} else if body := findChildByType(child, "class_body"); body != nil {
				extractJavaFuncs(body, src, pkg, file, funcs)
			} else if body := findChildByType(child, "interface_body"); body != nil {
				extractJavaFuncs(body, src, pkg, file, funcs)
			}
		}
	}
}

func extractJavaParamTypes(params ts.Node, src []byte) []string {
	var types []string
	for i := 0; i < int(params.ChildCount()); i++ {
		param := params.Child(i)
		if param.Type() == "formal_parameter" || param.Type() == "spread_parameter" {
			if typeNode := param.ChildByFieldName("type"); typeNode != nil {
				types = append(types, typeNode.Content(src))
			}
		}
	}
	return types
}

func extractJavaCallees(node ts.Node, src []byte) []string {
	seen := make(map[string]bool)
	var callees []string
	walkJavaCalls(node, src, seen, &callees)
	return callees
}

func walkJavaCalls(node ts.Node, src []byte, seen map[string]bool, callees *[]string) {
	if node.Type() == "method_invocation" {
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			name := nameNode.Content(src)
			if name != "" && !seen[name] {
				seen[name] = true
				*callees = append(*callees, name)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkJavaCalls(node.Child(i), src, seen, callees)
	}
}
