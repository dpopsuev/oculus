package locator

import (
	"strings"
	"testing"
)

func TestMatch_BareAndQualified(t *testing.T) {
	pool := []Hit{
		{Symbol: "Run", Component: "internal/core", File: "internal/core/main.go", Line: 10},
		{Symbol: "Run", Component: "cmd/app", File: "cmd/app/main.go", Line: 5},
		{Symbol: "Config", Component: "internal/core", File: "internal/core/config.go", Line: 3},
		{Symbol: "(*Engine).WarmLSP", Component: "engine", File: "engine/protocol.go", Line: 235},
	}

	p, _ := Parse("Config")
	hits := Match(p, pool)
	if len(hits) != 1 || hits[0].Symbol != "Config" {
		t.Fatalf("Config: got %+v", hits)
	}

	p, _ = Parse("Run")
	hits = Match(p, pool)
	if len(hits) != 2 {
		t.Fatalf("Run ambiguous: got %d", len(hits))
	}
	r := Resolve(p, pool)
	if r.Hit != nil || len(r.Candidates) != 2 || len(r.Escalations) == 0 {
		t.Fatalf("Resolve Run: hit=%v cands=%d esc=%v summary=%s", r.Hit, len(r.Candidates), r.Escalations, r.Summary)
	}

	p, _ = Parse("internal/core/main.go:Run")
	r = Resolve(p, pool)
	if r.Hit == nil || r.Hit.Component != "internal/core" {
		t.Fatalf("path:Symbol: %+v", r)
	}

	p, _ = Parse("engine/protocol.go:235:WarmLSP")
	r = Resolve(p, pool)
	if r.Hit == nil || r.Hit.Line != 235 {
		t.Fatalf("path:line:Symbol: %+v", r)
	}

	p, _ = Parse("Engine.WarmLSP")
	r = Resolve(p, pool)
	if r.Hit == nil || !strings.Contains(r.Hit.Symbol, "WarmLSP") {
		t.Fatalf("Parent.Symbol: %+v", r)
	}
}

func TestResolve_NotFound(t *testing.T) {
	p, _ := Parse("Missing")
	r := Resolve(p, nil)
	if r.Hit != nil || r.Summary == "" {
		t.Fatalf("%+v", r)
	}
}

func TestEscalations_PreferPathLine(t *testing.T) {
	p, _ := Parse("Run")
	hits := []Hit{
		{Symbol: "Run", Component: "internal/core", File: "internal/core/main.go", Line: 10},
		{Symbol: "Run", Component: "cmd/app", File: "cmd/app/main.go", Line: 5},
	}
	esc := Escalations(p, hits)
	for _, w := range []string{"internal/core/main.go:Run", "internal/core/main.go:10:Run", "cmd/app/main.go:5:Run"} {
		found := false
		for _, s := range esc {
			if s == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("escalations missing %q; got %v", w, esc)
		}
	}
}
