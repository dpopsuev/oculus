package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"

	"github.com/dpopsuev/oculus/v3/ts"
)

func init() {
	RegisterSource(lang.Cpp, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
		if lang.DetectLanguage(root) != lang.Cpp {
			return nil
		}
		funcs := ParseCppFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs, ExtractFileImports(root))
	})
}

// ParseCppFunctions parses .cpp/.hpp files via tree-sitter.
func ParseCppFunctions(root string) []oculus.Symbol {
	parser := ts.NewParser()
	parser.SetLanguage(ts.Cpp())

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
			if strings.HasPrefix(d.Name(), ".") || d.Name() == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		if ext != ".cpp" && ext != ".hpp" && ext != ".cc" && ext != ".h" {
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

		extractCppLangFuncs(tree.RootNode(), src, pkg, filepath.ToSlash(rel), &funcs)
		return nil
	})
	return funcs
}

// extractCppLangFuncs is the C++ variant of extractCLangFuncs with async colouring.
func extractCppLangFuncs(root ts.Node, src []byte, pkg, file string, funcs *[]oculus.Symbol) {
	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		switch child.Type() {
		case "function_definition":
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
				if rt := retType.Content(src); rt != "" && rt != "void" {
					returnTypes = []string{rt}
				}
			}
			var callees []string
			var asyncCallees map[string]string
			if body := child.ChildByFieldName("body"); body != nil {
				callees = extractCallExpressions(body, src)
				asyncCallees = extractCppAsyncCallees(body, src)
			}
			*funcs = append(*funcs, oculus.Symbol{
				Name: name, Package: pkg, File: file,
				Line: int(child.StartPoint().Row) + 1, EndLine: int(child.EndPoint().Row) + 1,
				ReturnTypes: returnTypes, ParamTypes: extractCParamTypes(declarator, src),
				Callees: callees, AsyncCallees: asyncCallees, Exported: true,
			})
		case "namespace_definition", "class_specifier":
			if body := child.ChildByFieldName("body"); body != nil {
				extractCppLangFuncs(body, src, pkg, file, funcs)
			}
		}
	}
}
