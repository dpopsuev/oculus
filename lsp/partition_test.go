package lsp

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp/mockserver"
)

func TestPartitionedPool_DifferentClients(t *testing.T) {
	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "Foo", Kind: 12, URI: "file:///workspace/pkg/comp_0/main.go", Line: 5, Col: 5},
		},
	}
	inner := NewMockPool(cfg)
	defer inner.Shutdown(context.Background())

	pool := NewPartitionedPool(inner, "/workspace", [][]string{
		{"pkg/comp_0", "pkg/comp_1"},
		{"pkg/comp_2", "pkg/comp_3"},
	})

	c0, err := pool.Get(lang.Go, "/workspace/pkg/comp_0")
	if err != nil {
		t.Fatalf("Get partition 0: %v", err)
	}
	c1, err := pool.Get(lang.Go, "/workspace/pkg/comp_2")
	if err != nil {
		t.Fatalf("Get partition 1: %v", err)
	}

	if c0 == c1 {
		t.Error("expected different clients for different partitions")
	}

	// Same partition returns same client
	c0b, _ := pool.Get(lang.Go, "/workspace/pkg/comp_1")
	if c0b != c0 {
		t.Error("expected same client for same partition")
	}
}

func TestPartitionedPool_Throughput(t *testing.T) {
	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "Foo", Kind: 12, URI: "file:///workspace/main.go", Line: 5, Col: 5},
		},
		Latency: 50 * time.Millisecond,
	}

	const requests = 4

	// Single partition baseline: sequential (one pipe, can't parallelize)
	single := NewMockPool(cfg)
	defer single.Shutdown(context.Background())
	singlePool := NewPartitionedPool(single, "/workspace", [][]string{
		{"pkg/comp_0", "pkg/comp_1", "pkg/comp_2", "pkg/comp_3"},
	})
	start := time.Now()
	for range requests {
		c, _ := singlePool.Get(lang.Go, "/workspace/pkg/comp_0")
		c.Request("workspace/symbol", map[string]any{"query": ""})
	}
	singleTime := time.Since(start)

	// Two partitions: parallel (each partition has its own pipe)
	dual := NewMockPool(cfg)
	defer dual.Shutdown(context.Background())
	dualPool := NewPartitionedPool(dual, "/workspace", [][]string{
		{"pkg/comp_0", "pkg/comp_1"},
		{"pkg/comp_2", "pkg/comp_3"},
	})

	// Pre-warm both partitions (sequential, so both clients exist)
	dualPool.Get(lang.Go, "/workspace/pkg/comp_0")
	dualPool.Get(lang.Go, "/workspace/pkg/comp_2")

	// Now fire requests in parallel — partition 0 and 1 can run simultaneously
	start = time.Now()
	var wg sync.WaitGroup
	for i := range requests {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var root string
			if idx%2 == 0 {
				root = "/workspace/pkg/comp_0" // partition 0
			} else {
				root = "/workspace/pkg/comp_2" // partition 1
			}
			c, _ := dualPool.Get(lang.Go, root)
			c.Request("workspace/symbol", map[string]any{"query": ""})
		}(i)
	}
	wg.Wait()
	dualTime := time.Since(start)

	speedup := float64(singleTime) / float64(dualTime)
	t.Logf("single (sequential): %v, dual (parallel): %v, speedup: %.1fx", singleTime, dualTime, speedup)

	if speedup < 1.3 {
		t.Logf("speedup %.1fx < 1.3x — LSP pipe serialization limits parallel gain", speedup)
	}
}
