package oculus

import "testing"

func TestCallResolver_SamePkg(t *testing.T) {
	funcs := []Symbol{
		{Name: "Foo", Package: "pkg/a", File: "pkg/a/main.go"},
		{Name: "Bar", Package: "pkg/a", File: "pkg/a/main.go"},
		{Name: "Bar", Package: "pkg/b", File: "pkg/b/util.go"},
	}
	r := NewCallResolver(funcs)

	res := r.Resolve("Bar", "pkg/a", "pkg/a/main.go", nil)
	if res.Symbol == nil {
		t.Fatal("expected resolution")
	}
	if res.Symbol.Package != "pkg/a" {
		t.Errorf("expected pkg/a, got %s", res.Symbol.Package)
	}
	if res.Confidence != ConfSamePkg {
		t.Errorf("expected confidence %v, got %v", ConfSamePkg, res.Confidence)
	}
	if res.Strategy != "same_pkg" {
		t.Errorf("expected strategy same_pkg, got %s", res.Strategy)
	}
}

func TestCallResolver_UniqueName(t *testing.T) {
	funcs := []Symbol{
		{Name: "Foo", Package: "pkg/a", File: "pkg/a/main.go"},
		{Name: "UniqueFunc", Package: "pkg/b", File: "pkg/b/util.go"},
	}
	r := NewCallResolver(funcs)

	res := r.Resolve("UniqueFunc", "pkg/a", "pkg/a/main.go", nil)
	if res.Symbol == nil {
		t.Fatal("expected resolution")
	}
	if res.Symbol.Package != "pkg/b" {
		t.Errorf("expected pkg/b, got %s", res.Symbol.Package)
	}
	if res.Confidence != ConfUniqueName {
		t.Errorf("expected confidence %v, got %v", ConfUniqueName, res.Confidence)
	}
}

func TestCallResolver_ImportMap(t *testing.T) {
	funcs := []Symbol{
		{Name: "Connect", Package: "db", File: "db/conn.go"},
		{Name: "Connect", Package: "net", File: "net/tcp.go"},
	}
	r := NewCallResolver(funcs)

	// With imports pointing to "db" package
	res := r.Resolve("Connect", "app", "app/main.go", []string{"db"})
	if res.Symbol == nil {
		t.Fatal("expected resolution")
	}
	if res.Symbol.Package != "db" {
		t.Errorf("expected db, got %s", res.Symbol.Package)
	}
	if res.Confidence != ConfImportMap {
		t.Errorf("expected confidence %v, got %v", ConfImportMap, res.Confidence)
	}
	if res.Strategy != "import_map" {
		t.Errorf("expected strategy import_map, got %s", res.Strategy)
	}
}

func TestCallResolver_SuffixMatch(t *testing.T) {
	funcs := []Symbol{
		{Name: "Process", Package: "pkg/a", File: "pkg/a/a.go"},
		{Name: "Process", Package: "pkg/b", File: "pkg/b/b.go"},
		{Name: "Process", Package: "pkg/c", File: "pkg/c/c.go"},
	}
	r := NewCallResolver(funcs)

	// No imports, not in same package, not unique — must NOT guess.
	res := r.Resolve("Process", "pkg/d", "pkg/d/d.go", nil)
	if res.Symbol != nil {
		t.Fatalf("ambiguous bare name must be unresolved, got %+v via %s", res.Symbol, res.Strategy)
	}

	// Unique import reachability still resolves.
	res = r.Resolve("Process", "pkg/d", "pkg/d/d.go", []string{"pkg/b"})
	if res.Symbol == nil || res.Symbol.Package != "pkg/b" {
		t.Fatalf("expected pkg/b via import map/suffix, got %+v", res.Symbol)
	}
}

func TestCallResolver_Unresolved(t *testing.T) {
	funcs := []Symbol{
		{Name: "Foo", Package: "pkg/a", File: "pkg/a/main.go"},
	}
	r := NewCallResolver(funcs)

	res := r.Resolve("DoesNotExist", "pkg/a", "pkg/a/main.go", nil)
	if res.Symbol != nil {
		t.Errorf("expected nil symbol, got %+v", res.Symbol)
	}
}

func TestCallResolver_ImportPrioritySamePkg(t *testing.T) {
	funcs := []Symbol{
		{Name: "Get", Package: "cache", File: "cache/cache.go"},
		{Name: "Get", Package: "app", File: "app/handler.go"},
	}
	r := NewCallResolver(funcs)

	// Import map has cache, but caller is in app — import wins (higher confidence)
	res := r.Resolve("Get", "app", "app/handler.go", []string{"cache"})
	if res.Symbol == nil {
		t.Fatal("expected resolution")
	}
	if res.Confidence != ConfImportMap {
		t.Errorf("expected import_map confidence %v, got %v (%s)", ConfImportMap, res.Confidence, res.Strategy)
	}
}
