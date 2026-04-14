package triage_test

import (
	"testing"

	"github.com/dpopsuev/oculus/v3/triage"
)

func TestNew_External(t *testing.T) {
	r := triage.New()
	if r == nil {
		t.Fatal("nil registry")
	}
}
