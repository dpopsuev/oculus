package cache

import "testing"

func TestSearchChunks_FindsFunc(t *testing.T) {
	chunks := []Chunk{
		{ID: "1", Symbol: "ScanProject", File: "engine/protocol.go", Text: "func (p *Engine) ScanProject(ctx context.Context, path string)", Kind: "function"},
		{ID: "2", Symbol: "WarmLSP", File: "engine/protocol.go", Text: "func (p *Engine) WarmLSP(ctx context.Context, path string)", Kind: "function"},
		{ID: "3", Symbol: "helper", File: "util.go", Text: "func helper() {}", Kind: "function"},
	}
	hits := SearchChunks(chunks, "where is ScanProject defined", 3)
	if len(hits) == 0 {
		t.Fatal("no hits")
	}
	if hits[0].Symbol != "ScanProject" {
		t.Fatalf("top=%q, want ScanProject", hits[0].Symbol)
	}
}

func TestBuildChunksFromFiles(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"go.mod":  "module c\ngo 1.21\n",
		"main.go": "package main\n\nfunc ScanProject() {}\n\nfunc main() {}\n",
	})
	chunks, err := BuildChunksFromFiles(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, c := range chunks {
		if c.Symbol == "ScanProject" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ScanProject chunk missing: %+v", chunks)
	}
}
