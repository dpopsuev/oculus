package oculus_test

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFixturesAreSyntheticAndRepositoryLocal(t *testing.T) {
	const maxTestFiles = 5_000
	const maxTestFileBytes = 2 << 20
	_, policyFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve fixture policy path")
	}
	root := filepath.Dir(policyFile)
	forbidden := []string{
		"/" + "home/",
		"/" + "Users/",
		"os.Getenv(" + `"HOME"` + ")",
		"os.UserHomeDir()",
		"oculus" + "Root(",
		"scan" + "Locus(",
		"filepath.Abs(" + `".."` + ")",
		"filepath.Abs(" + `"../.."` + ")",
		"root := " + `".."`,
		"root := " + `"../.."`,
	}

	testFiles := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && (entry.Name() == ".git" || entry.Name() == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") || path == policyFile {
			return nil
		}
		testFiles++
		if testFiles > maxTestFiles {
			return fmt.Errorf("test file count exceeds %d", maxTestFiles)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Size() > maxTestFileBytes {
			return fmt.Errorf("test file %s exceeds %d bytes", path, maxTestFileBytes)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, marker := range forbidden {
			if strings.Contains(string(content), marker) {
				rel, relErr := filepath.Rel(root, path)
				if relErr != nil {
					rel = path
				}
				t.Errorf("%s contains forbidden host-fixture marker %q", filepath.ToSlash(rel), marker)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan test files: %v", err)
	}
}
