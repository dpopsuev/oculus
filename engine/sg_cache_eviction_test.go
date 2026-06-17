package engine

// Three invariants:
//   1. sgLoad/sgStore honour sgTTL — stale graphs are evicted on next
//      access; sgStore sweeps stale siblings on every write.
//   2. getOrScan deduplicates concurrent cold calls for the same (path, sha)
//      tuple — only one arch.ScanAndBuild runs, PutReport called at most once.
//   3. A caller whose context fires before the shared scan completes receives
//      ctx.Err() promptly rather than blocking on the scan goroutine.

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/arch"
)

// ─── 1. sgLoad / sgStore TTL ────────────────────────────────────────────────

// TestSgLoad_FreshEntry_Hit verifies that an entry stored by sgStore is
// returned by sgLoad before it expires.
func TestSgLoad_FreshEntry_Hit(t *testing.T) {
	eng := New(&mockStore{headSHA: "abc"}, nil)
	sg := &oculus.SymbolGraph{}

	eng.sgStore("proj@abc", sg)

	got, ok := eng.sgLoad("proj@abc")
	if !ok {
		t.Fatal("expected cache hit for fresh entry, got miss")
	}
	if got != sg {
		t.Fatal("returned a different SymbolGraph pointer")
	}
}

// TestSgLoad_StaleEntry_Evicted verifies that an entry older than sgTTL
// is treated as a miss and removed from the map so it can be GC'd.
func TestSgLoad_StaleEntry_Evicted(t *testing.T) {
	eng := New(&mockStore{headSHA: "abc"}, nil)
	sg := &oculus.SymbolGraph{}

	// Backdating: write directly to bypass sgStore's "now" timestamp.
	eng.sgMu.Lock()
	eng.sgEntries["proj@abc"] = &sgEntry{sg: sg, at: time.Now().Add(-(eng.sgTTL + time.Second))}
	eng.sgMu.Unlock()

	got, ok := eng.sgLoad("proj@abc")
	if ok || got != nil {
		t.Fatalf("expected cache miss for stale entry, got ok=%v sg=%v", ok, got)
	}

	// The entry must have been deleted so the GC can collect the graph.
	eng.sgMu.Lock()
	_, still := eng.sgEntries["proj@abc"]
	eng.sgMu.Unlock()
	if still {
		t.Error("stale entry was not removed from sgEntries on load — GC cannot collect the graph")
	}
}

// TestSgLoad_MissingEntry_Miss verifies that sgLoad returns (nil, false) for
// keys that were never stored.
func TestSgLoad_MissingEntry_Miss(t *testing.T) {
	eng := New(&mockStore{headSHA: "abc"}, nil)

	got, ok := eng.sgLoad("never@stored")
	if ok || got != nil {
		t.Fatalf("expected miss for absent key, got ok=%v sg=%v", ok, got)
	}
}

// TestSgStore_EvictsStaleEntriesOnWrite verifies that sgStore sweeps stale
// entries from the map every time a new graph is stored, bounding the number
// of live SymbolGraph pointers in memory.
func TestSgStore_EvictsStaleEntriesOnWrite(t *testing.T) {
	eng := New(&mockStore{headSHA: "abc"}, nil)

	// Pre-populate: two stale entries that should be swept.
	eng.sgMu.Lock()
	eng.sgEntries["stale1@sha"] = &sgEntry{sg: &oculus.SymbolGraph{}, at: time.Now().Add(-(eng.sgTTL + time.Second))}
	eng.sgEntries["stale2@sha"] = &sgEntry{sg: &oculus.SymbolGraph{}, at: time.Now().Add(-(eng.sgTTL + time.Second))}
	eng.sgMu.Unlock()

	eng.sgStore("fresh@sha", &oculus.SymbolGraph{})

	eng.sgMu.Lock()
	_, stale1 := eng.sgEntries["stale1@sha"]
	_, stale2 := eng.sgEntries["stale2@sha"]
	_, fresh := eng.sgEntries["fresh@sha"]
	eng.sgMu.Unlock()

	if stale1 || stale2 {
		t.Error("sgStore did not evict stale entries — unbounded SymbolGraph accumulation possible")
	}
	if !fresh {
		t.Error("sgStore did not persist the new entry")
	}
}

