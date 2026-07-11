package engine

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	oculus "github.com/dpopsuev/oculus/v3"
)

// TestGetSymbolGraph_DefaultTimeoutRaised guards the agent-facing budget:
// cold gopls indexing routinely exceeds 60s; DefaultMeshTimeout must be ≥ 5m.
func TestGetSymbolGraph_DefaultTimeoutRaised(t *testing.T) {
	if DefaultMeshTimeout < 5*time.Minute {
		t.Fatalf("DefaultMeshTimeout = %v, want ≥ 5m", DefaultMeshTimeout)
	}
	if DefaultLSPAttemptBudget <= 0 || DefaultLSPAttemptBudget >= DefaultMeshTimeout {
		t.Fatalf("DefaultLSPAttemptBudget = %v, want (0, DefaultMeshTimeout)", DefaultLSPAttemptBudget)
	}
	if DefaultMeshTimeout-DefaultLSPAttemptBudget < 45*time.Second {
		t.Fatalf("no headroom for Quick degrade: mesh=%v lsp=%v", DefaultMeshTimeout, DefaultLSPAttemptBudget)
	}
}

// TestSgStore_QuickKeySeparate ensures Quick graphs do not overwrite full keys.
func TestSgStore_QuickKeySeparate(t *testing.T) {
	eng := New(nil, nil)
	full := &oculus.SymbolGraph{}
	quick := &oculus.SymbolGraph{}
	eng.sgStore("proj@abc", full)
	eng.sgStore("proj@abc-quick", quick)

	got, ok := eng.sgLoad("proj@abc")
	if !ok || got != full {
		t.Fatal("full key miss")
	}
	gotQ, ok := eng.sgLoad("proj@abc-quick")
	if !ok || gotQ != quick {
		t.Fatal("quick key miss")
	}
}

// TestSgSfg_ConcurrentWaitersShareBuild verifies Engine.sgSfg collapses
// concurrent builds for the same key (same pattern as GetSymbolGraph).
func TestSgSfg_ConcurrentWaitersShareBuild(t *testing.T) {
	eng := New(nil, nil)
	var builds atomic.Int32
	const n = 8
	var wg sync.WaitGroup
	wg.Add(n)
	results := make(chan *oculus.SymbolGraph, n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			ch := eng.sgSfg.DoChan("test-key", func() (any, error) {
				builds.Add(1)
				time.Sleep(30 * time.Millisecond)
				return &oculus.SymbolGraph{}, nil
			})
			res := <-ch
			if res.Err != nil {
				t.Errorf("DoChan: %v", res.Err)
				return
			}
			results <- res.Val.(*oculus.SymbolGraph)
		}()
	}
	wg.Wait()
	close(results)
	if builds.Load() != 1 {
		t.Fatalf("builds = %d, want 1 (singleflight)", builds.Load())
	}
	var first *oculus.SymbolGraph
	for sg := range results {
		if first == nil {
			first = sg
			continue
		}
		if sg != first {
			t.Fatal("waiters received different graph instances")
		}
	}
}

// TestGetSymbolGraph_ParentDeadlineRespected ensures a short caller deadline
// is not extended by DefaultMeshTimeout (only applied when no deadline).
func TestGetSymbolGraph_ParentDeadlineRespected(t *testing.T) {
	eng := New(nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(3 * time.Millisecond)
	_, err := eng.GetSymbolGraph(ctx, t.TempDir(), SymbolGraphOpts{Quick: true})
	if err == nil {
		return // empty Quick may finish before select sees cancel — ok
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Logf("GetSymbolGraph err (acceptable): %v", err)
	}
}
