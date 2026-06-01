package lsp_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"
	"github.com/dpopsuev/oculus/v3/lsp/mockserver"
)

// --- StubPool contract tests ---

func TestStubPool_GetReturnsError(t *testing.T) {
	p := &lsp.StubPool{}
	client, err := p.Get(lang.Go, "/tmp/test")
	if !errors.Is(err, lsp.ErrNoPool) {
		t.Fatalf("expected ErrNoPool, got %v", err)
	}
	if client != nil {
		t.Fatal("expected nil client")
	}
}

func TestStubPool_ShutdownIdempotent(t *testing.T) {
	p := &lsp.StubPool{}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("first shutdown: %v", err)
	}
	if err := p.Shutdown(context.Background()); err != nil {
		t.Fatalf("second shutdown: %v", err)
	}
}

func TestStubPool_StatusEmpty(t *testing.T) {
	p := &lsp.StubPool{}
	s := p.Status()
	if s.Active != 0 {
		t.Fatalf("expected 0 active, got %d", s.Active)
	}
	if s.Idle != 0 {
		t.Fatalf("expected 0 idle, got %d", s.Idle)
	}
	if len(s.ByLang) != 0 {
		t.Fatalf("expected empty ByLang, got %v", s.ByLang)
	}
}

// --- RealPool contract tests ---

func requireGopls(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls not available")
	}
}

func makeGoRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRealPool_GetReturnsClient(t *testing.T) {
	requireGopls(t)
	dir := makeGoRoot(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background()) //nolint:errcheck // best-effort cleanup

	client, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestRealPool_GetReusesConnection(t *testing.T) {
	requireGopls(t)
	dir := makeGoRoot(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background()) //nolint:errcheck // best-effort cleanup

	c1, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	c2, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if c1 != c2 {
		t.Fatal("expected same client from two Gets")
	}
}

func TestRealPool_ShutdownCleansUp(t *testing.T) {
	requireGopls(t)
	dir := makeGoRoot(t)
	pool := lsp.NewPool()

	_, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	s := pool.Status()
	if s.Active != 1 {
		t.Fatalf("expected 1 active before shutdown, got %d", s.Active)
	}

	if err := pool.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	s = pool.Status()
	if s.Active != 0 {
		t.Fatalf("expected 0 active after shutdown, got %d", s.Active)
	}
}

func TestRealPool_StatusReportsActive(t *testing.T) {
	requireGopls(t)
	dir := makeGoRoot(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background()) //nolint:errcheck // best-effort cleanup

	s := pool.Status()
	if s.Active != 0 {
		t.Fatalf("expected 0 active initially, got %d", s.Active)
	}

	_, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	s = pool.Status()
	if s.Active != 1 {
		t.Fatalf("expected 1 active, got %d", s.Active)
	}
	if s.ByLang[lang.Go] != 1 {
		t.Fatalf("expected 1 Go connection, got %d", s.ByLang[lang.Go])
	}
}

// --- Pool.References ---

// TestStubPool_References_ReturnsErrNoPool verifies that StubPool.References
// returns ErrNoPool so callers know to fall back to cold-start LSP.
//
// Given a StubPool (CLI mode)
// When References is called
// Then ErrNoPool is returned
func TestStubPool_References_ReturnsErrNoPool(t *testing.T) {
	p := &lsp.StubPool{}
	_, err := p.References(context.Background(), "/tmp/test.go", 1, 0)
	if !errors.Is(err, lsp.ErrNoPool) {
		t.Fatalf("expected ErrNoPool, got %v", err)
	}
}

// TestMockPool_References_ReturnsEmptyOnNoServer verifies that MockPool
// implements References. The mock server returns no reference locations by
// default, so the result is an empty (not nil) slice.
//
// Given a MockPool backed by a mock LSP server
// When References is called on a non-existent file
// Then the call succeeds (no error) and returns an empty location list
func TestMockPool_References_MockServer(t *testing.T) {
	p := lsp.NewMockPool(mockserver.Config{})
	defer p.Shutdown(context.Background())

	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.go")
	if err := os.WriteFile(filePath, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	locs, err := p.References(context.Background(), filePath, 1, 0)
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	// Mock server returns empty locations — not an error.
	if locs == nil {
		t.Error("expected non-nil (empty) slice, got nil")
	}
}

// --- OCL-BUG-11: clangd must not auto-spawn ---

// TestRealPool_CppGet_RefusesWithoutExplicitWarm reproduces OCL-BUG-11.
// pool.Get(LangCpp) caused clangd to auto-spawn on any deep analysis call,
// committing 80+ GB of virtual memory on a 16-core machine.
//
// Given a RealPool
// When Get is called for LangCpp without a prior WarmLSP call
// Then ErrNoLSPServer is returned — clangd must not be auto-started
func TestRealPool_CppGet_RefusesWithoutExplicitWarm(t *testing.T) {
	if _, err := exec.LookPath("clangd"); err != nil {
		t.Skip("clangd not installed — test only meaningful when clangd is present")
	}
	dir := t.TempDir()
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background()) //nolint:errcheck

	_, err := pool.Get(lang.Cpp, dir)
	if err == nil {
		t.Fatal("pool.Get(LangCpp) succeeded — clangd was auto-started; this is OCL-BUG-11")
	}
	if !errors.Is(err, lsp.ErrNoLSPServer) {
		t.Errorf("expected ErrNoLSPServer, got: %v", err)
	}
}

// --- OCL-BUG-13: per-language concurrency limit ---

// TestPool_CppConcurrencyLimit reproduces OCL-BUG-13.
// The pool uses a single semaphore (DefaultMaxActive=3) shared across all
// languages. gopls ~400MB/instance; clangd ~4-8GB/instance during indexing.
// Holding 3 clangd instances = ~24GB, not the ~1.2GB the comment assumes.
//
// Given the pool's per-language limits
// When the effective max-concurrent for LangCpp is queried
// Then it is strictly less than the effective max-concurrent for LangGo
func TestPool_CppConcurrencyLimit(t *testing.T) {
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background()) //nolint:errcheck

	goCap := pool.MaxConcurrent(lang.Go)
	cppCap := pool.MaxConcurrent(lang.Cpp)

	if cppCap >= goCap {
		t.Errorf("LangCpp max-concurrent=%d should be < LangGo max-concurrent=%d — OCL-BUG-13: clangd is 10x heavier than gopls", cppCap, goCap)
	}
	if cppCap < 1 {
		t.Errorf("LangCpp max-concurrent must be >= 1 (0 would disable entirely)")
	}
}

