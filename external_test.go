package oculus_test

import (
	"testing"

	"github.com/dpopsuev/oculus/analyzer"
)

func TestNewFallback_External(t *testing.T) {
	fa := analyzer.NewFallback(t.TempDir(), nil)
	if fa == nil {
		t.Fatal("nil fallback analyzer")
	}
}
