package analyzer

import (
	"testing"

	"github.com/dpopsuev/oculus/v3/ts"
)

// Grammar verification tests for all tree-sitter languages.
// Each test parses a minimal fixture and verifies the node types
// and field names that our parse_*.go files depend on.

func parseLang(t *testing.T, lang ts.Language, src string) ts.Node {
	t.Helper()
	parser := ts.NewParser()
	parser.SetLanguage(lang)
	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return tree.RootNode()
}

// --- Python ---

func TestGrammar_Python_FunctionDef(t *testing.T) {
	root := parseLang(t, ts.Python(), `def load_config(path: str) -> dict:
    return {"name": path}
`)
	fn := findChildByType(root, "function_definition")
	if fn == nil {
		t.Fatal("expected function_definition")
	}
	if nameNode := fn.ChildByFieldName("name"); nameNode == nil {
		t.Error("function_definition missing 'name' field")
	}
	if params := fn.ChildByFieldName("parameters"); params == nil {
		t.Error("function_definition missing 'parameters' field")
	}
	if retType := fn.ChildByFieldName("return_type"); retType == nil {
		t.Error("function_definition missing 'return_type' field")
	}
	if body := fn.ChildByFieldName("body"); body == nil {
		t.Error("function_definition missing 'body' field")
	}
}

func TestGrammar_Python_TypedParam(t *testing.T) {
	root := parseLang(t, ts.Python(), `def foo(x: int, y: str):
    pass
`)
	fn := findChildByType(root, "function_definition")
	params := fn.ChildByFieldName("parameters")
	var typed int
	for i := 0; i < params.ChildCount(); i++ {
		if params.Child(i).Type() == "typed_parameter" {
			typed++
		}
	}
	if typed != 2 {
		t.Errorf("expected 2 typed_parameter nodes, got %d", typed)
	}
}

func TestGrammar_Python_Call(t *testing.T) {
	root := parseLang(t, ts.Python(), `x = foo(1)
`)
	var found bool
	walkForType(root, "call", func(node ts.Node) {
		if fn := node.ChildByFieldName("function"); fn != nil {
			found = true
		}
	})
	if !found {
		t.Error("call node missing 'function' field")
	}
}

// --- TypeScript ---

func TestGrammar_TypeScript_FunctionDecl(t *testing.T) {
	root := parseLang(t, ts.TypeScript(), `function loadConfig(path: string): Config {
  return { name: path };
}
`)
	fn := findChildByType(root, "function_declaration")
	if fn == nil {
		t.Fatal("expected function_declaration")
	}
	if nameNode := fn.ChildByFieldName("name"); nameNode == nil {
		t.Error("missing 'name' field")
	}
	if params := fn.ChildByFieldName("parameters"); params == nil {
		t.Error("missing 'parameters' field")
	}
	if retType := fn.ChildByFieldName("return_type"); retType == nil {
		t.Error("missing 'return_type' field")
	}
	if body := fn.ChildByFieldName("body"); body == nil {
		t.Error("missing 'body' field")
	}
}

func TestGrammar_TypeScript_CallExpression(t *testing.T) {
	root := parseLang(t, ts.TypeScript(), `const x = foo(1);
`)
	var found bool
	walkForType(root, "call_expression", func(node ts.Node) {
		if fn := node.ChildByFieldName("function"); fn != nil {
			found = true
		}
	})
	if !found {
		t.Error("call_expression missing 'function' field")
	}
}

// --- Rust ---

func TestGrammar_Rust_FunctionItem(t *testing.T) {
	root := parseLang(t, ts.Rust(), `fn load_config(path: &str) -> Config {
    Config { name: path.to_string() }
}
`)
	fn := findChildByType(root, "function_item")
	if fn == nil {
		t.Fatal("expected function_item")
	}
	if nameNode := fn.ChildByFieldName("name"); nameNode == nil {
		t.Error("missing 'name' field")
	}
	if params := fn.ChildByFieldName("parameters"); params == nil {
		t.Error("missing 'parameters' field")
	}
	if retType := fn.ChildByFieldName("return_type"); retType == nil {
		t.Error("missing 'return_type' field")
	}
	if body := fn.ChildByFieldName("body"); body == nil {
		t.Error("missing 'body' field")
	}
}

func TestGrammar_Rust_ParamType(t *testing.T) {
	root := parseLang(t, ts.Rust(), `fn foo(x: i32, y: &str) {}
`)
	fn := findChildByType(root, "function_item")
	params := fn.ChildByFieldName("parameters")
	var count int
	for i := 0; i < params.ChildCount(); i++ {
		p := params.Child(i)
		if p.Type() == "parameter" {
			if p.ChildByFieldName("type") != nil {
				count++
			}
		}
	}
	if count != 2 {
		t.Errorf("expected 2 parameters with type field, got %d", count)
	}
}

// --- Java ---

func TestGrammar_Java_MethodDeclaration(t *testing.T) {
	root := parseLang(t, ts.Java(), `public class Main {
    public static String loadConfig(String path) {
        return path;
    }
}
`)
	// class_declaration > class_body > method_declaration
	classDecl := findChildByType(root, "class_declaration")
	if classDecl == nil {
		t.Fatal("expected class_declaration")
	}
	body := findChildByType(classDecl, "class_body")
	if body == nil {
		t.Fatal("expected class_body")
	}
	method := findChildByType(body, "method_declaration")
	if method == nil {
		t.Fatal("expected method_declaration")
	}
	if nameNode := method.ChildByFieldName("name"); nameNode == nil {
		t.Error("missing 'name' field")
	}
	if params := method.ChildByFieldName("parameters"); params == nil {
		t.Error("missing 'parameters' field — check formal_parameters vs parameters")
	}
	if body := method.ChildByFieldName("body"); body == nil {
		t.Error("missing 'body' field")
	}
}

