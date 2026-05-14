package analyzer

import (
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
)

func TestPython_AsyncEdgeKinds(t *testing.T) {
	dir := "testdata/py_async"
	funcs := ParsePythonFunctions(dir)
	if len(funcs) == 0 {
		t.Skip("no Python functions parsed")
	}

	asyncKinds := make(map[string][]string)
	for _, f := range funcs {
		for callee, kind := range f.AsyncCallees {
			asyncKinds[kind] = append(asyncKinds[kind], callee)
		}
	}

	if len(asyncKinds[oculus.CallEdgeAwait]) == 0 {
		t.Errorf("expected at least one %q edge; got: %v", oculus.CallEdgeAwait, asyncKinds)
	}
	if len(asyncKinds[oculus.CallEdgeTaskSpawn]) == 0 {
		t.Errorf("expected at least one %q edge; got: %v", oculus.CallEdgeTaskSpawn, asyncKinds)
	}
	t.Logf("async edges: await=%v task_spawn=%v", asyncKinds[oculus.CallEdgeAwait], asyncKinds[oculus.CallEdgeTaskSpawn])
}
