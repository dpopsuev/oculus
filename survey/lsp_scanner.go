package survey

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dpopsuev/oculus/v3/lsp"
	"github.com/dpopsuev/oculus/v3/model"
)

var errEmptyServerCmd = errors.New("lsp scanner: empty ServerCmd")

// extToLanguageID maps file extensions to LSP language identifiers.
// Language-agnostic: covers all languages Locus supports.
var extToLanguageID = map[string]string{
	".go":    "go",
	".rs":    "rust",
	".py":    "python",
	".ts":    "typescript",
	".tsx":   "typescriptreact",
	".js":    "javascript",
	".jsx":   "javascriptreact",
	".java":  "java",
	".kt":    "kotlin",
	".kts":   "kotlin",
	".c":     "c",
	".h":     "c",
	".cpp":   "cpp",
	".cc":    "cpp",
	".hpp":   "cpp",
	".cs":    "csharp",
	".swift": "swift",
	".zig":   "zig",
}

// lspScanTimeout is the maximum time any LSP server is allowed to run
// during a survey scan before it is killed and the scan returns an error.
const lspScanTimeout = 5 * time.Minute

// LSPScanner extracts structural metadata by communicating with an
// external LSP server. It is language-agnostic: the same code works
// with gopls, rust-analyzer, pyright, or any LSP-compliant server.
type LSPScanner struct {
	ServerCmd  string        // e.g. "gopls serve", "rust-analyzer", "pyright-langserver --stdio"
	Timeout    time.Duration // 0 = lspScanTimeout; set a small value in tests
	CallBudget int           // max callHierarchy roundtrips; 0 = unlimited
}

func (s *LSPScanner) Scan(root string) (*model.Project, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	parts := strings.Fields(s.ServerCmd)
	if len(parts) == 0 {
		return nil, errEmptyServerCmd
	}

	bin, err := exec.LookPath(parts[0])
	if err != nil {
		return nil, fmt.Errorf("lsp scanner: %s not found on PATH: %w", parts[0], err)
	}

	timeout := s.Timeout
	if timeout == 0 {
		timeout = lspScanTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, parts[1:]...)
	cmd.Dir = absRoot
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp scanner: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lsp scanner: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("lsp scanner: start %s: %w", parts[0], err)
	}
	// Reap the child on every return path so the process never becomes a
	// zombie. The explicit cmd.Wait() below covers the normal path; this
	// defer covers all early-return error paths.
	defer func() { _ = cmd.Wait() }()

	client := lsp.NewClient(stdout, stdin)

	proj, scanErr := s.runProtocol(client, absRoot)

	// Best-effort graceful shutdown. If the context already fired (timeout),
	// the process was killed by exec.CommandContext and these calls will
	// return immediately with pipe-broken errors, which we ignore.
	_ = shutdownLSP(client)
	stdin.Close()
	_ = cmd.Wait()

	if scanErr != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("lsp scanner: timed out after %s (%s): %w", timeout, parts[0], ctx.Err())
		}
		return nil, scanErr
	}
	// Scan protocol succeeded. If the context fired only during shutdown
	// (hang_exit scenario), we still return the collected data.
	return proj, nil
}

