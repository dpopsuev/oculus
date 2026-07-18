package lsp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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
	client   *Client
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	dead     atomic.Bool
	lastUsed time.Time
	encoding OffsetEncoding // negotiated position encoding
}

// DefaultTTL is how long an idle gopls stays alive before eviction.
const DefaultTTL = 30 * time.Minute

// DefaultMaxActive is the maximum number of concurrent gopls instances.
// Each gopls ~400MB, so 3 = ~1.2GB max LSP memory.
const DefaultMaxActive = 3

// ErrPoolAtCapacity is returned when all LSP slots are occupied and
// the wait timeout expires. Callers should fall back to non-LSP analyzers.
var ErrPoolAtCapacity = errors.New("lsp pool: at capacity")

// C and C++ servers (clangd) are 10× heavier than Go servers (gopls) during
// background indexing; cap them to 1 concurrent instance.
var defaultLangLimits = map[lang.Language]int{
	lang.C:   1,
	lang.Cpp: 1,
}

// PoolConfig holds optional overrides for pool tuning.
// Zero values fall back to package defaults.
type PoolConfig struct {
	MaxActive  int
	TTL        time.Duration
	LangLimits map[lang.Language]int
}

// RealPool manages reusable LSP server connections keyed by (language, root).
// Thread-safe via sync.Mutex. Concurrency bounded by semaphore.
type RealPool struct {
	mu         sync.Mutex
	conns      map[poolKey]*poolEntry
	stopped    bool
	ttl        time.Duration
	done       chan struct{}
	sem        chan struct{} // concurrency semaphore (pool-wide)
	langLimits map[lang.Language]int
	langActive map[lang.Language]int
}

// NewPool creates a new connection pool for long-running (serve) mode.
func NewPool() *RealPool {
	return NewPoolWithConfig(PoolConfig{})
}

// NewPoolWithConfig creates a connection pool with explicit overrides.
// Zero-valued fields fall back to package defaults.
func NewPoolWithConfig(cfg PoolConfig) *RealPool {
	maxActive := cfg.MaxActive
	if maxActive == 0 {
		maxActive = DefaultMaxActive
	}
	ttl := cfg.TTL
	if ttl == 0 {
		ttl = DefaultTTL
	}

	src := cfg.LangLimits
	if src == nil {
		src = defaultLangLimits
	}
	limits := make(map[lang.Language]int, len(src))
	maps.Copy(limits, src)

	p := &RealPool{
		conns:      make(map[poolKey]*poolEntry),
		ttl:        ttl,
		done:       make(chan struct{}),
		sem:        make(chan struct{}, maxActive),
		langLimits: limits,
		langActive: make(map[lang.Language]int),
	}
	go p.reapLoop()
	return p
}

func (p *RealPool) MaxConcurrent(language lang.Language) int {
	if limit, ok := p.langLimits[language]; ok {
		return limit
	}
	return DefaultMaxActive
}

// reapLoop periodically evicts idle entries past TTL.
func (p *RealPool) reapLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.reapIdle()
		}
	}
}

// ReapIdle evicts idle or dead entries and returns the number evicted.
func (p *RealPool) ReapIdle() int {
	p.mu.Lock()
	defer p.mu.Unlock()

	count := 0
	now := time.Now()
	for key, entry := range p.conns {
		if entry.dead.Load() || now.Sub(entry.lastUsed) > p.ttl {
			slog.Info("lsp pool: evicting idle server", slog.String("root", key.root), slog.Duration("idle", now.Sub(entry.lastUsed)))
			shutdownEntry(entry)
			delete(p.conns, key)
			select {
			case <-p.sem:
			default:
			}
			if p.langActive[key.lang] > 0 {
				p.langActive[key.lang]--
			}
			count++
		}
	}
	return count
}

func (p *RealPool) reapIdle() { p.ReapIdle() }

// PIDs returns the OS process IDs of all live LSP server processes.
func (p *RealPool) PIDs() []int {
	p.mu.Lock()
	defer p.mu.Unlock()

	var pids []int
	for _, entry := range p.conns {
		if !entry.dead.Load() && entry.cmd.Process != nil {
			pids = append(pids, entry.cmd.Process.Pid)
		}
	}
	return pids
}