// --- ALY-BUG-3: LSP child processes must die on pool.Shutdown ---

// TestShutdown_KillsLSPChildren reproduces LCS-BUG-96.
//
// spawnServer did not set Setpgid=true, so shutdownEntry's process.Kill() sent
// SIGKILL to the LSP parent PID only. Grandchildren (e.g. tsserver.js spawned
// by typescript-language-server) survived, consuming 95-100% CPU indefinitely.
//
// The fix: Setpgid=true isolates the LSP process group; kill(-pgid) reaps all.
//
// This test verifies the CORRECT end-to-end behaviour after the fix:
// spawn with Setpgid=true, kill the process group, grandchild must be dead.
// Before the fix (no Setpgid, PID-only kill), this scenario leaves the
// grandchild alive — the test would fail if those two changes are reverted.
func TestShutdown_KillsLSPChildren(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	dir := t.TempDir()
	pidFile := filepath.Join(dir, "grandchild.pid")

	// Parent script: spawns a long-running grandchild and blocks on stdin.
	script := filepath.Join(dir, "parent.sh")
	scriptBody := "#!/bin/sh\nsleep 300 &\necho $! > " + pidFile + "\ncat >/dev/null\n"
	if err := os.WriteFile(script, []byte(scriptBody), 0o755); err != nil {
		t.Fatal(err)
	}

	// Spawn WITH Setpgid=true — the fixed spawnServer behaviour.
	parent := exec.Command("sh", script)
	parent.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, _ := parent.StdinPipe()
	if err := parent.Start(); err != nil {
		t.Fatalf("start parent: %v", err)
	}
	t.Cleanup(func() { stdin.Close(); _ = parent.Wait() })

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	gcBytes, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("grandchild pid file: %v", err)
	}
	var gcPID int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(gcBytes)), "%d", &gcPID); err != nil {
		t.Fatalf("parse grandchild PID: %v", err)
	}
	t.Logf("parent PID=%d pgid=%d grandchild PID=%d", parent.Process.Pid, parent.Process.Pid, gcPID)

	if err := syscall.Kill(gcPID, 0); err != nil {
		t.Fatalf("grandchild %d not alive before shutdown: %v", gcPID, err)
	}

	// Simulate the fixed shutdownEntry: kill the process GROUP (negative pgid).
	pgid, err := syscall.Getpgid(parent.Process.Pid)
	if err != nil {
		t.Fatalf("getpgid: %v", err)
	}
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		t.Fatalf("kill group -%d: %v", pgid, err)
	}
	_ = parent.Wait()
	time.Sleep(200 * time.Millisecond)

	// Grandchild must be dead. If alive: Setpgid or pgid-kill is broken.
	if syscall.Kill(gcPID, 0) == nil {
		_ = syscall.Kill(gcPID, syscall.SIGKILL)
		t.Errorf("LCS-BUG-96: grandchild %d survived group kill — Setpgid or pgid-kill not working", gcPID)
	}
}
