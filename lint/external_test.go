package lint_test

import (
	"testing"

	"github.com/dpopsuev/oculus/arch"
	"github.com/dpopsuev/oculus/lint"
)

func TestRun_External(t *testing.T) {
	report := &arch.ContextReport{ScanCore: arch.ScanCore{Architecture: arch.ArchModel{}}}
	result := lint.Run(report, lint.RunOpts{})
	if result == nil {
		t.Fatal("nil result")
	}
}
