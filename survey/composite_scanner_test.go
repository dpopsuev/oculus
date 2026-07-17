package survey_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dpopsuev/oculus/v3/model"
	"github.com/dpopsuev/oculus/v3/survey"
	"github.com/dpopsuev/oculus/v3/testkit"
)

func TestCompositeScanMergesRustAndTS(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"Cargo.toml": `[workspace]
members = ["crates/core"]
`,
		"crates/core/Cargo.toml": `[package]
name = "core"
version = "0.1.0"

[dependencies]
serde = "1"
`,
		"crates/core/src/lib.rs": `pub fn process() {}
pub struct Engine {}
`,
		"client/package.json": `{"name": "client-app", "dependencies": {"three": "1.0"}}`,
		"client/src/main.ts": `import { Scene } from 'three'
export function init() {}
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

	sc := &survey.CompositeScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = ns
	}

	if _, ok := nsMap["core"]; !ok {
		t.Error("missing Rust crate namespace 'core'")
	}

	if _, ok := nsMap["client/src"]; !ok {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("missing TypeScript namespace 'client/src'; have: %v", allPaths)
	}

	if proj.DependencyGraph == nil {
		t.Fatal("dependency graph is nil")
	}

	coreEdges := proj.DependencyGraph.EdgesFrom("core")
	foundSerde := false
	for _, e := range coreEdges {
		if e.To == "serde" && e.External {
			foundSerde = true
		}
	}
	if !foundSerde {
		t.Error("missing Rust external edge core -> serde")
	}

	clientEdges := proj.DependencyGraph.EdgesFrom("client/src")
	foundThree := false
	for _, e := range clientEdges {
		if e.To == "client/three" || e.To == "three" {
			foundThree = true
		}
	}
	if !foundThree {
		t.Error("missing TypeScript external edge client/src -> three")
	}
}

func TestCompositeScanMergesPythonAndTS(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"pyproject.toml":   "[project]\nname = \"backend\"\n",
		"backend/main.py":  "def serve():\n    pass\n",
		"web/package.json": `{"name": "web-ui"}`,
		"web/src/index.ts": `export function mount() {}`,
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

	sc := &survey.CompositeScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = ns
	}

	if _, ok := nsMap["backend"]; !ok {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("missing Python namespace 'backend'; have: %v", allPaths)
	}

	if _, ok := nsMap["web/src"]; !ok {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("missing TypeScript namespace 'web/src'; have: %v", allPaths)
	}
}

func TestCompositeScan_GoAndRust(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		// Go sub-project at root.
		"go.mod":      "module example.com/goapp\n\ngo 1.21",
		"main.go":     "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hello\") }",
		"pkg/util.go": "package pkg\n\nfunc Helper() string { return \"help\" }",
		// Rust sub-project at root (workspace layout).
		"Cargo.toml":         "[workspace]\nmembers = [\"rustlib\"]",
		"rustlib/Cargo.toml": "[package]\nname = \"rustlib\"\nversion = \"0.1.0\"\nedition = \"2021\"",
		"rustlib/src/lib.rs": "pub fn greet() -> String { String::from(\"hello\") }",
	}

	if err := testkit.BuildFixture(dir, files); err != nil {
		t.Fatal(err)
	}

	sc := &survey.CompositeScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = ns
	}

	if len(nsMap) < 2 {
		t.Fatalf("expected at least 2 namespaces, got %d", len(nsMap))
	}

	// Verify Go namespaces carry the sub-project module path.
	// GoScanner produces import paths like "example.com/goapp" and
	// "example.com/goapp/pkg" from the module directive in go.mod.
	foundGoNS := false
	for path := range nsMap {
		if strings.Contains(path, "goapp") {
			foundGoNS = true
			break
		}
	}
	if !foundGoNS {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("no Go namespace containing 'goapp'; have: %v", allPaths)
	}

	// Verify the pkg sub-package is prefixed with the module path.
	foundPkg := false
	for path := range nsMap {
		if strings.HasSuffix(path, "/pkg") || strings.HasSuffix(path, "goapp/pkg") {
			foundPkg = true
			break
		}
	}
	if !foundPkg {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("no namespace ending with /pkg; have: %v", allPaths)
	}

	// Verify Rust namespace is present.
	if _, ok := nsMap["rustlib"]; !ok {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("missing Rust namespace 'rustlib'; have: %v", allPaths)
	}

	// Both languages must be represented in the scan results.
	if !foundGoNS {
		t.Error("Go language not represented in scan results")
	}
	if _, ok := nsMap["rustlib"]; !ok {
		t.Error("Rust language not represented in scan results")
	}
}

func TestCompositeScanAutoDetectsMultipleLanguages(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"Cargo.toml": `[package]
name = "backend"
version = "0.1.0"
`,
		"src/lib.rs":       `pub fn serve() {}`,
		"web/package.json": `{"name": "web-ui"}`,
		"web/src/index.ts": `export function mount() {}`,
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

	sc := &survey.AutoScanner{Override: "auto"}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(proj.Namespaces) < 2 {
		t.Errorf("expected at least 2 namespaces from composite scan, got %d", len(proj.Namespaces))
	}
}

// TestCompositeScanDiscoversNestedPythonPackages reproduces LCS-BUG-74:
// a monorepo with pyproject.toml files inside subdirectories (like
// deepagents libs/<pkg>/pyproject.toml) must be discovered as Python
// sub-projects, not silently fall through to CtagsScanner.
func TestCompositeScanDiscoversNestedPythonPackages(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		// No marker at repo root — typical for a multi-package monorepo.
		"libs/backend/pyproject.toml":          "[project]\nname = \"backend\"\n",
		"libs/backend/backend/__init__.py":      "",
		"libs/backend/backend/api.py":           "from backend.models import Record\nclass API:\n    pass\n",
		"libs/backend/backend/models.py":        "class Record:\n    pass\n",
		"libs/frontend/pyproject.toml":          "[project]\nname = \"frontend\"\n",
		"libs/frontend/frontend/__init__.py":    "",
		"libs/frontend/frontend/app.py":         "from backend.api import API\n",
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

	sc := &survey.CompositeScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(proj.Namespaces) == 0 {
		t.Fatal("expected Python namespaces from nested pyproject.toml discovery, got 0")
	}

	// Both sub-packages must be represented.
	nsMap := make(map[string]bool)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = true
	}

	for _, want := range []string{"libs/backend/backend", "libs/frontend/frontend"} {
		if !nsMap[want] {
			t.Errorf("missing namespace %q; have: %v", want, func() []string {
				keys := make([]string, 0, len(nsMap))
				for k := range nsMap {
					keys = append(keys, k)
				}
				return keys
			}())
		}
	}

	// Must produce internal dependency edges (not zero like before the fix).
	if proj.DependencyGraph == nil || len(proj.DependencyGraph.Edges) == 0 {
		t.Error("expected dependency edges from nested Python packages, got 0")
	}
}

// TestCompositeScanner_TSRootRustSubdir verifies that when TypeScript is at
// the repo root and Rust lives in a subdirectory, the Rust component is
// discovered. This is the LCS-NED-9 gap: Cargo.toml was missing from
// subProjectMarkers so Rust sub-directories were silently dropped.
//
// Given a repo with package.json at root and Cargo.toml in backend/
// When CompositeScanner scans the root
// Then both the TS namespace and the Rust namespace are returned
func TestCompositeScanner_TSRootRustSubdir(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"package.json":       `{"name": "frontend"}`,
		"src/index.ts":       `export function main() {}`,
		"backend/Cargo.toml": "[package]\nname = \"backend\"\nversion = \"0.1.0\"\n",
		"backend/src/lib.rs": "pub fn serve() {}",
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

	sc := &survey.CompositeScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.ImportPath] = ns
	}

	if _, ok := nsMap["src"]; !ok {
		t.Error("missing TypeScript namespace 'src'")
	}
	if _, ok := nsMap["backend"]; !ok {
		allPaths := make([]string, 0, len(nsMap))
		for k := range nsMap {
			allPaths = append(allPaths, k)
		}
		t.Errorf("missing Rust namespace 'backend'; have: %v", allPaths)
	}
}

// TestCompositeScanner_NoNamespaceDuplication verifies that a TypeScript
// npm-workspaces monorepo does not produce duplicate namespace entries.
//
// Given a TypeScript monorepo where discoverSubProjects finds "." (root) and
// individual "packages/*" sub-directories
// When CompositeScanner scans the monorepo root
// Then each namespace ImportPath appears exactly once in the result
func TestCompositeScanner_NoNamespaceDuplication(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		// Root workspace — discoverSubProjects finds "." from package.json
		"package.json": `{"name":"alef-test","workspaces":["packages/*"]}`,
		"tsconfig.json": `{
			"compilerOptions":{
				"paths":{
					"@alef/spine":  ["./packages/spine/src/index.ts"],
					"@alef/corpus": ["./packages/corpus/src/index.ts"]
				}
			}
		}`,
		// packages/spine — discoverSubProjects also finds this from package.json
		"packages/spine/package.json":   `{"name":"@alef/spine"}`,
		"packages/spine/tsconfig.json":  `{"extends":"../../tsconfig.json"}`,
		"packages/spine/src/index.ts":   "export function spineCore(): void {}\n",
		// packages/corpus — same
		"packages/corpus/package.json":  `{"name":"@alef/corpus"}`,
		"packages/corpus/tsconfig.json": `{"extends":"../../tsconfig.json"}`,
		"packages/corpus/src/index.ts":  "import { spineCore } from '@alef/spine';\nexport function corpusMain(): void { spineCore(); }\n",
	})

	sc := &survey.CompositeScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Check for duplicate ImportPaths.
	seen := make(map[string]int)
	for _, ns := range proj.Namespaces {
		seen[ns.ImportPath]++
	}
	for ip, count := range seen {
		if count > 1 {
			t.Errorf("namespace %q appears %d times (want 1) — CompositeScanner is duplicating namespaces", ip, count)
		}
	}

	// Confirm the cross-package edge is present exactly once.
	edgeCounts := make(map[[2]string]int)
	if proj.DependencyGraph != nil {
		for _, e := range proj.DependencyGraph.Edges {
			edgeCounts[[2]string{e.From, e.To}]++
		}
	}
	for endpoints, count := range edgeCounts {
		if count > 1 {
			t.Errorf("edge %q→%q weight=%d but appears %d times in raw edge list — unexpected duplication",
				endpoints[0], endpoints[1], count, count)
		}
	}
}

// TestCompositeScanner_CoLocatedRustAndTypeScript verifies ShojiWM-style
// polyglot roots: Cargo.toml and package.json both at "." must produce a
// composite scan (Rust + TS), not first-wins Rust-only.
func TestCompositeScanner_CoLocatedRustAndTypeScript(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"Cargo.toml":   "[package]\nname = \"shoji\"\nversion = \"0.1.0\"\n",
		"package.json": `{"name": "shoji-config"}`,
		"src/lib.rs":   "pub fn run() {}",
		"packages/config/index.ts": "export const cfg = 1;\n",
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

	if !survey.IsPolyglot(dir) {
		t.Fatal("expected IsPolyglot=true for Cargo.toml+package.json at root")
	}

	sc := &survey.AutoScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("auto scan: %v", err)
	}

	var hasRust, hasTS bool
	for _, ns := range proj.Namespaces {
		switch {
		case ns.ImportPath == "shoji" || ns.ImportPath == "src" || strings.Contains(ns.ImportPath, "lib"):
			hasRust = true
		case strings.Contains(ns.ImportPath, "packages") || strings.Contains(ns.ImportPath, "config"):
			hasTS = true
		}
		for _, f := range ns.Files {
			if strings.HasSuffix(f.Path, ".rs") {
				hasRust = true
			}
			if strings.HasSuffix(f.Path, ".ts") {
				hasTS = true
			}
		}
	}
	if !hasRust {
		t.Errorf("missing Rust namespaces; got %+v", nsPaths(proj))
	}
	if !hasTS {
		t.Errorf("missing TypeScript namespaces; got %+v", nsPaths(proj))
	}
}

func nsPaths(proj *model.Project) []string {
	out := make([]string, 0, len(proj.Namespaces))
	for _, ns := range proj.Namespaces {
		out = append(out, ns.ImportPath)
	}
	return out
}
