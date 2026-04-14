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
		return oculus.NewFuncIndexSource(funcs)
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

		// C++ uses same function_definition as C
		extractCLangFuncs(tree.RootNode(), src, pkg, filepath.ToSlash(rel), &funcs)
		return nil
	})
	return funcs
}
