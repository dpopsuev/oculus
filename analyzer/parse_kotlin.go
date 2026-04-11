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
	RegisterSource(lang.Kotlin, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
		if lang.DetectLanguage(root) != lang.Kotlin {
			return nil
		}
		funcs := ParseKotlinFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs)
	})
}

// ParseKotlinFunctions parses .kt files via tree-sitter.
func ParseKotlinFunctions(root string) []oculus.Symbol {
	parser := ts.NewParser()
	parser.SetLanguage(ts.Kotlin())

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
			if base == "build" || base == ".gradle" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".kt" {
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

		extractKotlinFuncs(tree.RootNode(), src, pkg, filepath.ToSlash(rel), &funcs)
		return nil
	})
	return funcs
}

func extractKotlinFuncs(root ts.Node, src []byte, pkg, file string, funcs *[]oculus.Symbol) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		switch child.Type() {
		case "function_declaration":
			nameNode := findChildByType(child, "simple_identifier")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)

			paramTypes := extractKotlinParamTypes(child, src)
			returnTypes := extractKotlinReturnType(child, src)

			var callees []string
			if body := findChildByType(child, "function_body"); body != nil {
				callees = extractKotlinCallees(body, src)
			}

			*funcs = append(*funcs, oculus.Symbol{
				Name: name, Package: pkg, File: file,
				Line: int(child.StartPoint().Row) + 1, EndLine: int(child.EndPoint().Row) + 1,
				ParamTypes: paramTypes, ReturnTypes: returnTypes,
				Callees: callees, Exported: true,
			})

		case "class_declaration", "object_declaration":
			if body := findChildByType(child, "class_body"); body != nil {
				extractKotlinFuncs(body, src, pkg, file, funcs)
			}
		}
	}
}

func extractKotlinParamTypes(funcNode ts.Node, src []byte) []string {
	params := funcNode.ChildByFieldName("parameters")
	if params == nil {
		params = findChildByType(funcNode, "function_value_parameters")
	}
	if params == nil {
		return nil
	}
	var types []string
	for i := 0; i < int(params.ChildCount()); i++ {
		param := params.Child(i)
		if param.Type() == "parameter" {
			if typeNode := param.ChildByFieldName("type"); typeNode != nil {
				types = append(types, typeNode.Content(src))
			} else if typeNode = findChildByType(param, "user_type"); typeNode != nil {
				types = append(types, typeNode.Content(src))
			}
		}
	}
	return types
}

func extractKotlinReturnType(funcNode ts.Node, src []byte) []string {
	if retType := funcNode.ChildByFieldName("return_type"); retType != nil {
		rt := retType.Content(src)
		if rt != "" && rt != "Unit" {
			return []string{rt}
		}
	}
	// Kotlin tree-sitter may encode return type differently
	for i := 0; i < int(funcNode.ChildCount()); i++ {
		child := funcNode.Child(i)
		if child.Type() == "user_type" || child.Type() == "nullable_type" {
			rt := child.Content(src)
			if rt != "" && rt != "Unit" {
				return []string{rt}
			}
		}
	}
	return nil
}

func extractKotlinCallees(node ts.Node, src []byte) []string {
	seen := make(map[string]bool)
	var callees []string
	walkKotlinCalls(node, src, seen, &callees)
	return callees
}

func walkKotlinCalls(node ts.Node, src []byte, seen map[string]bool, callees *[]string) {
	if node.Type() == "call_expression" {
		// Kotlin call: simple_identifier followed by call_suffix
		if nameNode := findChildByType(node, "simple_identifier"); nameNode != nil {
			name := nameNode.Content(src)
			if name != "" && !seen[name] {
				seen[name] = true
				*callees = append(*callees, name)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkKotlinCalls(node.Child(i), src, seen, callees)
	}
}

func findChildByType(node ts.Node, nodeType string) ts.Node {
	for i := 0; i < int(node.ChildCount()); i++ {
		if node.Child(i).Type() == nodeType {
			return node.Child(i)
		}
	}
	return nil
}
