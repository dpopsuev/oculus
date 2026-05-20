package survey

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3/model"
)

// buildFixture writes files into dir. Keys are relative paths, values are contents.
func buildFixture(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(abs), err)
		}
		if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

func TestResolvePackageImport(t *testing.T) {
	tests := []struct {
		name   string
		dirNS  map[string]*model.Namespace
		match  []string
		expect string
	}{
		{
			name: "exact match",
			dirNS: map[string]*model.Namespace{
				"domain": model.NewNamespace("domain", "domain"),
			},
			match:  []string{"import domain.Entity;", "domain"},
			expect: "domain",
		},
		{
			name: "nested path matches via filepath.Base",
			dirNS: map[string]*model.Namespace{
				"src/main/java/domain": model.NewNamespace("domain", "src/main/java/domain"),
			},
			match:  []string{"import domain.Entity;", "domain"},
			expect: "src/main/java/domain",
		},
		{
			name: "no match returns empty",
			dirNS: map[string]*model.Namespace{
				"domain": model.NewNamespace("domain", "domain"),
			},
			match:  []string{"import unknown.Foo;", "unknown"},
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolvePackageImport(tt.match, tt.dirNS)
			if got != tt.expect {
				t.Errorf("resolvePackageImport(%q) = %q, want %q", tt.match[1], got, tt.expect)
			}
		})
	}
}

func TestResolveModuleImport(t *testing.T) {
	tests := []struct {
		name   string
		dirNS  map[string]*model.Namespace
		match  []string
		expect string
	}{
		{
			name: "match via filepath.Base",
			dirNS: map[string]*model.Namespace{
				"Sources/Domain": model.NewNamespace("Domain", "Sources/Domain"),
			},
			match:  []string{"import Domain", "Domain"},
			expect: "Sources/Domain",
		},
		{
			name: "no match returns empty",
			dirNS: map[string]*model.Namespace{
				"Sources/Domain": model.NewNamespace("Domain", "Sources/Domain"),
			},
			match:  []string{"import Unknown", "Unknown"},
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveModuleImport(tt.match, tt.dirNS)
			if got != tt.expect {
				t.Errorf("resolveModuleImport(%q) = %q, want %q", tt.match[1], got, tt.expect)
			}
		})
	}
}

func TestExtractLanguageImports_Java(t *testing.T) {
	dir := t.TempDir()

	buildFixture(t, dir, map[string]string{
		"pom.xml":                "<project></project>",
		"src/domain/Entity.java": "package domain;\npublic class Entity {}\n",
		"src/adapter/Repo.java":  "package adapter;\nimport domain.Entity;\npublic class Repo {}\n",
	})

	dirNS := map[string]*model.Namespace{
		"src/domain": {
			Name:       "src/domain",
			ImportPath: "src/domain",
			Files:      []*model.File{model.NewFile("src/domain/Entity.java", "src/domain")},
		},
		"src/adapter": {
			Name:       "src/adapter",
			ImportPath: "src/adapter",
			Files:      []*model.File{model.NewFile("src/adapter/Repo.java", "src/adapter")},
		},
	}

	graph := extractLanguageImports(dir, model.LangJava, dirNS)
	if graph == nil {
		t.Fatal("extractLanguageImports returned nil")
	}

	edges := graph.EdgesFrom("src/adapter")
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge from src/adapter, got %d", len(edges))
	}
	if edges[0].To != "src/domain" {
		t.Errorf("edge target = %q, want %q", edges[0].To, "src/domain")
	}
	if edges[0].External {
		t.Error("edge should not be external")
	}
}

