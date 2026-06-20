package analyzer

import (
	"os"
	"path/filepath"
	"strings"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/ts"
)

// ExtractFileImports walks source files and extracts import paths per file.
// Returns a map from relative file path → imported module/package paths.
func ExtractFileImports(root string) oculus.FileImports {
	detected := lang.DetectLanguage(root)
	switch detected {
	case lang.Rust:
		return extractRustImports(root)
	case lang.Python:
		return extractPythonImports(root)
	case lang.TypeScript:
		return extractTSImports(root)
	case lang.Go:
		return extractGoImports(root)
	default:
		return nil
	}
}

func extractRustImports(root string) oculus.FileImports {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	parser := ts.NewParser()
	parser.SetLanguage(ts.Rust())

	imports := make(oculus.FileImports)
	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if lang.ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".rs" {
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
		relSlash := filepath.ToSlash(rel)

		var paths []string
		root := tree.RootNode()
		for i := 0; i < int(root.ChildCount()); i++ {
			child := root.Child(i)
			if child.Type() == "use_declaration" {
				raw := child.Content(src)
				raw = strings.TrimPrefix(raw, "use ")
				raw = strings.TrimSuffix(raw, ";")
				// "std::collections::HashMap" → "std::collections"
				if idx := strings.LastIndex(raw, "::"); idx >= 0 {
					raw = raw[:idx]
				}
				raw = strings.ReplaceAll(raw, "::", "/")
				if raw != "" {
					paths = append(paths, raw)
				}
			}
		}
		if len(paths) > 0 {
			imports[relSlash] = paths
		}
		return nil
	})
	return imports
}

func extractPythonImports(root string) oculus.FileImports {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	parser := ts.NewParser()
	parser.SetLanguage(ts.Python())

	imports := make(oculus.FileImports)
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
		relSlash := filepath.ToSlash(rel)

		var paths []string
		rootNode := tree.RootNode()
		for i := 0; i < int(rootNode.ChildCount()); i++ {
			child := rootNode.Child(i)
			switch child.Type() {
			case "import_statement":
				// import foo.bar → "foo.bar"
				if nameNode := child.ChildByFieldName("name"); nameNode != nil {
					paths = append(paths, nameNode.Content(src))
				}
			case "import_from_statement":
				// from foo.bar import baz → "foo.bar"
				if modNode := child.ChildByFieldName("module_name"); modNode != nil {
					paths = append(paths, modNode.Content(src))
				}
			}
		}
		if len(paths) > 0 {
			imports[relSlash] = paths
		}
		return nil
	})
	return imports
}

func extractTSImports(root string) oculus.FileImports {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	parser := ts.NewParser()
	parser.SetLanguage(ts.TypeScript())

	imports := make(oculus.FileImports)
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
		if ext != ".ts" && ext != ".tsx" {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".d.ts") {
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
		relSlash := filepath.ToSlash(rel)

		var paths []string
		rootNode := tree.RootNode()
		for i := 0; i < int(rootNode.ChildCount()); i++ {
			child := rootNode.Child(i)
			if child.Type() != "import_statement" {
				continue
			}
			// import { foo } from "./bar" → source is the string literal
			if source := child.ChildByFieldName("source"); source != nil {
				raw := source.Content(src)
				raw = strings.Trim(raw, `"'`)
				if raw != "" {
					paths = append(paths, raw)
				}
			}
		}
		if len(paths) > 0 {
			imports[relSlash] = paths
		}
		return nil
	})
	return imports
}

func extractGoImports(root string) oculus.FileImports {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil
	}
	parser := ts.NewParser()
	parser.SetLanguage(ts.Go())

	imports := make(oculus.FileImports)
	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == dirVendor || base == dirTestdata || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(d.Name()) != ".go" {
			return nil
		}
		if strings.HasSuffix(d.Name(), "_test.go") {
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
		relSlash := filepath.ToSlash(rel)

		var paths []string
		rootNode := tree.RootNode()
		for i := 0; i < int(rootNode.ChildCount()); i++ {
			child := rootNode.Child(i)
			if child.Type() != "import_declaration" {
				continue
			}
			content := child.Content(src)
			for _, line := range strings.Split(content, "\n") {
				line = strings.TrimSpace(line)
				line = strings.Trim(line, `"`)
				if line == "" || line == "import" || line == "(" || line == ")" {
					continue
				}
				// Handle aliased imports: alias "path" → extract path
				if idx := strings.LastIndex(line, `"`); idx > 0 {
					start := strings.Index(line, `"`)
					if start >= 0 && start < idx {
						line = line[start+1 : idx]
					}
				}
				if strings.Contains(line, "/") || strings.Contains(line, ".") {
					paths = append(paths, line)
				}
			}
		}
		if len(paths) > 0 {
			imports[relSlash] = paths
		}
		return nil
	})
	return imports
}
