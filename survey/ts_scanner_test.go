package survey_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3/model"
	"github.com/dpopsuev/oculus/v3/survey"
)

func setupTSProject(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestTSScanExtractsNamespacesAndSymbols(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"package.json": `{"name": "my-app", "dependencies": {"three": "^0.172.0"}}`,
		"src/main.ts": `import { Scene } from 'three'
import { helper } from './utils'

export function init() {}
export class App {}
`,
		"src/utils.ts": `export const VERSION = "1.0"
export interface Config {
  name: string
}

export type ID = string
`,
	})

	sc := &survey.TypeScriptScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.Path != "my-app" {
		t.Errorf("path = %q, want my-app", proj.Path)
	}
	if proj.Language != model.LangTypeScript {
		t.Errorf("language = %v, want TypeScript", proj.Language)
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.Name] = ns
	}

	src, ok := nsMap["src"]
	if !ok {
		t.Fatal("missing namespace src")
	}

	symMap := make(map[string]*model.Symbol)
	for _, s := range src.Symbols {
		symMap[s.Name] = s
	}

	if s, ok := symMap["init"]; !ok {
		t.Error("missing export function init")
	} else if s.Kind != model.SymbolFunction {
		t.Errorf("init.kind = %v, want function", s.Kind)
	}

	if s, ok := symMap["App"]; !ok {
		t.Error("missing export class App")
	} else if s.Kind != model.SymbolClass {
		t.Errorf("App.kind = %v, want class", s.Kind)
	}

	if s, ok := symMap["VERSION"]; !ok {
		t.Error("missing export const VERSION")
	} else if s.Kind != model.SymbolVariable {
		t.Errorf("VERSION.kind = %v, want variable", s.Kind)
	}

	if s, ok := symMap["Config"]; !ok {
		t.Error("missing export interface Config")
	} else if s.Kind != model.SymbolInterface {
		t.Errorf("Config.kind = %v, want interface", s.Kind)
	}

	if s, ok := symMap["ID"]; !ok {
		t.Error("missing export type ID")
	} else if s.Kind != model.SymbolTypeParameter {
		t.Errorf("ID.kind = %v, want type-parameter", s.Kind)
	}
}

func TestTSScanBuildsImportGraph(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"package.json": `{"name": "graph-test", "dependencies": {"three": "1.0", "@msgpack/msgpack": "3.0"}}`,
		"src/main.ts": `import { Scene } from 'three'
import { helper } from './utils'
import { pack } from '@msgpack/msgpack'

export function run() {}
`,
		"src/utils.ts": `export function helper() { return 42 }
`,
		"src/modes/gameplay.ts": `import { run } from '../main'

export function startGame() {}
`,
	})

	sc := &survey.TypeScriptScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.DependencyGraph == nil {
		t.Fatal("dependency graph is nil")
	}

	srcEdges := proj.DependencyGraph.EdgesFrom("src")
	foundThree := false
	foundMsgpack := false
	for _, e := range srcEdges {
		if e.To == "three" && e.External {
			foundThree = true
		}
		if e.To == "@msgpack/msgpack" && e.External {
			foundMsgpack = true
		}
	}
	if !foundThree {
		t.Error("missing external edge src -> three")
	}
	if !foundMsgpack {
		t.Error("missing external edge src -> @msgpack/msgpack")
	}

	modesEdges := proj.DependencyGraph.EdgesFrom("src/modes")
	foundSrc := false
	for _, e := range modesEdges {
		if e.To == "src" && !e.External {
			foundSrc = true
		}
	}
	if !foundSrc {
		t.Error("missing internal edge src/modes -> src")
	}
}

func TestTSScanSkipsImportType(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"package.json": `{"name": "type-import-test"}`,
		"src/core/types.ts": `import type { GlobeRenderer } from '../globe'

export interface ModeContext {
  globe: GlobeRenderer
}
`,
		"src/globe/index.ts": `import type { Vec3 } from '../types'

export class GlobeRenderer {}
`,
		"src/types.ts": `export type Vec3 = [number, number, number]
`,
	})

	sc := &survey.TypeScriptScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	// core/types.ts uses `import type` from globe — this should NOT create
	// a dependency edge because type-only imports are erased at compile time.
	coreEdges := proj.DependencyGraph.EdgesFrom("src/core")
	for _, e := range coreEdges {
		if e.To == "src/globe" {
			t.Errorf("import type should not create dependency edge: src/core -> src/globe")
		}
	}

	// globe/index.ts uses `import type` from types — also should not create edge.
	globeEdges := proj.DependencyGraph.EdgesFrom("src/globe")
	for _, e := range globeEdges {
		if e.To == "src" || e.To == "(root)" {
			t.Errorf("import type should not create dependency edge: src/globe -> %s", e.To)
		}
	}
}

