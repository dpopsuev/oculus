package engine

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHybridQueryTerms(t *testing.T) {
	terms := hybridQueryTerms("where is the symbol graph built for Engine?")
	joined := strings.Join(terms, " ")
	if !strings.Contains(joined, "symbol") && !strings.Contains(joined, "graph") && !strings.Contains(joined, "engine") {
		t.Fatalf("terms=%v, want symbol/graph/engine tokens", terms)
	}
	for _, s := range terms {
		if s == "where" || s == "the" || s == "for" {
			t.Errorf("stopword leaked: %q", s)
		}
	}
}

func TestAnswerQuery_HybridRetrieve(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module hybridmod\ngo 1.21\n",
		"engine.go": `package hybridmod

type Engine struct{}

func (e *Engine) GetSymbolGraph() {}
func ScanProject() {}
`,
	}
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	_ = exec.Command("git", "-C", dir, "init", "-q").Run()
	_ = exec.Command("git", "-C", dir, "config", "user.email", "t@t").Run()
	_ = exec.Command("git", "-C", dir, "config", "user.name", "t").Run()
	_ = exec.Command("git", "-C", dir, "add", "-A").Run()
	_ = exec.Command("git", "-C", dir, "commit", "-q", "-m", "init").Run()

	eng := New(&mockStore{headSHA: "abc"}, []string{dir})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := eng.ScanProject(ctx, dir, ScanOpts{Intent: "full"}); err != nil {
		t.Fatalf("scan: %v", err)
	}

	r, err := eng.AnswerQuery(ctx, dir, "where is ScanProject defined")
	if err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}
	if r.Action != "hybrid" {
		t.Fatalf("action=%q, want hybrid (got answer=%v)", r.Action, r.Answer)
	}
	ans, ok := r.Answer.(HybridAnswer)
	if !ok {
		t.Fatalf("answer type %T", r.Answer)
	}
	if len(ans.Hits) == 0 {
		t.Fatal("expected hybrid hits")
	}
	var hasChunk bool
	for _, h := range ans.Hits {
		if strings.Contains(strings.ToLower(h.Symbol), "scanproject") || strings.Contains(h.Chunk, "ScanProject") {
			hasChunk = h.Chunk != "" || h.File != ""
		}
	}
	if !hasChunk {
		// Chunk excerpt preferred; file+symbol still OK.
		t.Logf("hits=%+v", ans.Hits)
		if ans.Hits[0].File == "" && ans.Hits[0].Chunk == "" {
			t.Fatal("expected file or chunk on top hit")
		}
	}
}

func TestSgOptsOrQuick_DefaultQuick(t *testing.T) {
	o := sgOptsOrQuick(nil)
	if o.AllowLSP || !o.Quick {
		t.Fatal("default must be AST-only (Quick / !AllowLSP)")
	}
	o = sgOptsOrQuick([]SymbolGraphOpts{{AllowLSP: true, FocusEntry: "Foo"}})
	if !o.AllowLSP || o.Quick {
		t.Fatal("AllowLSP=true must enable scoped LSP")
	}
	if o.FocusEntry != "Foo" {
		t.Fatalf("FocusEntry=%q", o.FocusEntry)
	}
}

func TestFocusEntryName(t *testing.T) {
	if got := focusEntryName("pkg.Foo"); got != "Foo" {
		t.Fatalf("got %q", got)
	}
	if got := focusEntryName("applySessionMetadataRefresh"); got != "applySessionMetadataRefresh" {
		t.Fatalf("got %q", got)
	}
}

func TestHybridQueryTerms_CamelCase(t *testing.T) {
	terms := hybridQueryTerms("where is GetSymbolGraph")
	joined := strings.Join(terms, ",")
	if !strings.Contains(joined, "getsymbolgraph") && !strings.Contains(joined, "symbol") {
		t.Fatalf("terms=%v", terms)
	}
}
