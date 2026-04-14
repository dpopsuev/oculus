package lint_test

import (
	"context"
	"runtime"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/lint"
)

// oculusRoot is the path to the Oculus repo root relative to this file.
const oculusRoot = ".."

func scanSelf(tb testing.TB) *arch.ContextReport {
	tb.Helper()
	report, err := arch.ScanAndBuild(context.Background(), oculusRoot, arch.ScanOpts{
		Intent:       arch.IntentHealth,
		ExcludeTests: true,
	})
	if err != nil {
		tb.Fatalf("ScanAndBuild: %v", err)
	}
	return report
}

// BenchmarkLintRun benchmarks lint.Run on the Locus codebase itself.
func BenchmarkLintRun(b *testing.B) {
	report := scanSelf(b)
	b.ResetTimer()
	b.ReportAllocs()
	for b.Loop() {
		lint.Run(context.Background(), report, lint.RunOpts{Root: oculusRoot})
	}
}

// TestLintMemoryBudget verifies that lint.Run stays under a reasonable
// memory budget when processing the Locus codebase.
func TestLintMemoryBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping memory budget test in -short mode")
	}

	report := scanSelf(t)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	result := lint.Run(context.Background(), report, lint.RunOpts{Root: oculusRoot})

	runtime.ReadMemStats(&after)

	if result == nil {
		t.Fatal("Run returned nil")
	}

	// Budget: lint.Run should allocate less than 3 GB for a large codebase.
	// The scan includes class/impl analysis which dominates allocation.
	const budgetBytes = 3 * 1024 * 1024 * 1024
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("lint.Run allocated %d bytes (%.2f MB), violations: %d, score: %.1f",
		allocated, float64(allocated)/(1024*1024), len(result.Violations), result.Score)

	if allocated > budgetBytes {
		t.Errorf("lint.Run allocated %d bytes (%.2f MB), budget is %d bytes (%.0f MB)",
			allocated, float64(allocated)/(1024*1024), budgetBytes, float64(budgetBytes)/(1024*1024))
	}
}
