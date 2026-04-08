package triage_test

import (
	"testing"

	"github.com/dpopsuev/oculus/triage"
)

func TestNew_External(t *testing.T) {
	r := triage.New()
	if r == nil {
		t.Fatal("nil registry")
	}
}
