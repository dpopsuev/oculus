package analyzer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lsp"
	"github.com/dpopsuev/oculus/v3/lsp/mockserver"
)

func TestLSPDeep_TypeScriptWorkspaceSymbolEmpty_FallsBackToDocumentSymbol(t *testing.T) {
	dir := setupTypeScriptCallFixture(t)
	fileURI := pathToURI(filepath.Join(dir, "src", "main.ts"))

	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "Run", Kind: 12, URI: fileURI, Line: 0},
			{Name: "Decorate", Kind: 12, URI: fileURI, Line: 4},
		},
		Edges: []mockserver.CallEdge{
			{FromName: "Run", ToName: "Decorate", ToURI: fileURI, ToLine: 4},
		},
		// Keep workspace/symbol empty to force documentSymbol fallback path.
		IndexingDelay: 24 * time.Hour,
	}

	pool := lsp.NewMockPool(cfg)
	defer pool.Shutdown(context.Background())

	da := NewLSPDeepWithPool(dir, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cg, err := da.CallGraph(ctx, dir, oculus.CallGraphOpts{Depth: 3})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if len(cg.Nodes) == 0 {
		t.Fatal("expected non-empty nodes via documentSymbol fallback")
	}
	if len(cg.Edges) == 0 {
		t.Fatal("expected non-empty edges via documentSymbol fallback")
	}
}

func TestDeepFallback_TypeScript_AllowsSourceFallbackWhenLSPEmpty(t *testing.T) {
	dir := setupTypeScriptCallFixture(t)

	// LSP returns no symbols/edges. Source fallback should still win for TS.
	pool := lsp.NewMockPool(mockserver.Config{})
	defer pool.Shutdown(context.Background())

	da := NewDeepFallback(dir, pool)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cg, err := da.CallGraph(ctx, dir, oculus.CallGraphOpts{Entry: "Run", Depth: 3})
	if err != nil {
		t.Fatalf("CallGraph: %v", err)
	}
	if len(cg.Edges) == 0 {
		t.Fatal("expected source fallback edges when LSP is empty")
	}
}

func setupTypeScriptCallFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	files := map[string]string{
		"package.json": `{"name":"ts-fallback-test","private":true}`,
		"tsconfig.json": `{
  "compilerOptions": { "target": "ES2020", "module": "commonjs" },
  "include": ["src/**/*"]
}`,
		"src/main.ts": `export function Run(input: string): string {
  return Decorate(input);
}

function Decorate(input: string): string {
  return "[" + input + "]";
}
`,
	}
	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", abs, err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", abs, err)
		}
	}
	return dir
}
