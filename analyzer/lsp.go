package analyzer

import (
	"github.com/dpopsuev/oculus"
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"
)

var (
	// ErrLSPFieldRefs is returned when LSP field refs analysis is attempted.
	ErrLSPFieldRefs = errors.New("LSP field refs: not implemented (use tree-sitter)")
	// ErrLSPEntryPoints is returned when LSP entry points analysis is attempted.
	ErrLSPEntryPoints = errors.New("LSP entry points: not implemented (use tree-sitter)")
	// ErrLSPNestingDepth is returned when LSP nesting depth analysis is attempted.
	ErrLSPNestingDepth = errors.New("LSP nesting depth: not applicable (use tree-sitter)")
	// ErrLSPNoServer is returned when no LSP server is found for the language.
	ErrLSPNoServer = errors.New("no LSP server for language")
	// ErrSymbolNotFound is returned when a workspace symbol is not found.
	ErrSymbolNotFound = errors.New("symbol not found")
	// ErrCallHierarchyNotFound is returned when no call hierarchy item is found.
	ErrCallHierarchyNotFound = errors.New("no call hierarchy item found")
	// ErrContentLengthMissing is returned when Content-Length header is missing from LSP response.
	ErrContentLengthMissing = errors.New("missing Content-Length header")
	// ErrCallChainEntryNotFound is returned when the entry function is not found for call chain analysis.
	ErrCallChainEntryNotFound = errors.New("call hierarchy: entry not found")
	// ErrLSPRequest is returned when an LSP request returns an error response.
	ErrLSPRequest = errors.New("lsp request error")
)

// LSPAnalyzer extracts type-level metadata via an LSP server. It uses
// typeHierarchy, callHierarchy, and implementation requests for ~99%
// semantic accuracy. Falls through to tree-sitter on timeout or error.
type LSPAnalyzer struct {
	Timeout time.Duration // per-request timeout; default 30s
	pool    lsp.Pool      // optional connection pool (nil = cold-start per request)
}

func (a *LSPAnalyzer) Classes(root string) ([]oculus.ClassInfo, error) {
	conn, cleanup, err := a.startServer(root)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return conn.documentClasses(root)
}

func (a *LSPAnalyzer) Implements(root string) ([]oculus.ImplEdge, error) {
	conn, cleanup, err := a.startServer(root)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return conn.implementations(root)
}

func (a *LSPAnalyzer) FieldRefs(root string) ([]oculus.FieldRef, error) {
	return nil, ErrLSPFieldRefs
}

func (a *LSPAnalyzer) CallChain(root, entry string, depth int) ([]oculus.Call, error) {
	conn, cleanup, err := a.startServer(root)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	return conn.callChain(root, entry, depth)
}

func (a *LSPAnalyzer) EntryPoints(root string) ([]oculus.EntryPoint, error) {
	return nil, ErrLSPEntryPoints
}

func (a *LSPAnalyzer) NestingDepth(root string) ([]oculus.NestingResult, error) {
	return nil, ErrLSPNestingDepth
}

// --- LSP connection wrapper ---

// lspConn wraps oculus/lsp.Client with LSP protocol lifecycle methods.
type lspConn struct {
	*lsp.Client
}

func newLSPConn(r interface{ Read([]byte) (int, error) }, w interface{ Write([]byte) (int, error) }) *lspConn {
	return &lspConn{Client: lsp.NewClient(r, w)}
}

func (c *lspConn) initialize(root string) error {
	rootURI := pathToURI(root)
	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"documentSymbol": map[string]any{"hierarchicalDocumentSymbolSupport": true},
				"typeHierarchy":  map[string]any{},
				"callHierarchy":  map[string]any{},
				"implementation": map[string]any{},
			},
		},
	}
	if _, err := c.Request("initialize", params); err != nil {
		return err
	}
	return c.Notify("initialized", struct{}{})
}

func (c *lspConn) shutdown() {
	_, _ = c.Request("shutdown", nil)
	_ = c.Notify("exit", nil)
}