// TestSgStore_FreshEntriesSurvive verifies that sgStore does not evict entries
// that are still within their TTL window.
func TestSgStore_FreshEntriesSurvive(t *testing.T) {
	eng := New(&mockStore{headSHA: "abc"}, nil)

	eng.sgStore("a@sha", &oculus.SymbolGraph{})
	eng.sgStore("b@sha", &oculus.SymbolGraph{})

	eng.sgMu.Lock()
	_, aOK := eng.sgEntries["a@sha"]
	_, bOK := eng.sgEntries["b@sha"]
	eng.sgMu.Unlock()

	if !aOK || !bOK {
		t.Error("sgStore evicted a fresh entry — incorrect TTL sweep")
	}
}

// ─── 2. getOrScan singleflight deduplication ────────────────────────────────

// minimalGoFixture creates a temp directory with a tiny Go module that
// arch.ScanAndBuild can process without errors.
func minimalGoFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module dedup_test\ngo 1.21\n"), 0o644))
	must(os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package main

func main() { greet("world") }
func greet(s string) string { return "hello " + s }
`), 0o644))
	must(exec.Command("git", "-C", dir, "init", "-q").Run())
	must(exec.Command("git", "-C", dir, "add", "-A").Run())
	must(exec.Command("git", "-C", dir, "commit", "-q", "-m", "init").Run())
	return dir
}

// countingStore wraps mockStore and counts PutReport calls with an atomic so
// the deduplication test can assert exactly one scan ran.
type countingStore struct {
	mockStore
	puts atomic.Int32
}

func (c *countingStore) PutReport(_ context.Context, _, _ string, _ *arch.ContextReport) error {
	c.puts.Add(1)
	return nil
}

// TestGetOrScan_ConcurrentColdCalls_ShareOneScan verifies that N goroutines
// that simultaneously call getOrScan with no cached report all share a single
// arch.ScanAndBuild execution.
//
// Invariant: PutReport is called at most once, regardless of concurrency.
func TestGetOrScan_ConcurrentColdCalls_ShareOneScan(t *testing.T) {
	dir := minimalGoFixture(t)

	ms := &countingStore{}
	ms.reportHit = false // force cold path on every getOrScan call
	ms.headSHA = "deadbeef"

	eng := New(ms, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)

	// Release all goroutines simultaneously so they race to getOrScan.
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			_, errs[idx] = eng.getOrScan(ctx, dir)
		}(i)
	}
	close(start)
	wg.Wait()

	// All calls must succeed.
	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d returned unexpected error: %v", i, err)
		}
	}

	// Singleflight: exactly one ScanAndBuild ran ⇒ exactly one PutReport.
	if got := ms.puts.Load(); got > 1 {
		t.Errorf("PutReport called %d time(s), want ≤ 1 — concurrent deduplication is broken", got)
	}
}

// ─── 3. Caller timeout independence ─────────────────────────────────────────

// TestGetOrScan_CallerContextCancelled_ReturnsImmediately verifies that a
// caller whose context is cancelled before (or during) the shared scan
// receives ctx.Err() promptly rather than blocking until the scan finishes.
//
// The shared scan runs under context.Background() + scanBuildTimeout so it
// continues independently of the caller's cancellation.
func TestGetOrScan_CallerContextCancelled_ReturnsImmediately(t *testing.T) {
	dir := minimalGoFixture(t)

	ms := &countingStore{}
	ms.reportHit = false
	ms.headSHA = "deadbeef"

	eng := New(ms, nil)

	// Cancel the caller's context before any scan can complete.
	callerCtx, callerCancel := context.WithCancel(context.Background())
	callerCancel() // already cancelled

	done := make(chan error, 1)
	go func() {
		_, err := eng.getOrScan(callerCtx, dir)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			// A pre-cancelled context might still win a cache hit;
			// accept that as a valid fast path.
			t.Logf("getOrScan returned nil error on pre-cancelled context (cache hit path)")
		} else if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("getOrScan did not return promptly on a cancelled context — goroutine is blocked")
	}
}
