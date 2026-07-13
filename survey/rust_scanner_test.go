package survey_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3/model"
	"github.com/dpopsuev/oculus/v3/survey"
)

func setupCrate(t *testing.T, files map[string]string) string {
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

func TestRustScanSingleCrate(t *testing.T) {
	dir := setupCrate(t, map[string]string{
		"Cargo.toml": `[package]
name = "my-crate"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = "1"
`,
		"src/lib.rs": `pub fn hello() -> String {
    "hello".to_string()
}

pub struct Config {
    pub name: String,
}

fn internal() {}
`,
	})

	sc := &survey.RustScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if proj.Path != "my-crate" {
		t.Errorf("path = %q, want my-crate", proj.Path)
	}
	if proj.Language != model.LangRust {
		t.Errorf("language = %v, want Rust", proj.Language)
	}
	if len(proj.Namespaces) != 1 {
		t.Fatalf("namespaces = %d, want 1", len(proj.Namespaces))
	}

	ns := proj.Namespaces[0]
	if ns.Name != "my-crate" {
		t.Errorf("ns.name = %q, want my-crate", ns.Name)
	}

	symMap := make(map[string]*model.Symbol)
	for _, s := range ns.Symbols {
		symMap[s.Name] = s
	}

	if _, ok := symMap["hello"]; !ok {
		t.Error("missing pub fn hello")
	}
	if _, ok := symMap["Config"]; !ok {
		t.Error("missing pub struct Config")
	}
	if _, ok := symMap["internal"]; ok {
		t.Error("internal (non-pub) fn should not be extracted")
	}

	if proj.DependencyGraph == nil {
		t.Fatal("dependency graph is nil")
	}
	edges := proj.DependencyGraph.EdgesFrom("my-crate")
	foundSerde := false
	for _, e := range edges {
		if e.To == "serde" && e.External {
			foundSerde = true
		}
	}
	if !foundSerde {
		t.Error("missing external edge to serde")
	}
}

func TestRustScanWorkspace(t *testing.T) {
	dir := setupCrate(t, map[string]string{
		"Cargo.toml": `[workspace]
members = ["crates/core", "crates/server"]
`,
		"crates/core/Cargo.toml": `[package]
name = "my-core"
version = "0.1.0"

[dependencies]
serde = "1"
`,
		"crates/core/src/lib.rs": `pub trait Handler {
    fn handle(&self);
}

pub enum Status {
    Active,
    Inactive,
}
`,
		"crates/server/Cargo.toml": `[package]
name = "my-server"
version = "0.1.0"

[dependencies]
my-core = { path = "../core" }
tokio = "1"
`,
		"crates/server/src/main.rs": `pub fn start() {}

pub struct Server {
    port: u16,
}
`,
	})

	sc := &survey.RustScanner{}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(proj.Namespaces) != 2 {
		t.Fatalf("namespaces = %d, want 2", len(proj.Namespaces))
	}

	nsMap := make(map[string]*model.Namespace)
	for _, ns := range proj.Namespaces {
		nsMap[ns.Name] = ns
	}

	core, ok := nsMap["my-core"]
	if !ok {
		t.Fatal("missing namespace my-core")
	}
	coreSyms := make(map[string]bool)
	for _, s := range core.Symbols {
		coreSyms[s.Name] = true
	}
	if !coreSyms["Handler"] {
		t.Error("missing pub trait Handler in core")
	}
	if !coreSyms["Status"] {
		t.Error("missing pub enum Status in core")
	}

	server, ok := nsMap["my-server"]
	if !ok {
		t.Fatal("missing namespace my-server")
	}
	serverSyms := make(map[string]bool)
	for _, s := range server.Symbols {
		serverSyms[s.Name] = true
	}
	if !serverSyms["start"] {
		t.Error("missing pub fn start in server")
	}
	if !serverSyms["Server"] {
		t.Error("missing pub struct Server in server")
	}

	// Internal edge: server -> core
	serverEdges := proj.DependencyGraph.EdgesFrom("my-server")
	foundCore := false
	foundTokio := false
	for _, e := range serverEdges {
		if e.To == "my-core" && !e.External {
			foundCore = true
		}
		if e.To == "tokio" && e.External {
			foundTokio = true
		}
	}
	if !foundCore {
		t.Error("missing internal edge server -> core")
	}
	if !foundTokio {
		t.Error("missing external edge server -> tokio")
	}
}

// TestRustScan_FileLevel_MultiModule verifies that FileLevel granularity splits a
// single-crate repo into per-.rs namespaces with mod/use edges (Seeshell-shaped).
func TestRustScan_FileLevel_MultiModule(t *testing.T) {
	dir := setupCrate(t, map[string]string{
		"Cargo.toml": `[package]
name = "seeshell-lite"
version = "0.1.0"
edition = "2021"

[dependencies]
serde = "1"
`,
		"src/lib.rs": `pub mod action;
pub mod journal;
pub mod vm_harness;

pub use action::ActionService;
`,
		"src/action.rs": `use crate::journal::Journal;

pub struct ActionService;

impl ActionService {
    pub fn run(&self, j: &Journal) {}
}
`,
		"src/journal.rs": `pub struct Journal;

pub fn record() {}
`,
		"src/vm_harness/mod.rs": `pub mod qmp;

pub struct Harness;
`,
		"src/vm_harness/qmp.rs": `use crate::journal::Journal;

pub struct QmpClient;

impl QmpClient {
    pub fn connect(&self, _j: &Journal) {}
}
`,
		"src/bin/tool.rs": `fn main() {}
`,
		"tests/integration.rs": `#[test]
fn smoke() {}
`,
	})

	sc := &survey.RustScanner{Granularity: survey.FileLevel}
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}

	if len(proj.Namespaces) < 4 {
		names := make([]string, len(proj.Namespaces))
		for i, ns := range proj.Namespaces {
			names[i] = ns.Name
		}
		t.Fatalf("want ≥4 file namespaces, got %d: %v", len(proj.Namespaces), names)
	}

	nsMap := map[string]bool{}
	for _, ns := range proj.Namespaces {
		nsMap[ns.Name] = true
	}
	for _, want := range []string{"src/lib.rs", "src/action.rs", "src/journal.rs", "src/vm_harness/mod.rs", "src/vm_harness/qmp.rs"} {
		if !nsMap[want] {
			t.Errorf("missing namespace %q", want)
		}
	}

	hasEdge := func(from, to string) bool {
		for _, e := range proj.DependencyGraph.EdgesFrom(from) {
			if e.To == to && !e.External {
				return true
			}
		}
		return false
	}
	if !hasEdge("src/lib.rs", "src/action.rs") {
		t.Error("missing mod edge lib → action")
	}
	if !hasEdge("src/lib.rs", "src/journal.rs") {
		t.Error("missing mod edge lib → journal")
	}
	if !hasEdge("src/lib.rs", "src/vm_harness/mod.rs") {
		t.Error("missing mod edge lib → vm_harness")
	}
	if !hasEdge("src/action.rs", "src/journal.rs") {
		t.Error("missing use crate::journal edge action → journal")
	}
	if !hasEdge("src/vm_harness/mod.rs", "src/vm_harness/qmp.rs") {
		t.Error("missing mod edge vm_harness → qmp")
	}
	if !hasEdge("src/vm_harness/qmp.rs", "src/journal.rs") {
		t.Error("missing use crate::journal edge qmp → journal")
	}

	// Default crate-level scan must remain one namespace.
	crateScan := &survey.RustScanner{}
	crateProj, err := crateScan.Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(crateProj.Namespaces) != 1 {
		t.Errorf("default scan namespaces=%d, want 1 (backward compat)", len(crateProj.Namespaces))
	}
}
