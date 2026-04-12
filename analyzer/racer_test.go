package analyzer

import (
	"context"
	"testing"
	"time"
)

// TestRacer_FastestWins verifies the first non-empty result wins,
// even if a higher-quality attempt is still running.
func TestRacer_FastestWins(t *testing.T) {
	r := NewRacer(
		func(s string) bool { return s == "" },
		Attempt[string]{Name: "slow-lsp", Quality: QualityLSP, Fn: func(ctx context.Context) (string, error) {
			time.Sleep(500 * time.Millisecond)
			return "lsp-result", nil
		}},
		Attempt[string]{Name: "fast-ts", Quality: QualityTreeSitter, Fn: func(ctx context.Context) (string, error) {
			time.Sleep(10 * time.Millisecond)
			return "treesitter-result", nil
		}},
	)

	start := time.Now()
	result, err := r.Race(context.Background())
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Race: %v", err)
	}
	if result.Value != "treesitter-result" {
		t.Errorf("value = %q, want treesitter-result (fastest)", result.Value)
	}
	if result.Winner != "fast-ts" {
		t.Errorf("winner = %q, want fast-ts", result.Winner)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("elapsed %v > 200ms — should return as soon as fastest finishes", elapsed)
	}
}

// TestRacer_SlowHighQualityCaches verifies that a slower, higher-quality
// result is cached in background and returned on subsequent calls.
func TestRacer_SlowHighQualityCaches(t *testing.T) {
	r := NewRacer(
		func(s string) bool { return s == "" },
		Attempt[string]{Name: "slow-lsp", Quality: QualityLSP, Fn: func(ctx context.Context) (string, error) {
			time.Sleep(100 * time.Millisecond)
			return "lsp-result", nil
		}},
		Attempt[string]{Name: "fast-ts", Quality: QualityTreeSitter, Fn: func(ctx context.Context) (string, error) {
			time.Sleep(10 * time.Millisecond)
			return "treesitter-result", nil
		}},
	)

	// First call: fast-ts wins.
	result1, _ := r.Race(context.Background())
	if result1.Quality != QualityTreeSitter {
		t.Errorf("first call quality = %d, want %d (TreeSitter)", result1.Quality, QualityTreeSitter)
	}

	// Wait for slow-lsp to finish and cache.
	time.Sleep(200 * time.Millisecond)

	// Second call: should return cached LSP result (higher quality).
	result2, _ := r.Race(context.Background())
	if result2.Quality != QualityLSP {
		t.Errorf("second call quality = %d, want %d (LSP cached)", result2.Quality, QualityLSP)
	}
	if !result2.Cached {
		t.Error("second call should be cached")
	}
	if result2.Value != "lsp-result" {
		t.Errorf("cached value = %q, want lsp-result", result2.Value)
	}
}

// TestRacer_MetadataCorrect verifies winner name, quality, and elapsed are set.
func TestRacer_MetadataCorrect(t *testing.T) {
	r := NewRacer(
		func(n int) bool { return n == 0 },
		Attempt[int]{Name: "counter", Quality: QualityGoAST, Fn: func(ctx context.Context) (int, error) {
			return 42, nil
		}},
	)

	result, err := r.Race(context.Background())
	if err != nil {
		t.Fatalf("Race: %v", err)
	}
	if result.Value != 42 {
		t.Errorf("value = %d, want 42", result.Value)
	}
	if result.Winner != "counter" {
		t.Errorf("winner = %q, want counter", result.Winner)
	}
	if result.Quality != QualityGoAST {
		t.Errorf("quality = %d, want %d", result.Quality, QualityGoAST)
	}
	if result.Elapsed == 0 {
		t.Error("elapsed should be > 0")
	}
}

// TestRacer_ContextCancel returns quickly when context is cancelled.
func TestRacer_ContextCancel(t *testing.T) {
	r := NewRacer(
		func(s string) bool { return s == "" },
		Attempt[string]{Name: "slow", Quality: QualityLSP, Fn: func(ctx context.Context) (string, error) {
			select {
			case <-time.After(5 * time.Second):
				return "slow-result", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	result, _ := r.Race(ctx)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("took %v, should cancel quickly", elapsed)
	}
	if result.Value != "" {
		t.Errorf("expected empty result on cancel, got %q", result.Value)
	}
}

// TestRacer_Invalidate clears cache, forcing fresh race.
func TestRacer_Invalidate(t *testing.T) {
	callCount := 0
	r := NewRacer(
		func(n int) bool { return n == 0 },
		Attempt[int]{Name: "counter", Quality: QualityGoAST, Fn: func(ctx context.Context) (int, error) {
			callCount++
			return callCount, nil
		}},
	)

	// First call.
	r1, _ := r.Race(context.Background())
	if r1.Value != 1 {
		t.Errorf("first = %d, want 1", r1.Value)
	}

	// Second call — cached, same value.
	r2, _ := r.Race(context.Background())
	if r2.Value != 1 || !r2.Cached {
		t.Errorf("second = %d cached=%v, want 1/true", r2.Value, r2.Cached)
	}

	// Invalidate.
	r.Invalidate()
	if r.CachedQuality() != 0 {
		t.Error("cache should be empty after invalidate")
	}

	// Third call — fresh race.
	r3, _ := r.Race(context.Background())
	if r3.Value != 2 {
		t.Errorf("third = %d, want 2 (fresh call)", r3.Value)
	}
}

// TestRacer_AllEmpty returns zero value when no attempt produces data.
func TestRacer_AllEmpty(t *testing.T) {
	r := NewRacer(
		func(s string) bool { return s == "" },
		Attempt[string]{Name: "empty1", Quality: QualityRegex, Fn: func(ctx context.Context) (string, error) {
			return "", nil
		}},
		Attempt[string]{Name: "empty2", Quality: QualityTreeSitter, Fn: func(ctx context.Context) (string, error) {
			return "", nil
		}},
	)

	result, err := r.Race(context.Background())
	if err != nil {
		t.Fatalf("Race: %v", err)
	}
	if result.Value != "" {
		t.Errorf("value = %q, want empty", result.Value)
	}
}