// documentClasses uses textDocument/documentSymbol on all source files.
//
//nolint:unparam // error return kept for API consistency with oculus.TypeAnalyzer interface
func (c *lspConn) documentClasses(root string) ([]oculus.ClassInfo, error) {
	files := findSrcFiles(root)
	var classes []oculus.ClassInfo
	for _, f := range files {
		syms, err := c.documentSymbols(f, root)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(root, f)
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if pkg == "." {
			pkg = pkgRoot
		}
		for _, sym := range syms {
			var kind string
			switch sym.Kind {
			case 23: // struct
				kind = kindStruct
			case 11: // interface
				kind = kindInterface
			case 5: // class
				kind = "class"
			default:
				continue
			}
			relFile := filepath.ToSlash(rel)
			ci := oculus.ClassInfo{
				Name:     sym.Name,
				Package:  pkg,
				Kind:     kind,
				Exported: isExported(sym.Name),
				File:     relFile,
				Line:     sym.Line + 1,
				EndLine:  sym.EndLine,
			}
			for _, ch := range sym.Children {
				switch ch.Kind {
				case 8: // field
					ci.Fields = append(ci.Fields, oculus.FieldInfo{
						Name:     ch.Name,
						Exported: isExported(ch.Name),
						Line:     ch.Line + 1,
					})
				case 6: // method
					ci.Methods = append(ci.Methods, oculus.MethodInfo{
						Name:      ch.Name,
						Signature: ch.Name,
						Exported:  isExported(ch.Name),
						File:      relFile,
						Line:      ch.Line + 1,
						EndLine:   ch.EndLine,
					})
				}
			}
			classes = append(classes, ci)
		}
	}
	return classes, nil
}

// implementations uses textDocument/implementation to find interface edges.
//
//nolint:unparam // error return kept for API consistency with oculus.TypeAnalyzer interface
func (c *lspConn) implementations(root string) ([]oculus.ImplEdge, error) {
	// LSP textDocument/implementation requires a specific position.
	// We first get all interface symbols via documentSymbol, then query
	// implementations at each interface name position.
	files := findSrcFiles(root)
	var edges []oculus.ImplEdge
	for _, f := range files {
		syms, err := c.documentSymbols(f, root)
		if err != nil {
			continue
		}
		for _, sym := range syms {
			if sym.Kind != 11 { // interface
				continue
			}
			implParams := map[string]any{
				"textDocument": map[string]string{"uri": pathToURI(f)},
				"position":     map[string]int{"line": sym.Line, "character": sym.Col},
			}
			impls, err := c.Request("textDocument/implementation", implParams)
			if err != nil {
				continue
			}
			var locations []lspLocation
			if json.Unmarshal(impls, &locations) != nil {
				continue
			}
			for _, loc := range locations {
				implName := resolveNameAtURI(loc.URI, loc.Range.Start.Line)
				if implName != "" {
					edges = append(edges, oculus.ImplEdge{
						From: implName,
						To:   sym.Name,
						Kind: "implements",
					})
				}
			}
		}
	}
	return edges, nil
}

// callChain uses callHierarchy/outgoingCalls recursively.
func (c *lspConn) callChain(root, entry string, maxDepth int) ([]oculus.Call, error) {
	if maxDepth <= 0 {
		maxDepth = 5
	}
	// Find the entry function via workspace/symbol
	item, err := c.findCallHierarchyItem(root, entry)
	if err != nil || item == nil {
		return nil, fmt.Errorf("%w: %q", ErrCallChainEntryNotFound, entry)
	}

	var calls []oculus.Call
	visited := make(map[string]bool)
	var walk func(it *callHierarchyItem, depth int)
	walk = func(it *callHierarchyItem, depth int) {
		if depth > maxDepth || visited[it.Name] {
			return
		}
		visited[it.Name] = true
		outgoing, err := c.Request("callHierarchy/outgoingCalls", map[string]any{"item": it})
		if err != nil {
			return
		}
		var outs []outgoingCallItem
		if json.Unmarshal(outgoing, &outs) != nil {
			return
		}
		for _, out := range outs {
			calls = append(calls, oculus.Call{
				Caller:  it.Name,
				Callee:  out.To.Name,
				Package: uriToPackage(out.To.URI, root),
				Line:    out.To.Range.Start.Line + 1,
			})
			walk(&out.To, depth+1)
		}
	}
	walk(item, 0)
	return calls, nil
}

// --- Shared LSP response types ---

// lspPosition is a zero-indexed line/character position in an LSP response.
type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// lspRange is a start/end position range in an LSP response.
type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

// lspLocation is a URI + range pair used by workspace/symbol responses.
type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

// callHierarchyItem represents a function/method in the call hierarchy protocol.
type callHierarchyItem struct {
	Name   string   `json:"name"`
	Kind   int      `json:"kind"`
	Detail string   `json:"detail"`
	URI    string   `json:"uri"`
	Range  lspRange `json:"range"`
}

