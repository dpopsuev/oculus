package arch_test

import (
	"context"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/internal/testfixture"
)

func BenchmarkScanAndBuild(b *testing.B) {
	root := testfixture.Repository(b, "go")
	b.ReportAllocs()
	for b.Loop() {
		_, err := arch.ScanAndBuild(context.Background(), root, arch.ScanOpts{
			Intent:       arch.IntentHealth,
			ExcludeTests: true,
		})
		if err != nil {
			b.Fatalf("ScanAndBuild: %v", err)
		}
	}
}
