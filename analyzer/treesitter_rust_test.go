package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func setupRustTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\nname = \"test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("testdata/rust_types/lib.rs")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "lib.rs"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRustClasses(t *testing.T) {
	dir := setupRustTestRepo(t)
	a := &TreeSitterAnalyzer{}
	classes, err := a.Classes(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	found := map[string]bool{}
	for _, c := range classes {
		found[c.Name] = true
		switch c.Name {
		case "Config":
			if c.Kind != "struct" {
				t.Errorf("Config: expected struct, got %s", c.Kind)
			}
			if !c.Exported {
				t.Error("Config: expected exported")
			}
			if len(c.Fields) < 3 {
				t.Errorf("Config: expected 3+ fields, got %d", len(c.Fields))
			}
			pubFields := 0
			for _, f := range c.Fields {
				if f.Exported {
					pubFields++
				}
			}
			if pubFields != 2 {
				t.Errorf("Config: expected 2 pub fields, got %d", pubFields)
			}
		case "InternalState":
			if c.Kind != "struct" {
				t.Errorf("InternalState: expected struct, got %s", c.Kind)
			}
			if c.Exported {
				t.Error("InternalState: expected private")
			}
		case "Color":
			if c.Kind != "struct" {
				t.Errorf("Color (enum): expected struct, got %s", c.Kind)
			}
		case "Drawable":
			if c.Kind != "trait" {
				t.Errorf("Drawable: expected trait, got %s", c.Kind)
			}
			if len(c.Methods) < 2 {
				t.Errorf("Drawable: expected 2+ methods, got %d", len(c.Methods))
			}
		case "Button":
			if c.Kind != "struct" {
				t.Errorf("Button: expected struct, got %s", c.Kind)
			}
			hasNew := false
			for _, m := range c.Methods {
				if m.Name == "new" {
					hasNew = true
				}
			}
			if !hasNew {
				t.Error("Button: missing 'new' method from inherent impl")
			}
		}
	}

	for _, name := range []string{"Config", "InternalState", "Color", "Drawable", "Clickable", "Button", "Serializable"} {
		if !found[name] {
			t.Errorf("missing type %s", name)
		}
	}
}

func TestRustImplements(t *testing.T) {
	dir := setupRustTestRepo(t)
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
		{"Button", "Drawable", "implements"},
		{"Button", "Clickable", "implements"},
		{"Config", "Serializable", "implements"},
		{"Clickable", "Drawable", "inherits"},
	}
	for _, e := range expected {
		if !edgeSet[e] {
			t.Errorf("missing edge: %s --%s--> %s", e.from, e.kind, e.to)
		}
	}
}
