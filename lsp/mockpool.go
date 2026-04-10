package lsp

import (
	"context"
	"io"
	"sync"

	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp/mockserver"
)

// MockPool implements Pool backed by mock LSP servers.
// Thread-safe. Each Get() spawns a mock server goroutine.
type MockPool struct {
	cfg   mockserver.Config
	mu    sync.Mutex
	conns map[poolKey]*mockEntry
}

type mockEntry struct {
	client *Client
	sr     io.Closer // server's read pipe — close to stop server goroutine
	sw     io.Closer // server's write pipe
	cw     io.Closer // client's write pipe (stdin to server)
	cr     io.Closer // client's read pipe (stdout from server)
}

// NewMockPool creates a pool backed by mock LSP servers.
func NewMockPool(cfg mockserver.Config) *MockPool {
	return &MockPool{
		cfg:   cfg,
		conns: make(map[poolKey]*mockEntry),
	}
}

func (p *MockPool) Get(language lang.Language, root string) (*Client, error) {
	p.mu.Lock()
	key := poolKey{lang: language, root: root}
	if entry, ok := p.conns[key]; ok {
		p.mu.Unlock()
		return entry.client, nil
	}
	p.mu.Unlock()

	// Spawn outside the lock — initialize can be slow with latency.
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	go func() {
		mockserver.Serve(sr, sw, p.cfg)
		sw.Close()
	}()

	client := NewClient(cr, cw)
	if _, err := client.Request("initialize", map[string]any{
		"processId":  nil,
		"rootUri":    "file://" + root,
		"capabilities": map[string]any{},
	}); err != nil {
		cw.Close()
		sr.Close()
		return nil, err
	}
	_ = client.Notify("initialized", struct{}{})

	entry := &mockEntry{client: client, sr: sr, sw: sw, cw: cw, cr: cr}

	p.mu.Lock()
	// Double-check — another goroutine may have created the same key.
	if existing, ok := p.conns[key]; ok {
		p.mu.Unlock()
		// Clean up the duplicate we just created.
		cw.Close()
		sr.Close()
		return existing.client, nil
	}
	p.conns[key] = entry
	p.mu.Unlock()

	return client, nil
}

func (p *MockPool) Release(lang.Language, string) {}

func (p *MockPool) Shutdown(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, entry := range p.conns {
		// Close client write pipe — unblocks server read, server exits.
		entry.cw.Close()
		// Close server read pipe for good measure.
		entry.sr.Close()
		delete(p.conns, key)
	}
	return nil
}

func (p *MockPool) Status() PoolStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	byLang := make(map[lang.Language]int)
	for key := range p.conns {
		byLang[key.lang]++
	}
	return PoolStatus{Active: len(p.conns), ByLang: byLang}
}
