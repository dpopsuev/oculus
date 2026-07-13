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

// DocSymbol is a hierarchical documentSymbol entry. Lines are 1-based.
type DocSymbol struct {
	Name           string     `json:"name"`
	Kind           int        `json:"kind"`
	StartLine      int        `json:"start_line"`
	EndLine        int        `json:"end_line"`
	SelectionLine  int        `json:"selection_line"`
	SelectionChar  int        `json:"selection_char,omitempty"`
	Children       []DocSymbol `json:"children,omitempty"`
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

	// Definition returns go-to-definition locations for the symbol at the given
	// position (line 1-based, char 0-based). Sends textDocument/didOpen then
	// textDocument/definition. Returns ErrNoPool from StubPool.
	Definition(ctx context.Context, file string, line, char int) ([]Location, error)

	// DocumentSymbols returns hierarchical symbols for file via
	// textDocument/documentSymbol (after didOpen). Returns ErrNoPool from StubPool.
	DocumentSymbols(ctx context.Context, file string) ([]DocSymbol, error)

	// MaxConcurrent returns the maximum number of simultaneous LSP server
	// instances allowed for the given language. Resource-heavy servers
	// (e.g. clangd) have a lower limit than lightweight ones (e.g. gopls).
	MaxConcurrent(language lang.Language) int

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
				"definition":        map[string]any{},
				"references":        map[string]any{},
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
	return poolPositionRequest(ctx, pool, file, line, char, "textDocument/references", map[string]any{
		"includeDeclaration": false,
	})
}

// poolDefinition is the shared Definition implementation.
func poolDefinition(ctx context.Context, pool Pool, file string, line, char int) ([]Location, error) {
	return poolPositionRequest(ctx, pool, file, line, char, "textDocument/definition", nil)
}

// poolDocumentSymbols opens file and issues textDocument/documentSymbol.
func poolDocumentSymbols(ctx context.Context, pool Pool, file string) ([]DocSymbol, error) {
	language := fileLanguage(file)
	root := workspaceRootForFile(file)

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

	raw, err := client.RequestContext(ctx, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]any{"uri": uri},
	})
	if err != nil {
		return nil, err
	}
	return parseDocSymbols(raw), nil
}

func parseDocSymbols(raw json.RawMessage) []DocSymbol {
	type lspPos struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	}
	type lspRange struct {
		Start lspPos `json:"start"`
		End   lspPos `json:"end"`
	}
	type lspDoc struct {
		Name           string   `json:"name"`
		Kind           int      `json:"kind"`
		Range          lspRange `json:"range"`
		SelectionRange lspRange `json:"selectionRange"`
		Children       []lspDoc `json:"children,omitempty"`
	}
	var convert func(s lspDoc) DocSymbol
	convert = func(s lspDoc) DocSymbol {
		ds := DocSymbol{
			Name:          s.Name,
			Kind:          s.Kind,
			StartLine:     s.Range.Start.Line + 1,
			EndLine:       s.Range.End.Line + 1,
			SelectionLine: s.SelectionRange.Start.Line + 1,
			SelectionChar: s.SelectionRange.Start.Character,
		}
		for _, ch := range s.Children {
			ds.Children = append(ds.Children, convert(ch))
		}
		return ds
	}
	var docs []lspDoc
	if err := json.Unmarshal(raw, &docs); err == nil && len(docs) > 0 {
		out := make([]DocSymbol, len(docs))
		for i, d := range docs {
			out[i] = convert(d)
		}
		return out
	}
	// SymbolInformation[] fallback (flat, no children / full range)
	type lspInfo struct {
		Name     string   `json:"name"`
		Kind     int      `json:"kind"`
		Location struct {
			URI   string   `json:"uri"`
			Range lspRange `json:"range"`
		} `json:"location"`
	}
	var infos []lspInfo
	if err := json.Unmarshal(raw, &infos); err == nil && len(infos) > 0 {
		out := make([]DocSymbol, len(infos))
		for i, info := range infos {
			out[i] = DocSymbol{
				Name:          info.Name,
				Kind:          info.Kind,
				StartLine:     info.Location.Range.Start.Line + 1,
				EndLine:       info.Location.Range.End.Line + 1,
				SelectionLine: info.Location.Range.Start.Line + 1,
				SelectionChar: info.Location.Range.Start.Character,
			}
		}
		return out
	}
	return nil
}

// poolPositionRequest opens file and issues a position-based LSP request.
// line is 1-based; char is 0-based (LSP character).
func poolPositionRequest(ctx context.Context, pool Pool, file string, line, char int, method string, refContext map[string]any) ([]Location, error) {
	language := fileLanguage(file)
	root := workspaceRootForFile(file)

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

	params := map[string]any{
		"textDocument": map[string]any{"uri": uri},
		"position":     map[string]any{"line": line - 1, "character": char},
	}
	if method == "textDocument/references" {
		params["context"] = refContext
	}
	raw, err := client.RequestContext(ctx, method, params)
	if err != nil {
		return []Location{}, nil
	}

	return parseLSPLocations(raw), nil
}

func parseLSPLocations(raw json.RawMessage) []Location {
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
	toLocs := func(locs []lspLoc) []Location {
		out := make([]Location, len(locs))
		for i, l := range locs {
			out[i] = Location{URI: l.URI, Line: l.Range.Start.Line + 1}
		}
		return out
	}
	// definition may return Location | Location[] | LocationLink[]
	var locs []lspLoc
	if err := json.Unmarshal(raw, &locs); err == nil && len(locs) > 0 {
		return toLocs(locs)
	}
	var one lspLoc
	if err := json.Unmarshal(raw, &one); err == nil && one.URI != "" {
		return toLocs([]lspLoc{one})
	}
	type lspLink struct {
		TargetURI   string   `json:"targetUri"`
		TargetRange lspRange `json:"targetRange"`
	}
	var links []lspLink
	if err := json.Unmarshal(raw, &links); err == nil && len(links) > 0 {
		out := make([]Location, len(links))
		for i, l := range links {
			out[i] = Location{URI: l.TargetURI, Line: l.TargetRange.Start.Line + 1}
		}
		return out
	}
	return []Location{}
}

// workspaceRootForFile walks up from file looking for a project marker so
// Definition/References reuse the same WarmLSP client as the workspace root.
func workspaceRootForFile(file string) string {
	dir := filepath.Dir(file)
	markers := []string{"go.mod", "Cargo.toml", "package.json", "pyproject.toml", "tsconfig.json"}
	for d := dir; ; d = filepath.Dir(d) {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(d, m)); err == nil {
				return d
			}
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
	}
	return dir
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
