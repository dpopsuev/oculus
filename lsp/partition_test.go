package lsp

import (
	"context"
	"testing"
	"time"

	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp/mockserver"
)

func TestPartitionedPool_DifferentClients(t *testing.T) {
	cfg := mockserver.Config{
		Symbols: []mockserver.Symbol{
			{Name: "Foo", Kind: 12, URI: "file:///workspace/part0/main.go", Line: 5, Col: 5},
			{Name: "Bar", Kind: 12, URI: "file:///workspace/part1/main.go", Line: 5, Col: 5},
		},
	}
	inner := NewMockPool(cfg)
	defer inner.Shutdown(context.Background())

	pool := NewPartitionedPool(inner, "/workspace", [][]string{
		{"pkg/comp_0", "pkg/comp_1"}, // partition 0
		{"pkg/comp_2", "pkg/comp_3"}, // partition 1
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
		Latency: 50 * time.Millisecond, // artificial delay
	}

	// Single partition baseline
	single := NewMockPool(cfg)
	singlePool := NewPartitionedPool(single, "/workspace", [][]string{
		{"pkg/comp_0", "pkg/comp_1", "pkg/comp_2", "pkg/comp_3"},
	})
	start := time.Now()
	for i := 0; i < 4; i++ {
		c, _ := singlePool.Get(lang.Go, "/workspace")
		c.Request("workspace/symbol", map[string]any{"query": ""})
	}
	singleTime := time.Since(start)
	single.Shutdown(context.Background())

	// Two partitions — should be ~2x faster if parallel
	dual := NewMockPool(cfg)
	dualPool := NewPartitionedPool(dual, "/workspace", [][]string{
		{"pkg/comp_0", "pkg/comp_1"},
		{"pkg/comp_2", "pkg/comp_3"},
	})
	start = time.Now()
	for i := 0; i < 4; i++ {
		c, _ := dualPool.Get(lang.Go, "/workspace")
		c.Request("workspace/symbol", map[string]any{"query": ""})
	}
	dualTime := time.Since(start)
	dual.Shutdown(context.Background())

	t.Logf("single: %v, dual: %v, speedup: %.1fx", singleTime, dualTime, float64(singleTime)/float64(dualTime))
}
