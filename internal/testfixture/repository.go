package testfixture

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// Repository copies a committed synthetic fixture into an isolated Git repository.
func Repository(t testing.TB, name string) string {
	t.Helper()
	if name == "" || filepath.Base(name) != name {
		t.Fatalf("invalid fixture name %q", name)
	}
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test fixture directory")
	}
	source := filepath.Join(filepath.Dir(sourceFile), "..", "..", "testdata", "testkit", name)
	destination := filepath.Join(t.TempDir(), name)
	if err := copyTree(source, destination); err != nil {
		t.Fatalf("copy synthetic %s fixture: %v", name, err)
	}
	if err := initGit(destination); err != nil {
		t.Fatalf("initialize synthetic %s fixture: %v", name, err)
	}
	return destination
}

func copyTree(source, destination string) error {
	const maxFiles = 1_000
	const maxBytes = 32 << 20
	files := 0
	bytes := int64(0)
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("fixture symlink is not allowed: %s", relative)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		bytes += info.Size()
		if files > maxFiles || bytes > maxBytes {
			return fmt.Errorf("fixture exceeds %d files or %d bytes", maxFiles, maxBytes)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target, content, 0o600); err != nil {
			return err
		}
		return nil
	})
}

func initGit(root string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	environment := append(os.Environ(),
		"GIT_AUTHOR_NAME=fixture",
		"GIT_AUTHOR_EMAIL=fixture@example.invalid",
		"GIT_COMMITTER_NAME=fixture",
		"GIT_COMMITTER_EMAIL=fixture@example.invalid",
	)
	for _, arguments := range [][]string{
		{"init", "--quiet"},
		{"add", "--all"},
		{"commit", "--quiet", "--message", "fixture"},
	} {
		command := exec.CommandContext(ctx, "git", arguments...)
		command.Dir = root
		command.Env = environment
		if output, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %s: %w", arguments[0], output, err)
		}
	}
	return nil
}
