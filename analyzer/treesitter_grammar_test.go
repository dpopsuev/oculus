package analyzer

import (
	"testing"

	"github.com/dpopsuev/oculus/ts"
)

// These tests verify tree-sitter Go grammar node types and field names.
// If the grammar updates and renames nodes, these tests catch it immediately
// instead of silently returning empty results (like BUG-45).

func parseGo(t *testing.T, src string) ts.Node {
	t.Helper()
	parser := ts.NewParser()
	parser.SetLanguage(ts.Go())
	tree, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return tree.RootNode()
}

func TestGrammar_TypeDeclaration_StructFields(t *testing.T) {
	root := parseGo(t, `package p
type Foo struct {
	Name string
	Age  int
}`)
	// type_declaration > type_spec > name (identifier) + type (struct_type)
	typeDecl := findChildByType(root, "type_declaration")
	if typeDecl == nil {
		t.Fatal("expected type_declaration node")
	}
	spec := findChildByType(typeDecl, "type_spec")
	if spec == nil {
		t.Fatal("expected type_spec node")
	}
	nameNode := spec.ChildByFieldName("name")
	if nameNode == nil {
		t.Fatal("type_spec missing 'name' field — grammar changed?")
	}
	if nameNode.Content([]byte(`package p
type Foo struct {
	Name string
	Age  int
}`)) == "" {
		t.Error("name node has empty content")
	}
	typeNode := spec.ChildByFieldName("type")
	if typeNode == nil {
		t.Fatal("type_spec missing 'type' field — grammar changed?")
	}
	if typeNode.Type() != "struct_type" {
		t.Errorf("type node = %q, want struct_type", typeNode.Type())
	}
}

func TestGrammar_InterfaceMethods(t *testing.T) {
	src := `package p
type Reader interface {
	Read(p []byte) (int, error)
	Close() error
}`
	root := parseGo(t, src)
	typeDecl := findChildByType(root, "type_declaration")
	spec := findChildByType(typeDecl, "type_spec")
	typeNode := spec.ChildByFieldName("type")

	if typeNode == nil || typeNode.Type() != "interface_type" {
		t.Fatalf("expected interface_type, got %v", typeNode)
	}

	// Verify method nodes exist — could be "method_spec" or "method_elem"
	var methodNodes []ts.Node
	for i := 0; i < typeNode.ChildCount(); i++ {
		child := typeNode.Child(i)
		if child.Type() == "method_spec" || child.Type() == "method_elem" {
			methodNodes = append(methodNodes, child)
		}
	}
	if len(methodNodes) != 2 {
		t.Fatalf("expected 2 method nodes, got %d (grammar changed method node type?)", len(methodNodes))
	}

	// Verify method name extraction works
	for _, m := range methodNodes {
		nameNode := m.ChildByFieldName("name")
		if nameNode == nil {
			nameNode = findChildByType(m, "field_identifier")
		}
		if nameNode == nil {
			t.Errorf("method node %q has no name — grammar changed field name?", m.Type())
		}
	}
	t.Logf("method node type: %q (grammar version verified)", methodNodes[0].Type())
}

func TestGrammar_FunctionDeclaration(t *testing.T) {
	src := `package p
func Foo(x int, y string) (bool, error) {
	return true, nil
}`
	root := parseGo(t, src)

	fn := findChildByType(root, "function_declaration")
	if fn == nil {
		t.Fatal("expected function_declaration node")
	}

	nameNode := fn.ChildByFieldName("name")
	if nameNode == nil {
		t.Fatal("function_declaration missing 'name' field")
	}

	params := fn.ChildByFieldName("parameters")
	if params == nil {
		t.Fatal("function_declaration missing 'parameters' field")
	}

	body := fn.ChildByFieldName("body")
	if body == nil {
		t.Fatal("function_declaration missing 'body' field")
	}

	t.Logf("function fields: name=%q params=%d body=%q", nameNode.Content([]byte(src)), params.ChildCount(), body.Type())
}

func TestGrammar_MethodDeclaration(t *testing.T) {
	src := `package p
type S struct{}
func (s *S) Bar(x int) string {
	return ""
}`
	root := parseGo(t, src)

	// Find method_declaration
	var method ts.Node
	for i := 0; i < root.ChildCount(); i++ {
		child := root.Child(i)
		if child.Type() == "method_declaration" {
			method = child
			break
		}
	}
	if method == nil {
		t.Fatal("expected method_declaration node")
	}

	nameNode := method.ChildByFieldName("name")
	if nameNode == nil {
		t.Fatal("method_declaration missing 'name' field")
	}

	receiver := method.ChildByFieldName("receiver")
	if receiver == nil {
		t.Fatal("method_declaration missing 'receiver' field")
	}

	t.Logf("method fields: name=%q receiver=%q", nameNode.Content([]byte(src)), receiver.Content([]byte(src)))
}

func TestGrammar_CallExpression(t *testing.T) {
	src := `package p
func main() {
	foo(1, 2)
	x.bar()
}`
	root := parseGo(t, src)

	fn := findChildByType(root, "function_declaration")
	body := fn.ChildByFieldName("body")

	var calls []ts.Node
	collectCalls(body, &calls)
	if len(calls) != 2 {
		t.Fatalf("expected 2 call_expression nodes, got %d", len(calls))
	}

	// Verify "function" field exists on call_expression
	for _, call := range calls {
		fnNode := call.ChildByFieldName("function")
		if fnNode == nil {
			t.Errorf("call_expression missing 'function' field — grammar changed?")
		}
	}
}

func collectCalls(node ts.Node, calls *[]ts.Node) {
	if node.Type() == "call_expression" {
		*calls = append(*calls, node)
	}
	for i := 0; i < node.ChildCount(); i++ {
		collectCalls(node.Child(i), calls)
	}
}

func TestGrammar_StructFieldDeclaration(t *testing.T) {
	src := `package p
type Config struct {
	Name string
	Port int
}`
	root := parseGo(t, src)
	typeDecl := findChildByType(root, "type_declaration")
	spec := findChildByType(typeDecl, "type_spec")
	typeNode := spec.ChildByFieldName("type")

	// struct_type should contain field_declaration_list
	fieldList := findChildByType(typeNode, "field_declaration_list")
	if fieldList == nil {
		t.Fatal("struct_type missing field_declaration_list — grammar changed?")
	}

	fieldCount := 0
	for i := 0; i < fieldList.ChildCount(); i++ {
		if fieldList.Child(i).Type() == "field_declaration" {
			fieldCount++
		}
	}
	if fieldCount != 2 {
		t.Errorf("expected 2 field_declarations, got %d", fieldCount)
	}
}