func TestTSScanSkipsNodeModules(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"package.json":              `{"name": "skip-test"}`,
		"src/main.ts":               `export function main() {}`,
		"node_modules/foo/index.js": `export function foo() {}`,
		"dist/bundle.js":            `export function bundled() {}`,
	})

	sc := &survey.TypeScriptScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(proj.Namespaces) != 1 {
		t.Errorf("namespaces = %d, want 1 (only src)", len(proj.Namespaces))
	}
}

// --- File-level granularity mode ---

// TestTSScan_FileLevel_EachFileIsNamespace verifies that when TypeScriptScanner
// is configured with FileLevel granularity, each .ts file becomes its own
// namespace instead of being grouped by directory.
//
// Given a directory with 3 .ts files
// When TypeScriptScanner{Granularity: FileLevel}.Scan(root) is called
// Then there are 3 namespaces, one per file, with the file path as ImportPath
func TestTSScan_FileLevel_EachFileIsNamespace(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"package.json": `{"name":"test-pkg"}`,
		"src/foo.ts":   "export function alpha() {}\n",
		"src/bar.ts":   "export function beta() {}\n",
		"src/baz.ts":   "export function gamma() {}\n",
	})

	sc := &survey.TypeScriptScanner{Granularity: survey.FileLevel}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	nsMap := make(map[string]bool)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = true
	}

	for _, want := range []string{"src/foo.ts", "src/bar.ts", "src/baz.ts"} {
		if !nsMap[want] {
			t.Errorf("expected namespace %q; have: %v", want, func() []string {
				keys := make([]string, 0, len(nsMap))
				for k := range nsMap {
					keys = append(keys, k)
				}
				return keys
			}())
		}
	}

	if len(proj.Namespaces) != 3 {
		t.Errorf("expected 3 namespaces (one per file), got %d", len(proj.Namespaces))
	}
}

// TestFileLevel_RelativeImportsProduceEdges verifies FileLevel import edges
// resolve to concrete file keys (src/a.ts → src/b.ts), not self-collapsed dirs.
func TestFileLevel_RelativeImportsProduceEdges(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"package.json": `{"name":"test-pkg"}`,
		"src/a.ts":     "import { b } from './b';\nexport function a() {}\n",
		"src/b.ts":     "export function b() {}\n",
	})

	sc := &survey.TypeScriptScanner{Granularity: survey.FileLevel}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if proj.DependencyGraph == nil || len(proj.DependencyGraph.Edges) == 0 {
		t.Fatalf("expected FileLevel relative-import edges, got 0 (namespaces=%d)", len(proj.Namespaces))
	}

	found := false
	for _, e := range proj.DependencyGraph.Edges {
		if !e.External && e.From == "src/a.ts" && e.To == "src/b.ts" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected edge src/a.ts → src/b.ts; edges=%+v", proj.DependencyGraph.Edges)
	}
}

// TestTSScan_DirLevel_IsDefault verifies that TypeScriptScanner with no
// Granularity set behaves identically to the existing directory-level scan.
func TestTSScan_DirLevel_IsDefault(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"package.json": `{"name":"test-pkg"}`,
		"src/foo.ts":   "export function alpha() {}\n",
		"src/bar.ts":   "export function beta() {}\n",
	})

	sc := &survey.TypeScriptScanner{} // default = DirLevel
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Both files should be grouped under "src".
	nsMap := make(map[string]bool)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = true
	}
	if !nsMap["src"] {
		t.Errorf("expected namespace 'src' (dir-level), have: %v", nsMap)
	}
	if nsMap["src/foo.ts"] || nsMap["src/bar.ts"] {
		t.Error("file-level namespace paths should not appear in default (dir-level) mode")
	}
}
