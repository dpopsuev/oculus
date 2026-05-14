package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAsyncContextAt verifies that asyncContextAt returns the correct edge kind
// for async call sites, and "" for synchronous ones.
//
// Each sub-test writes a small source file, then asks asyncContextAt about the
// position of a specific call site.  Line/col are 1-based.
func TestAsyncContextAt(t *testing.T) {
	tests := []struct {
		name     string
		ext      string
		src      string
		line     int // 1-based line of the call site
		col      int // 1-based col of the call site
		wantKind string
	}{
		// Go ---------------------------------------------------------------
		{
			name: "go_named_goroutine",
			ext:  ".go",
			src: `package p
func run() {
	go produce(ch)
}
`,
			line: 3, col: 5, // "go" keyword
			wantKind: "goroutine",
		},
		{
			name: "go_anon_goroutine",
			ext:  ".go",
			src: `package p
func run() {
	go func() { _ = 1 }()
}
`,
			line: 3, col: 5, // inside go_statement
			wantKind: "goroutine",
		},
		{
			name: "go_channel_send",
			ext:  ".go",
			src: `package p
func produce(ch chan<- int) {
	ch <- 42
}
`,
			line: 3, col: 2, // "ch" in send_statement
			wantKind: "channel_send",
		},
		{
			name: "go_channel_recv_method",
			ext:  ".go",
			src: `package p
import "context"
func run(ctx context.Context) {
	<-ctx.Done()
}
`,
			line: 4, col: 2, // "<-" unary
			wantKind: "channel_recv",
		},
		{
			name: "go_sync_call",
			ext:  ".go",
			src: `package p
func run() {
	compute()
}
`,
			line: 3, col: 2,
			wantKind: "",
		},
		// TypeScript -------------------------------------------------------
		{
			name: "ts_await",
			ext:  ".ts",
			src: `async function run() {
  const x = await fetch("url");
}
`,
			line: 2, col: 13, // inside await_expression
			wantKind: "await_call",
		},
		{
			name: "ts_promise_chain",
			ext:  ".ts",
			src: `function run() {
  fetch("url").then(handleData);
}
`,
			line: 2, col: 16, // ".then"
			wantKind: "promise_chain",
		},
		{
			name: "ts_sync",
			ext:  ".ts",
			src: `function run() {
  compute();
}
`,
			line: 2, col: 3,
			wantKind: "",
		},
		// Python -----------------------------------------------------------
		{
			name: "python_await",
			ext:  ".py",
			src: `async def run():
    data = await fetch()
`,
			line: 2, col: 12, // inside await
			wantKind: "await_call",
		},
		{
			name: "python_sync",
			ext:  ".py",
			src: `def run():
    compute()
`,
			line: 2, col: 5,
			wantKind: "",
		},
		// Rust -------------------------------------------------------------
		{
			name: "rust_await",
			ext:  ".rs",
			src: `async fn run() {
    let x = fetch("url").await;
}
`,
			line: 2, col: 13, // inside await_expression
			wantKind: "await_call",
		},
		{
			name: "rust_spawn",
			ext:  ".rs",
			src: `async fn run() {
    tokio::spawn(async { work().await });
}
`,
			line: 2, col: 5, // inside call_expression with spawn
			wantKind: "task_spawn",
		},
		// Unknown extension ------------------------------------------------
		{
			name:     "unknown_ext",
			ext:      ".lua",
			src:      "local x = 1\n",
			line:     1, col: 1,
			wantKind: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "f"+tt.ext)
			if err := os.WriteFile(file, []byte(tt.src), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}

			// Clear the src cache between tests.
			srcCache.mu.Lock()
			delete(srcCache.m, file)
			srcCache.mu.Unlock()

			got := asyncContextAt(dir, file, tt.line, tt.col)
			if got != tt.wantKind {
				t.Errorf("asyncContextAt line=%d col=%d: got %q, want %q\nsrc:\n%s",
					tt.line, tt.col, got, tt.wantKind, tt.src)
			}
		})
	}
}

// TestAsyncContextAt_ZeroLine ensures zero line returns "" without panicking.
func TestAsyncContextAt_ZeroLine(t *testing.T) {
	got := asyncContextAt("/some/root", "file.go", 0, 0)
	if got != "" {
		t.Errorf("want \"\", got %q", got)
	}
}
