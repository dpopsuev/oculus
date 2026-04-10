package lsp

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dpopsuev/oculus/lang"
)

// PartitionedPool wraps an inner Pool and dispatches Get() calls to
// partition-specific roots. Each partition maps a set of component
// prefixes to a virtual root, so the inner pool spawns separate LSP
// server instances per partition. N partitions = N pipes = N× throughput.
//
// Language-agnostic: works with any LSP server (gopls, pyright, etc.).
type PartitionedPool struct {
	inner      Pool
	workspace  string
	partitions []partition
}

type partition struct {
	prefixes []string // component path prefixes (e.g., "pkg/comp_0")
	root     string   // partition root path
}

// NewPartitionedPool creates a pool that dispatches to N partitions.
// groups[i] contains the component path prefixes for partition i.
// Each partition gets its own LSP server via the inner pool.
func NewPartitionedPool(inner Pool, workspace string, groups [][]string) *PartitionedPool {
	parts := make([]partition, len(groups))
	for i, g := range groups {
		// Each partition uses a unique root so the inner pool spawns a separate server.
		root := filepath.Join(workspace, fmt.Sprintf(".lsp-part-%d", i))
		// Sort prefixes longest-first for greedy matching.
		sorted := make([]string, len(g))
		copy(sorted, g)
		sort.Slice(sorted, func(a, b int) bool { return len(sorted[a]) > len(sorted[b]) })
		parts[i] = partition{prefixes: sorted, root: root}
	}
	return &PartitionedPool{inner: inner, workspace: workspace, partitions: parts}
}

// Get resolves which partition a path belongs to and returns the
// corresponding LSP client from the inner pool.
func (p *PartitionedPool) Get(language lang.Language, root string) (*Client, error) {
	absRoot, _ := filepath.Abs(root)
	rel, err := filepath.Rel(p.workspace, absRoot)
	if err != nil {
		rel = absRoot
	}
	rel = filepath.ToSlash(rel)

	// Find matching partition by component prefix.
	for i := range p.partitions {
		for _, prefix := range p.partitions[i].prefixes {
			if rel == prefix || strings.HasPrefix(rel, prefix+"/") || rel == "." {
				return p.inner.Get(language, p.partitions[i].root)
			}
		}
	}

	// Default: first partition (workspace root).
	if len(p.partitions) > 0 {
		return p.inner.Get(language, p.partitions[0].root)
	}
	return p.inner.Get(language, root)
}

// Release delegates to the inner pool.
func (p *PartitionedPool) Release(language lang.Language, root string) {
	p.inner.Release(language, root)
}

// Shutdown delegates to the inner pool.
func (p *PartitionedPool) Shutdown(ctx context.Context) error {
	return p.inner.Shutdown(ctx)
}

// Status delegates to the inner pool.
func (p *PartitionedPool) Status() PoolStatus {
	return p.inner.Status()
}

// PartitionCount returns the number of partitions.
func (p *PartitionedPool) PartitionCount() int {
	return len(p.partitions)
}
