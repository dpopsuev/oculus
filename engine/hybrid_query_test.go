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

	r, err := eng.AnswerQuery(ctx, dir, "where is GetSymbolGraph defined")
	if err != nil {
		t.Fatalf("AnswerQuery: %v", err)
	}
	if r.Action != "hybrid" {
		t.Fatalf("action=%q, want hybrid (got answer=%v)", r.Action, r.Answer)
	}
}

func TestSgOptsOrQuick_DefaultQuick(t *testing.T) {
	o := sgOptsOrQuick(nil)
	if !o.Quick {
		t.Fatal("default must be Quick")
	}
	o = sgOptsOrQuick([]SymbolGraphOpts{{Quick: false}})
	if o.Quick {
		t.Fatal("explicit deep must disable Quick")
	}
}
