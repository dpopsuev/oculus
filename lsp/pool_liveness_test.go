package lsp_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"
)

func TestRealPool_DeadServerDetection(t *testing.T) {
	requireGopls(t)
	dir := makeGoRoot(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())

	client, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if err := pool.KillServer(lang.Go, dir); err != nil {
		t.Fatalf("KillServer: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	_, err = client.Request("workspace/symbol", map[string]string{"query": "main"})
	if err == nil {
		t.Fatal("expected error from dead server")
	}
	if !errors.Is(err, lsp.ErrServerDead) {
		t.Fatalf("expected ErrServerDead, got: %v", err)
	}
}

func TestRealPool_AutoRestart(t *testing.T) {
	requireGopls(t)
	dir := makeGoRoot(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())

	_, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}

	if err := pool.KillServer(lang.Go, dir); err != nil {
		t.Fatalf("KillServer: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	client2, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("second Get (should respawn): %v", err)
	}

	result, err := client2.Request("workspace/symbol", map[string]string{"query": ""})
	if err != nil {
		t.Fatalf("request on respawned server: %v", err)
	}
	if len(result) == 0 {
		t.Log("empty result (expected for minimal fixture)")
	}
}

func TestRealPool_ZombieReaping(t *testing.T) {
	requireGopls(t)
	dir := makeGoRoot(t)
	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())

	for i := range 3 {
		_, err := pool.Get(lang.Go, dir)
		if err != nil {
			t.Fatalf("Get %d: %v", i, err)
		}

		if err := pool.KillServer(lang.Go, dir); err != nil {
			t.Fatalf("KillServer %d: %v", i, err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	s := pool.Status()
	if s.Active > 0 {
		t.Logf("active connections after 3 kills: %d (dead entries not yet evicted — expected until next Get)", s.Active)
	}

	// Final Get should spawn fresh — no zombies blocking it
	client, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("final Get: %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestRealPool_ConcurrencyCap(t *testing.T) {
	requireGopls(t)

	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())

	// DefaultMaxActive is 3. Create 5 roots.
	var roots []string
	for i := range 5 {
		dir := t.TempDir()
		writeGoMod(t, dir, i)
		roots = append(roots, dir)
	}

	var wg sync.WaitGroup
	var succeeded atomic.Int32
	var atCapacity atomic.Int32

	for _, root := range roots {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			_, err := pool.Get(lang.Go, r)
			if err != nil {
				if errors.Is(err, lsp.ErrPoolAtCapacity) {
					atCapacity.Add(1)
					return
				}
				t.Errorf("Get(%s): %v", r, err)
				return
			}
			succeeded.Add(1)
		}(root)
	}

	wg.Wait()

	s := succeeded.Load()
	c := atCapacity.Load()
	t.Logf("succeeded=%d, at_capacity=%d", s, c)

	if s > 3 {
		t.Errorf("expected max 3 concurrent gopls (DefaultMaxActive), got %d", s)
	}
	if s+c != 5 {
		t.Errorf("expected 5 total attempts, got succeeded=%d + capacity=%d = %d", s, c, s+c)
	}
}

func writeGoMod(t *testing.T, dir string, i int) {
	t.Helper()
	content := fmt.Sprintf("module test%d\ngo 1.21\n", i)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRealPool_PreWarmFileCount(t *testing.T) {
	requireGopls(t)

	dir := t.TempDir()
	// Create a Go module with 5 source files
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module warmtest\ngo 1.21\n"), 0o644)
	for i := range 5 {
		content := fmt.Sprintf("package warmtest\n\nfunc Func%d() {}\n", i)
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.go", i)), []byte(content), 0o644)
	}

	pool := lsp.NewPool()
	defer pool.Shutdown(context.Background())

	// Get spawns and initializes (no prewarm)
	client, err := pool.Get(lang.Go, dir)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	// workspace/symbol should return symbols from all 5 files
	result, err := client.Request("workspace/symbol", map[string]string{"query": "Func"})
	if err != nil {
		t.Fatalf("workspace/symbol: %v", err)
	}
	if len(result) < 50 {
		t.Logf("workspace/symbol returned %d bytes (expected symbols from 5 files)", len(result))
	}

	// Now warm explicitly
	if err := pool.Warm(lang.Go, dir); err != nil {
		t.Fatalf("Warm: %v", err)
	}

	// After warm, same query should still work (and be faster, though we don't assert timing)
	result2, err := client.Request("workspace/symbol", map[string]string{"query": "Func"})
	if err != nil {
		t.Fatalf("workspace/symbol after warm: %v", err)
	}
	if len(result2) < len(result) {
		t.Errorf("expected at least %d bytes after warm, got %d", len(result), len(result2))
	}
}
