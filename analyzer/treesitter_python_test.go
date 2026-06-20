package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func setupPythonTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = \"test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("testdata/python_types/models.py")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models.py"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestPythonClasses(t *testing.T) {
	dir := setupPythonTestRepo(t)
	a := &TreeSitterAnalyzer{}
	classes, err := a.Classes(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	found := map[string]bool{}
	for _, c := range classes {
		found[c.Name] = true
		switch c.Name {
		case "Animal":
			if c.Kind != "class" {
				t.Errorf("Animal: expected class, got %s", c.Kind)
			}
			if !c.Exported {
				t.Error("Animal: expected exported")
			}
			methodNames := map[string]bool{}
			for _, m := range c.Methods {
				methodNames[m.Name] = true
			}
			if !methodNames["__init__"] {
				t.Error("Animal: missing __init__ method")
			}
			if !methodNames["speak"] {
				t.Error("Animal: missing speak method")
			}
		case "Dog":
			if c.Kind != "class" {
				t.Errorf("Dog: expected class, got %s", c.Kind)
			}
			if len(c.Methods) < 2 {
				t.Errorf("Dog: expected 2+ methods, got %d", len(c.Methods))
			}
		case "_Internal":
			if c.Exported {
				t.Error("_Internal: expected private")
			}
		}
	}

	for _, name := range []string{"Animal", "Dog", "GuideDog", "Cat", "_Internal", "MultiParent"} {
		if !found[name] {
			t.Errorf("missing class %s", name)
		}
	}
}

func TestPythonImplements(t *testing.T) {
	dir := setupPythonTestRepo(t)
	a := &TreeSitterAnalyzer{}
	edges, err := a.Implements(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	type edgeKey struct{ from, to, kind string }
	edgeSet := make(map[edgeKey]bool)
	for _, e := range edges {
		edgeSet[edgeKey{e.From, e.To, e.Kind}] = true
	}

	expected := []edgeKey{
		{"Dog", "Animal", "extends"},
		{"GuideDog", "Dog", "extends"},
		{"Cat", "Animal", "extends"},
		{"MultiParent", "Cat", "extends"},
		{"MultiParent", "Dog", "extends"},
	}
	for _, e := range expected {
		if !edgeSet[e] {
			t.Errorf("missing edge: %s --%s--> %s", e.from, e.kind, e.to)
		}
	}

	// Should NOT include "object" as parent
	for _, e := range edges {
		if e.To == "object" {
			t.Errorf("unexpected edge to 'object': %s --%s--> object", e.From, e.Kind)
		}
	}
}
