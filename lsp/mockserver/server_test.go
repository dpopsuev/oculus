package mockserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"testing"
)

// testClient is a minimal JSON-RPC client for testing — avoids importing lsp.Client.
type testClient struct {
	w      io.Writer
	r      *bufio.Reader
	nextID int
}

func newTestClient(r io.Reader, w io.Writer) *testClient {
	return &testClient{w: w, r: bufio.NewReader(r), nextID: 1}
}

func (c *testClient) request(method string, params any) (json.RawMessage, error) {
	id := c.nextID
	c.nextID++
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	body, _ := json.Marshal(msg)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	c.w.Write([]byte(header))
	c.w.Write(body)

	for {
		contentLen := -1
		for {
			line, err := c.r.ReadString('\n')
			if err != nil {
				return nil, err
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
			return nil, fmt.Errorf("missing Content-Length")
		}
		respBody := make([]byte, contentLen)
		io.ReadFull(c.r, respBody)
		var resp struct {
			ID     *int            `json:"id"`
			Result json.RawMessage `json:"result"`
		}
		json.Unmarshal(respBody, &resp)
		if resp.ID != nil && *resp.ID == id {
			return resp.Result, nil
		}
	}
}

func (c *testClient) notify(method string, params any) {
	msg := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	body, _ := json.Marshal(msg)
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	c.w.Write([]byte(header))
	c.w.Write(body)
}

func TestMockServer_Initialize(t *testing.T) {
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	cfg := Config{
		Symbols: []Symbol{
			{Name: "Foo", Kind: 12, URI: "file:///test/main.go", Line: 5, Col: 5},
		},
	}

	go func() {
		Serve(sr, sw, cfg)
		sw.Close()
	}()

	client := newTestClient(cr, cw)

	result, err := client.request("initialize", map[string]any{
		"processId": nil, "rootUri": "file:///test", "capabilities": map[string]any{},
	})
	if err != nil {
		t.Fatalf("initialize: %v", err)
	}
	var caps struct {
		Capabilities struct {
			CallHierarchyProvider bool `json:"callHierarchyProvider"`
		} `json:"capabilities"`
	}
	json.Unmarshal(result, &caps)
	if !caps.Capabilities.CallHierarchyProvider {
		t.Error("expected callHierarchyProvider=true")
	}

	result, err = client.request("workspace/symbol", map[string]any{"query": ""})
	if err != nil {
		t.Fatalf("workspace/symbol: %v", err)
	}
	var symbols []struct{ Name string `json:"name"` }
	json.Unmarshal(result, &symbols)
	if len(symbols) != 1 || symbols[0].Name != "Foo" {
		t.Errorf("expected [Foo], got %v", symbols)
	}

	client.request("shutdown", nil)
	client.notify("exit", nil)
	cw.Close()
	t.Log("mock LSP server: OK")
}

func TestMockServer_CallHierarchy(t *testing.T) {
	sr, cw := io.Pipe()
	cr, sw := io.Pipe()

	cfg := Config{
		Symbols: []Symbol{
			{Name: "Main", Kind: 12, URI: "file:///test/main.go", Line: 10, Col: 5},
			{Name: "Helper", Kind: 12, URI: "file:///test/helper.go", Line: 3, Col: 5},
		},
		Edges: []CallEdge{
			{FromName: "Main", ToName: "Helper", ToURI: "file:///test/helper.go", ToLine: 3, ToCol: 5},
		},
	}

	go func() {
		Serve(sr, sw, cfg)
		sw.Close()
	}()

	client := newTestClient(cr, cw)
	client.request("initialize", map[string]any{"processId": nil, "rootUri": "file:///test", "capabilities": map[string]any{}})
	client.notify("initialized", struct{}{})

	result, err := client.request("textDocument/prepareCallHierarchy", map[string]any{
		"textDocument": map[string]string{"uri": "file:///test/main.go"},
		"position":     map[string]int{"line": 10, "character": 5},
	})
	if err != nil {
		t.Fatalf("prepareCallHierarchy: %v", err)
	}
	var items []struct{ Name string `json:"name"` }
	json.Unmarshal(result, &items)
	if len(items) != 1 || items[0].Name != "Main" {
		t.Errorf("expected [Main], got %v", items)
	}

	result, err = client.request("callHierarchy/outgoingCalls", map[string]any{
		"item": map[string]any{"name": "Main"},
	})
	if err != nil {
		t.Fatalf("outgoingCalls: %v", err)
	}
	var outs []struct{ To struct{ Name string `json:"name"` } `json:"to"` }
	json.Unmarshal(result, &outs)
	if len(outs) != 1 || outs[0].To.Name != "Helper" {
		t.Errorf("expected [{Helper}], got %v", outs)
	}

	client.request("shutdown", nil)
	client.notify("exit", nil)
	cw.Close()
	t.Log("mock LSP call hierarchy: OK")
}
