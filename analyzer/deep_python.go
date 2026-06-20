package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"

	"github.com/dpopsuev/oculus/v3/ts"
)

func init() {
	RegisterSource(lang.Python, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
		if lang.DetectLanguage(root) != lang.Python {
			return nil
		}
		funcs := ParsePythonFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs, ExtractFileImports(root))
	})
}

// ParsePythonFunctions parses all .py files and returns SourceFuncs
// with type annotations extracted from tree-sitter AST.
func ParsePythonFunctions(root string) []oculus.Symbol {
	parser := ts.NewParser()
	parser.SetLanguage(ts.Python())

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

		extractPySourceFuncs(tree.RootNode(), src, pkg, filepath.ToSlash(rel), &funcs)
		return nil
	})
	return funcs
}

func extractPySourceFuncs(root ts.Node, src []byte, pkg, file string, funcs *[]oculus.Symbol) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		isAsync := child.Type() == "async_function_definition" || pyHasAsyncKeyword(child, src)
		if child.Type() == "function_definition" || child.Type() == "async_function_definition" {
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)
			if name == "" {
				continue
			}

			// Extract parameter types from annotations.
			var paramTypes []string
			if params := child.ChildByFieldName("parameters"); params != nil {
				paramTypes = extractPyParamTypes(params, src)
			}

			// Extract return type annotation.
			var returnTypes []string
			if retType := child.ChildByFieldName("return_type"); retType != nil {
				rt := strings.TrimSpace(retType.Content(src))
				if rt != "" && rt != "None" {
					returnTypes = []string{rt}
				}
			}

			// Extract callees from body.
			var callees []string
			var asyncCallees map[string]string
			if body := child.ChildByFieldName("body"); body != nil {
				callees = extractPyCallees(body, src)
					if isAsync {
					asyncCallees = extractPyAsyncCallees(body, src)
				}
			}

			exported := len(name) > 0 && !strings.HasPrefix(name, "_")

			*funcs = append(*funcs, oculus.Symbol{
				Name:         name,
				Package:      pkg,
				File:         file,
				Line:         int(child.StartPoint().Row) + 1,
				EndLine:      int(child.EndPoint().Row) + 1,
				ParamTypes:   paramTypes,
				ReturnTypes:  returnTypes,
				Callees:      callees,
				AsyncCallees: asyncCallees,
				Exported:     exported,
			})
		}
		// Recurse into class definitions to find methods.
		if child.Type() == "class_definition" {
			if bodyNode := child.ChildByFieldName("body"); bodyNode != nil {
				extractPySourceFuncs(bodyNode, src, pkg, file, funcs)
			}
		}
	}
}

// extractPyParamTypes extracts type annotations from Python function parameters.
// Handles: def foo(x: int, y: str, z: list[str]) -> ...
func extractPyParamTypes(params ts.Node, src []byte) []string {
	var types []string
	for i := 0; i < int(params.ChildCount()); i++ {
		param := params.Child(i)
		// Skip delimiters, self/cls
		if param.Type() != "typed_parameter" && param.Type() != "default_parameter" && param.Type() != "identifier" {
			continue
		}
		name := ""
		if param.Type() == "identifier" {
			name = param.Content(src)
		} else if nameNode := param.ChildByFieldName("name"); nameNode != nil {
			name = nameNode.Content(src)
		}
		if name == "self" || name == "cls" {
			continue
		}
		if param.Type() == "typed_parameter" {
			if typeNode := param.ChildByFieldName("type"); typeNode != nil {
				types = append(types, typeNode.Content(src))
			}
		}
		// default_parameter with type: handled via typed_default_parameter
		if param.Type() == "typed_default_parameter" {
			if typeNode := param.ChildByFieldName("type"); typeNode != nil {
				types = append(types, typeNode.Content(src))
			}
		}
	}
	return types
}

// isPyExported checks if a Python name is public (doesn't start with _).
func isPyExported(name string) bool {
	if len(name) == 0 {
		return false
	}
	r := rune(name[0])
	return unicode.IsLetter(r) && r != '_'
}

func extractPyCallees(node ts.Node, src []byte) []string {
	seen := make(map[string]bool)
	var callees []string
	collectPyCalls(node, src, seen, &callees)
	return callees
}

func collectPyCalls(node ts.Node, src []byte, seen map[string]bool, callees *[]string) {
	collectTreeSitterCalls(node, src, "call", "function", pyNameExtractor, seen, callees)
}

// extractPyAsyncCallees detects async seams in a Python async function body:
//   - await <call>          → CallEdgeAwait
//   - asyncio.create_task() → CallEdgeTaskSpawn
//   - asyncio.gather()      → CallEdgeTaskSpawn
func extractPyAsyncCallees(body ts.Node, src []byte) map[string]string {
	out := make(map[string]string)
	var walk func(ts.Node)
	walk = func(n ts.Node) {
		if n.Type() == "await" {
			// tree-sitter Python emits [await keyword, expr] as children
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				switch child.Type() {
				case "call":
					if fn := child.ChildByFieldName("function"); fn != nil {
						if name := pyNameExtractor(fn, src); name != "" {
							out[name] = oculus.CallEdgeAwait
						}
					}
				case "identifier":
					// await coroutine_var (not the keyword token itself)
					if name := child.Content(src); name != "await" && name != "" {
						out[name] = oculus.CallEdgeAwait
					}
				}
			}
		}
		if n.Type() == "call" {
			if fn := n.ChildByFieldName("function"); fn != nil {
				// asyncio.create_task(coro) / asyncio.gather(*coros)
				raw := fn.Content(src)
				if raw == "asyncio.create_task" || raw == "asyncio.gather" || raw == "create_task" || raw == "gather" {
					// Extract first argument as the spawned target.
					if args := n.ChildByFieldName("arguments"); args != nil {
						for i := 0; i < int(args.ChildCount()); i++ {
							arg := args.Child(i)
							if arg.Type() == "identifier" {
								out[arg.Content(src)] = oculus.CallEdgeTaskSpawn
							}
						}
					}
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

// pyHasAsyncKeyword reports whether a function_definition node
// has an "async" keyword as its first non-whitespace child token.
func pyHasAsyncKeyword(node ts.Node, src []byte) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.IsNamed() {
			break // named child means we're past the keywords
		}
		if child.Content(src) == "async" {
			return true
		}
	}
	return false
}

func pyNameExtractor(fn ts.Node, src []byte) string {
	switch fn.Type() {
	case "identifier":
		return fn.Content(src)
	case "attribute":
		if attr := fn.ChildByFieldName("attribute"); attr != nil {
			return attr.Content(src)
		}
	}
	return ""
}
