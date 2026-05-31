package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/dpopsuev/oculus/v3/lang"
)

// ErrNoPool is returned when no pool is available (CLI mode).
var ErrNoPool = errors.New("lsp: no connection pool available")

// ErrPoolShutDown is returned when Get is called on a shut-down pool.
var ErrPoolShutDown = errors.New("lsp pool: shut down")

// ErrNoLSPServer is returned when no LSP server is configured for a language.
var ErrNoLSPServer = errors.New("lsp pool: no server for language")

// OffsetEncoding represents the character offset encoding negotiated with the
// LSP server. The default is UTF-16 as per the LSP spec.
type OffsetEncoding int

const (
	// UTF16 is the default LSP offset encoding (UTF-16 code units).
	UTF16 OffsetEncoding = iota
	// UTF8 uses byte offsets (advertised as "utf-8" in positionEncoding).
	UTF8
	// UTF32 uses Unicode codepoint offsets (advertised as "utf-32").
	UTF32
)

// initializeResult is the minimal subset of LSP InitializeResult we parse.
type initializeResult struct {
	Capabilities struct {
		PositionEncoding string `json:"positionEncoding,omitempty"`
	} `json:"capabilities"`
	// clangd and older servers put offsetEncoding at the top level.
	OffsetEncoding string `json:"offsetEncoding,omitempty"`
}

// parseOffsetEncoding extracts the negotiated encoding from an initialize
// response. Falls back to UTF16 for unknown or missing values.
func parseOffsetEncoding(raw []byte) OffsetEncoding {
	var result initializeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return UTF16
	}
	enc := result.Capabilities.PositionEncoding
	if enc == "" {
		enc = result.OffsetEncoding
	}
	switch enc {
	case "utf-8":
		return UTF8
	case "utf-32":
		return UTF32
	default:
		return UTF16
	}
}

// Location is a source position returned by LSP textDocument/references.
// Line is 1-based (converted from LSP 0-based).
type Location struct {
	URI  string `json:"uri"`
	Line int    `json:"line"`
}

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

	// References returns all reference locations for the symbol at the given
	// position in file (line and char are 1-based). Sends textDocument/didOpen
	// then textDocument/references. Returns ErrNoPool from StubPool.
	References(ctx context.Context, file string, line, char int) ([]Location, error)

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
				"documentSymbol":    map[string]any{"hierarchicalDocumentSymbolSupport": true},
				"typeHierarchy":     map[string]any{},
				"callHierarchy":     map[string]any{},
				"implementation":    map[string]any{},
				"hover":             map[string]any{},
				"publishDiagnostics": map[string]any{
					"relatedInformation": true,
					"versionSupport":     true,
				},
			},
			"workspace": map[string]any{
				"symbol":        map[string]any{"dynamicRegistration": false},
				"configuration": true,
			},
			// Advertise UTF-8 preference; server may respond with utf-8
			// to avoid UTF-16 surrogate-pair math on our side.
			"general": map[string]any{
				"positionEncodings": []string{"utf-8", "utf-16"},
			},
		},
	}
}

// Initialize performs the LSP initialize/initialized handshake using
// canonical params. Returns the negotiated OffsetEncoding so callers can
// apply correct byte↔UTF-16 conversions for position fields.
func Initialize(client *Client, root string) (OffsetEncoding, error) {
	rootURI := "file://" + root
	raw, err := client.Request("initialize", InitializeParams(rootURI))
	if err != nil {
		slog.Error("lsp: initialize failed", "root", root, "error", err)
		return UTF16, err
	}
	enc := parseOffsetEncoding(raw)
	slog.Info("lsp: initialized", "root", root, "encoding", enc, "response_bytes", len(raw))
	if err := client.Notify("initialized", struct{}{}); err != nil {
		return enc, err
	}
	return enc, nil
}