func TestExtractLanguageImports_Kotlin(t *testing.T) {
	dir := t.TempDir()

	buildFixture(t, dir, map[string]string{
		"build.gradle.kts":     "plugins { kotlin(\"jvm\") }",
		"src/domain/Entity.kt": "package domain\ndata class Entity(val id: String)\n",
		"src/adapter/Repo.kt":  "package adapter\nimport domain.Entity\nclass Repo {}\n",
	})

	dirNS := map[string]*model.Namespace{
		"src/domain": {
			Name:       "src/domain",
			ImportPath: "src/domain",
			Files:      []*model.File{model.NewFile("src/domain/Entity.kt", "src/domain")},
		},
		"src/adapter": {
			Name:       "src/adapter",
			ImportPath: "src/adapter",
			Files:      []*model.File{model.NewFile("src/adapter/Repo.kt", "src/adapter")},
		},
	}

	graph := extractLanguageImports(dir, model.LangKotlin, dirNS)
	if graph == nil {
		t.Fatal("extractLanguageImports returned nil")
	}

	edges := graph.EdgesFrom("src/adapter")
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge from src/adapter, got %d", len(edges))
	}
	if edges[0].To != "src/domain" {
		t.Errorf("edge target = %q, want %q", edges[0].To, "src/domain")
	}
	if edges[0].External {
		t.Error("edge should not be external")
	}
}

func TestExtractLanguageImports_CSharp(t *testing.T) {
	dir := t.TempDir()

	buildFixture(t, dir, map[string]string{
		"Project.csproj":   "<Project></Project>",
		"Domain/Entity.cs": "namespace Domain { public class Entity {} }\n",
		"Adapter/Repo.cs":  "using Domain;\nnamespace Adapter { public class Repo {} }\n",
	})

	dirNS := map[string]*model.Namespace{
		"Domain": {
			Name:       "Domain",
			ImportPath: "Domain",
			Files:      []*model.File{model.NewFile("Domain/Entity.cs", "Domain")},
		},
		"Adapter": {
			Name:       "Adapter",
			ImportPath: "Adapter",
			Files:      []*model.File{model.NewFile("Adapter/Repo.cs", "Adapter")},
		},
	}

	graph := extractLanguageImports(dir, model.LangCSharp, dirNS)
	if graph == nil {
		t.Fatal("extractLanguageImports returned nil")
	}

	edges := graph.EdgesFrom("Adapter")
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge from Adapter, got %d", len(edges))
	}
	if edges[0].To != "Domain" {
		t.Errorf("edge target = %q, want %q", edges[0].To, "Domain")
	}
	if edges[0].External {
		t.Error("edge should not be external")
	}
}

func TestResolveToNamespace_Python(t *testing.T) {
	pkgSet := map[string]bool{
		"domain": true,
	}

	tests := []struct {
		name      string
		importKey string
		expect    string
	}{
		{
			name:      "exact match",
			importKey: "domain",
			expect:    "domain",
		},
		{
			name:      "prefix match",
			importKey: "domain/entity",
			expect:    "domain",
		},
		{
			name:      "no match returns input with dots",
			importKey: "unknown",
			expect:    "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveToNamespace(tt.importKey, pkgSet)
			if got != tt.expect {
				t.Errorf("resolveToNamespace(%q) = %q, want %q", tt.importKey, got, tt.expect)
			}
		})
	}
}

