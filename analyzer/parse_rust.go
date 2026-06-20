package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"

	"github.com/dpopsuev/oculus/v3/ts"
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
		return oculus.NewFuncIndexSource(funcs, ExtractFileImports(root))
	})
}

// ParseRustFunctions parses .rs files via tree-sitter and returns SourceFuncs.
func ParseRustFunctions(root string) []oculus.Symbol {
	parser := ts.NewParser()
	parser.SetLanguage(ts.Rust())

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
		tree, err := parser.Parse(src)
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

func extractRustFuncs(root ts.Node, src []byte, pkg, file string, funcs *[]oculus.Symbol) {
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

			isAsync := rustHasAsyncModifier(child, src)
			var callees []string
			var asyncCallees map[string]string
			if body := child.ChildByFieldName("body"); body != nil {
				callees = extractCallExpressions(body, src)
				if isAsync {
					asyncCallees = extractRustAsyncCallees(body, src)
				}
			}

			exported := !strings.HasPrefix(name, "_")

			*funcs = append(*funcs, oculus.Symbol{
				Name: name, Package: pkg, File: file,
				Line: int(child.StartPoint().Row) + 1, EndLine: int(child.EndPoint().Row) + 1,
				ParamTypes: paramTypes, ReturnTypes: returnTypes,
				Callees: callees, AsyncCallees: asyncCallees, Exported: exported,
			})

		case "impl_item":
			if body := child.ChildByFieldName("body"); body != nil {
				extractRustFuncs(body, src, pkg, file, funcs)
			}
		}
	}
}

func extractRustParamTypes(params ts.Node, src []byte) []string {
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

// rustHasAsyncModifier reports whether a function_item node has an async modifier.
func rustHasAsyncModifier(fn ts.Node, src []byte) bool {
	for i := 0; i < int(fn.ChildCount()); i++ {
		child := fn.Child(i)
		if child.Type() == "function_modifiers" {
			for j := 0; j < int(child.ChildCount()); j++ {
				if child.Child(j).Content(src) == "async" {
					return true
				}
			}
		}
	}
	return false
}

// extractRustAsyncCallees detects async seams in a Rust async function body:
//   - expr.await            → CallEdgeAwait
//   - tokio::spawn / task::spawn → CallEdgeTaskSpawn
//   - tx.send(v)            → CallEdgeChanSend (heuristic: method named "send")
func extractRustAsyncCallees(body ts.Node, src []byte) map[string]string {
	out := make(map[string]string)
	var walk func(ts.Node)
	walk = func(n ts.Node) {
		switch n.Type() {
		case "await_expression":
			// expr.await — the inner expression is the first child
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Type() == "call_expression" {
					if fn := child.ChildByFieldName("function"); fn != nil {
						if name := extractSimpleName(fn, src); name != "" {
							out[name] = oculus.CallEdgeAwait
						}
					}
				} else if child.Type() == "field_expression" {
					// fetch(...).await — field_expression wraps the call
					if name := extractSimpleName(child, src); name != "" && name != "await" {
						out[name] = oculus.CallEdgeAwait
					}
				}
			}
		case "call_expression":
			if fn := n.ChildByFieldName("function"); fn != nil {
				name := extractSimpleName(fn, src)
				switch name {
				case "spawn":
					out[name] = oculus.CallEdgeTaskSpawn
				case "send":
					// tx.send(v) — heuristic channel send
					out[name] = oculus.CallEdgeChanSend
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(body)
	return out
}

// extractCallExpressions is a generic tree-sitter call extractor that works
// for languages using call_expression nodes (Rust, Java, C, C++, etc.).
func extractCallExpressions(node ts.Node, src []byte) []string {
	seen := make(map[string]bool)
	var callees []string
	walkCallExpressions(node, src, seen, &callees)
	return callees
}

func walkCallExpressions(node ts.Node, src []byte, seen map[string]bool, callees *[]string) {
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
func extractSimpleName(node ts.Node, src []byte) string {
	content := node.Content(src)
	// Handle qualified names: foo::bar::baz → baz, foo.bar → bar
	if idx := strings.LastIndexAny(content, ".:"); idx >= 0 {
		content = content[idx+1:]
	}
	return content
}
