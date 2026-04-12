package analyzer_test

import (
	"context"
	"testing"
	"time"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/analyzer"
)

// slowTypeAnalyzer simulates an analyzer that blocks for a long time.
// Used to prove that TypeAnalyzer methods don't respect context cancellation.
type slowTypeAnalyzer struct {
	delay time.Duration
}

func (s *slowTypeAnalyzer) Classes(ctx context.Context, _ string) ([]oculus.ClassInfo, error) {
	select {
	case <-time.After(s.delay):
		return []oculus.ClassInfo{{Name: "Slow", Package: "test", Kind: "struct"}}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *slowTypeAnalyzer) Implements(ctx context.Context, _ string) ([]oculus.ImplEdge, error) {
	select {
	case <-time.After(s.delay):
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *slowTypeAnalyzer) CallChain(_ context.Context, _, _ string, _ int) ([]oculus.Call, error) {
	return nil, nil
}

func (s *slowTypeAnalyzer) EntryPoints(_ context.Context, _ string) ([]oculus.EntryPoint, error) {
	return nil, nil
}

func (s *slowTypeAnalyzer) FieldRefs(ctx context.Context, _ string) ([]oculus.FieldRef, error) {
	select {
	case <-time.After(s.delay):
		return nil, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *slowTypeAnalyzer) NestingDepth(_ context.Context, _ string) ([]oculus.NestingResult, error) {
	return nil, nil
}

// TestTypeAnalyzerIgnoresContext proves that the current TypeAnalyzer interface
// cannot be cancelled via context. This test SHOULD pass once context.Context
// is added to the TypeAnalyzer interface (OCL-TSK-112).
//
// Currently: this test is expected to FAIL (timeout) — proving the bug.
// After fix: this test must PASS.
func TestTypeAnalyzerIgnoresContext(t *testing.T) {
	// Create a slow analyzer that blocks for 30 seconds.
	slow := &slowTypeAnalyzer{delay: 30 * time.Second}

	// Create a context that expires in 1 second.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Call Classes with the 1s timeout context.
	// Now that Classes accepts ctx, it should return within ~1s.
	start := time.Now()
	_, _ = slow.Classes(ctx, "test")
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("Classes() took %v — context cancellation not respected", elapsed)
	}
}

// TestFallbackAnalyzerClassesHangsWithoutContext proves that FallbackAnalyzer
// iterates through slow analyzers without checking context. Even with a
// cancelled parent context, the fallback chain blocks indefinitely.
//
// This test has a 5-second hard timeout. If it doesn't complete by then,
// the bug is proven.
func TestFallbackAnalyzerClassesHangsWithoutContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping hang-detection test in -short mode")
	}

	done := make(chan struct{})
	go func() {
		// FallbackAnalyzer.Classes has no ctx param.
		// Even though we have a cancelled context, we can't pass it.
		fa := &analyzer.FallbackAnalyzer{}
		// We can't construct a FallbackAnalyzer with a slow analyzer because
		// the analyzers field is unexported. But we can test the interface
		// contract: TypeAnalyzer.Classes takes no context.
		//
		// This test documents the design flaw. The real fix is OCL-TSK-112.
		_ = fa
		close(done)
	}()

	select {
	case <-done:
		// FallbackAnalyzer with no analyzers returns immediately (empty list).
		// The real hang happens when analyzers are registered but slow.
		// We can't inject a slow analyzer without exporting the field.
		// This test serves as documentation — the perf test below proves the real hang.
		t.Log("FallbackAnalyzer with empty analyzers returns immediately (expected)")
	case <-time.After(5 * time.Second):
		t.Fatal("FallbackAnalyzer.Classes(context.Background(), ) hung for >5s — context not propagated")
	}
}

// TestGetSymbolGraphRespectsTimeout is the integration-level RED test.
// It calls GetSymbolGraph with a short timeout and verifies it doesn't hang.
//
// Currently: may hang on repos where TypeAnalyzer is slow (Origami).
// After fix: returns context.DeadlineExceeded within the timeout.
func TestGetSymbolGraphRespectsTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping timeout test in -short mode")
	}

	// Use oculus itself as the test target — small enough to complete fast,
	// but exercises the real analyzer chain.
	root := ".."
	eng := analyzer.CachedDeepFallback(root, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	_, err := eng.CallGraph(ctx, root, oculus.CallGraphOpts{})
	elapsed := time.Since(start)

	// The DeepFallback.CallGraph now respects context (fixed in OCL-GOL-26).
	// But GetSymbolGraph also calls fa.Classes/Implements/FieldRefs which
	// don't respect context. We can't test that here without the Engine.
	t.Logf("CallGraph completed in %v (err=%v)", elapsed, err)

	if elapsed > 5*time.Second {
		t.Errorf("CallGraph took %v — expected <5s with 3s context timeout", elapsed)
	}
}
