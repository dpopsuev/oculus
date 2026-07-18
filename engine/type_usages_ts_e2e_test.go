package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
)

// End-to-end: scan a tiny TS monorepo and confirm type_usages finds both the
// declaring package and the import-type consumer (DiscussionRef pattern).
func TestTypeUsages_TSImportTypeCrossPackage(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"package.json": `{"name":"e2e-type-usages","private":true}`,
		"tsconfig.json": `{
  "compilerOptions": {
    "paths": {
      "@alef/kernel": ["./packages/kernel/src/index.ts"]
    }
  }
}`,
		"packages/kernel/src/index.ts": `export interface DiscussionRef {
  forumId: string
}
export function makeRef(): DiscussionRef { return { forumId: "f" } }
`,
		"packages/foundry/src/index.ts": `import type { DiscussionRef } from "@alef/kernel"
import { makeRef } from "@alef/kernel"

export type FoundryOpts = { discussion?: DiscussionRef }
export function use(): FoundryOpts { return { discussion: makeRef() } }
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

	report, err := arch.ScanAndBuild(context.Background(), dir, arch.ScanOpts{Intent: arch.IntentFull})
	if err != nil {
		t.Fatalf("ScanAndBuild: %v", err)
	}
	r := typeUsages(report, "DiscussionRef")
	if len(r.Files) < 2 {
		t.Fatalf("want ≥2 files (kernel decl + foundry import type), got %d: %+v summary=%s",
			len(r.Files), r.Files, r.Summary)
	}
	seen := map[string]bool{}
	for _, f := range r.Files {
		seen[f.Component] = true
	}
	if !seen["packages/kernel/src"] {
		t.Fatalf("missing declaring package; got %v", seen)
	}
	if !seen["packages/foundry/src"] {
		t.Fatalf("missing import-type consumer; got %v", seen)
	}
}
