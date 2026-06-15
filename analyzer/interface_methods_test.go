package analyzer

import (
	"context"
	"testing"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/ts"
)

// TestTreeSitter_ExtractsInterfaceMethods verifies tree-sitter extracts
// method_spec from Go interface declarations.
func TestTreeSitter_ExtractsInterfaceMethods(t *testing.T) {
	root := "../"
	a := &TreeSitterAnalyzer{}
	classes, err := a.Classes(context.Background(), root)
	if err != nil {
		t.Fatalf("TreeSitter Classes: %v", err)
	}

	for _, ci := range classes {
		if ci.Kind == "interface" && ci.Name == "DeepAnalyzer" {
			if len(ci.Methods) == 0 {
				t.Errorf("TreeSitter: DeepAnalyzer has 0 methods — extractGoInterfaceMethods broken")
			} else {
				t.Logf("TreeSitter: DeepAnalyzer has %d methods: %v", len(ci.Methods), methodNames(ci.Methods))
			}
			return
		}
	}
	t.Error("DeepAnalyzer interface not found in TreeSitter Classes output")
}

// TestTreeSitter_InterfaceNodeStructure dumps the tree-sitter node types
// inside an interface_type to find where method_spec lives.
func TestTreeSitter_InterfaceNodeStructure(t *testing.T) {
	root := "../"
	a := &TreeSitterAnalyzer{}
	err := a.walkGoFiles(root, func(tree ts.Tree, src []byte, pkg, file string) {
		if file != "interfaces.go" {
			return
		}
		rootNode := tree.RootNode()
		for i := 0; i < rootNode.ChildCount(); i++ {
			child := rootNode.Child(i)
			if child.Type() != "type_declaration" {
				continue
			}
			for j := 0; j < child.ChildCount(); j++ {
				spec := child.Child(j)
				if spec.Type() != "type_spec" {
					continue
				}
				nameNode := spec.ChildByFieldName("name")
				typeNode := spec.ChildByFieldName("type")
				if nameNode == nil || typeNode == nil || typeNode.Type() != "interface_type" {
					continue
				}
				name := nameNode.Content(src)
				if name != "DeepAnalyzer" {
					continue
				}
				t.Logf("interface %s node structure:", name)
				dumpNode(t, typeNode, src, 0)
				return
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
}

func dumpNode(t *testing.T, node ts.Node, src []byte, depth int) {
	t.Helper()
	if depth > 6 {
		return
	}
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}
	label := node.Type()
	if node.ChildCount() == 0 && node.IsNamed() {
		label += " = " + node.Content(src)
	}
	t.Logf("%s%s", indent, label)
	for i := 0; i < node.ChildCount(); i++ {
		dumpNode(t, node.Child(i), src, depth+1)
	}
}

func methodNames(methods []oculus.MethodInfo) []string {
	names := make([]string, len(methods))
	for i, m := range methods {
		names[i] = m.Name
	}
	return names
}

// TestInterface_MethodsPopulated verifies that interface_metrics populates
// method counts correctly. Uses Oculus itself as fixture — it has
// DeepAnalyzer (3 methods), TypeAnalyzer (6 methods), etc.
func TestInterface_MethodsPopulated(t *testing.T) {
	root := "../"
	fa := NewFallback(root, nil)

	classes, err := fa.Classes(context.Background(), root)
	if err != nil {
		t.Fatalf("Classes: %v", err)
	}
	if len(classes) == 0 {
		t.Fatal("expected classes from Oculus repo")
	}

	knownInterfaces := map[string]int{
		"DeepAnalyzer":  3,
		"ClassAnalyzer": 2,
		"CallAnalyzer":  2,
		"SymbolSource":  3,
	}

	found := 0
	for _, ci := range classes {
		if ci.Kind != "interface" {
			continue
		}
		expected, ok := knownInterfaces[ci.Name]
		if !ok {
			continue
		}
		found++

		if len(ci.Methods) == 0 {
			t.Errorf("interface %s has 0 methods, want %d", ci.Name, expected)
			continue
		}

		if len(ci.Methods) < expected {
			t.Errorf("interface %s has %d methods, want >= %d", ci.Name, len(ci.Methods), expected)
		} else {
			t.Logf("interface %s: %d methods", ci.Name, len(ci.Methods))
		}
	}

	if found == 0 {
		t.Error("none of the known interfaces found in Classes output")
	}
}
