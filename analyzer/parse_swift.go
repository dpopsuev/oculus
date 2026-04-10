package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/swift"
)

func init() {
	RegisterSource(lang.Swift, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
		if lang.DetectLanguage(root) != lang.Swift {
			return nil
		}
		funcs := ParseSwiftFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs)
	})
}

// ParseSwiftFunctions parses .swift files via tree-sitter.
func ParseSwiftFunctions(root string) []oculus.SourceFunc {
	parser := sitter.NewParser()
	parser.SetLanguage(swift.GetLanguage())

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil
	}

	var funcs []oculus.SourceFunc

	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".build" || base == "Packages" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".swift" {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		tree, err := parser.ParseCtx(context.Background(), nil, src)
		if err != nil {
			return nil
		}

		rel, _ := filepath.Rel(absRoot, path)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = pkgRoot
		}

		extractSwiftFuncs(tree.RootNode(), src, pkg, filepath.ToSlash(rel), &funcs)
		return nil
	})
	return funcs
}

func extractSwiftFuncs(root *sitter.Node, src []byte, pkg, file string, funcs *[]oculus.SourceFunc) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		switch child.Type() {
		case "function_declaration":
			nameNode := findChildByType(child, "simple_identifier")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)

			// Swift params are direct `parameter` children of function_declaration
			paramTypes := extractSwiftDirectParams(child, src)

			// Return type is a `user_type` child after the `->`
			var returnTypes []string
			if ut := findChildByType(child, "user_type"); ut != nil {
				rt := ut.Content(src)
				if rt != "" && rt != "Void" {
					returnTypes = []string{rt}
				}
			}

			var callees []string
			if body := findChildByType(child, "function_body"); body != nil {
				callees = extractSwiftCallees(body, src)
			}

			*funcs = append(*funcs, oculus.SourceFunc{
				Name: name, Package: pkg, File: file,
				Line: int(child.StartPoint().Row) + 1, EndLine: int(child.EndPoint().Row) + 1,
				ParamTypes: paramTypes, ReturnTypes: returnTypes,
				Callees: callees, Exported: true,
			})

		case "class_declaration", "struct_declaration", "extension_declaration":
			// Swift class bodies may use different child types
			for j := 0; j < int(child.ChildCount()); j++ {
				sub := child.Child(j)
				if sub.Type() == "class_body" || sub.Type() == "function_declaration" {
					extractSwiftFuncs(sub, src, pkg, file, funcs)
				}
			}
		}
	}
}

func extractSwiftCallees(node *sitter.Node, src []byte) []string {
	seen := make(map[string]bool)
	var callees []string
	walkSwiftCalls(node, src, seen, &callees)
	return callees
}

func walkSwiftCalls(node *sitter.Node, src []byte, seen map[string]bool, callees *[]string) {
	if node.Type() == "call_expression" {
		// Swift call: simple_identifier or navigation_expression
		if nameNode := findChildByType(node, "simple_identifier"); nameNode != nil {
			name := nameNode.Content(src)
			if name != "" && !seen[name] {
				seen[name] = true
				*callees = append(*callees, name)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkSwiftCalls(node.Child(i), src, seen, callees)
	}
}

// extractSwiftDirectParams finds parameter nodes that are direct children
// of the function_declaration (Swift doesn't wrap them in a parameters node).
func extractSwiftDirectParams(funcNode *sitter.Node, src []byte) []string {
	var types []string
	for i := 0; i < int(funcNode.ChildCount()); i++ {
		param := funcNode.Child(i)
		if param.Type() == "parameter" {
			// Parameter has user_type child for the type annotation
			if ut := findChildByType(param, "user_type"); ut != nil {
				types = append(types, ut.Content(src))
			} else if pt := findChildByType(param, "array_type"); pt != nil {
				types = append(types, pt.Content(src))
			}
		}
	}
	return types
}