// outgoingCallItem wraps a callHierarchyItem in callHierarchy/outgoingCalls responses.
type outgoingCallItem struct {
	To callHierarchyItem `json:"to"`
}

// workspaceSymbol represents a symbol returned by workspace/symbol.
type workspaceSymbol struct {
	Name     string      `json:"name"`
	Kind     int         `json:"kind"`
	Location lspLocation `json:"location"`
}

// locationOrLink handles both Location and LocationLink formats from
// textDocument/definition responses.
type locationOrLink struct {
	URI         string    `json:"uri"`
	Range       lspRange  `json:"range"`
	TargetURI   string    `json:"targetUri"`
	TargetRange *lspRange `json:"targetRange"`
}

// hoverResult represents the textDocument/hover response.
type hoverResult struct {
	Contents struct {
		Value string `json:"value"`
	} `json:"contents"`
}

// docSymbolEntry represents a document symbol from textDocument/documentSymbol.
type docSymbolEntry struct {
	Name           string           `json:"name"`
	Kind           int              `json:"kind"`
	Range          lspRange         `json:"range"`
	SelectionRange lspRange         `json:"selectionRange"`
	Children       []docSymbolEntry `json:"children,omitempty"`
}

// parseSignatureTypes extracts parameter and return types from a function signature.
// Dispatches to language-specific parsers based on the signature prefix.
func parseSignatureTypes(sig string) (paramTypes, returnTypes []string) {
	sig = strings.TrimSpace(sig)
	switch {
	// "function " must precede "func" — "function" starts with "func".
	case strings.HasPrefix(sig, "function "):
		return parseTSSig(sig)
	case strings.HasPrefix(sig, "func"):
		return parseGoSig(sig)
	case strings.HasPrefix(sig, "def "):
		return parsePythonSig(sig)
	case strings.HasPrefix(sig, "fn "), strings.HasPrefix(sig, "pub fn "):
		return parseRustSig(sig)
	default:
		// Try C/C++ style: "ReturnType FuncName(params)"
		if strings.Contains(sig, "(") {
			return parseCCppSig(sig)
		}
		return nil, nil
	}
}

// parseGoSig parses "func(x int, y string) (*Result, error)" or "func Name(...)".
func parseGoSig(sig string) (paramTypes, returnTypes []string) {
	openParen := strings.Index(sig, "(")
	if openParen < 0 {
		return nil, nil
	}
	closeParen := findMatchingParen(sig, openParen)
	if closeParen < 0 {
		return nil, nil
	}
	paramStr := sig[openParen+1 : closeParen]
	paramTypes = extractTypesFromParamList(paramStr)

	rest := strings.TrimSpace(sig[closeParen+1:])
	if rest == "" {
		return paramTypes, nil
	}
	if strings.HasPrefix(rest, "(") {
		closeReturn := findMatchingParen(rest, 0)
		if closeReturn > 0 {
			returnStr := rest[1:closeReturn]
			returnTypes = extractTypesFromParamList(returnStr)
		}
	} else {
		returnTypes = []string{strings.TrimSpace(rest)}
	}
	return paramTypes, returnTypes
}

// parsePythonSig parses "def load_config(path: str) -> dict".
func parsePythonSig(sig string) ([]string, []string) {
	openParen := strings.Index(sig, "(")
	if openParen < 0 {
		return nil, nil
	}
	closeParen := findMatchingParen(sig, openParen)
	if closeParen < 0 {
		return nil, nil
	}
	paramStr := sig[openParen+1 : closeParen]
	var params []string
	for _, p := range splitParams(paramStr) {
		p = strings.TrimSpace(p)
		if p == "" || p == "self" || p == "cls" {
			continue
		}
		if colon := strings.LastIndex(p, ":"); colon >= 0 {
			params = append(params, strings.TrimSpace(p[colon+1:]))
		}
	}
	var returns []string
	rest := sig[closeParen+1:]
	if arrow := strings.Index(rest, "->"); arrow >= 0 {
		ret := strings.TrimSpace(rest[arrow+2:])
		if ret != "" && ret != "None" {
			returns = append(returns, ret)
		}
	}
	return params, returns
}