func TestGrammar_Java_MethodInvocation(t *testing.T) {
	root := parseLang(t, ts.Java(), `class X { void f() { foo(); } }
`)
	var found bool
	walkForType(root, "method_invocation", func(node ts.Node) {
		if nameNode := node.ChildByFieldName("name"); nameNode != nil {
			found = true
		}
	})
	if !found {
		t.Error("method_invocation missing 'name' field")
	}
}

// --- C ---

func TestGrammar_C_FunctionDefinition(t *testing.T) {
	root := parseLang(t, ts.C(), `int load_config(const char* path) {
    return 0;
}
`)
	fn := findChildByType(root, "function_definition")
	if fn == nil {
		t.Fatal("expected function_definition")
	}
	if retType := fn.ChildByFieldName("type"); retType == nil {
		t.Error("missing 'type' field (return type)")
	}
	if decl := fn.ChildByFieldName("declarator"); decl == nil {
		t.Error("missing 'declarator' field")
	} else {
		if decl.Type() != "function_declarator" {
			t.Errorf("declarator type = %q, want function_declarator", decl.Type())
		}
	}
	if body := fn.ChildByFieldName("body"); body == nil {
		t.Error("missing 'body' field")
	}
}

// --- C++ ---

func TestGrammar_Cpp_FunctionDefinition(t *testing.T) {
	root := parseLang(t, ts.Cpp(), `std::string loadConfig(const std::string& path) {
    return path;
}
`)
	fn := findChildByType(root, "function_definition")
	if fn == nil {
		t.Fatal("expected function_definition")
	}
	if decl := fn.ChildByFieldName("declarator"); decl == nil {
		t.Error("missing 'declarator' field")
	}
	if body := fn.ChildByFieldName("body"); body == nil {
		t.Error("missing 'body' field")
	}
}

// --- Kotlin ---

func TestGrammar_Kotlin_FunctionDeclaration(t *testing.T) {
	root := parseLang(t, ts.Kotlin(), `fun loadConfig(path: String): Config {
    return Config(name = path)
}
`)
	fn := findChildByType(root, "function_declaration")
	if fn == nil {
		t.Fatal("expected function_declaration")
	}
	// Kotlin uses simple_identifier for name
	nameNode := findChildByType(fn, "simple_identifier")
	if nameNode == nil {
		t.Error("function_declaration missing simple_identifier (name)")
	}
	// Kotlin uses function_value_parameters
	params := findChildByType(fn, "function_value_parameters")
	if params == nil {
		t.Error("missing function_value_parameters")
	}
	// Body
	body := findChildByType(fn, "function_body")
	if body == nil {
		t.Error("missing function_body")
	}
}

func TestGrammar_Kotlin_CallExpression(t *testing.T) {
	root := parseLang(t, ts.Kotlin(), `fun main() { val x = loadConfig("y") }
`)
	var found bool
	walkForType(root, "call_expression", func(node ts.Node) {
		if nameNode := findChildByType(node, "simple_identifier"); nameNode != nil {
			found = true
		}
	})
	if !found {
		t.Error("call_expression missing simple_identifier")
	}
}

// --- Swift ---

func TestGrammar_Swift_FunctionDeclaration(t *testing.T) {
	root := parseLang(t, ts.Swift(), `func loadConfig(path: String) -> Config {
    return Config(name: path)
}
`)
	fn := findChildByType(root, "function_declaration")
	if fn == nil {
		t.Fatal("expected function_declaration")
	}
	nameNode := findChildByType(fn, "simple_identifier")
	if nameNode == nil {
		t.Error("missing simple_identifier (name)")
	}
	body := findChildByType(fn, "function_body")
	if body == nil {
		t.Error("missing function_body")
	}
}

// --- C# ---

func TestGrammar_CSharp_MethodDeclaration(t *testing.T) {
	root := parseLang(t, ts.CSharp(), `class Program {
    static string LoadConfig(string path) {
        return path;
    }
}
`)
	classDecl := findChildByType(root, "class_declaration")
	if classDecl == nil {
		t.Fatal("expected class_declaration")
	}
	body := findChildByType(classDecl, "declaration_list")
	if body == nil {
		t.Fatal("expected declaration_list")
	}
	method := findChildByType(body, "method_declaration")
	if method == nil {
		t.Fatal("expected method_declaration")
	}
	if nameNode := method.ChildByFieldName("name"); nameNode == nil {
		t.Error("missing 'name' field")
	}
	if params := method.ChildByFieldName("parameters"); params == nil {
		t.Error("missing 'parameters' field")
	}
	if methodBody := method.ChildByFieldName("body"); methodBody == nil {
		t.Error("missing 'body' field")
	}
}

func TestGrammar_CSharp_InvocationExpression(t *testing.T) {
	root := parseLang(t, ts.CSharp(), `class X { void F() { Foo(); } }
`)
	var found bool
	walkForType(root, "invocation_expression", func(node ts.Node) {
		found = true
	})
	if !found {
		t.Error("expected invocation_expression node")
	}
}

// --- helper ---

func walkForType(node ts.Node, nodeType string, fn func(ts.Node)) {
	if node.Type() == nodeType {
		fn(node)
	}
	for i := 0; i < node.ChildCount(); i++ {
		walkForType(node.Child(i), nodeType, fn)
	}
}
