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
	clang "github.com/smacker/go-tree-sitter/c"
)

func init() {
	RegisterSource(lang.C, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
		if lang.DetectLanguage(root) != lang.C {
			return nil
		}
		funcs := ParseCFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs)
	})
}

// ParseCFunctions parses .c/.h files via tree-sitter.
func ParseCFunctions(root string) []oculus.SourceFunc {
	parser := sitter.NewParser()
	parser.SetLanguage(clang.GetLanguage())

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
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext != ".c" && ext != ".h" {
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

		extractCLangFuncs(tree.RootNode(), src, pkg, filepath.ToSlash(rel), &funcs)
		return nil
	})
	return funcs
}

func extractCLangFuncs(root *sitter.Node, src []byte, pkg, file string, funcs *[]oculus.SourceFunc) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		if child.Type() != "function_definition" {
			continue
		}

		// C function: return_type name(params) { body }
		declarator := child.ChildByFieldName("declarator")
		if declarator == nil {
			continue
		}

		name := extractCFuncName(declarator, src)
		if name == "" {
			continue
		}

		var returnTypes []string
		if retType := child.ChildByFieldName("type"); retType != nil {
			rt := retType.Content(src)
			if rt != "" && rt != "void" {
				returnTypes = []string{rt}
			}
		}

		var paramTypes []string
		paramTypes = extractCParamTypes(declarator, src)

		var callees []string
		if body := child.ChildByFieldName("body"); body != nil {
			callees = extractCallExpressions(body, src)
		}

		*funcs = append(*funcs, oculus.SourceFunc{
			Name: name, Package: pkg, File: file,
			Line: int(child.StartPoint().Row) + 1, EndLine: int(child.EndPoint().Row) + 1,
			ParamTypes: paramTypes, ReturnTypes: returnTypes,
			Callees: callees, Exported: true,
		})
	}
}

func extractCFuncName(declarator *sitter.Node, src []byte) string {
	// function_declarator → declarator (identifier) + parameters
	if declarator.Type() == "function_declarator" {
		if nameNode := declarator.ChildByFieldName("declarator"); nameNode != nil {
			// Could be pointer_declarator wrapping identifier
			return extractSimpleName(nameNode, src)
		}
	}
	// Fallback: try direct
	if nameNode := declarator.ChildByFieldName("name"); nameNode != nil {
		return nameNode.Content(src)
	}
	return ""
}

func extractCParamTypes(declarator *sitter.Node, src []byte) []string {
	var types []string
	// Find the parameter_list inside the function_declarator
	if declarator.Type() != "function_declarator" {
		return nil
	}
	if params := declarator.ChildByFieldName("parameters"); params != nil {
		for i := 0; i < int(params.ChildCount()); i++ {
			param := params.Child(i)
			if param.Type() == "parameter_declaration" {
				if typeNode := param.ChildByFieldName("type"); typeNode != nil {
					types = append(types, typeNode.Content(src))
				}
			}
		}
	}
	return types
}