// parseTSSig parses "function loadConfig(path: string): Config".
func parseTSSig(sig string) ([]string, []string) {
	openParen := strings.Index(sig, "(")
	if openParen < 0 {
		return nil, nil
	}
	closeParen := findMatchingParen(sig, openParen)
	if closeParen < 0 {
		return nil, nil
	}
	paramStr := sig[openParen+1 : closeParen]
	var params []string
	for _, p := range splitParams(paramStr) {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if colon := strings.Index(p, ":"); colon >= 0 {
			params = append(params, strings.TrimSpace(p[colon+1:]))
		}
	}
	var returns []string
	rest := strings.TrimSpace(sig[closeParen+1:])
	if strings.HasPrefix(rest, ":") {
		ret := strings.TrimSpace(rest[1:])
		if ret != "" && ret != "void" {
			returns = append(returns, ret)
		}
	}
	return params, returns
}

// parseRustSig parses "fn load_config(path: &str) -> Config" or "pub fn ...".
func parseRustSig(sig string) ([]string, []string) {
	openParen := strings.Index(sig, "(")
	if openParen < 0 {
		return nil, nil
	}
	closeParen := findMatchingParen(sig, openParen)
	if closeParen < 0 {
		return nil, nil
	}
	paramStr := sig[openParen+1 : closeParen]
	var params []string
	for _, p := range splitParams(paramStr) {
		p = strings.TrimSpace(p)
		if p == "" || p == "self" || p == "&self" || p == "&mut self" {
			continue
		}
		if colon := strings.Index(p, ":"); colon >= 0 {
			params = append(params, strings.TrimSpace(p[colon+1:]))
		}
	}
	var returns []string
	rest := sig[closeParen+1:]
	if arrow := strings.Index(rest, "->"); arrow >= 0 {
		ret := strings.TrimSpace(rest[arrow+2:])
		if ret != "" && ret != "()" {
			returns = append(returns, ret)
		}
	}
	return params, returns
}

// parseCCppSig parses "Config loadConfig(const std::string &path)".
// Return type precedes the function name; params use "type name" order.
func parseCCppSig(sig string) ([]string, []string) {
	openParen := strings.Index(sig, "(")
	if openParen < 0 {
		return nil, nil
	}
	closeParen := findMatchingParen(sig, openParen)
	if closeParen < 0 {
		return nil, nil
	}
	// Return type: everything before function name (last word before '(')
	prefix := strings.TrimSpace(sig[:openParen])
	prefixParts := strings.Fields(prefix)
	var returns []string
	if len(prefixParts) >= 2 {
		retType := strings.Join(prefixParts[:len(prefixParts)-1], " ")
		if retType != "void" {
			returns = append(returns, retType)
		}
	}
	// Params: "type name" order — all but last word is the type
	paramStr := sig[openParen+1 : closeParen]
	var params []string
	for _, p := range splitParams(paramStr) {
		p = strings.TrimSpace(p)
		if p == "" || p == "void" {
			continue
		}
		parts := strings.Fields(p)
		if len(parts) >= 2 {
			params = append(params, strings.Join(parts[:len(parts)-1], " "))
		} else if len(parts) == 1 {
			params = append(params, parts[0])
		}
	}
	return params, returns
}

