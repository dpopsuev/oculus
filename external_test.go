package oculus_test

import (
	"testing"

	"github.com/dpopsuev/oculus"
)

func TestNewFallback_External(t *testing.T) {
	fa := oculus.NewFallback(t.TempDir(), nil)
	if fa == nil {
		t.Fatal("nil fallback analyzer")
	}
}