func (s *LSPScanner) runProtocol(client *lsp.Client, root string) (*model.Project, error) {
	rootURI := pathToURI(root)

	initParams := lsp.InitializeParams(rootURI)
	initParams["processId"] = os.Getpid()

	_, err := client.Request("initialize", initParams)
	if err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}

	if err := client.Notify("initialized", struct{}{}); err != nil {
		return nil, fmt.Errorf("initialized notification: %w", err)
	}

	goFiles, err := findSourceFiles(root)
	if err != nil {
		return nil, fmt.Errorf("find source files: %w", err)
	}

	proj := model.NewProject(filepath.Base(root))
	proj.DependencyGraph = model.NewDependencyGraph()

	nsMap := make(map[string]*model.Namespace)
	var callables []callablePos
	var hoverTargets []hoverTarget

	for _, f := range goFiles {
		fileURI := pathToURI(f)
		content, readErr := os.ReadFile(f)
		if readErr != nil {
			continue
		}

		langID := extToLanguageID[filepath.Ext(f)]
		if langID == "" {
			langID = "plaintext"
		}
		err := client.Notify("textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{
				"uri":        fileURI,
				"languageId": langID,
				"version":    1,
				"text":       string(content),
			},
		})
		if err != nil {
			continue
		}

		result, err := client.Request("textDocument/documentSymbol", map[string]any{
			"textDocument": map[string]string{"uri": fileURI},
		})
		if err != nil {
			return nil, fmt.Errorf("documentSymbol %s: %w", filepath.Base(f), err)
		}

		var symbols []lspDocumentSymbol
		if json.Unmarshal(result, &symbols) != nil {
			var flat []lspSymbolInformation
			if json.Unmarshal(result, &flat) == nil {
				for _, sym := range flat {
					addSymbolToNS(nsMap, sym.ContainerName, sym.Name, sym.Kind, "", 0)
				}
			}
			continue
		}

		rel, relErr := filepath.Rel(root, f)
		if relErr != nil {
			rel = f
		}
		dir := filepath.Dir(rel)
		nsKey := filepath.ToSlash(dir)
		if nsKey == "." {
			nsKey = nsRoot
		}

		for _, sym := range symbols {
			line := 0
			if sym.Range.Start.Line > 0 {
				line = sym.Range.Start.Line + 1 // LSP lines are 0-based
			}
			addSymbolToNS(nsMap, nsKey, sym.Name, sym.Kind, filepath.ToSlash(rel), line)

			if isCallableKind(sym.Kind) {
				callables = append(callables, callablePos{
					uri:   fileURI,
					line:  sym.SelectionRange.Start.Line,
					char:  sym.SelectionRange.Start.Character,
					nsKey: nsKey,
					name:  sym.Name,
				})
			}

			if isExportedSymbol(sym.Name) {
				hoverTargets = append(hoverTargets, hoverTarget{
					uri:   fileURI,
					line:  sym.SelectionRange.Start.Line,
					char:  sym.SelectionRange.Start.Character,
					nsKey: nsKey,
					name:  sym.Name,
				})
			}
		}
	}

	total := len(callables)
	budget := s.CallBudget
	if budget > 0 && len(callables) > budget {
		callables = prioritizeCallables(callables, budget)
	}
	extractCallEdges(client, root, callables, proj.DependencyGraph, budget)

	if budget > 0 {
		crawled := len(callables)
		if budget < crawled {
			crawled = budget
		}
		proj.CrawlStats = &model.CrawlStats{
			Total:   total,
			Crawled: crawled,
			Skipped: total - crawled,
		}
	}

	extractHoverSignatures(client, hoverTargets, nsMap)

	for _, ns := range nsMap {
		proj.AddNamespace(ns)
	}

	return proj, nil
}

func addSymbolToNS(nsMap map[string]*model.Namespace, nsKey, name string, kind int, filePath string, line int) {
	if nsKey == "" {
		nsKey = nsRoot
	}
	ns := nsMap[nsKey]
	if ns == nil {
		ns = model.NewNamespace(nsKey, nsKey)
		nsMap[nsKey] = ns
	}
	ns.AddSymbol(&model.Symbol{
		Name:     name,
		Kind:     model.SymbolKind(kind),
		Exported: isExportedSymbol(name),
		File:     filePath,
		Line:     line,
	})
}

func findSourceFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if ShouldSkipDir(base) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(d.Name())
		if _, ok := extToLanguageID[ext]; ok {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func shutdownLSP(client *lsp.Client) error {
	_, err := client.Request("shutdown", nil)
	if err != nil {
		return err
	}
	return client.Notify("exit", nil)
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

func isExportedSymbol(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}

type lspSymbolInformation struct {
	Name          string `json:"name"`
	Kind          int    `json:"kind"`
	ContainerName string `json:"containerName"`
	Location      struct {
		URI string `json:"uri"`
	} `json:"location"`
}

type lspDocumentSymbol struct {
	Name           string              `json:"name"`
	Kind           int                 `json:"kind"`
	Range          lspRange            `json:"range"`
	SelectionRange lspRange            `json:"selectionRange"`
	Children       []lspDocumentSymbol `json:"children,omitempty"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// callablePos records the LSP position of a callable symbol for the
// call hierarchy pass.
type callablePos struct {
	uri   string
	line  int // 0-based LSP line
	char  int // 0-based LSP character
	nsKey string
	name  string
}

type lspCallHierarchyItem struct {
	Name           string   `json:"name"`
	Kind           int      `json:"kind"`
	URI            string   `json:"uri"`
	Range          lspRange `json:"range"`
	SelectionRange lspRange `json:"selectionRange"`
}

type lspOutgoingCall struct {
	To         lspCallHierarchyItem `json:"to"`
	FromRanges []lspRange           `json:"fromRanges"`
}

func isCallableKind(kind int) bool {
	switch model.SymbolKind(kind) {
	case model.SymbolFunction, model.SymbolMethod, model.SymbolConstructor:
		return true
	}
	return false
}

var entryPointNames = map[string]bool{
	"main": true, "init": true, "Main": true,
	"__main__": true, "run": false,
}

func isEntryPoint(name string) bool {
	return entryPointNames[name] || strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark")
}

func prioritizeCallables(callables []callablePos, budget int) []callablePos {
	var entries, exported, rest []callablePos
	for _, c := range callables {
		switch {
		case isEntryPoint(c.name):
			entries = append(entries, c)
		case isExportedSymbol(c.name):
			exported = append(exported, c)
		default:
			rest = append(rest, c)
		}
	}
	result := make([]callablePos, 0, budget)
	for _, group := range [][]callablePos{entries, exported, rest} {
		for _, c := range group {
			if len(result) >= budget {
				return result
			}
			result = append(result, c)
		}
	}
	return result
}

const lspConcurrency = 10

// extractCallEdges calls prepareCallHierarchy + outgoingCalls concurrently
// for each callable symbol and adds the resulting edges to the dependency graph.
// When budget > 0, stops after that many roundtrips.
// Errors from individual requests are silently ignored.
func extractCallEdges(client *lsp.Client, root string, callables []callablePos, graph *model.DependencyGraph, budget int) {
	var mu sync.Mutex
	sem := make(chan struct{}, lspConcurrency)
	var wg sync.WaitGroup
	var spent int64

	for _, c := range callables {
		if budget > 0 && atomic.LoadInt64(&spent) >= int64(budget) {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(c callablePos) {
			defer func() { <-sem; wg.Done() }()
			if budget > 0 && atomic.AddInt64(&spent, 1) > int64(budget) {
				return
			}

			result, err := client.Request("textDocument/prepareCallHierarchy", map[string]any{
				"textDocument": map[string]any{"uri": c.uri},
				"position":     map[string]int{"line": c.line, "character": c.char},
			})
			if err != nil {
				return
			}

			var items []lspCallHierarchyItem
			if json.Unmarshal(result, &items) != nil || len(items) == 0 {
				return
			}

			outResult, err := client.Request("callHierarchy/outgoingCalls", map[string]any{
				"item": items[0],
			})
			if err != nil {
				return
			}

			var calls []lspOutgoingCall
			if json.Unmarshal(outResult, &calls) != nil {
				return
			}

			mu.Lock()
			for _, call := range calls {
				targetNS := uriToNSKey(call.To.URI, root)
				if targetNS == "" {
					continue
				}
				graph.AddCallEdge(c.nsKey, targetNS)
			}
			mu.Unlock()
		}(c)
	}
	wg.Wait()
}

type hoverTarget struct {
	uri   string
	line  int
	char  int
	nsKey string
	name  string
}

type lspHoverResult struct {
	Contents lspMarkupContent `json:"contents"`
}

type lspMarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

func extractHoverSignatures(client *lsp.Client, targets []hoverTarget, nsMap map[string]*model.Namespace) {
	var mu sync.Mutex
	sem := make(chan struct{}, lspConcurrency)
	var wg sync.WaitGroup

	for _, t := range targets {
		wg.Add(1)
		sem <- struct{}{}
		go func(t hoverTarget) {
			defer func() { <-sem; wg.Done() }()

			result, err := client.Request("textDocument/hover", map[string]any{
				"textDocument": map[string]any{"uri": t.uri},
				"position":     map[string]int{"line": t.line, "character": t.char},
			})
			if err != nil {
				return
			}

			var hover lspHoverResult
			if json.Unmarshal(result, &hover) != nil || hover.Contents.Value == "" {
				return
			}

			sig := cleanHoverSignature(hover.Contents.Value)
			if sig == "" {
				return
			}

			mu.Lock()
			ns := nsMap[t.nsKey]
			if ns != nil {
				for _, s := range ns.Symbols {
					if s.Name == t.name && s.Signature == "" {
						s.Signature = sig
						break
					}
				}
			}
			mu.Unlock()
		}(t)
	}
	wg.Wait()
}

func cleanHoverSignature(raw string) string {
	lines := strings.Split(raw, "\n")
	var sig []string
	inFence := false
	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if inFence {
				break
			}
			inFence = true
			continue
		}
		if !inFence {
			continue
		}
		sig = append(sig, line)
	}
	if len(sig) == 0 {
		return strings.TrimSpace(raw)
	}
	return strings.TrimSpace(strings.Join(sig, "\n"))
}

func uriToNSKey(uri, root string) string {
	path := strings.TrimPrefix(uri, "file://")
	rel, err := filepath.Rel(root, filepath.FromSlash(path))
	if err != nil {
		return ""
	}
	dir := filepath.Dir(rel)
	nsKey := filepath.ToSlash(dir)
	if nsKey == "." {
		return nsRoot
	}
	return nsKey
}