// OpenRootMarkers sends textDocument/didOpen for each root marker file found
// in the workspace root. This primes language servers (notably gopls) that
// refuse to publish diagnostics until they have seen at least one file.
func OpenRootMarkers(client *Client, root string, markers []string) {
	for _, pattern := range markers {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err != nil {
			continue
		}
		for _, match := range matches {
			content, err := os.ReadFile(match)
			if err != nil {
				continue
			}
			uri := "file://" + filepath.ToSlash(match)
			langID := extToLangID(filepath.Ext(match))
			if err := client.Notify("textDocument/didOpen", map[string]any{
				"textDocument": map[string]any{
					"uri":        uri,
					"languageId": langID,
					"version":    1,
					"text":       string(content),
				},
			}); err != nil {
				slog.Warn("lsp: openRootMarkers: didOpen failed", "file", match, "error", err)
			}
		}
	}
}

// extToLangID maps common root marker extensions to LSP language identifiers.
func extToLangID(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".mod":
		return "go.mod"
	case ".sum":
		return "go.sum"
	case ".rs":
		return "rust"
	case ".toml":
		return "toml"
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".mjs", ".cjs", ".jsx":
		return "javascript"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	default:
		return "plaintext"
	}
}

// PositionToByteOffset converts a UTF-16 character offset to a byte offset
// in the given line of text. Necessary because the LSP spec defaults to
// UTF-16 code units while Go strings use UTF-8 bytes.
//
// Handles:
//   - ASCII (1 UTF-16 unit = 1 byte)
//   - BMP characters (1 UTF-16 unit = 2-3 UTF-8 bytes)
//   - Supplementary characters like emoji (2 UTF-16 units = 4 UTF-8 bytes)
//
// If utf16Char is beyond the end of line, returns len(line).
func PositionToByteOffset(line string, utf16Char int) int {
	if utf16Char <= 0 {
		return 0
	}
	var units int
	for byteOff, r := range line {
		if units >= utf16Char {
			return byteOff
		}
		w := 1
		if r >= 0x10000 { // supplementary → surrogate pair in UTF-16
			w = 2
		}
		if utf16Char < units+w {
			return byteOff
		}
		units += w
	}
	return len(line)
}

// PoolStatus reports the current state of the connection pool.
type PoolStatus struct {
	Active int                   `json:"active"`
	Idle   int                   `json:"idle"`
	ByLang map[lang.Language]int `json:"by_language"`
}

// poolReferences is the shared References implementation for RealPool and
// MockPool. It detects the language from the file extension, gets a warm
// client, sends textDocument/didOpen then textDocument/references, and
// unmarshals the []Location response.
func poolReferences(ctx context.Context, pool Pool, file string, line, char int) ([]Location, error) {
	language := fileLanguage(file)
	root := filepath.Dir(file)

	client, err := pool.Get(language, root)
	if err != nil {
		return nil, err
	}
	defer pool.Release(language, root)

	content, _ := os.ReadFile(file)
	uri := "file://" + filepath.ToSlash(file)
	langID := extToLangID(filepath.Ext(file))
	_ = client.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": langID,
			"version":    1,
			"text":       string(content),
		},
	})

	// LSP positions are 0-based; callers pass 1-based.
	raw, err := client.RequestContext(ctx, "textDocument/references", map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line - 1, "character": char},
		"context":      map[string]any{"includeDeclaration": false},
	})
	if err != nil {
		return []Location{}, nil // no references is not an error
	}

	type lspPos struct {
		Line int `json:"line"`
	}
	type lspRange struct {
		Start lspPos `json:"start"`
	}
	type lspLoc struct {
		URI   string   `json:"uri"`
		Range lspRange `json:"range"`
	}
	var locs []lspLoc
	if err := json.Unmarshal(raw, &locs); err != nil || len(locs) == 0 {
		return []Location{}, nil
	}
	out := make([]Location, len(locs))
	for i, l := range locs {
		out[i] = Location{URI: l.URI, Line: l.Range.Start.Line + 1}
	}
	return out, nil
}

// fileLanguage infers the LSP language from a file extension.
func fileLanguage(file string) lang.Language {
	switch strings.ToLower(filepath.Ext(file)) {
	case ".go":
		return lang.Go
	case ".rs":
		return lang.Rust
	case ".py":
		return lang.Python
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		return lang.TypeScript
	default:
		return lang.Unknown
	}
}
