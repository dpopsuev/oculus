package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3"
)

func TestPython_CallGraph_ViaFuncIndex(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"pyproject.toml":  "[project]\nname = \"testapp\"\n",
		"app/__init__.py": "",
		"app/main.py": `
def main():
    result = process_data()
    send_result(result)

def process_data():
    data = fetch_data()
    return transform(data)

def fetch_data():
    return [1, 2, 3]

def transform(data):
    return [x * 2 for x in data]

def send_result(result):
    print(result)
`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}

	funcs := ParsePythonFunctions(dir)
	if len(funcs) == 0 {
		t.Fatal("expected parsed functions")
	}

	src := oculus.NewFuncIndexSource(funcs)
	p := &oculus.SymbolPipeline{Source: src, Root: dir}

	cg, err := p.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if len(cg.Nodes) == 0 {
		t.Error("expected nodes")
	}
	if len(cg.Edges) == 0 {
		t.Error("expected edges")
	}

	// Verify the call chain: main -> process_data, send_result
	callees := make(map[string][]string)
	for _, e := range cg.Edges {
		callees[e.Caller] = append(callees[e.Caller], e.Callee)
	}
	if _, ok := callees["main"]; !ok {
		t.Error("expected main in call graph")
	}

	t.Logf("Python CallGraph: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func TestPython_TypedEdges(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"pyproject.toml": "[project]\nname = \"test\"\n",
		"main.py": `def load_config(path: str) -> dict:
    return {"name": path}

def transform(cfg: dict) -> list:
    return list(cfg.values())

def main():
    cfg = load_config("app.yaml")
    result = transform(cfg)
`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}

	funcs := ParsePythonFunctions(dir)
	src := oculus.NewFuncIndexSource(funcs)
	p := &oculus.SymbolPipeline{Source: src, Root: dir}

	cg, err := p.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	typed := 0
	for _, e := range cg.Edges {
		if len(e.ParamTypes) > 0 || len(e.ReturnTypes) > 0 {
			typed++
			t.Logf("  %s → %s (params=%v returns=%v)", e.Caller, e.Callee, e.ParamTypes, e.ReturnTypes)
		}
	}
	if typed == 0 {
		t.Error("expected typed edges from Python type annotations")
	}
	t.Logf("Python typed edges: %d/%d", typed, len(cg.Edges))
}

func TestPython_NonPythonRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	funcs := ParsePythonFunctions(dir)
	if len(funcs) != 0 {
		t.Error("expected 0 functions for non-Python repo")
	}
}