func TestRequireEdgeDetection_JavaScript(t *testing.T) {
	dir := t.TempDir()

	buildFixture(t, dir, map[string]string{
		"package.json":           `{"name":"test"}`,
		"src/domain/entity.js":   "class Entity {}\nmodule.exports = { Entity };\n",
		"src/adapter/handler.js": "const { Entity } = require('../domain/entity');\nclass Handler {}\n",
	})

	sc := &TypeScriptScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.DependencyGraph == nil {
		t.Fatal("dependency graph is nil")
	}

	edges := proj.DependencyGraph.EdgesFrom("src/adapter")
	found := false
	for _, e := range edges {
		if e.To == "src/domain" && !e.External {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("missing internal edge src/adapter -> src/domain; edges: %+v", edges)
	}
}

// TestExtractCIncludesRootRelative reproduces LCS-BUG-74 (C side):
// a #include "subdir/header.h" whose actual file lives at <root>/subdir/header.h
// (compiled with -I <root>, neovim-style) must produce edge src→subdir,
// not a phantom edge to src/subdir (old file-relative resolution).
func TestExtractCIncludesRootRelative(t *testing.T) {
	dir := t.TempDir()

	buildFixture(t, dir, map[string]string{
		"CMakeLists.txt": "cmake_minimum_required(VERSION 3.0)\n",
		// Header lives at root/utils/helper.h (project-root-relative include).
		"utils/helper.h": "// helper\n",
		// Source includes it as "utils/helper.h" — standard -I <root> pattern.
		"core/main.c": "#include \"utils/helper.h\"\nint main() { return 0; }\n",
	})

	deps := extractCIncludes(dir)
	if deps == nil {
		t.Fatal("extractCIncludes returned nil")
	}

	// Must produce the real edge core → utils.
	edges := deps.EdgesFrom("core")
	found := false
	for _, e := range edges {
		if e.To == "utils" && !e.External {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected root-relative edge core→utils; all edges from core: %+v", edges)
	}

	// Must NOT produce the phantom edge core → core/utils.
	for _, e := range deps.Edges {
		if e.From == "core" && e.To == "core/utils" {
			t.Errorf("phantom edge core→core/utils must not exist")
		}
	}
}

// TestExtractLuaRequires reproduces LCS-BUG-74 (Lua side):
// require("module.sub") calls should produce dependency edges between
// directories, just like Python/Java/C# imports.
func TestExtractLuaRequires(t *testing.T) {
	dir := t.TempDir()

	buildFixture(t, dir, map[string]string{
		"plugin/init.lua": "local engine = require(\"core.engine\")\n",
		"core/engine.lua": "-- engine module\nreturn {}\n",
	})

	dirNS := map[string]*model.Namespace{
		"plugin": {
			Name:       "plugin",
			ImportPath: "plugin",
			Files:      []*model.File{model.NewFile("plugin/init.lua", "plugin")},
		},
		"core": {
			Name:       "core",
			ImportPath: "core",
			Files:      []*model.File{model.NewFile("core/engine.lua", "core")},
		},
	}

	graph := extractLanguageImports(dir, model.LangLua, dirNS)
	if graph == nil {
		t.Fatal("extractLanguageImports returned nil for LangLua — Lua require() not implemented")
	}

	edges := graph.EdgesFrom("plugin")
	found := false
	for _, e := range edges {
		if e.To == "core" && !e.External {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected edge plugin→core from require(\"core.engine\"); edges: %+v", edges)
	}
}

// TestExtractCIncludesSrcSearchPath reproduces LCS-BUG-74 (neovim C side):
// #include "nvim/api.h" from src/nvim/main.c should resolve via the
// -I src search path to src/nvim/api.h, producing edge src/nvim → src/nvim/api.
func TestExtractCIncludesSrcSearchPath(t *testing.T) {
	dir := t.TempDir()

	buildFixture(t, dir, map[string]string{
		"CMakeLists.txt":          "cmake_minimum_required(VERSION 3.0)\n",
		"src/nvim/api/handler.h":  "// handler\n",
		"src/nvim/main.c":         "#include \"nvim/api/handler.h\"\nint main() { return 0; }\n",
	})

	deps := extractCIncludes(dir)
	if deps == nil {
		t.Fatal("extractCIncludes returned nil")
	}

	// Must resolve via src/ search path: src/nvim → src/nvim/api.
	edges := deps.EdgesFrom("src/nvim")
	found := false
	for _, e := range edges {
		if e.To == "src/nvim/api" && !e.External {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected src/nvim→src/nvim/api via -I src resolution; all edges: %+v", deps.Edges)
	}

	// Must NOT produce phantom edge src/nvim → src/nvim/nvim/api.
	for _, e := range deps.Edges {
		if e.From == "src/nvim" && e.To == "src/nvim/nvim/api" {
			t.Errorf("phantom edge src/nvim→src/nvim/nvim/api must not exist")
		}
	}
}
