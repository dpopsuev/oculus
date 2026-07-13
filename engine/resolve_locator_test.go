package engine

import (
	"context"
	"testing"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/model"
)

func testReportForLocator() *arch.ContextReport {
	r := testReportWithFilePaths()
	// Give Config an explicit line; add a Go-style method symbol.
	for i := range r.Architecture.Services {
		svc := &r.Architecture.Services[i]
		if svc.Name != "internal/core" {
			continue
		}
		for j := range svc.Symbols {
			if svc.Symbols[j].Name == "Config" {
				svc.Symbols[j].Line = 42
			}
			if svc.Symbols[j].Name == "Run" {
				svc.Symbols[j].Line = 10
			}
		}
		svc.Symbols = append(svc.Symbols, model.Symbol{
			Name:     "(*Engine).Warm",
			Exported: true,
			File:     "internal/core/engine.go",
			Line:     100,
		})
	}
	return r
}

func TestResolveLocator_UniquePathLine(t *testing.T) {
	store := newMockStore(testReportForLocator())
	eng := New(store, []string{"/tmp"})

	r, err := eng.ResolveLocator(context.Background(), "/tmp", "internal/core/config.go:42:Config")
	if err != nil {
		t.Fatal(err)
	}
	if r.Hit == nil {
		t.Fatalf("want unique hit, got %+v", r)
	}
	if r.Hit.Symbol != "Config" || r.Hit.Line != 42 {
		t.Errorf("hit=%+v", r.Hit)
	}
}

func TestResolveLocator_AmbiguousBare(t *testing.T) {
	store := newMockStore(testReportForLocator())
	eng := New(store, []string{"/tmp"})

	// Run exists in internal/core (and possibly only there with file paths —
	// cmd/app has "main" only). Use a name that appears once… add duplicate.
	// Config is unique; use Init which is only in core — make bare "Get" from store.
	r, err := eng.ResolveLocator(context.Background(), "/tmp", "Get")
	if err != nil {
		t.Fatal(err)
	}
	if r.Hit == nil {
		t.Fatalf("Get should be unique in store: %+v", r)
	}

	// Ambiguous: inject second Config
	rep := testReportForLocator()
	for i := range rep.Architecture.Services {
		if rep.Architecture.Services[i].Name == "internal/store" {
			rep.Architecture.Services[i].Symbols = append(rep.Architecture.Services[i].Symbols, model.Symbol{
				Name: "Config", Exported: true, File: "internal/store/cfg.go", Line: 1,
			})
		}
	}
	eng = New(newMockStore(rep), []string{"/tmp"})
	r, err = eng.ResolveLocator(context.Background(), "/tmp", "Config")
	if err != nil {
		t.Fatal(err)
	}
	if r.Hit != nil || len(r.Candidates) < 2 || len(r.Escalations) == 0 {
		t.Fatalf("want ambiguous Config: %+v", r)
	}
}

func TestResolveLocator_ParentMethod(t *testing.T) {
	store := newMockStore(testReportForLocator())
	eng := New(store, []string{"/tmp"})

	r, err := eng.ResolveLocator(context.Background(), "/tmp", "Engine.Warm")
	if err != nil {
		t.Fatal(err)
	}
	if r.Hit == nil {
		t.Fatalf("Engine.Warm: %+v", r)
	}
}

func TestResolveLocator_BadParse(t *testing.T) {
	eng, _ := newTestEngine()
	_, err := eng.ResolveLocator(context.Background(), "/tmp", "")
	if err == nil {
		t.Fatal("expected parse error")
	}
}
