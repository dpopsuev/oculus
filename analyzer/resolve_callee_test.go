package analyzer

import "testing"

func TestResolveCallee_AmbiguousBareNameDropped(t *testing.T) {
	allFuncs := map[string]cgFuncDef{
		"pkg/a.Process": {name: "Process", pkg: "pkg/a"},
		"pkg/b.Process": {name: "Process", pkg: "pkg/b"},
		"pkg/c.Helper":  {name: "Helper", pkg: "pkg/c"},
	}

	key, pkg, ok := resolveCallee("Process", "pkg/d", allFuncs, false)
	if ok || key != "" || pkg != "" {
		t.Fatalf("ambiguous Process: key=%q pkg=%q ok=%v; want drop", key, pkg, ok)
	}

	key, pkg, ok = resolveCallee("Helper", "pkg/d", allFuncs, false)
	if !ok || key != "pkg/c.Helper" || pkg != "pkg/c" {
		t.Fatalf("unique Helper: key=%q pkg=%q ok=%v", key, pkg, ok)
	}

	key, pkg, ok = resolveCallee("Process", "pkg/a", allFuncs, false)
	if !ok || key != "pkg/a.Process" || pkg != "pkg/a" {
		t.Fatalf("same-pkg Process: key=%q pkg=%q ok=%v", key, pkg, ok)
	}
}

func TestResolveCallee_MemberSkipsUniqueName(t *testing.T) {
	allFuncs := map[string]cgFuncDef{
		"engine.add": {name: "add", pkg: "engine"},
		"session.normalize": {name: "normalize", pkg: "session"},
		"session.helper":    {name: "helper", pkg: "session"},
	}

	// seen.add → unique engine.add must NOT bind
	key, pkg, ok := resolveCallee("add", "session", allFuncs, true)
	if ok || key != "" || pkg != "" {
		t.Fatalf("member add: key=%q pkg=%q ok=%v; want drop", key, pkg, ok)
	}

	// same-pkg member still resolves
	key, pkg, ok = resolveCallee("helper", "session", allFuncs, true)
	if !ok || key != "session.helper" || pkg != "session" {
		t.Fatalf("same-pkg member helper: key=%q pkg=%q ok=%v", key, pkg, ok)
	}

	// bare unique still works
	key, pkg, ok = resolveCallee("add", "session", allFuncs, false)
	if !ok || key != "engine.add" {
		t.Fatalf("bare unique add: key=%q pkg=%q ok=%v", key, pkg, ok)
	}
}
