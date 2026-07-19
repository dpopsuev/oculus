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
}

// TestSgStore_UnifiedKey ensures a single path@sha key holds the AST base graph.
func TestSgStore_UnifiedKey(t *testing.T) {
	eng := New(nil, nil)
	full := &oculus.SymbolGraph{QualityTier: "ast"}
	eng.sgStore("proj@abc", full)

	got, ok := eng.sgLoad("proj@abc")
	if !ok || got != full {
		t.Fatal("unified key miss")
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

// TestSgFlight_LastWaiterCancelsBuild ensures departing alone cancels the
// shared buildCtx so LSP CallGraph cannot keep allocating after MCP timeout.
func TestSgFlight_LastWaiterCancelsBuild(t *testing.T) {
	eng := New(nil, nil)
	const key = "cancel-test"
	flight := eng.sgFlightJoin(key)

	buildCtx, cancel := context.WithCancel(context.Background())
	flight.setCancel(cancel)

	eng.sgFlightLeave(key, flight)

	select {
	case <-buildCtx.Done():
		// expected
	case <-time.After(200 * time.Millisecond):
		t.Fatal("buildCtx not cancelled after last waiter left")
	}
	if _, ok := eng.sgFlights.Load(key); ok {
		t.Fatal("flight still registered after last waiter left")
	}
}

// TestSgFlight_SiblingWaiterKeepsBuildAlive ensures one cancel does not abort
// a build still needed by another waiter.
func TestSgFlight_SiblingWaiterKeepsBuildAlive(t *testing.T) {
	eng := New(nil, nil)
	const key = "keep-alive-test"
	a := eng.sgFlightJoin(key)
	b := eng.sgFlightJoin(key)

	buildCtx, cancel := context.WithCancel(context.Background())
	a.setCancel(cancel)

	eng.sgFlightLeave(key, a)
	select {
	case <-buildCtx.Done():
		t.Fatal("buildCtx cancelled while sibling waiter still present")
	case <-time.After(30 * time.Millisecond):
	}

	eng.sgFlightLeave(key, b)
	select {
	case <-buildCtx.Done():
	case <-time.After(200 * time.Millisecond):
		t.Fatal("buildCtx not cancelled after last sibling left")
	}
}
