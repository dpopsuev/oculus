package lsp

import (
	"context"
	"errors"
	"log/slog"

	"github.com/dpopsuev/oculus/v3/lang"
)

// ErrNoPool is returned when no pool is available (CLI mode).
var ErrNoPool = errors.New("lsp: no connection pool available")

// ErrPoolShutDown is returned when Get is called on a shut-down pool.
var ErrPoolShutDown = errors.New("lsp pool: shut down")

// ErrNoLSPServer is returned when no LSP server is configured for a language.
var ErrNoLSPServer = errors.New("lsp pool: no server for language")

// Pool manages reusable LSP server connections. In long-running mode
// (locus serve), connections are kept alive across requests. In CLI mode,
// pool is nil and analyzers fall back to cold-start per request.
type Pool interface {
	// Get returns a warm LSP client for the given language and workspace root.
	// If no connection exists, one is lazily started. Returns ErrNoPool from
	// StubPool, or a spawn error if the LSP server can't be started.
	Get(language lang.Language, root string) (*Client, error)

	// Release signals that the caller is done with the connection. The pool
	// keeps it alive for future callers. Does not close the connection.
	Release(language lang.Language, root string)

	// Shutdown gracefully stops all managed LSP servers. Sends LSP shutdown
	// and exit notifications, then kills processes.
	Shutdown(ctx context.Context) error

	// Status returns the current pool state for health reporting.
	Status() PoolStatus
}

// InitializeParams builds the canonical LSP initialize request params.
// All callers (RealPool, MockPool, ContainerPool, lspConn) must use this
// to avoid capability drift across initialize paths.
func InitializeParams(rootURI string) map[string]any {
	return map[string]any{
		"processId": nil,
		"rootUri":   rootURI,
		"workspaceFolders": []map[string]any{
			{"uri": rootURI, "name": "root"},
		},
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"documentSymbol": map[string]any{"hierarchicalDocumentSymbolSupport": true},
				"typeHierarchy":  map[string]any{},
				"callHierarchy":  map[string]any{},
				"implementation": map[string]any{},
				"hover":          map[string]any{},
			},
			"workspace": map[string]any{
				"symbol": map[string]any{"dynamicRegistration": false},
			},
		},
	}
}

// Initialize performs the LSP initialize/initialized handshake using
// canonical params. Returns the server capabilities response.
func Initialize(client *Client, root string) error {
	rootURI := "file://" + root
	initResult, err := client.Request("initialize", InitializeParams(rootURI))
	if err != nil {
		slog.Error("lsp: initialize failed", "root", root, "error", err)
		return err
	}
	slog.Info("lsp: initialized", "root", root, "response_bytes", len(initResult))
	return client.Notify("initialized", struct{}{})
}

// PoolStatus reports the current state of the connection pool.
type PoolStatus struct {
	Active int                   `json:"active"`
	Idle   int                   `json:"idle"`
	ByLang map[lang.Language]int `json:"by_language"`
}