// findMatchingParen finds the index of the closing paren matching the open paren at pos.
func findMatchingParen(s string, pos int) int {
	depth := 0
	for i := pos; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// extractTypesFromParamList splits "x int, y string, z *Config" into ["int", "string", "*Config"].
func extractTypesFromParamList(paramStr string) []string {
	paramStr = strings.TrimSpace(paramStr)
	if paramStr == "" {
		return nil
	}
	var types []string
	// Split by comma, respecting nested parens/brackets
	params := splitParams(paramStr)
	for _, p := range params {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		// Last token is the type (e.g., "x int" → "int", "y *Config" → "*Config")
		// But unnamed params are just the type (e.g., "int", "*Config")
		parts := strings.Fields(p)
		if len(parts) == 0 {
			continue
		}
		types = append(types, parts[len(parts)-1])
	}
	return types
}

// splitParams splits a comma-separated param list, respecting nested brackets.
func splitParams(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

func (c *lspConn) findCallHierarchyItem(root, name string) (*callHierarchyItem, error) {
	// Strategy 1: workspace/symbol — fast, supported by gopls and most servers.
	wsResult, err := c.Request("workspace/symbol", map[string]any{"query": name})
	if err == nil {
		var symbols []workspaceSymbol
		if json.Unmarshal(wsResult, &symbols) == nil {
			for _, s := range symbols {
				if s.Name != name || (s.Kind != 12 && s.Kind != 6) {
					continue
				}
				if item := c.prepareCallHierarchyAt(s.Location.URI, s.Location.Range.Start.Line, s.Location.Range.Start.Character); item != nil {
					return item, nil
				}
			}
		}
	}

	// Strategy 2: documentSymbol fallback — needed for servers like pyright
	// that return empty workspace/symbol but support documentSymbol.
	for _, f := range findSrcFiles(root) {
		syms, err := c.documentSymbols(f, root)
		if err != nil {
			continue
		}
		for _, sym := range syms {
			if sym.Name != name || (sym.Kind != 12 && sym.Kind != 6) {
				continue
			}
			uri := pathToURI(f)
			if item := c.prepareCallHierarchyAt(uri, sym.Line, sym.Col); item != nil {
				return item, nil
			}
		}
	}

	return nil, fmt.Errorf("%w: %q", ErrSymbolNotFound, name)
}

// prepareCallHierarchyAt sends prepareCallHierarchy at a specific position.
func (c *lspConn) prepareCallHierarchyAt(uri string, line, col int) *callHierarchyItem {
	result, err := c.Request("textDocument/prepareCallHierarchy", map[string]any{
		"textDocument": map[string]string{"uri": uri},
		"position":     map[string]int{"line": line, "character": col},
	})
	if err != nil {
		return nil
	}
	var items []callHierarchyItem
	if json.Unmarshal(result, &items) != nil || len(items) == 0 {
		return nil
	}
	return &items[0]
}

type docSymbol struct {
	Name     string      `json:"name"`
	Kind     int         `json:"kind"`
	Line     int         `json:"-"`
	EndLine  int         `json:"-"`
	Col      int         `json:"-"`
	Children []docSymbol `json:"children,omitempty"`
}

func (c *lspConn) documentSymbols(file, _ string) ([]docSymbol, error) {
	uri := pathToURI(file)
	content, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	langID := extToLanguageID(filepath.Ext(file))
	_ = c.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": langID, "version": 1, "text": string(content),
		},
	})
	result, err := c.Request("textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]string{"uri": uri},
	})
	if err != nil {
		return nil, err
	}
	var symbols []docSymbolEntry
	if json.Unmarshal(result, &symbols) != nil {
		return nil, nil
	}
	out := make([]docSymbol, 0, len(symbols))
	for _, s := range symbols {
		ds := docSymbol{
			Name: s.Name, Kind: s.Kind,
			Line:    s.SelectionRange.Start.Line,
			EndLine: s.Range.End.Line + 1,
			Col:     s.SelectionRange.Start.Character,
		}
		for _, ch := range s.Children {
			ds.Children = append(ds.Children, docSymbol{
				Name: ch.Name, Kind: ch.Kind,
				Line:    ch.SelectionRange.Start.Line,
				EndLine: ch.Range.End.Line + 1,
				Col:     ch.SelectionRange.Start.Character,
			})
		}
		out = append(out, ds)
	}
	return out, nil
}

// --- helpers ---

func (a *LSPAnalyzer) startServer(root string) (*lspConn, func(), error) {
	// Try pool first (warm connection).
	if a.pool != nil {
		detected := lang.DetectLanguage(root)
		client, err := a.pool.Get(detected, root)
		if err == nil {
			conn := &lspConn{Client: client}
			release := func() { a.pool.Release(detected, root) }
			return conn, release, nil
		}
	}

	// Fall through to existing cold-start logic.
	detected := lang.DetectLanguage(root)
	cmdStr := lang.DefaultLSPServer(detected)
	if cmdStr == "" {
		return nil, nil, fmt.Errorf("%w: %v", ErrLSPNoServer, detected)
	}
	parts := strings.Fields(cmdStr)
	bin, err := exec.LookPath(parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf("lsp server %s not found: %w", parts[0], err)
	}
	absRoot, _ := filepath.Abs(root)
	cmd := exec.Command(bin, parts[1:]...)
	cmd.Dir = absRoot
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start %s: %w", parts[0], err)
	}
	conn := newLSPConn(stdout, stdin)
	if err := conn.initialize(absRoot); err != nil {
		stdin.Close()
		_ = cmd.Wait() // best-effort cleanup after init failure
		return nil, nil, err
	}
	cleanup := func() {
		conn.shutdown()
		stdin.Close()
		_ = cmd.Wait() // best-effort cleanup
	}
	return conn, cleanup, nil
}

