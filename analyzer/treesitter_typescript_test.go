package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func setupTSTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tsconfig.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("testdata/ts_types/models.ts")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "models.ts"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestTSClasses(t *testing.T) {
	dir := setupTSTestRepo(t)
	a := &TreeSitterAnalyzer{}
	classes, err := a.Classes(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	found := map[string]bool{}
	for _, c := range classes {
		found[c.Name] = true
		switch c.Name {
		case "Serializable":
			if c.Kind != "interface" {
				t.Errorf("Serializable: expected interface, got %s", c.Kind)
			}
			if len(c.Methods) < 2 {
				t.Errorf("Serializable: expected 2+ methods, got %d", len(c.Methods))
			}
		case "Loggable":
			if c.Kind != "interface" {
				t.Errorf("Loggable: expected interface, got %s", c.Kind)
			}
		case "Animal":
			if c.Kind != "class" {
				t.Errorf("Animal: expected class, got %s", c.Kind)
			}
			if len(c.Methods) < 2 {
				t.Errorf("Animal: expected 2+ methods (constructor + speak), got %d", len(c.Methods))
			}
			if len(c.Fields) < 2 {
				t.Errorf("Animal: expected 2+ fields (name, age), got %d", len(c.Fields))
			}
		case "Dog":
			if c.Kind != "class" {
				t.Errorf("Dog: expected class, got %s", c.Kind)
			}
		case "Shape":
			if c.Kind != "class" {
				t.Errorf("Shape (abstract): expected class, got %s", c.Kind)
			}
		}
	}

	for _, name := range []string{"Serializable", "Printable", "Loggable", "Animal", "Dog", "GuideDog", "Shape", "Circle", "_InternalHelper"} {
		if !found[name] {
			t.Errorf("missing type %s", name)
		}
	}
}

func TestTSImplements(t *testing.T) {
	dir := setupTSTestRepo(t)
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
		{"Dog", "Serializable", "implements"},
		{"GuideDog", "Dog", "extends"},
		{"GuideDog", "Loggable", "implements"},
		{"Loggable", "Serializable", "extends"},
		{"Loggable", "Printable", "extends"},
		{"Circle", "Shape", "extends"},
	}
	for _, e := range expected {
		if !edgeSet[e] {
			t.Errorf("missing edge: %s --%s--> %s", e.from, e.kind, e.to)
		}
	}
}
