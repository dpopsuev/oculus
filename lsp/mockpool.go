package lsp

import (
	"context"
	"io"

	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp/mockserver"
)

// MockPool implements Pool backed by mock LSP servers.
// Each Get() spawns a mock server with the given config in a goroutine.
type MockPool struct {
	cfg   mockserver.Config
	conns map[poolKey]*mockEntry
}

type mockEntry struct {
	client *Client
	sw     io.WriteCloser
	cw     io.WriteCloser
}

// NewMockPool creates a pool backed by mock LSP servers.
func NewMockPool(cfg mockserver.Config) *MockPool {
	return &MockPool{
		cfg:   cfg,
		conns: make(map[poolKey]*mockEntry),
	}
}

func (p *MockPool) Get(language lang.Language, root string) (*Client, error) {
	key := poolKey{lang: language, root: root}
	if entry, ok := p.conns[key]; ok {
		return entry.client, nil
	}

	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	go func() {
		mockserver.Serve(sr, sw, p.cfg)
		sw.Close()
	}()

	client := NewClient(cr, cw)
	// Initialize
	if _, err := client.Request("initialize", map[string]any{
		"processId": nil,
		"rootUri":   "file://" + root,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"callHierarchy": map[string]any{},
			},
		},
	}); err != nil {
		cw.Close()
		return nil, err
	}
	_ = client.Notify("initialized", struct{}{})

	p.conns[key] = &mockEntry{client: client, sw: sw, cw: cw}
	return client, nil
}

func (p *MockPool) Release(lang.Language, string) {}

func (p *MockPool) Shutdown(_ context.Context) error {
	for key, entry := range p.conns {
		entry.client.Request("shutdown", nil)
		entry.client.Notify("exit", nil)
		entry.cw.Close()
		delete(p.conns, key)
	}
	return nil
}

func (p *MockPool) Status() PoolStatus {
	byLang := make(map[lang.Language]int)
	for key := range p.conns {
		byLang[key.lang]++
	}
	return PoolStatus{Active: len(p.conns), ByLang: byLang}
}
