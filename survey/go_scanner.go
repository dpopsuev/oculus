package survey

import (
	"bufio"
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/dpopsuev/oculus/v3/model"
)

var errNoModuleDirective = errors.New("no module directive found")

// GoScanner extracts structural metadata from Go source trees.
type GoScanner struct{}

func (s *GoScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	modPath, err := readModulePath(filepath.Join(absRoot, "go.mod"))
	if err != nil {
		return nil, err
	}

	mod := model.NewProject(modPath)
	mod.Language = model.LangGo
	mod.DependencyGraph = model.NewDependencyGraph()

	pkgs := make(map[string]*model.Namespace)

	err = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if ShouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}

		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return err
		}

		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil
		}

		dir := filepath.Dir(rel)
		pkgName := f.Name.Name

		var importPath string
		if dir == "." {
			importPath = modPath
		} else {
			importPath = modPath + "/" + filepath.ToSlash(dir)
		}

		// External test packages (package foo_test) are distinct components.
		// They appear as separate nodes in the import graph so coverage_gap
		// detection can see cross-package test imports.
		if strings.HasSuffix(pkgName, "_test") {
			importPath += "_test"
		}

		pkg, ok := pkgs[importPath]
		if !ok {
			pkg = model.NewNamespace(pkgName, importPath)
			pkgs[importPath] = pkg
		}

		fileObj := model.NewFile(filepath.ToSlash(rel), pkgName)
		if tokFile := fset.File(f.Pos()); tokFile != nil {
			fileObj.Lines = tokFile.LineCount()
		}
		pkg.AddFile(fileObj)

		extractSymbols(f, fset, filepath.ToSlash(rel), pkg)
		extractImports(f, importPath, modPath, mod.DependencyGraph)

		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, p := range slices.Sorted(maps.Keys(pkgs)) {
		pkg := pkgs[p]
		slices.SortFunc(pkg.Files, func(a, b *model.File) int {
			return cmp.Compare(a.Path, b.Path)
		})
		mod.AddNamespace(pkg)
	}

	return mod, nil
}

func extractSymbols(f *ast.File, fset *token.FileSet, filePath string, pkg *model.Namespace) {
	seen := make(map[string]bool)
	for _, s := range pkg.Symbols {
		seen[s.Name] = true
	}

	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv != nil {
				continue
			}
			name := d.Name.Name
			if seen[name] {
				continue
			}
			seen[name] = true
			pkg.AddSymbol(&model.Symbol{
				Name:      name,
				Kind:      model.SymbolFunction,
				Exported:  ast.IsExported(name),
				Signature: formatFuncSignature(fset, d),
				File:      filePath,
				Line:      fset.Position(d.Pos()).Line,
				EndLine:   fset.Position(d.End()).Line,
			})

		case *ast.GenDecl:
			extractGenDeclSymbols(d, fset, filePath, pkg, seen)
		}
	}
}

// extractGenDeclSymbols extracts type and value symbols from a GenDecl and
// adds them to the namespace. Shared between GoScanner and PackagesScanner.
func extractGenDeclSymbols(d *ast.GenDecl, fset *token.FileSet, filePath string, ns *model.Namespace, seen map[string]bool) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			name := s.Name.Name
			if seen[name] {
				continue
			}
			seen[name] = true
			kind := model.SymbolStruct
			sig := ""
			switch t := s.Type.(type) {
			case *ast.InterfaceType:
				kind = model.SymbolInterface
				sig = formatInterfaceSignature(fset, name, t)
			case *ast.StructType:
				sig = formatStructSignature(fset, name, t)
			}
			ns.AddSymbol(&model.Symbol{
				Name:      name,
				Kind:      kind,
				Exported:  ast.IsExported(name),
				Signature: sig,
				File:      filePath,
				Line:      fset.Position(s.Pos()).Line,
				EndLine:   fset.Position(s.End()).Line,
			})

		case *ast.ValueSpec:
			for _, ident := range s.Names {
				name := ident.Name
				if seen[name] {
					continue
				}
				seen[name] = true
				kind := model.SymbolVariable
				if d.Tok == token.CONST {
					kind = model.SymbolConstant
				}
				ns.AddSymbol(&model.Symbol{
					Name:     name,
					Kind:     kind,
					Exported: ast.IsExported(name),
					File:     filePath,
					Line:     fset.Position(ident.Pos()).Line,
					EndLine:  fset.Position(s.End()).Line,
				})
			}
		}
	}
}

