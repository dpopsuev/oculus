package lsp

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dpopsuev/oculus/v3/lang"
)

// poolKey uniquely identifies an LSP server connection by language and workspace root.
type poolKey struct {
	lang lang.Language
	root string
}

// poolEntry holds a live LSP server connection.
type poolEntry struct {
	client *Client
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	dead   atomic.Bool
}

// RealPool manages reusable LSP server connections keyed by (language, root).
// Thread-safe via sync.Mutex.
type RealPool struct {
	mu      sync.Mutex
	conns   map[poolKey]*poolEntry
	stopped bool
}

// NewPool creates a new connection pool for long-running (serve) mode.
func NewPool() *RealPool {
	return &RealPool{
		conns: make(map[poolKey]*poolEntry),
	}
}

// Get returns a warm LSP client for the given language and workspace root.
// If no connection exists, one is lazily spawned.
func (p *RealPool) Get(language lang.Language, root string) (*Client, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	key := poolKey{lang: language, root: absRoot}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return nil, ErrPoolShutDown
	}

	if entry, ok := p.conns[key]; ok {
		if entry.dead.Load() {
			slog.Warn("lsp pool: evicting dead server", slog.String("root", absRoot))
			delete(p.conns, key)
		} else {
			return entry.client, nil
		}
	}

	entry, err := spawnServer(language, absRoot)
	if err != nil {
		return nil, err
	}
	p.conns[key] = entry
	return entry.client, nil
}

// Release signals that the caller is done with the connection.
// The pool keeps it alive for future callers.
func (p *RealPool) Release(lang.Language, string) {
	// no-op: connection stays alive in pool
}

// Shutdown gracefully stops all managed LSP servers.
func (p *RealPool) Shutdown(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopped = true
	for key, entry := range p.conns {
		shutdownEntry(entry)
		delete(p.conns, key)
	}
	return nil
}

// Status returns the current pool state for health reporting.
func (p *RealPool) Status() PoolStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	byLang := make(map[lang.Language]int)
	for key := range p.conns {
		byLang[key.lang]++
	}
	return PoolStatus{
		Active: len(p.conns),
		Idle:   0,
		ByLang: byLang,
	}
}

// KillServer force-kills the LSP server for a given root. Test-only.
func (p *RealPool) KillServer(language lang.Language, root string) error {
	absRoot, _ := filepath.Abs(root)
	key := poolKey{lang: language, root: absRoot}

	p.mu.Lock()
	entry, ok := p.conns[key]
	p.mu.Unlock()

	if !ok {
		return fmt.Errorf("no server for %s", root)
	}
	if entry.cmd.Process != nil {
		return entry.cmd.Process.Kill()
	}
	return fmt.Errorf("no process")
}

// spawnServer starts a new LSP server process and performs the initialize handshake.
func spawnServer(language lang.Language, absRoot string) (*poolEntry, error) {
	cmdStr := lang.DefaultLSPServer(language)
	if cmdStr == "" {
		return nil, fmt.Errorf("%w: %v", ErrNoLSPServer, language)
	}

	parts := strings.Fields(cmdStr)
	bin, err := exec.LookPath(parts[0])
	if err != nil {
		return nil, fmt.Errorf("lsp pool: server %s not found: %w", parts[0], err)
	}

	cmd := exec.Command(bin, parts[1:]...)
	cmd.Dir = absRoot
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp pool: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp pool: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp pool: start %s: %w", parts[0], err)
	}

	client := NewClient(stdout, stdin)

	if err := initialize(client, absRoot); err != nil {
		stdin.Close()
		_ = cmd.Wait()
		return nil, fmt.Errorf("lsp pool: initialize: %w", err)
	}

	entry := &poolEntry{
		client: client,
		cmd:    cmd,
		stdin:  stdin,
	}

	go func() {
		err := cmd.Wait()
		entry.dead.Store(true)
		if err != nil {
			slog.Warn("lsp pool: server exited unexpectedly", slog.String("root", absRoot), slog.Any("error", err))
		}
	}()

	return entry, nil
}

// initialize performs the LSP initialize/initialized handshake.
func initialize(client *Client, root string) error {
	return Initialize(client, root)
}

// shutdownEntry sends LSP shutdown+exit and cleans up process resources.
// If the server doesn't exit within 3 seconds, it is force-killed to
// prevent orphaned processes on the host.
func shutdownEntry(entry *poolEntry) {
	if entry.dead.Load() {
		entry.stdin.Close()
		return
	}
	_, _ = entry.client.Request("shutdown", nil)
	_ = entry.client.Notify("exit", nil)
	entry.stdin.Close()

	deadline := time.After(3 * time.Second)
	for !entry.dead.Load() {
		select {
		case <-deadline:
			if entry.cmd.Process != nil {
				_ = entry.cmd.Process.Kill()
			}
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}
