// Package testcontainer provides a Docker-backed lsp.Pool for integration testing.
// LSP servers run inside containers, communicating via stdin/stdout (same transport
// as local servers). Workspaces are volume-mounted at their host path so URIs need
// no translation.
package testcontainer

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"
)

// DefaultImage is the Docker image name built from lsp/testcontainer/Dockerfile.
const DefaultImage = "oculus-lsp-test"

type poolKey struct {
	lang lang.Language
	root string
}

type containerEntry struct {
	client *lsp.Client
	cmd    *exec.Cmd
	stdin  io.WriteCloser
}

// ContainerPool implements lsp.Pool by running LSP servers in Docker containers.
type ContainerPool struct {
	image   string
	mu      sync.Mutex
	conns   map[poolKey]*containerEntry
	stopped bool
}

// NewPool creates a ContainerPool backed by the given Docker image.
func NewPool(image string) *ContainerPool {
	if image == "" {
		image = DefaultImage
	}
	return &ContainerPool{
		image: image,
		conns: make(map[poolKey]*containerEntry),
	}
}

// Available checks if Docker is installed and the test image exists.
func Available(image string) error {
	if image == "" {
		image = DefaultImage
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found: %w", err)
	}
	out, err := exec.Command("docker", "image", "inspect", image).CombinedOutput()
	if err != nil {
		return fmt.Errorf("image %s not found (run 'make docker-lsp'): %s", image, string(out))
	}
	return nil
}

// Get returns a warm LSP client running in a Docker container.
func (p *ContainerPool) Get(language lang.Language, root string) (*lsp.Client, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		absRoot = root
	}
	key := poolKey{lang: language, root: absRoot}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.stopped {
		return nil, lsp.ErrPoolShutDown
	}
	if entry, ok := p.conns[key]; ok {
		return entry.client, nil
	}

	entry, err := p.spawnContainer(language, absRoot)
	if err != nil {
		return nil, err
	}
	p.conns[key] = entry
	return entry.client, nil
}

// Release is a no-op; containers stay alive for reuse within the test.
func (p *ContainerPool) Release(lang.Language, string) {}

// Shutdown stops all running containers.
func (p *ContainerPool) Shutdown(_ context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.stopped = true
	for key, entry := range p.conns {
		shutdownContainer(entry)
		delete(p.conns, key)
	}
	return nil
}

// Status reports the pool state.
func (p *ContainerPool) Status() lsp.PoolStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	byLang := make(map[lang.Language]int)
	for key := range p.conns {
		byLang[key.lang]++
	}
	return lsp.PoolStatus{Active: len(p.conns), ByLang: byLang}
}

func (p *ContainerPool) spawnContainer(language lang.Language, absRoot string) (*containerEntry, error) {
	cmdStr := lang.DefaultLSPServer(language)
	if cmdStr == "" {
		return nil, fmt.Errorf("%w: %v", lsp.ErrNoLSPServer, language)
	}

	parts := strings.Fields(cmdStr)
	// Same-path mount: container sees files at the same absolute path as host.
	// This means all file:// URIs work transparently — no rewriting needed.
	// The :z suffix handles SELinux relabeling on systems with enforcing mode.
	args := []string{
		"run", "-i", "--rm",
		"-v", absRoot + ":" + absRoot + ":z",
		"-w", absRoot,
		p.image,
	}
	args = append(args, parts...)

	cmd := exec.Command("docker", args...)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("container stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("container stdout pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start container for %v: %w", language, err)
	}

	client := lsp.NewClient(stdout, stdin)
	if err := initializeLSP(client, absRoot); err != nil {
		stdin.Close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return nil, fmt.Errorf("container lsp initialize (%v): %w", language, err)
	}

	return &containerEntry{client: client, cmd: cmd, stdin: stdin}, nil
}

func initializeLSP(client *lsp.Client, root string) error {
	rootURI := "file://" + root
	params := map[string]any{
		"processId": nil,
		"rootUri":   rootURI,
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"documentSymbol": map[string]any{"hierarchicalDocumentSymbolSupport": true},
				"typeHierarchy":  map[string]any{},
				"callHierarchy":  map[string]any{},
				"implementation": map[string]any{},
				"hover":          map[string]any{},
			},
		},
	}
	if _, err := client.Request("initialize", params); err != nil {
		return err
	}
	return client.Notify("initialized", struct{}{})
}

func shutdownContainer(entry *containerEntry) {
	_, _ = entry.client.Request("shutdown", nil)
	_ = entry.client.Notify("exit", nil)
	entry.stdin.Close()

	done := make(chan struct{})
	go func() {
		_ = entry.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		if entry.cmd.Process != nil {
			_ = entry.cmd.Process.Kill()
		}
		<-done
	}
}
