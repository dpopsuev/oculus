package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestClientWriteMessage(t *testing.T) {
	var buf bytes.Buffer
	c := NewClient(strings.NewReader(""), &buf)

	err := c.writeMessage(JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
	})
	if err != nil {
		t.Fatalf("writeMessage: %v", err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, "Content-Length: ") {
		t.Errorf("output missing Content-Length header: %q", out)
	}
	if !strings.Contains(out, "\r\n\r\n") {
		t.Errorf("output missing header terminator: %q", out)
	}

	parts := strings.SplitN(out, "\r\n\r\n", 2)
	if len(parts) != 2 {
		t.Fatalf("unexpected format: %q", out)
	}
	var req JSONRPCRequest
	if err := json.Unmarshal([]byte(parts[1]), &req); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if req.Method != "initialize" {
		t.Errorf("method = %q, want initialize", req.Method)
	}
}

func TestClientReadMessage(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"result":{"capabilities":{}}}`
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)

	c := NewClient(strings.NewReader(msg), nil)
	resp, err := c.readMessage()
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	if resp.ID == nil || *resp.ID != 1 {
		t.Errorf("id = %v, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %v", resp.Error)
	}
	if resp.Result == nil {
		t.Error("result is nil")
	}
}

func TestClientRoundTrip(t *testing.T) {
	respBody := `{"jsonrpc":"2.0","id":1,"result":{"serverInfo":{"name":"test"}}}`
	respMsg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(respBody), respBody)

	var reqBuf bytes.Buffer
	c := NewClient(strings.NewReader(respMsg), &reqBuf)

	result, err := c.Request("initialize", map[string]any{"processId": 1})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}

	if !strings.Contains(string(result), "test") {
		t.Errorf("result = %s, want to contain 'test'", result)
	}

	sent := reqBuf.String()
	if !strings.Contains(sent, "initialize") {
		t.Errorf("sent request missing method: %s", sent)
	}
}

func TestClientNotify(t *testing.T) {
	var buf bytes.Buffer
	c := NewClient(strings.NewReader(""), &buf)

	err := c.Notify("initialized", struct{}{})
	if err != nil {
		t.Fatalf("Notify: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "initialized") {
		t.Errorf("notification missing method: %s", out)
	}
	if strings.Contains(out, `"id"`) {
		t.Errorf("notification should not have an id: %s", out)
	}
}

func TestClientReadError(t *testing.T) {
	body := `{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"invalid request"}}`
	msg := fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(body), body)

	c := NewClient(strings.NewReader(msg), nil)
	resp, err := c.readMessage()
	if err != nil {
		t.Fatalf("readMessage: %v", err)
	}
	if resp.Error == nil {
		t.Fatal("expected error in response")
	}
	if resp.Error.Code != -32600 {
		t.Errorf("error code = %d, want -32600", resp.Error.Code)
	}
}

// TestClient_RequestAfterReaderDeathReturnsServerDead is the regression test
// for PIV-BUG-2 (zombie tsserver processes).
//
// The race that creates zombies:
//
//  1. LSP server exits → stdout EOF → reader goroutine closes all *existing*
//     pending channels and exits.
//  2. shutdownEntry (or lspConn.shutdown) creates a *new* Request, storing a
//     fresh pending channel that the now-dead reader goroutine can never reach.
//  3. RequestContext blocks on that channel with context.Background() → hangs
//     forever → cmd.Wait() is never called → process stays zombie.
//
// The test simulates the exact window:
//   - stdinR is kept open so the write to stdinW succeeds (the server's stdin
//     read end is still alive, as happens when tsserver has spawned children that
//     inherited the fd, or during the brief OS-close race).
//   - pw (server stdout) is closed → reader goroutine processes EOF and exits.
//   - A new RequestContext call is then made; the orphaned channel never fires.
//
// WANT: ErrServerDead returned immediately upon detecting the dead reader.
// GOT (bug): context.DeadlineExceeded after the timeout — request hung.
func TestClient_RequestAfterReaderDeathReturnsServerDead(t *testing.T) {
	// pr/pw: server stdout pipe.  Client reads from pr; we ("server") write to pw.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	// stdinR/stdinW: server stdin pipe.  Client writes to stdinW; stdinR is kept
	// open so that writes succeed — this simulates the race window where the
	// server process has exited but something (a child process, OS close-order)
	// still holds the read end, letting our write go into the kernel buffer
	// without an immediate EPIPE.
	stdinR, stdinW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdinR.Close()
	defer stdinW.Close()

	c := NewClient(pr, stdinW)

	// Trigger the lazy reader goroutine.
	c.startReader()

	// Simulate server exit: closing pw makes our reader goroutine read EOF,
	// close all *existing* pending channels, and exit.
	pw.Close()

	// Give the reader goroutine time to process the EOF, drain pending, and exit.
	// After this sleep the reader is dead but no new reader will start
	// (readerOnce already fired).
	time.Sleep(100 * time.Millisecond)

	// Now call RequestContext — this is what shutdownEntry does.
	// A new pending channel is stored; the dead reader goroutine can never
	// dispatch to it.  With context.Background() the call would block forever.
	// We use a short timeout to let the test fail fast and show the error.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err = c.RequestContext(ctx, "shutdown", nil)

	// WANT: ErrServerDead returned immediately (reader-death detected).
	// GOT (bug): context.DeadlineExceeded after 500 ms — hung on orphaned channel.
	if !errors.Is(err, ErrServerDead) {
		t.Errorf(
			"RequestContext after reader death: got %v, want ErrServerDead\n"+
				"The request hung on an orphaned pending channel instead of detecting "+
				"the dead reader immediately. This is the mechanism that produces zombie "+
				"tsserver processes (PIV-BUG-2): cmd.Wait() is never reached because "+
				"lspConn.shutdown / shutdownEntry calls Request(\"shutdown\", "+
				"context.Background()) after the reader goroutine has already exited.",
			err,
		)
	}
}

func TestClientMissingContentLength(t *testing.T) {
	msg := "Content-Type: application/json\r\n\r\n{}"
	c := NewClient(strings.NewReader(msg), nil)
	_, err := c.readMessage()
	if err == nil {
		t.Fatal("expected error for missing Content-Length")
	}
}
