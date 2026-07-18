package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3"
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

// Set.add must not unique-name-resolve to a sole exported add() elsewhere
// (alef residual: normalizeSessionTags → engine.add).
func TestTypeScript_MemberCall_NoUniqueNameBind(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"package.json":  `{"name": "member-collision"}`,
		"tsconfig.json": `{}`,
		"session/tags.ts": `
export function normalizeSessionTags(tags: readonly string[]): string[] {
	const seen = new Set<string>();
	const out: string[] = [];
	for (const tag of tags) {
		if (seen.has(tag)) continue;
		seen.add(tag);
		out.push(tag);
	}
	return out;
}
`,
		"engine/sse.ts": `
export class SSEHub {
	add(res: unknown): void {
		void res;
	}
}
`,
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	funcs := ParseTypeScriptFunctions(dir)
	var norm *oculus.Symbol
	for i := range funcs {
		if funcs[i].Name == "normalizeSessionTags" {
			norm = &funcs[i]
			break
		}
	}
	if norm == nil {
		t.Fatal("normalizeSessionTags not parsed")
	}
	foundMemberAdd := false
	for _, m := range norm.MemberCallees {
		if m == "add" {
			foundMemberAdd = true
		}
	}
	if !foundMemberAdd {
		t.Fatalf("expected MemberCallees to include add, got %v (Callees=%v)", norm.MemberCallees, norm.Callees)
	}
	if line := norm.CallLines["add"]; line == 0 {
		t.Fatal("expected CallLines[add] call-site line")
	}

	src := oculus.NewFuncIndexSource(funcs)
	p := &oculus.SymbolPipeline{Source: src, Root: dir}
	cg, err := p.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "normalizeSessionTags", Depth: 3})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	for _, e := range cg.Edges {
		if e.Caller == "normalizeSessionTags" && e.Callee == "add" {
			t.Fatalf("false edge normalizeSessionTags → add (pkg=%s line=%d)", e.CalleePkg, e.Line)
		}
	}
}
