package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus"
)

func TestTypeScript_CallGraph_ViaFuncIndex(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"package.json": `{"name": "testapp"}`,
		"tsconfig.json": `{}`,
		"src/main.ts": `
export function main() {
    const data = fetchData()
    const result = processData(data)
    sendResult(result)
}

function fetchData(): number[] {
    return [1, 2, 3]
}

function processData(data: number[]): number[] {
    return transform(data)
}

function transform(data: number[]): number[] {
    return data.map(x => x * 2)
}

function sendResult(result: number[]) {
    console.log(result)
}
`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}

	funcs := ParseTypeScriptFunctions(dir)
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

	callees := make(map[string][]string)
	for _, e := range cg.Edges {
		callees[e.Caller] = append(callees[e.Caller], e.Callee)
	}
	if _, ok := callees["main"]; !ok {
		t.Error("expected main in call graph")
	}

	t.Logf("TypeScript CallGraph: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func TestTypeScript_TypedEdges(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"tsconfig.json": `{"compilerOptions": {"target": "es2020"}}`,
		"package.json":  `{"name": "test"}`,
		"main.ts": `function loadConfig(path: string): Config {
  return { name: path };
}

function transform(cfg: Config): string {
  return cfg.name;
}

function main() {
  const cfg = loadConfig("app.yaml");
  const result = transform(cfg);
}
`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}

	funcs := ParseTypeScriptFunctions(dir)
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
		t.Error("expected typed edges from TypeScript type annotations")
	}
	t.Logf("TypeScript typed edges: %d/%d", typed, len(cg.Edges))
}

func TestTypeScript_ArrowFunctions(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"tsconfig.json": `{}`,
		"index.ts": `
const greet = (name: string) => {
    return formatName(name)
}

const formatName = (name: string) => {
    return name.toUpperCase()
}
`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o644)
	}

	funcs := ParseTypeScriptFunctions(dir)
	src := oculus.NewFuncIndexSource(funcs)
	p := &oculus.SymbolPipeline{Source: src, Root: dir}

	cg, err := p.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "greet", Depth: 3})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}

	found := false
	for _, e := range cg.Edges {
		if e.Caller == "greet" && e.Callee == "formatName" {
			found = true
		}
	}
	if !found {
		t.Error("expected edge greet -> formatName")
	}
	t.Logf("Arrow function CallGraph: %d nodes, %d edges", len(cg.Nodes), len(cg.Edges))
}

func TestTypeScript_NonTSRepo(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o644)

	funcs := ParseTypeScriptFunctions(dir)
	if len(funcs) != 0 {
		t.Error("expected 0 functions for non-TS repo")
	}
}