// Get returns a warm LSP client for the given language and workspace root.
// If no connection exists, one is lazily spawned. When the pool is at
// MaxActive, the least-recently-used idle server is evicted so a new root
// can be admitted. Falls back to a 10s wait, then ErrPoolAtCapacity.
func (p *RealPool) Get(language lang.Language, root string) (*Client, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	key := poolKey{lang: language, root: absRoot}

	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return nil, ErrPoolShutDown
	}

	if entry, ok := p.conns[key]; ok {
		if entry.dead.Load() {
			slog.Warn("lsp pool: evicting dead server", slog.String("root", absRoot))
			delete(p.conns, key)
			// Release the semaphore slot held by the dead entry.
			select {
			case <-p.sem:
			default:
			}
		} else {
			entry.lastUsed = time.Now()
			p.mu.Unlock()
			return entry.client, nil
		}
	}
	if limit, ok := p.langLimits[language]; ok && p.langActive[language] >= limit {
		p.mu.Unlock()
		return nil, fmt.Errorf("lsp pool: %v: per-language concurrency limit (%d) reached", language, limit)
	}
	p.langActive[language]++
	p.mu.Unlock()

	if !p.acquireSlot() {
		p.mu.Lock()
		p.langActive[language]--
		p.mu.Unlock()
		return nil, ErrPoolAtCapacity
	}

	entry, err := spawnServer(language, absRoot)
	if err != nil {
		<-p.sem
		p.mu.Lock()
		p.langActive[language]--
		p.mu.Unlock()
		return nil, err
	}

	p.mu.Lock()
	p.conns[key] = entry
	p.mu.Unlock()
	return entry.client, nil
}

// acquireSlot obtains one pool-wide semaphore token. On capacity it evicts
// the LRU connection once, then waits up to 10s before failing.
func (p *RealPool) acquireSlot() bool {
	select {
	case p.sem <- struct{}{}:
		return true
	default:
	}
	if p.evictLRU() > 0 {
		select {
		case p.sem <- struct{}{}:
			return true
		default:
		}
	}
	select {
	case p.sem <- struct{}{}:
		return true
	case <-time.After(10 * time.Second):
		return false
	}
}