func pathToURI(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	slash := filepath.ToSlash(abs)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return "file://" + slash
}

func uriToPackage(uri, root string) string {
	absRoot, _ := filepath.Abs(root)
	path := strings.TrimPrefix(uri, "file://")
	rel, err := filepath.Rel(absRoot, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(filepath.Dir(rel))
}

func uriToRelPath(uri, root string) string {
	absRoot, _ := filepath.Abs(root)
	path := strings.TrimPrefix(uri, "file://")
	rel, err := filepath.Rel(absRoot, path)
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// hoverAt calls textDocument/hover and returns the markdown content.
// Works with any LSP server (gopls, rust-analyzer, typescript-language-server, etc.).
func (c *lspConn) hoverAt(file string, line, col int) (string, error) {
	uri := pathToURI(file)
	c.ensureOpen(file)
	result, err := c.Request("textDocument/hover", map[string]any{
		"textDocument": map[string]string{"uri": uri},
		"position":     map[string]int{"line": line, "character": col},
	})
	if err != nil {
		return "", err
	}
	var hover hoverResult
	if json.Unmarshal(result, &hover) != nil {
		return "", nil
	}
	return hover.Contents.Value, nil
}

// definitionLocation holds the result of textDocument/definition.
type definitionLocation struct {
	URI  string
	Line int
	Col  int
}

// definitionAt calls textDocument/definition and returns the first result.
func (c *lspConn) definitionAt(file string, line, col int) (*definitionLocation, error) {
	uri := pathToURI(file)
	c.ensureOpen(file)
	result, err := c.Request("textDocument/definition", map[string]any{
		"textDocument": map[string]string{"uri": uri},
		"position":     map[string]int{"line": line, "character": col},
	})
	if err != nil {
		return nil, err
	}
	// Response can be Location | Location[] | LocationLink[]
	var locs []locationOrLink
	if json.Unmarshal(result, &locs) != nil || len(locs) == 0 {
		return nil, nil
	}
	loc := locs[0]
	defURI := loc.URI
	defLine := loc.Range.Start.Line
	defCol := loc.Range.Start.Character
	// Handle LocationLink format (targetUri/targetRange)
	if loc.TargetURI != "" {
		defURI = loc.TargetURI
		if loc.TargetRange != nil {
			defLine = loc.TargetRange.Start.Line
			defCol = loc.TargetRange.Start.Character
		}
	}
	return &definitionLocation{URI: defURI, Line: defLine, Col: defCol}, nil
}

// ensureOpen sends textDocument/didOpen if not already tracked.
func (c *lspConn) ensureOpen(file string) {
	uri := pathToURI(file)
	content, err := os.ReadFile(file)
	if err != nil {
		return
	}
	langID := extToLanguageID(filepath.Ext(file))
	_ = c.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": uri, "languageId": langID, "version": 1, "text": string(content),
		},
	})
}

// extToLanguageID maps file extensions to LSP language identifiers.
func extToLanguageID(ext string) string {
	switch ext {
	case extRust:
		return "rust"
	case extPy:
		return "python"
	case extTS, extTSX:
		return "typescript"
	case extJS, extJSX:
		return "javascript"
	case extJava:
		return "java"
	case extC, extH:
		return "c"
	case extCpp, extHpp:
		return "cpp"
	case extKt:
		return "kotlin"
	case extZig:
		return "zig"
	case extSwift:
		return "swift"
	case extCS:
		return "csharp"
	default:
		return "go"
	}
}

func resolveNameAtURI(uri string, line int) string {
	path := strings.TrimPrefix(uri, "file://")
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	cur := 0
	for scanner.Scan() {
		if cur == line {
			text := strings.TrimSpace(scanner.Text())
			// Extract type name from "type Foo struct" pattern
			if strings.HasPrefix(text, "type ") {
				parts := strings.Fields(text)
				if len(parts) >= 2 {
					return parts[1]
				}
			}
			return text
		}
		cur++
	}
	return ""
}

func findSrcFiles(root string) []string {
	absRoot, _ := filepath.Abs(root)
	var files []string
	_ = filepath.WalkDir(absRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == dirVendor || base == dirTestdata || strings.HasPrefix(base, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		switch ext {
		case extGo, extRust, extPy, extTS, extJS, extJava,
			extC, extCpp, extKt, extZig, extSwift, extCS:
			if !strings.HasSuffix(d.Name(), "_test.go") {
				files = append(files, path)
			}
		}
		return nil
	})
	return files
}
