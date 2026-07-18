package survey_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3/survey"
)

// Composite merge must preserve TypeImports / imports-type from TS sub-scans.
func TestCompositePreservesTypeImports(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"package.json": `{"name":"comp-type","private":true}`,
		"tsconfig.json": `{"compilerOptions":{"paths":{"@alef/kernel":["./packages/kernel/src/index.ts"]}}}`,
		"packages/kernel/package.json":  `{"name":"@alef/kernel"}`,
		"packages/kernel/src/index.ts":  `export interface DiscussionRef { id: string }`,
		"packages/foundry/package.json": `{"name":"@alef/foundry"}`,
		"packages/foundry/src/index.ts": "import type { DiscussionRef } from \"@alef/kernel\"\nexport type O = { d?: DiscussionRef }\n",
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

	proj, err := (&survey.CompositeScanner{}).Scan(dir)
	if err != nil {
		t.Fatalf("composite scan: %v", err)
	}
	var foundry bool
	for _, ns := range proj.Namespaces {
		for _, ti := range ns.TypeImports {
			if ti.Name == "DiscussionRef" {
				foundry = true
			}
		}
	}
	if !foundry {
		t.Fatalf("composite dropped TypeImports; namespaces=%d", len(proj.Namespaces))
	}
}