// evictLRU removes the least-recently-used live connection and frees its
// semaphore slot. Returns 1 if an entry was evicted, 0 otherwise.
func (p *RealPool) evictLRU() int {
	p.mu.Lock()
	var (
		oldestKey   poolKey
		oldestEntry *poolEntry
		oldestTime  time.Time
		found       bool
	)
	for key, entry := range p.conns {
		if entry.dead.Load() {
			delete(p.conns, key)
			select {
			case <-p.sem:
			default:
			}
			if p.langActive[key.lang] > 0 {
				p.langActive[key.lang]--
			}
			found = true
			oldestEntry = nil
			break
		}
		if !found || entry.lastUsed.Before(oldestTime) {
			found = true
			oldestKey = key
			oldestEntry = entry
			oldestTime = entry.lastUsed
		}
	}
	if oldestEntry == nil {
		p.mu.Unlock()
		if found {
			return 1 // cleaned a dead entry
		}
		return 0
	}
	delete(p.conns, oldestKey)
	if p.langActive[oldestKey.lang] > 0 {
		p.langActive[oldestKey.lang]--
	}
	root := oldestKey.root
	idle := time.Since(oldestTime)
	p.mu.Unlock()

	slog.Info("lsp pool: admit-time LRU eviction",
		slog.String("root", root),
		slog.Duration("idle", idle))
	shutdownEntry(oldestEntry)
	select {
	case <-p.sem:
	default:
	}
	return 1
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
	select {
	case <-p.done:
	default:
		close(p.done)
	}
	for key, entry := range p.conns {
		shutdownEntry(entry)
		delete(p.conns, key)
		select {
		case <-p.sem:
		default:
		}
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

// References implements Pool.References for RealPool using poolReferences.
func (p *RealPool) References(ctx context.Context, file string, line, char int) ([]Location, error) {
	return poolReferences(ctx, p, file, line, char)
}

// Definition implements Pool.Definition for RealPool using poolDefinition.
func (p *RealPool) Definition(ctx context.Context, file string, line, char int) ([]Location, error) {
	return poolDefinition(ctx, p, file, line, char)
}

// DocumentSymbols implements Pool.DocumentSymbols for RealPool.
func (p *RealPool) DocumentSymbols(ctx context.Context, file string) ([]DocSymbol, error) {
	return poolDocumentSymbols(ctx, p, file)
}

// PrepareRename implements Pool.PrepareRename for RealPool.
func (p *RealPool) PrepareRename(ctx context.Context, file string, line, char int) (*PrepareResult, error) {
	return poolPrepareRename(ctx, p, file, line, char)
}

// Rename implements Pool.Rename for RealPool.
func (p *RealPool) Rename(ctx context.Context, file string, line, char int, newName string) (*WorkspaceEdit, error) {
	return poolRename(ctx, p, file, line, char, newName)
}

// spawnServer starts a new LSP server process and performs the initialize handshake.
// It looks up the registry entry for language, checks root markers (unless the
// server has SingleFileSupport), then spawns and initializes the process.
func spawnServer(language lang.Language, absRoot string) (*poolEntry, error) {
	regEntry := lang.DefaultServerEntry(language)
	if regEntry == nil {
		return nil, fmt.Errorf("%w: %v", ErrNoLSPServer, language)
	}

	if regEntry.SkipAutoStart {
		return nil, fmt.Errorf("%w: %v (requires manual configuration)", ErrNoLSPServer, language)
	}

	// Require root markers for servers that need a project root, unless the
	// server explicitly supports single-file mode.
	if !regEntry.SingleFileSupport && len(regEntry.RootMarkers) > 0 {
		if !lang.HasRootMarkers(absRoot, regEntry.RootMarkers) {
			return nil, fmt.Errorf("lsp pool: %v: no root markers found in %s",
				language, absRoot)
		}
	}

	bin, err := exec.LookPath(regEntry.Command)
	if err != nil {
		return nil, fmt.Errorf("lsp pool: server %s not found: %w", regEntry.Command, err)
	}

	cmd := exec.Command(bin, regEntry.Args...)
	cmd.Dir = absRoot
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp pool: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp pool: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp pool: start %s: %w", regEntry.Command, err)
	}

	client := NewClient(stdout, stdin)

	initOpts := mergeInitOptions(regEntry.InitOptions, language)
	enc, err := initialize(client, absRoot, initOpts)
	if err != nil {
		stdin.Close()
		_ = cmd.Wait()
		return nil, fmt.Errorf("lsp pool: initialize: %w", err)
	}

	// Open root marker files so servers like gopls establish workspace views.
	OpenRootMarkers(client, absRoot, regEntry.RootMarkers)

	entry := &poolEntry{
		client:   client,
		cmd:      cmd,
		stdin:    stdin,
		lastUsed: time.Now(),
		encoding: enc,
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

// initialize performs the LSP initialize/initialized handshake and returns
// the negotiated OffsetEncoding.
func initialize(client *Client, root string, initOptions ...map[string]any) (OffsetEncoding, error) {
	return Initialize(client, root, initOptions...)
}

// mergeInitOptions copies registry InitOptions and injects TypeScript
// tsserver.path from LOCUS_TSSERVER_PATH or a discovered global install so
// foreign clones without node_modules/typescript still initialize.
func mergeInitOptions(base map[string]any, language lang.Language) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	if language != lang.TypeScript && language != lang.JavaScript {
		if len(out) == 0 {
			return nil
		}
		return out
	}
	tsPath := resolveTSServerPath()
	if tsPath == "" {
		if len(out) == 0 {
			return nil
		}
		return out
	}
	tsserver, _ := out["tsserver"].(map[string]any)
	if tsserver == nil {
		tsserver = map[string]any{}
	}
	if _, ok := tsserver["path"]; !ok {
		tsserver["path"] = tsPath
	}
	out["tsserver"] = tsserver
	return out
}

// ResolveTSServerPath returns a tsserver.js path for typescript-language-server.
// Order: LOCUS_TSSERVER_PATH → npm root -g → common install locations.
func ResolveTSServerPath() string {
	return resolveTSServerPath()
}

// resolveTSServerPath returns a tsserver.js path for typescript-language-server.
// Order: LOCUS_TSSERVER_PATH → npm root -g → common install locations.
func resolveTSServerPath() string {
	if p := os.Getenv("LOCUS_TSSERVER_PATH"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if out, err := exec.Command("npm", "root", "-g").Output(); err == nil {
		candidate := filepath.Join(strings.TrimSpace(string(out)), "typescript", "lib", "tsserver.js")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	for _, c := range []string{
		"/usr/local/lib/node_modules/typescript/lib/tsserver.js",
		"/usr/lib/node_modules/typescript/lib/tsserver.js",
	} {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// Warm pre-warms the gopls index for a workspace root by sending
// textDocument/didOpen for Go files. Call explicitly via MCP action —
// not called automatically on spawn to avoid OOM on large workspaces.
func (p *RealPool) Warm(language lang.Language, root string) error {
	client, err := p.Get(language, root)
	if err != nil {
		return err
	}
	absRoot, _ := filepath.Abs(root)
	prewarm(client, absRoot)
	return nil
}

func prewarm(client *Client, root string) {
	var files []string
	filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() && (name == "vendor" || name == "node_modules" || name == ".git" || name == "testdata") {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			files = append(files, path)
		}
		return nil
	})

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		uri := "file://" + f
		_ = client.Notify("textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{
				"uri":        uri,
				"languageId": "go",
				"version":    1,
				"text":       string(content),
			},
		})
	}

	slog.Info("lsp pool: pre-warmed", slog.String("root", root), slog.Int("files", len(files)))
}

// shutdownEntry sends LSP shutdown+exit and cleans up process resources.
// If the server doesn't exit within 3 seconds it kills the process group so
// grandchildren (e.g. tsserver) are reaped along with the parent.
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
				pgid, err := syscall.Getpgid(entry.cmd.Process.Pid)
				if err == nil {
					_ = syscall.Kill(-pgid, syscall.SIGKILL)
				} else {
					_ = entry.cmd.Process.Kill()
				}
			}
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
}
