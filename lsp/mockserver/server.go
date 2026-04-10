// Package mockserver provides a mock LSP server for testing.
// Responds to JSON-RPC over stdin/stdout with canned fixtures.
// Language-agnostic — works as a stand-in for gopls, pyright, or any LSP server.
package mockserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Symbol is a canned workspace symbol for the mock.
type Symbol struct {
	Name string `json:"name"`
	Kind int    `json:"kind"` // 12=function, 6=method
	URI  string `json:"uri"`
	Line int    `json:"line"`
	Col  int    `json:"col"`
}

// CallEdge is a canned outgoing call for the mock.
type CallEdge struct {
	FromName string
	ToName   string
	ToURI    string
	ToLine   int
	ToCol    int
}

// Config controls the mock server behavior.
type Config struct {
	Symbols  []Symbol   // workspace/symbol results
	Edges    []CallEdge // callHierarchy/outgoingCalls results
	Latency  time.Duration // artificial delay per response
}

// Serve runs the mock LSP server on the given reader/writer pair.
// Blocks until the reader is closed or exit is received.
func Serve(r io.Reader, w io.Writer, cfg Config) error {
	reader := bufio.NewReader(r)
	for {
		method, id, params, err := readRequest(reader)
		if err != nil {
			return nil // EOF = clean exit
		}

		if cfg.Latency > 0 {
			time.Sleep(cfg.Latency)
		}

		var result any
		switch method {
		case "initialize":
			result = map[string]any{
				"capabilities": map[string]any{
					"textDocumentSync": 1,
					"callHierarchyProvider": true,
					"workspaceSymbolProvider": true,
					"hoverProvider": true,
				},
			}
		case "initialized":
			continue // notification, no response
		case "shutdown":
			result = nil
		case "exit":
			return nil
		case "workspace/symbol":
			result = buildWorkspaceSymbols(cfg.Symbols)
		case "textDocument/prepareCallHierarchy":
			result = buildPrepareCallHierarchy(cfg.Symbols, params)
		case "callHierarchy/outgoingCalls":
			result = buildOutgoingCalls(cfg.Edges, params)
		case "textDocument/hover":
			result = map[string]any{
				"contents": map[string]any{"value": "func stub()"},
			}
		case "textDocument/documentSymbol":
			result = buildDocumentSymbols(cfg.Symbols, params)
		case "textDocument/didOpen":
			continue // notification
		default:
			result = nil
		}

		if id != nil {
			if err := writeResponse(w, id, result); err != nil {
				return err
			}
		}
	}
}

func readRequest(r *bufio.Reader) (method string, id any, params json.RawMessage, err error) {
	contentLen := -1
	for {
		line, readErr := r.ReadString('\n')
		if readErr != nil {
			return "", nil, nil, readErr
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLen, _ = strconv.Atoi(val)
		}
	}
	if contentLen < 0 {
		return "", nil, nil, fmt.Errorf("missing Content-Length")
	}
	body := make([]byte, contentLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return "", nil, nil, err
	}
	var msg struct {
		Method string          `json:"method"`
		ID     any             `json:"id"`
		Params json.RawMessage `json:"params"`
	}
	if json.Unmarshal(body, &msg) != nil {
		return "", nil, nil, fmt.Errorf("invalid JSON")
	}
	return msg.Method, msg.ID, msg.Params, nil
}

func writeResponse(w io.Writer, id any, result any) error {
	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	body, _ := json.Marshal(resp)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

func buildWorkspaceSymbols(symbols []Symbol) []map[string]any {
	result := make([]map[string]any, 0, len(symbols))
	for _, s := range symbols {
		result = append(result, map[string]any{
			"name": s.Name,
			"kind": s.Kind,
			"location": map[string]any{
				"uri": s.URI,
				"range": map[string]any{
					"start": map[string]int{"line": s.Line, "character": s.Col},
				},
			},
		})
	}
	return result
}

func buildPrepareCallHierarchy(symbols []Symbol, params json.RawMessage) []map[string]any {
	var p struct {
		Position struct {
			Line int `json:"line"`
		} `json:"position"`
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	json.Unmarshal(params, &p)

	for _, s := range symbols {
		if s.URI == p.TextDocument.URI && s.Line == p.Position.Line {
			return []map[string]any{{
				"name": s.Name,
				"kind": s.Kind,
				"uri":  s.URI,
				"range": map[string]any{
					"start": map[string]int{"line": s.Line, "character": s.Col},
					"end":   map[string]int{"line": s.Line, "character": s.Col + len(s.Name)},
				},
				"selectionRange": map[string]any{
					"start": map[string]int{"line": s.Line, "character": s.Col},
					"end":   map[string]int{"line": s.Line, "character": s.Col + len(s.Name)},
				},
			}}
		}
	}
	return nil
}

func buildOutgoingCalls(edges []CallEdge, params json.RawMessage) []map[string]any {
	var p struct {
		Item struct {
			Name string `json:"name"`
		} `json:"item"`
	}
	json.Unmarshal(params, &p)

	var result []map[string]any
	for _, e := range edges {
		if e.FromName == p.Item.Name {
			result = append(result, map[string]any{
				"to": map[string]any{
					"name": e.ToName,
					"kind": 12,
					"uri":  e.ToURI,
					"range": map[string]any{
						"start": map[string]int{"line": e.ToLine, "character": e.ToCol},
						"end":   map[string]int{"line": e.ToLine, "character": e.ToCol + len(e.ToName)},
					},
				},
				"fromRanges": []map[string]any{
					{"start": map[string]int{"line": 0, "character": 0}, "end": map[string]int{"line": 0, "character": 0}},
				},
			})
		}
	}
	return result
}

func buildDocumentSymbols(symbols []Symbol, params json.RawMessage) []map[string]any {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	json.Unmarshal(params, &p)

	var result []map[string]any
	for _, s := range symbols {
		if s.URI == p.TextDocument.URI {
			result = append(result, map[string]any{
				"name": s.Name,
				"kind": s.Kind,
				"range": map[string]any{
					"start": map[string]int{"line": s.Line, "character": s.Col},
					"end":   map[string]int{"line": s.Line + 5, "character": 0},
				},
				"selectionRange": map[string]any{
					"start": map[string]int{"line": s.Line, "character": s.Col},
					"end":   map[string]int{"line": s.Line, "character": s.Col + len(s.Name)},
				},
			})
		}
	}
	return result
}
