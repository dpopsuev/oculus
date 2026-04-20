package context_test

import (
	"testing"

	oculusctx "github.com/dpopsuev/oculus/v3/context"
	"github.com/dpopsuev/oculus/v3/testkit"
)

func TestReadContext_Empty(t *testing.T) {
	dir := t.TempDir()

	// Initialize a git repo so ResolveHEAD works.
	repoDir := t.TempDir()
	if err := testkit.BuildFixture(repoDir, map[string]string{"dummy.txt": "x"}); err != nil {
		t.Fatal(err)
	}
	if err := testkit.InitGitRepo(repoDir); err != nil {
		t.Fatal(err)
	}

	s := oculusctx.New(dir)
	entry, err := s.Read(repoDir, oculusctx.ScopeProject, "main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry != nil {
		t.Errorf("expected nil for empty context, got %+v", entry)
	}
}

func TestWriteContext_Creates(t *testing.T) {
	dir := t.TempDir()

	repoDir := t.TempDir()
	if err := testkit.BuildFixture(repoDir, map[string]string{"dummy.txt": "x"}); err != nil {
		t.Fatal(err)
	}
	if err := testkit.InitGitRepo(repoDir); err != nil {
		t.Fatal(err)
	}

	s := oculusctx.New(dir)
	err := s.Write(repoDir, oculusctx.ScopeProject, "main", "abc123", "This is project context.")
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	entry, err := s.Read(repoDir, oculusctx.ScopeProject, "main")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry after Write")
	}
}

func TestReadContext_Content(t *testing.T) {
	dir := t.TempDir()

	repoDir := t.TempDir()
	if err := testkit.BuildFixture(repoDir, map[string]string{"dummy.txt": "x"}); err != nil {
		t.Fatal(err)
	}
	if err := testkit.InitGitRepo(repoDir); err != nil {
		t.Fatal(err)
	}

	s := oculusctx.New(dir)
	content := "Architecture notes for the project."
	sha := "deadbeef1234"
	err := s.Write(repoDir, oculusctx.ScopeFile, "src/main.go", sha, content)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	entry, err := s.Read(repoDir, oculusctx.ScopeFile, "src/main.go")
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Content != content {
		t.Errorf("Content = %q, want %q", entry.Content, content)
	}
	if entry.SHA != sha {
		t.Errorf("SHA = %q, want %q", entry.SHA, sha)
	}
	if entry.Scope != oculusctx.ScopeFile {
		t.Errorf("Scope = %q, want %q", entry.Scope, oculusctx.ScopeFile)
	}
	if entry.Target != "src/main.go" {
		t.Errorf("Target = %q, want %q", entry.Target, "src/main.go")
	}
}
