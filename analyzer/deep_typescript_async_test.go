package analyzer

import (
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
)

func TestTypeScript_AsyncEdgeKinds(t *testing.T) {
	dir := "testdata/ts_async"
	funcs := ParseTypeScriptFunctions(dir)
	if len(funcs) == 0 {
		t.Skip("no TypeScript functions parsed")
	}

	asyncKinds := make(map[string][]string) // kind → callee names
	for _, f := range funcs {
		for callee, kind := range f.AsyncCallees {
			asyncKinds[kind] = append(asyncKinds[kind], callee)
		}
	}

	if len(asyncKinds[oculus.CallEdgeAwait]) == 0 {
		t.Errorf("expected at least one %q edge; async callees: %v", oculus.CallEdgeAwait, asyncKinds)
	}
	if len(asyncKinds[oculus.CallEdgePromise]) == 0 {
		t.Errorf("expected at least one %q edge; async callees: %v", oculus.CallEdgePromise, asyncKinds)
	}
	t.Logf("async edges: await=%v promise=%v", asyncKinds[oculus.CallEdgeAwait], asyncKinds[oculus.CallEdgePromise])
}
