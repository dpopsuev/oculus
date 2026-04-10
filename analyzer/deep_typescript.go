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
	RegisterSource(lang.TypeScript, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
		if lang.DetectLanguage(root) != lang.TypeScript {
			return nil
		}
		funcs := ParseTypeScriptFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs)
	})
}

// ParseTypeScriptFunctions parses all TS/JS files and returns SourceFuncs
// with type annotations extracted from tree-sitter AST.
func ParseTypeScriptFunctions(root string) []oculus.SourceFunc {
	parser := ts.NewParser()
	parser.SetLanguage(ts.TypeScript())

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
			if lang.ShouldSkipTSDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext != extTS && ext != extTSX && ext != extJS && ext != extJSX {
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

		extractTSSourceFuncs(tree.RootNode(), src, pkg, filepath.ToSlash(rel), &funcs)
		return nil
	})
	return funcs
}

func extractTSSourceFuncs(root ts.Node, src []byte, pkg, file string, funcs *[]oculus.SourceFunc) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		switch child.Type() {
		case "function_declaration", "method_definition":
			nameNode := child.ChildByFieldName("name")
			if nameNode == nil {
				continue
			}
			name := nameNode.Content(src)
			if name == "" {
				continue
			}

			paramTypes := extractTSParamTypes(child, src)
			returnTypes := extractTSReturnType(child, src)

			body := child.ChildByFieldName("body")
			var callees []string
			if body != nil {
				callees = extractTSCallees(body, src)
			}

			*funcs = append(*funcs, oculus.SourceFunc{
				Name:        name,
				Package:     pkg,
				File:        file,
				Line:        int(child.StartPoint().Row) + 1,
				EndLine:     int(child.EndPoint().Row) + 1,
				ParamTypes:  paramTypes,
				ReturnTypes: returnTypes,
				Callees:     callees,
				Exported:    true, // TS functions are public by default
			})

		case "export_statement", "lexical_declaration":
			extractTSSourceFuncs(child, src, pkg, file, funcs)

		case "variable_declarator":
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")
			if nameNode != nil && valueNode != nil && isArrowOrFunction(valueNode) {
				name := nameNode.Content(src)
				body := valueNode.ChildByFieldName("body")
				var callees []string
				if body != nil {
					callees = extractTSCallees(body, src)
				}
				paramTypes := extractTSParamTypes(valueNode, src)
				returnTypes := extractTSReturnType(valueNode, src)

				*funcs = append(*funcs, oculus.SourceFunc{
					Name:        name,
					Package:     pkg,
					File:        file,
					Line:        int(child.StartPoint().Row) + 1,
					EndLine:     int(child.EndPoint().Row) + 1,
					ParamTypes:  paramTypes,
					ReturnTypes: returnTypes,
					Callees:     callees,
					Exported:    true,
				})
			}

		case "class_declaration":
			if bodyNode := child.ChildByFieldName("body"); bodyNode != nil {
				extractTSSourceFuncs(bodyNode, src, pkg, file, funcs)
			}
		}
	}
}

// extractTSParamTypes extracts type annotations from TS function parameters.
// Handles: function foo(x: string, y: number): ...
func extractTSParamTypes(funcNode ts.Node, src []byte) []string {
	params := funcNode.ChildByFieldName("parameters")
	if params == nil {
		return nil
	}
	var types []string
	for i := 0; i < int(params.ChildCount()); i++ {
		param := params.Child(i)
		// required_parameter, optional_parameter have a "type" field
		if typeNode := param.ChildByFieldName("type"); typeNode != nil {
			// Type annotation node wraps the actual type
			t := typeNode.Content(src)
			// Strip leading ": " if present
			t = strings.TrimPrefix(t, ": ")
			if t != "" {
				types = append(types, t)
			}
		}
	}
	return types
}

// extractTSReturnType extracts the return type annotation.
// Handles: function foo(): string { ... }
func extractTSReturnType(funcNode ts.Node, src []byte) []string {
	retType := funcNode.ChildByFieldName("return_type")
	if retType == nil {
		return nil
	}
	t := retType.Content(src)
	t = strings.TrimPrefix(t, ": ")
	t = strings.TrimSpace(t)
	if t == "" || t == "void" {
		return nil
	}
	return []string{t}
}

func isArrowOrFunction(node ts.Node) bool {
	t := node.Type()
	return t == "arrow_function" || t == "function" || t == "function_expression"
}

func extractTSCallees(node ts.Node, src []byte) []string {
	seen := make(map[string]bool)
	var callees []string
	collectTSCalls(node, src, seen, &callees)
	return callees
}

func collectTSCalls(node ts.Node, src []byte, seen map[string]bool, callees *[]string) {
	collectTreeSitterCalls(node, src, "call_expression", "function", tsNameExtractor, seen, callees)
}

func tsNameExtractor(fn ts.Node, src []byte) string {
	switch fn.Type() {
	case "identifier":
		return fn.Content(src)
	case "member_expression":
		if prop := fn.ChildByFieldName("property"); prop != nil {
			return prop.Content(src)
		}
	}
	return ""
}
