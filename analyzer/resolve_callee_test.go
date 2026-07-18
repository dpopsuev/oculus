package analyzer

import "testing"

func TestResolveCallee_AmbiguousBareNameDropped(t *testing.T) {
	allFuncs := map[string]cgFuncDef{
		"pkg/a.Process": {name: "Process", pkg: "pkg/a"},
		"pkg/b.Process": {name: "Process", pkg: "pkg/b"},
		"pkg/c.Helper":  {name: "Helper", pkg: "pkg/c"},
	}

	key, pkg, ok := resolveCallee("Process", "pkg/d", allFuncs)
	if ok || key != "" || pkg != "" {
		t.Fatalf("ambiguous Process: key=%q pkg=%q ok=%v; want drop", key, pkg, ok)
	}

	key, pkg, ok = resolveCallee("Helper", "pkg/d", allFuncs)
	if !ok || key != "pkg/c.Helper" || pkg != "pkg/c" {
		t.Fatalf("unique Helper: key=%q pkg=%q ok=%v", key, pkg, ok)
	}

	key, pkg, ok = resolveCallee("Process", "pkg/a", allFuncs)
	if !ok || key != "pkg/a.Process" || pkg != "pkg/a" {
		t.Fatalf("same-pkg Process: key=%q pkg=%q ok=%v", key, pkg, ok)
	}
}
