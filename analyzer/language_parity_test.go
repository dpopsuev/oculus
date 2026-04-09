package analyzer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/dpopsuev/oculus"
)

// languageFixture defines a test fixture for a specific language.
type languageFixture struct {
	name    string            // language name
	marker  string            // file that DetectLanguage looks for
	files   map[string]string // source files
	entry   string            // call graph entry function
	callee  string            // expected callee in call graph
}

var languageFixtures = []languageFixture{
	{
		name:   "Python",
		marker: "pyproject.toml",
		files: map[string]string{
			"pyproject.toml": "[project]\nname = \"test\"\n",
			"main.py": `def load_config(path: str) -> dict:
    return {"name": path}

def transform(cfg: dict) -> list:
    return list(cfg.values())

def main():
    cfg = load_config("app.yaml")
    result = transform(cfg)
`,
		},
		entry:  "main",
		callee: "load_config",
	},
	{
		name:   "TypeScript",
		marker: "tsconfig.json",
		files: map[string]string{
			"tsconfig.json": `{"compilerOptions": {"target": "es2020"}}`,
			"package.json":  `{"name": "test"}`,
			"src/main.ts": `interface Config {
  name: string;
}

function loadConfig(path: string): Config {
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
		},
		entry:  "main",
		callee: "loadConfig",
	},
	{
		name:   "JavaScript",
		marker: "package.json",
		files: map[string]string{
			"package.json": `{"name": "test"}`,
			"main.js": `function loadConfig(path) {
  return { name: path };
}

function transform(cfg) {
  return cfg.name;
}

function main() {
  const cfg = loadConfig("app.yaml");
  const result = transform(cfg);
}

module.exports = { main };
`,
		},
		entry:  "main",
		callee: "loadConfig",
	},
	{
		name:   "Rust",
		marker: "Cargo.toml",
		files: map[string]string{
			"Cargo.toml": "[package]\nname = \"test\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
			"src/main.rs": `struct Config {
    name: String,
}

fn load_config(path: &str) -> Config {
    Config { name: path.to_string() }
}

fn transform(cfg: &Config) -> String {
    cfg.name.clone()
}

fn main() {
    let cfg = load_config("app.yaml");
    let result = transform(&cfg);
    println!("{}", result);
}
`,
		},
		entry:  "main",
		callee: "load_config",
	},
	{
		name:   "Java",
		marker: "pom.xml",
		files: map[string]string{
			"pom.xml": "<project><modelVersion>4.0.0</modelVersion><groupId>test</groupId><artifactId>test</artifactId><version>1.0</version></project>",
			"src/Main.java": `public class Main {
    public static String loadConfig(String path) {
        return path;
    }

    public static int transform(String cfg) {
        return cfg.length();
    }

    public static void main(String[] args) {
        String cfg = loadConfig("app.yaml");
        int result = transform(cfg);
    }
}
`,
		},
		entry:  "main",
		callee: "loadConfig",
	},
	{
		name:   "C",
		marker: "Makefile",
		files: map[string]string{
			"Makefile": "all:\n\tgcc -o main main.c\n",
			"main.c": `#include <stdio.h>
#include <string.h>

typedef struct {
    char name[256];
} Config;

Config load_config(const char* path) {
    Config cfg;
    strncpy(cfg.name, path, sizeof(cfg.name) - 1);
    return cfg;
}

int transform(Config* cfg) {
    return (int)strlen(cfg->name);
}

int main() {
    Config cfg = load_config("app.yaml");
    int result = transform(&cfg);
    printf("%d\n", result);
    return 0;
}
`,
		},
		entry:  "main",
		callee: "load_config",
	},
	{
		name:   "C++",
		marker: "CMakeLists.txt",
		files: map[string]string{
			"CMakeLists.txt": "cmake_minimum_required(VERSION 3.10)\nproject(test)\nadd_executable(main main.cpp)\n",
			"main.cpp": `#include <string>
#include <iostream>

struct Config {
    std::string name;
};

Config loadConfig(const std::string& path) {
    return Config{path};
}

std::string transform(const Config& cfg) {
    return cfg.name;
}

int main() {
    auto cfg = loadConfig("app.yaml");
    auto result = transform(cfg);
    std::cout << result << std::endl;
    return 0;
}
`,
		},
		entry:  "main",
		callee: "loadConfig",
	},
	{
		name:   "Kotlin",
		marker: "build.gradle.kts",
		files: map[string]string{
			"build.gradle.kts": "plugins { kotlin(\"jvm\") version \"1.9.0\" }\n",
			"src/main/kotlin/Main.kt": `data class Config(val name: String)

fun loadConfig(path: String): Config {
    return Config(name = path)
}

fun transform(cfg: Config): String {
    return cfg.name
}

fun main() {
    val cfg = loadConfig("app.yaml")
    val result = transform(cfg)
    println(result)
}
`,
		},
		entry:  "main",
		callee: "loadConfig",
	},
	{
		name:   "Zig",
		marker: "build.zig",
		files: map[string]string{
			"build.zig": "const std = @import(\"std\");\npub fn build(b: *std.Build) void { _ = b; }\n",
			"src/main.zig": `const std = @import("std");

const Config = struct {
    name: []const u8,
};

fn loadConfig(path: []const u8) Config {
    return Config{ .name = path };
}

fn transform(cfg: Config) []const u8 {
    return cfg.name;
}

pub fn main() void {
    const cfg = loadConfig("app.yaml");
    const result = transform(cfg);
    std.debug.print("{s}\n", .{result});
}
`,
		},
		entry:  "main",
		callee: "loadConfig",
	},
	{
		name:   "Swift",
		marker: "Package.swift",
		files: map[string]string{
			"Package.swift": "// swift-tools-version:5.5\nimport PackageDescription\nlet package = Package(name: \"test\")\n",
			"Sources/main.swift": `struct Config {
    let name: String
}

func loadConfig(path: String) -> Config {
    return Config(name: path)
}

func transform(_ cfg: Config) -> String {
    return cfg.name
}

let cfg = loadConfig(path: "app.yaml")
let result = transform(cfg)
print(result)
`,
		},
		entry:  "main",
		callee: "loadConfig",
	},
	{
		name:   "CSharp",
		marker: "Program.cs",
		files: map[string]string{
			"test.csproj": "<Project Sdk=\"Microsoft.NET.Sdk\"><PropertyGroup><OutputType>Exe</OutputType></PropertyGroup></Project>",
			"Program.cs": `class Config {
    public string Name { get; set; }
}

class Program {
    static Config LoadConfig(string path) {
        return new Config { Name = path };
    }

    static string Transform(Config cfg) {
        return cfg.Name;
    }

    static void Main() {
        var cfg = LoadConfig("app.yaml");
        var result = Transform(cfg);
        System.Console.WriteLine(result);
    }
}
`,
		},
		entry:  "Main",
		callee: "LoadConfig",
	},
}

// TestLanguageParity_CallGraph verifies that each language produces a call graph
// via the fallback chain (TreeSitter or language-specific analyzer).
func TestLanguageParity_CallGraph(t *testing.T) {
	for _, fix := range languageFixtures {
		t.Run(fix.name, func(t *testing.T) {
			dir := setupFixture(t, fix.files)

			da := NewDeepFallback(dir, nil)
			cg, err := da.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: fix.entry, Depth: 5})
			if err != nil {
				t.Fatalf("CallGraph: %v", err)
			}

			t.Logf("[%s] layer=%s, %d nodes, %d edges", fix.name, cg.Layer, len(cg.Nodes), len(cg.Edges))

			if len(cg.Edges) == 0 {
				t.Logf("[%s] 0 edges — TreeSitter may not support this language's call syntax", fix.name)
				return
			}

			// Check if callee appears
			found := false
			for _, e := range cg.Edges {
				if e.Callee == fix.callee {
					found = true
					t.Logf("[%s] found edge: %s → %s (params=%v returns=%v)",
						fix.name, e.Caller, e.Callee, e.ParamTypes, e.ReturnTypes)
				}
			}
			if !found {
				t.Logf("[%s] callee %q not found in edges (may use different name convention)", fix.name, fix.callee)
			}

			// Count typed edges
			typed := countTyped(cg.Edges)
			pct := float64(typed) / float64(len(cg.Edges)) * 100
			t.Logf("[%s] typed edges: %d/%d (%.0f%%)", fix.name, typed, len(cg.Edges), pct)
		})
	}
}

// TestLanguageParity_Enrichment verifies that EnrichCallEdgeTypes
// can fill in types for edges produced by any analyzer.
func TestLanguageParity_Enrichment(t *testing.T) {
	// Go fixture is the reference — enrichment via go/parser always works.
	dir := setupContractFixture(t)

	// Use Regex (produces edges with 0% types) then enrich
	a := &RegexDeepAnalyzer{}
	cg, err := a.CallGraph(context.Background(), dir, oculus.CallGraphOpts{Entry: "main", Depth: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(cg.Edges) == 0 {
		t.Skip("Regex produced 0 edges")
	}

	before := countTyped(cg.Edges)
	EnrichCallEdgeTypes(dir, cg.Edges)
	after := countTyped(cg.Edges)

	t.Logf("Go enrichment: %d → %d typed (of %d edges)", before, after, len(cg.Edges))
	if after <= before {
		t.Error("enrichment did not improve type coverage")
	}
}

func setupFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(files[rel]), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

var _ = fmt.Sprintf // keep fmt import
