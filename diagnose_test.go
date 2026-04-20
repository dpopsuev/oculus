package oculus_test

import (
	"testing"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/book"
	"github.com/dpopsuev/oculus/v3/testkit"
)

func TestDiagnose_HighFanOut(t *testing.T) {
	sg := testkit.FixtureGraph()
	bg, err := book.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}

	// A has fan-out=2, probe it and verify book was queried.
	result := oculus.Diagnose(sg, bg, "pkg1.A")
	if result == nil {
		t.Fatal("expected non-nil DiagnoseResult")
	}
	if result.Probe == nil {
		t.Fatal("expected non-nil Probe in result")
	}
	if result.Probe.FQN != "pkg1.A" {
		t.Errorf("Probe.FQN = %q, want pkg1.A", result.Probe.FQN)
	}
	if result.Book == nil {
		t.Fatal("expected non-nil Book in result")
	}
	// A's kind is "function", so at minimum the book should be queried with "function".
	// The result should have at least one entry.
	if len(result.Book.Entries) == 0 {
		t.Error("expected at least one Book entry")
	}
}

func TestDiagnose_UnknownSymbol(t *testing.T) {
	sg := testkit.FixtureGraph()
	bg, err := book.LoadEmbedded()
	if err != nil {
		t.Fatal(err)
	}

	result := oculus.Diagnose(sg, bg, "pkg99.DoesNotExist")
	if result != nil {
		t.Error("expected nil for unknown symbol")
	}
}
