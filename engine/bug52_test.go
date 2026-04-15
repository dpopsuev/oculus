package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestBug52_GetCallers_StructConstruction reproduces LCS-BUG-52:
// GetCallers only finds function call edges, misses struct literal
// constructions like Config{Name: "x"}.
//
// Fixture has:
//   - func NewConfig(name string) Config { return Config{Name: name} }  ← function call
//   - func main() { cfg := Config{Name: "direct"} }                    ← struct literal
//
// GetCallers("Config") should find BOTH sites. Currently only finds
// function calls (NewConfig calling Config constructor), not literals.
func TestBug52_GetCallers_StructConstruction(t *testing.T) {
	dir := t.TempDir()

	files := map[string]string{
		"go.mod": "module testmod\ngo 1.21\n",
		"main.go": `package main

type Config struct {
	Name string
	Port int
}

func NewConfig(name string) Config {
	return Config{Name: name}
}

func LoadFromFile(path string) Config {
	return Config{Name: path, Port: 8080}
}

func main() {
	cfg1 := NewConfig("app")
	cfg2 := Config{Name: "direct", Port: 3000}
	_ = cfg1
	_ = cfg2
}
`,
	}

	for name, content := range files {
		p := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(content), 0o600)
	}

	eng := New(&mockStore{headSHA: "test"}, []string{dir})

	report, err := eng.GetCallers(context.Background(), dir, "Config")
	if err != nil {
		t.Fatalf("GetCallers: %v", err)
	}

	t.Logf("callers of Config: %d", len(report.Callers))
	for _, c := range report.Callers {
		t.Logf("  caller=%s pkg=%s file=%s line=%d", c.Caller, c.CallerPkg, c.File, c.Line)
	}

	// BUG-52: We expect to find construction sites where Config{} is used.
	// Currently GetCallers only searches CallGraph edges (function calls),
	// so it misses struct literal constructions.
	if len(report.Callers) == 0 {
		t.Error("BUG-52: GetCallers('Config') returned 0 callers — misses struct literal constructions")
	}

	// Specifically: NewConfig, LoadFromFile, and main all construct Config{}.
	// At minimum, main's direct construction Config{Name: "direct"} should appear.
	foundDirect := false
	for _, c := range report.Callers {
		if c.Caller == "main" {
			foundDirect = true
		}
	}
	if !foundDirect {
		t.Error("BUG-52: main() constructs Config{} directly but not found in callers")
	}
}
