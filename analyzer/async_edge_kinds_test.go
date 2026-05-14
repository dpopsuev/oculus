package analyzer

import (
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
)

// helpers ----------------------------------------------------------------

func collectAsyncKinds(funcs []oculus.Symbol) map[string][]string {
	out := make(map[string][]string)
	for _, f := range funcs {
		for callee, kind := range f.AsyncCallees {
			out[kind] = append(out[kind], callee)
		}
	}
	return out
}

func requireKind(t *testing.T, kinds map[string][]string, kind string) {
	t.Helper()
	if len(kinds[kind]) == 0 {
		t.Errorf("expected at least one %q edge; got: %v", kind, kinds)
	}
}

// Rust -------------------------------------------------------------------

func TestRust_AsyncEdgeKinds(t *testing.T) {
	funcs := ParseRustFunctions("testdata/rust_async/src")
	if len(funcs) == 0 {
		t.Skip("no Rust functions parsed")
	}
	kinds := collectAsyncKinds(funcs)
	requireKind(t, kinds, oculus.CallEdgeAwait)
	requireKind(t, kinds, oculus.CallEdgeTaskSpawn)
	t.Logf("async edges: %v", kinds)
}

// Java -------------------------------------------------------------------

func TestJava_AsyncEdgeKinds(t *testing.T) {
	funcs := ParseJavaFunctions("testdata/java_async")
	if len(funcs) == 0 {
		t.Skip("no Java functions parsed")
	}
	kinds := collectAsyncKinds(funcs)
	requireKind(t, kinds, oculus.CallEdgeTaskSpawn)
	requireKind(t, kinds, oculus.CallEdgePromise)
	t.Logf("async edges: %v", kinds)
}

// Kotlin -----------------------------------------------------------------

func TestKotlin_AsyncEdgeKinds(t *testing.T) {
	funcs := ParseKotlinFunctions("testdata/kotlin_async")
	if len(funcs) == 0 {
		t.Skip("no Kotlin functions parsed")
	}
	kinds := collectAsyncKinds(funcs)
	requireKind(t, kinds, oculus.CallEdgeTaskSpawn)
	t.Logf("async edges: %v", kinds)
}

// C# ---------------------------------------------------------------------

func TestCSharp_AsyncEdgeKinds(t *testing.T) {
	funcs := ParseCSharpFunctions("testdata/csharp_async")
	if len(funcs) == 0 {
		t.Skip("no C# functions parsed")
	}
	kinds := collectAsyncKinds(funcs)
	requireKind(t, kinds, oculus.CallEdgeAwait)
	t.Logf("async edges: %v", kinds)
}

// Swift ------------------------------------------------------------------

func TestSwift_AsyncEdgeKinds(t *testing.T) {
	funcs := ParseSwiftFunctions("testdata/swift_async")
	if len(funcs) == 0 {
		t.Skip("no Swift functions parsed")
	}
	kinds := collectAsyncKinds(funcs)
	requireKind(t, kinds, oculus.CallEdgeAwait)
	t.Logf("async edges: %v", kinds)
}

// C ----------------------------------------------------------------------

func TestC_ThreadEdgeKinds(t *testing.T) {
	funcs := ParseCFunctions("testdata/c_threads")
	if len(funcs) == 0 {
		t.Skip("no C functions parsed")
	}
	kinds := collectAsyncKinds(funcs)
	requireKind(t, kinds, oculus.CallEdgeGoroutine)
	t.Logf("async edges: %v", kinds)
}

// C++ --------------------------------------------------------------------

func TestCpp_AsyncEdgeKinds(t *testing.T) {
	funcs := ParseCppFunctions("testdata/cpp_async")
	if len(funcs) == 0 {
		t.Skip("no C++ functions parsed")
	}
	kinds := collectAsyncKinds(funcs)
	requireKind(t, kinds, oculus.CallEdgeTaskSpawn)
	requireKind(t, kinds, oculus.CallEdgeGoroutine)
	t.Logf("async edges: %v", kinds)
}