func extractImports(f *ast.File, fromPkg, modPath string, graph *model.DependencyGraph) {
	for _, imp := range f.Imports {
		to := strings.Trim(imp.Path.Value, `"`)
		external := !strings.HasPrefix(to, modPath)
		graph.AddEdge(fromPkg, to, external)
	}
}

func readModulePath(goModPath string) (string, error) {
	f, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module ")), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("%s: %w", goModPath, errNoModuleDirective)
}

// ScanFile implements FileScanner. It parses a single Go source file and
// returns a Project containing that file's package namespace and symbols.
// The module path is read from the nearest go.mod up the directory tree;
// when none is found the package directory name is used as a fallback.
func (s *GoScanner) ScanFile(path string) (*model.Project, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, absPath, nil, 0)
	if err != nil {
		return nil, err
	}

	// Walk up to find go.mod for the module path.
	modPath := ""
	dir := filepath.Dir(absPath)
	for d := dir; ; d = filepath.Dir(d) {
		candidate := filepath.Join(d, "go.mod")
		if mp, e := readModulePath(candidate); e == nil {
			// Convert the file's directory to an import path relative to the module root.
			rel, _ := filepath.Rel(d, dir)
			if rel == "." || rel == "" {
				modPath = mp
			} else {
				modPath = mp + "/" + filepath.ToSlash(rel)
			}
			break
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
	}
	if modPath == "" {
		modPath = filepath.Base(dir)
	}

	pkg := model.NewNamespace(f.Name.Name, modPath)
	rel := filepath.Base(absPath)
	fileObj := model.NewFile(rel, f.Name.Name)
	if tokFile := fset.File(f.Pos()); tokFile != nil {
		fileObj.Lines = tokFile.LineCount()
	}
	pkg.AddFile(fileObj)
	extractSymbols(f, fset, rel, pkg)

	proj := model.NewProject(modPath)
	proj.Language = model.LangGo
	proj.DependencyGraph = model.NewDependencyGraph()
	proj.AddNamespace(pkg)
	return proj, nil
}

func formatFuncSignature(fset *token.FileSet, d *ast.FuncDecl) string {
	stub := &ast.FuncDecl{
		Name: d.Name,
		Type: d.Type,
		Recv: d.Recv,
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, stub); err != nil {
		return ""
	}
	return strings.TrimSpace(buf.String())
}

func formatInterfaceSignature(fset *token.FileSet, name string, t *ast.InterfaceType) string {
	if t.Methods == nil || len(t.Methods.List) == 0 {
		return fmt.Sprintf("interface %s {}", name)
	}
	methods := make([]string, 0, len(t.Methods.List))
	for _, m := range t.Methods.List {
		var buf bytes.Buffer
		if err := printer.Fprint(&buf, fset, m.Type); err != nil {
			continue
		}
		sig := strings.TrimPrefix(buf.String(), "func")
		for _, n := range m.Names {
			methods = append(methods, n.Name+sig)
		}
	}
	return fmt.Sprintf("interface %s { %s }", name, strings.Join(methods, "; "))
}

func formatStructSignature(fset *token.FileSet, name string, t *ast.StructType) string {
	if t.Fields == nil || len(t.Fields.List) == 0 {
		return fmt.Sprintf("struct %s {}", name)
	}
	fields := make([]string, 0, len(t.Fields.List))
	for _, f := range t.Fields.List {
		var buf bytes.Buffer
		if err := printer.Fprint(&buf, fset, f.Type); err != nil {
			continue
		}
		typStr := buf.String()
		if len(f.Names) == 0 {
			fields = append(fields, typStr)
			continue
		}
		for _, n := range f.Names {
			fields = append(fields, n.Name+" "+typStr)
		}
	}
	return fmt.Sprintf("struct %s { %s }", name, strings.Join(fields, "; "))
}
