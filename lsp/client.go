// Package lsp provides a JSON-RPC 2.0 client for Language Server Protocol
// communication. Part of the Oculus symbol resolution library.
package lsp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

// ErrMissingContentLength is returned when an LSP message lacks the header.
var ErrMissingContentLength = errors.New("missing Content-Length header")

// ErrServerDead is returned when the LSP server process has exited or the
// pipe is broken. The pool should evict and respawn on this error.
var ErrServerDead = errors.New("lsp server dead")

// Client implements the JSON-RPC 2.0 transport for LSP communication
// over a stdin/stdout pipe pair. Thread-safe: a single reader goroutine
// dispatches responses by request ID. Writes are serialized via mutex.
type Client struct {
	w       io.Writer
	r       *bufio.Reader
	mu      sync.Mutex // protects nextID
	wmu     sync.Mutex // serializes writes
	nextID  int
	pending sync.Map   // id → chan *JSONRPCResponse
	readerOnce sync.Once
	readerErr  error
}

// NewClient creates an LSP client from reader/writer pairs (typically
// connected to an LSP server's stdin/stdout).
func NewClient(r io.Reader, w io.Writer) *Client {
	return &Client{
		w:      w,
		r:      bufio.NewReader(r),
		nextID: 1,
	}
}

// JSONRPCRequest is a JSON-RPC 2.0 request message.
type JSONRPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// JSONRPCResponse is a JSON-RPC 2.0 response message.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError represents an error in a JSON-RPC response.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *JSONRPCError) Error() string {
	return fmt.Sprintf("LSP error %d: %s", e.Code, e.Message)
}

// Request sends a JSON-RPC request and reads the response, skipping
// any interleaved notifications from the server.
func (c *Client) Request(method string, params any) (json.RawMessage, error) {
	return c.RequestContext(context.Background(), method, params)
}

// startReader launches the single reader goroutine that dispatches
// responses to waiting callers by request ID.
func (c *Client) startReader() {
	c.readerOnce.Do(func() {
		go func() {
			for {
				resp, err := c.readMessage()
				if err != nil {
					c.readerErr = err
					// Notify all pending callers
					c.pending.Range(func(key, value any) bool {
						ch := value.(chan *JSONRPCResponse)
						close(ch)
						c.pending.Delete(key)
						return true
					})
					return
				}
				if resp.ID == nil || resp.Method != "" {
					continue // skip notifications
				}
				if ch, ok := c.pending.LoadAndDelete(*resp.ID); ok {
					ch.(chan *JSONRPCResponse) <- resp
				}
			}
		}()
	})
}

// RequestContext sends a JSON-RPC request with context support.
// Writes are serialized. Reads are dispatched by the single reader goroutine.
func (c *Client) RequestContext(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.startReader()

	c.mu.Lock()
	id := c.nextID
	c.nextID++
	c.mu.Unlock()

	// Register pending response channel before writing
	ch := make(chan *JSONRPCResponse, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	// Serialized write
	writeDone := make(chan error, 1)
	go func() {
		writeDone <- c.writeMessage(req)
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			if errors.Is(err, io.ErrClosedPipe) || errors.Is(err, os.ErrClosed) {
				return nil, fmt.Errorf("lsp request %s: %w", method, ErrServerDead)
			}
			return nil, fmt.Errorf("lsp request %s: %w", method, err)
		}
	case <-ctx.Done():
		return nil, fmt.Errorf("lsp write %s: %w", method, ctx.Err())
	}

	// Wait for response dispatched by reader goroutine
	select {
	case resp, ok := <-ch:
		if !ok {
			// Reader goroutine died
			if c.readerErr != nil {
				return nil, fmt.Errorf("lsp response %s: %w", method, c.readerErr)
			}
			return nil, fmt.Errorf("lsp response %s: %w", method, ErrServerDead)
		}
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("lsp read %s: %w", method, ctx.Err())
	}
}

// Notify sends a JSON-RPC notification (no response expected).
func (c *Client) Notify(method string, params any) error {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.writeMessage(req)
}

func (c *Client) writeMessage(msg any) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(c.w, header); err != nil {
		return err
	}
	_, err = c.w.Write(body)
	return err
}

func (c *Client) readMessage() (*JSONRPCResponse, error) {
	contentLen := -1
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) {
				return nil, fmt.Errorf("%w: %w", ErrServerDead, err)
			}
			return nil, fmt.Errorf("reading header: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			val := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			contentLen, err = strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length %q: %w", val, err)
			}
		}
	}

	if contentLen < 0 {
		return nil, fmt.Errorf("%w: %w", ErrServerDead, ErrMissingContentLength)
	}

	body := make([]byte, contentLen)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, fmt.Errorf("reading body: %w", err)
	}

	var resp JSONRPCResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &resp, nil
}
