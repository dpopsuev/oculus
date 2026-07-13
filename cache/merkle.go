package cache

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dpopsuev/oculus/v3/lang"
)

// Index is a Merkle fingerprint of a working tree's source files.
type Index struct {
	Root   string            // hex sha256 of sorted path\0leaf lines
	Leaves map[string]string // slash-relative path → leaf hash
}

// merkleSourceExts are file extensions included in the Merkle walk.
var merkleSourceExts = map[string]bool{
	".go": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true,
	".py": true, ".rs": true, ".java": true, ".kt": true, ".c": true,
	".h": true, ".cpp": true, ".cc": true, ".hpp": true, ".cs": true,
	".swift": true, ".zig": true,
}

// merkleManifests are project marker files included even without a source ext.
var merkleManifests = map[string]bool{
	"go.mod": true, "go.sum": true,
	"package.json": true, "tsconfig.json": true,
	"Cargo.toml": true, "Cargo.lock": true,
	"pyproject.toml": true, "setup.py": true,
	"pom.xml": true, "build.gradle": true, "build.gradle.kts": true,
	"CMakeLists.txt": true, "Package.swift": true, "build.zig": true,
}

func isMerkleFile(rel string) bool {
	base := filepath.Base(rel)
	if merkleManifests[base] {
		return true
	}
	return merkleSourceExts[strings.ToLower(filepath.Ext(base))]
}

// BuildMerkle walks root and returns a content Merkle index of source files.
// Directories skipped by lang.ShouldSkipDir are excluded.
func BuildMerkle(root string) (Index, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return Index{}, err
	}
	leaves := make(map[string]string)
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := d.Name()
		if d.IsDir() {
			if path != abs && lang.ShouldSkipDir(name) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(abs, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !isMerkleFile(rel) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h := sha256.Sum256(append(append([]byte(rel), 0), data...))
		leaves[rel] = fmt.Sprintf("%x", h)
		return nil
	})
	if err != nil {
		return Index{}, err
	}
	return Index{Root: merkleRoot(leaves), Leaves: leaves}, nil
}

func merkleRoot(leaves map[string]string) string {
	paths := make([]string, 0, len(leaves))
	for p := range leaves {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		_, _ = fmt.Fprintf(h, "%s\x00%s\n", p, leaves[p])
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Diff returns paths that were added, removed, or whose leaf hash changed
// between a and b (union of symmetric difference).
func Diff(a, b Index) []string {
	seen := make(map[string]bool)
	var out []string
	for p, ha := range a.Leaves {
		if hb, ok := b.Leaves[p]; !ok || ha != hb {
			out = append(out, p)
			seen[p] = true
		}
	}
	for p := range b.Leaves {
		if !seen[p] {
			if _, ok := a.Leaves[p]; !ok {
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}

// Packages maps file paths to package directories (slash paths).
// Files in the repo root map to "(root)".
func Packages(paths []string) []string {
	set := make(map[string]bool)
	for _, p := range paths {
		p = filepath.ToSlash(p)
		dir := filepath.ToSlash(filepath.Dir(p))
		if dir == "." || dir == "" {
			dir = "(root)"
		}
		set[dir] = true
	}
	out := make([]string, 0, len(set))
	for d := range set {
		out = append(out, d)
	}
	sort.Strings(out)
	return out
}
