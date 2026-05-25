package survey_test

// ts_path_aliases_test.go verifies that the TypeScript scanner reads
// tsconfig.json compilerOptions.paths and resolves path-alias imports to
// internal edges rather than treating them as external package dependencies.
//
// Without this, any monorepo that uses path aliases (the idiomatic npm
// workspaces pattern) produces zero cross-package edges, making blast-radius,
// risk-scores, coupling, and cycle detection useless for the real architecture.

import (
	"testing"

	"github.com/dpopsuev/oculus/v3/model"
	"github.com/dpopsuev/oculus/v3/survey"
)

// TestTSScan_PathAliases_ExactMatch verifies that an exact tsconfig path alias
// maps a scoped import to the correct local namespace.
//
// Given a monorepo with tsconfig.json paths:
//   "@myapp/core": ["./packages/core/src/index.ts"]
//   "@myapp/utils": ["./packages/utils/src/index.ts"]
// When packages/app/src/index.ts imports from both aliases
// Then the dependency graph contains internal edges to the resolved namespaces
func TestTSScan_PathAliases_ExactMatch(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"tsconfig.json": `{
			"compilerOptions": {
				"paths": {
					"@myapp/core":  ["./packages/core/src/index.ts"],
					"@myapp/utils": ["./packages/utils/src/index.ts"]
				}
			}
		}`,
		"packages/core/src/index.ts":  "export function coreFunc(): void {}\n",
		"packages/utils/src/index.ts": "export function utilFunc(): void {}\n",
		"packages/app/src/index.ts": `
import { coreFunc } from '@myapp/core';
import { utilFunc } from '@myapp/utils';

export function run(): void { coreFunc(); utilFunc(); }
`,
	})

	sc := newTSScanner()
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	edges := edgeIndex(proj)

	if !edges["packages/app/src"]["packages/core/src"] {
		t.Errorf("missing internal edge packages/app/src → packages/core/src (via @myapp/core alias)\n"+
			"all edges from packages/app/src: %v", keys(edges["packages/app/src"]))
	}
	if !edges["packages/app/src"]["packages/utils/src"] {
		t.Errorf("missing internal edge packages/app/src → packages/utils/src (via @myapp/utils alias)\n"+
			"all edges from packages/app/src: %v", keys(edges["packages/app/src"]))
	}

	// The alias imports must NOT appear as external edges.
	for to := range edges["packages/app/src"] {
		if to == "@myapp/core" || to == "@myapp/utils" {
			t.Errorf("alias import incorrectly treated as external: packages/app/src → %q", to)
		}
	}
}

// TestTSScan_PathAliases_GlobPattern verifies that a wildcard tsconfig path
// alias (the "@scope/*" pattern common in monorepos) is resolved correctly.
//
// Given tsconfig.json paths: "@packages/*": ["./packages/*/src/index.ts"]
// When organ imports from @packages/spine
// Then an internal edge organ → spine is created, not an external edge
func TestTSScan_PathAliases_GlobPattern(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"tsconfig.json": `{
			"compilerOptions": {
				"paths": {
					"@packages/*": ["./packages/*/src/index.ts"]
				}
			}
		}`,
		"packages/spine/src/index.ts": "export function spineFunc(): void {}\n",
		"packages/organ/src/index.ts": `
import { spineFunc } from '@packages/spine';

export function organFunc(): void { spineFunc(); }
`,
	})

	sc := newTSScanner()
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	edges := edgeIndex(proj)

	if !edges["packages/organ/src"]["packages/spine/src"] {
		t.Errorf("missing internal edge packages/organ/src → packages/spine/src (via @packages/spine glob alias)\n"+
			"all edges from packages/organ/src: %v", keys(edges["packages/organ/src"]))
	}
	if edges["packages/organ/src"]["@packages/spine"] {
		t.Errorf("glob alias incorrectly treated as external: packages/organ/src → @packages/spine")
	}
}

// TestTSScan_PathAliases_TsconfigBase verifies that paths defined in a
// tsconfig.base.json (common in monorepos that extend a root config) are
// also resolved.
//
// Given tsconfig.json extends tsconfig.base.json which defines paths
// When a package imports via the alias
// Then the edge resolves to the local namespace
func TestTSScan_PathAliases_TsconfigBase(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"tsconfig.json": `{ "extends": "./tsconfig.base.json" }`,
		"tsconfig.base.json": `{
			"compilerOptions": {
				"paths": {
					"@base/lib": ["./packages/lib/src/index.ts"]
				}
			}
		}`,
		"packages/lib/src/index.ts": "export function libFunc(): void {}\n",
		"packages/consumer/src/index.ts": `
import { libFunc } from '@base/lib';

export function consume(): void { libFunc(); }
`,
	})

	sc := newTSScanner()
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	edges := edgeIndex(proj)

	if !edges["packages/consumer/src"]["packages/lib/src"] {
		t.Errorf("missing internal edge packages/consumer/src → packages/lib/src (via tsconfig.base.json alias)\n"+
			"all edges from packages/consumer/src: %v", keys(edges["packages/consumer/src"]))
	}
}

// TestTSScan_PathAliases_UnknownAliasStillExternal verifies that imports not
// covered by any path alias continue to be treated as external dependencies.
//
// Given tsconfig.json paths that do NOT cover 'lodash'
// When a file imports from 'lodash'
// Then an external edge is created (existing behavior preserved)
func TestTSScan_PathAliases_UnknownAliasStillExternal(t *testing.T) {
	dir := setupTSProject(t, map[string]string{
		"tsconfig.json": `{
			"compilerOptions": {
				"paths": {
					"@myapp/core": ["./packages/core/src/index.ts"]
				}
			}
		}`,
		"packages/core/src/index.ts": "export function coreFunc(): void {}\n",
		"packages/app/src/index.ts": `
import { coreFunc } from '@myapp/core';
import { cloneDeep } from 'lodash';

export function run() { coreFunc(); cloneDeep({}); }
`,
	})

	sc := newTSScanner()
	proj, err := sc.Scan(dir)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	edges := edgeIndex(proj)

	// @myapp/core resolves internally.
	if !edges["packages/app/src"]["packages/core/src"] {
		t.Errorf("missing internal edge for resolved alias @myapp/core")
	}
	// lodash is not in paths — must remain external.
	if !edges["packages/app/src"]["lodash"] {
		t.Errorf("lodash (not in paths) should still produce an external edge")
	}
}

// --- helpers ---

// newTSScanner returns a TypeScriptScanner — extracted so tests read cleanly.
func newTSScanner() *survey.TypeScriptScanner { return &survey.TypeScriptScanner{} }

// edgeIndex builds a from→{to:true} lookup from the project's dependency graph.
// Only internal edges (External==false) are indexed under their "To" key;
// external edges use "@pkg" as the key so callers can distinguish them.
func edgeIndex(proj *model.Project) map[string]map[string]bool {
	idx := make(map[string]map[string]bool)
	if proj.DependencyGraph == nil {
		return idx
	}
	for _, e := range proj.DependencyGraph.Edges {
		if idx[e.From] == nil {
			idx[e.From] = make(map[string]bool)
		}
		idx[e.From][e.To] = true
	}
	return idx
}

// keys returns the keys of a map as a slice, for use in error messages.
func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
