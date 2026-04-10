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
	"github.com/smacker/go-tree-sitter/rust"
)

func init() {
	RegisterSource(lang.Rust, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
		if lang.DetectLanguage(root) != lang.Rust {
			return nil
		}
		funcs := ParseRustFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs)
	})
}

// ParseRustFunctions parses .rs files via tree-sitter and returns SourceFuncs.
func ParseRustFunctions(root string) []oculus.SourceFunc {
	parser := sitter.NewParser()
	parser.SetLanguage(rust.GetLanguage())

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
			if base == "target" || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".rs" {
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

		extractRustFuncs(tree.RootNode(), src, pkg, filepath.ToSlash(rel), &funcs)
		return nil
	})
	return funcs
}

func extractRustFuncs(root *sitter.Node, src []byte, pkg, file string, funcs *[]oculus.SourceFunc) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		switch child.Type() {
		case "function_item":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)

			var paramTypes []string
			if params := child.ChildByFieldName("parameters"); params != nil {
				paramTypes = extractRustParamTypes(params, src)
			}

			var returnTypes []string
			if retType := child.ChildByFieldName("return_type"); retType != nil {
				rt := retType.Content(src)
				rt = strings.TrimPrefix(rt, "-> ")
				rt = strings.TrimSpace(rt)
				if rt != "" && rt != "()" {
					returnTypes = []string{rt}
				}
			}

			var callees []string
			if body := child.ChildByFieldName("body"); body != nil {
				callees = extractCallExpressions(body, src)
			}

			exported := !strings.HasPrefix(name, "_")

			*funcs = append(*funcs, oculus.SourceFunc{
				Name: name, Package: pkg, File: file,
				Line: int(child.StartPoint().Row) + 1, EndLine: int(child.EndPoint().Row) + 1,
				ParamTypes: paramTypes, ReturnTypes: returnTypes,
				Callees: callees, Exported: exported,
			})

		case "impl_item":
			if body := child.ChildByFieldName("body"); body != nil {
				extractRustFuncs(body, src, pkg, file, funcs)
			}
		}
	}
}

func extractRustParamTypes(params *sitter.Node, src []byte) []string {
	var types []string
	for i := 0; i < int(params.ChildCount()); i++ {
		param := params.Child(i)
		if param.Type() == "parameter" || param.Type() == "self_parameter" {
			if param.Type() == "self_parameter" {
				continue
			}
			if typeNode := param.ChildByFieldName("type"); typeNode != nil {
				types = append(types, typeNode.Content(src))
			}
		}
	}
	return types
}

// extractCallExpressions is a generic tree-sitter call extractor that works
// for languages using call_expression nodes (Rust, Java, C, C++, etc.).
func extractCallExpressions(node *sitter.Node, src []byte) []string {
	seen := make(map[string]bool)
	var callees []string
	walkCallExpressions(node, src, seen, &callees)
	return callees
}

func walkCallExpressions(node *sitter.Node, src []byte, seen map[string]bool, callees *[]string) {
	if node.Type() == "call_expression" {
		if fn := node.ChildByFieldName("function"); fn != nil {
			name := extractSimpleName(fn, src)
			if name != "" && !seen[name] {
				seen[name] = true
				*callees = append(*callees, name)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		walkCallExpressions(node.Child(i), src, seen, callees)
	}
}

// extractSimpleName gets the last identifier from a possibly qualified name.
func extractSimpleName(node *sitter.Node, src []byte) string {
	content := node.Content(src)
	// Handle qualified names: foo::bar::baz → baz, foo.bar → bar
	if idx := strings.LastIndexAny(content, ".:"); idx >= 0 {
		content = content[idx+1:]
	}
	return content
}
