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
	RegisterSource(lang.TypeScript, 80, func(root string, _ lsp.Pool) oculus.SymbolSource {
		if lang.DetectLanguage(root) != lang.TypeScript {
			return nil
		}
		funcs := ParseTypeScriptFunctions(root)
		if len(funcs) == 0 {
			return nil
		}
		return oculus.NewFuncIndexSource(funcs, ExtractFileImports(root))
	})
}

// ParseTypeScriptFunctions parses all TS/JS files and returns SourceFuncs
// with type annotations extracted from tree-sitter AST.
func ParseTypeScriptFunctions(root string) []oculus.Symbol {
	parser := ts.NewParser()
	parser.SetLanguage(ts.TypeScript())

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

func extractTSSourceFuncs(root ts.Node, src []byte, pkg, file string, funcs *[]oculus.Symbol) {
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
			sym := oculus.Symbol{
				Name:        name,
				Package:     pkg,
				File:        file,
				Line:        int(child.StartPoint().Row) + 1,
				EndLine:     int(child.EndPoint().Row) + 1,
				ParamTypes:  paramTypes,
				ReturnTypes: returnTypes,
				Exported:    true, // TS functions are public by default
			}
			if body != nil {
				applyTSCallSites(&sym, body, src)
			}
			*funcs = append(*funcs, sym)

		case "export_statement", "lexical_declaration":
			extractTSSourceFuncs(child, src, pkg, file, funcs)

		case "variable_declarator":
			nameNode := child.ChildByFieldName("name")
			valueNode := child.ChildByFieldName("value")
			if nameNode != nil && valueNode != nil && isArrowOrFunction(valueNode) {
				name := nameNode.Content(src)
				body := valueNode.ChildByFieldName("body")
				paramTypes := extractTSParamTypes(valueNode, src)
				returnTypes := extractTSReturnType(valueNode, src)

				sym := oculus.Symbol{
					Name:        name,
					Package:     pkg,
					File:        file,
					Line:        int(child.StartPoint().Row) + 1,
					EndLine:     int(child.EndPoint().Row) + 1,
					ParamTypes:  paramTypes,
					ReturnTypes: returnTypes,
					Exported:    true,
				}
				if body != nil {
					applyTSCallSites(&sym, body, src)
				}
				*funcs = append(*funcs, sym)
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

func applyTSCallSites(sym *oculus.Symbol, body ts.Node, src []byte) {
	sites := extractTSCallSites(body, src)
	sym.Callees = sites.bare
	sym.MemberCallees = sites.member
	sym.AsyncCallees = sites.asyncBare
	sym.MemberAsyncCallees = sites.asyncMember
	if len(sites.lines) > 0 {
		sym.CallLines = sites.lines
	}
}

type tsCallSites struct {
	bare        []string
	member      []string
	asyncBare   map[string]string
	asyncMember map[string]string
	lines       map[string]int
}

func extractTSCallSites(node ts.Node, src []byte) tsCallSites {
	seenBare := make(map[string]bool)
	seenMember := make(map[string]bool)
	out := tsCallSites{lines: make(map[string]int)}

	var walk func(ts.Node)
	walk = func(n ts.Node) {
		if n.Type() == "call_expression" {
			if fn := n.ChildByFieldName("function"); fn != nil {
				name, member := tsCalleeName(fn, src)
				if name != "" {
					line := int(fn.StartPoint().Row) + 1
					out.lines[name] = line
					if member {
						if !seenMember[name] {
							seenMember[name] = true
							out.member = append(out.member, name)
						}
					} else if !seenBare[name] {
						seenBare[name] = true
						out.bare = append(out.bare, name)
					}
				}
			}
		}
		for i := 0; i < int(n.ChildCount()); i++ {
			walk(n.Child(i))
		}
	}
	walk(node)

	out.asyncBare, out.asyncMember = extractTSAsyncCalleesSplit(node, src)
	return out
}

// extractTSCallees returns bare identifier callees (legacy helper for tests).
func extractTSCallees(node ts.Node, src []byte) []string {
	return extractTSCallSites(node, src).bare
}

// extractTSAsyncCallees walks a function body and returns async seams
// (bare + member merged; prefer extractTSAsyncCalleesSplit for resolution).
func extractTSAsyncCallees(node ts.Node, src []byte) map[string]string {
	bare, member := extractTSAsyncCalleesSplit(node, src)
	if len(bare) == 0 {
		return member
	}
	if len(member) == 0 {
		return bare
	}
	out := make(map[string]string, len(bare)+len(member))
	for k, v := range bare {
		out[k] = v
	}
	for k, v := range member {
		out[k] = v
	}
	return out
}

// extractTSAsyncCalleesSplit separates await/promise callees by member vs bare.
//   - await_expression wrapping a call_expression  → CallEdgeAwait
//   - .then/.catch/.finally callback identifiers     → CallEdgePromise (bare)
func extractTSAsyncCalleesSplit(node ts.Node, src []byte) (bare, member map[string]string) {
	bare = make(map[string]string)
	member = make(map[string]string)
	var walk func(ts.Node)
	walk = func(n ts.Node) {
		switch n.Type() {
		case "await_expression":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Type() == "call_expression" {
					if fn := child.ChildByFieldName("function"); fn != nil {
						if name, isMember := tsCalleeName(fn, src); name != "" {
							if isMember {
								member[name] = oculus.CallEdgeAwait
							} else {
								bare[name] = oculus.CallEdgeAwait
							}
						}
					}
				}
			}
		case "call_expression":
			if fn := n.ChildByFieldName("function"); fn != nil && fn.Type() == "member_expression" {
				if prop := fn.ChildByFieldName("property"); prop != nil {
					switch prop.Content(src) {
					case "then", "catch", "finally":
						if args := n.ChildByFieldName("arguments"); args != nil {
							for i := 0; i < int(args.ChildCount()); i++ {
								arg := args.Child(i)
								if arg.Type() == "identifier" {
									bare[arg.Content(src)] = oculus.CallEdgePromise
								}
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
	walk(node)
	if len(bare) == 0 {
		bare = nil
	}
	if len(member) == 0 {
		member = nil
	}
	return bare, member
}

func tsCalleeName(fn ts.Node, src []byte) (name string, member bool) {
	switch fn.Type() {
	case "identifier":
		return fn.Content(src), false
	case "member_expression":
		if prop := fn.ChildByFieldName("property"); prop != nil {
			return prop.Content(src), true
		}
	}
	return "", false
}

func tsNameExtractor(fn ts.Node, src []byte) string {
	name, _ := tsCalleeName(fn, src)
	return name
}
