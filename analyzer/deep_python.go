package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/python"
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
		return oculus.NewFuncIndexSource(funcs)
	})
}

// ParsePythonFunctions parses all .py files and returns SourceFuncs
// with type annotations extracted from tree-sitter AST.
func ParsePythonFunctions(root string) []oculus.SourceFunc {
	parser := sitter.NewParser()
	parser.SetLanguage(python.GetLanguage())

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
		tree, parseErr := parser.ParseCtx(context.Background(), nil, src)
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

func extractPySourceFuncs(root *sitter.Node, src []byte, pkg, file string, funcs *[]oculus.SourceFunc) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
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
			if body := child.ChildByFieldName("body"); body != nil {
				callees = extractPyCallees(body, src)
			}

			exported := len(name) > 0 && !strings.HasPrefix(name, "_")

			*funcs = append(*funcs, oculus.SourceFunc{
				Name:        name,
				Package:     pkg,
				File:        file,
				Line:        int(child.StartPoint().Row) + 1,
				EndLine:     int(child.EndPoint().Row) + 1,
				ParamTypes:  paramTypes,
				ReturnTypes: returnTypes,
				Callees:     callees,
				Exported:    exported,
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
func extractPyParamTypes(params *sitter.Node, src []byte) []string {
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

func extractPyCallees(node *sitter.Node, src []byte) []string {
	seen := make(map[string]bool)
	var callees []string
	collectPyCalls(node, src, seen, &callees)
	return callees
}

func collectPyCalls(node *sitter.Node, src []byte, seen map[string]bool, callees *[]string) {
	collectTreeSitterCalls(node, src, "call", "function", pyNameExtractor, seen, callees)
}

func pyNameExtractor(fn *sitter.Node, src []byte) string {
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
