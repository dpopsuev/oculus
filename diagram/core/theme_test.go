package core

import (
	"strings"
	"testing"
)

func TestClassifyHealth(t *testing.T) {
	tests := []struct {
		fanIn, churn int
		want         Health
	}{
		{0, 0, Healthy},
		{2, 7, Healthy},
		{SickFanIn, SickChurn, Sick},
		{4, 10, Sick},
		{FatalFanIn, FatalChurn, Fatal},
		{10, 30, Fatal},
	}
	for _, tt := range tests {
		got := ClassifyHealth(tt.fanIn, tt.churn)
		if got != tt.want {
			t.Errorf("ClassifyHealth(%d, %d) = %d, want %d", tt.fanIn, tt.churn, got, tt.want)
		}
	}
}

func TestThemeColorResolve(t *testing.T) {
	tc := ThemeColor{Dark: "#111", Light: "#222", Natural: "#333"}
	tests := []struct {
		mode, want string
	}{
		{"dark", "#111"},
		{"light", "#222"},
		{ThemeNatural, "#333"},
		{"", "#333"},          // default falls to natural
		{"unknown", "#333"},   // unknown falls to natural
	}
	for _, tt := range tests {
		got := tc.Resolve(tt.mode)
		if got != tt.want {
			t.Errorf("Resolve(%q) = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestDefaultTheme(t *testing.T) {
	theme := DefaultTheme()
	if len(theme.Colors) == 0 {
		t.Fatal("DefaultTheme has no colors")
	}
	if len(theme.Shapes) == 0 {
		t.Fatal("DefaultTheme has no shapes")
	}
	if len(theme.Diagrams) == 0 {
		t.Fatal("DefaultTheme has no diagrams")
	}
	// Verify expected color keys exist
	for _, key := range []string{"green", "yellow", "red", "blue", "surface", "text", "muted", "canvas"} {
		if _, ok := theme.Colors[key]; !ok {
			t.Errorf("DefaultTheme missing color %q", key)
		}
	}
	// Verify expected shape keys exist
	for _, key := range []string{"healthy", "sick", "fatal", "component", "entry"} {
		if _, ok := theme.Shapes[key]; !ok {
			t.Errorf("DefaultTheme missing shape %q", key)
		}
	}
}

func TestThemeResolve(t *testing.T) {
	theme := DefaultTheme()

	for _, mode := range []string{"dark", "light", ThemeNatural, ""} {
		rt := theme.Resolve(mode)
		if rt == nil {
			t.Fatalf("Resolve(%q) returned nil", mode)
		}
		if len(rt.ResolvedColors) == 0 {
			t.Errorf("Resolve(%q) has no resolved colors", mode)
		}
		if len(rt.ShapeHex) == 0 {
			t.Errorf("Resolve(%q) has no resolved shapes", mode)
		}
	}

	// Empty mode defaults to natural
	rt := theme.Resolve("")
	if rt.Mode != ThemeNatural {
		t.Errorf("Resolve('') mode = %q, want %q", rt.Mode, ThemeNatural)
	}
}

func TestResolvedThemeClassDefs(t *testing.T) {
	rt := DefaultTheme().Resolve(ThemeNatural)
	defs := rt.ClassDefs()
	if defs == "" {
		t.Fatal("ClassDefs returned empty string")
	}
	if !strings.Contains(defs, "classDef") {
		t.Error("ClassDefs missing 'classDef' directive")
	}
	if !strings.Contains(defs, "healthy") {
		t.Error("ClassDefs missing 'healthy' class")
	}
	if !strings.Contains(defs, "fill:") {
		t.Error("ClassDefs missing 'fill:' property")
	}
}

func TestResolvedThemeInitDirective(t *testing.T) {
	rt := DefaultTheme().Resolve(ThemeNatural)
	directive := rt.InitDirective()
	if !strings.Contains(directive, "init") {
		t.Error("InitDirective missing 'init'")
	}
	if !strings.Contains(directive, "theme") {
		t.Error("InitDirective missing 'theme'")
	}
	if !strings.Contains(directive, "primaryColor") {
		t.Error("InitDirective missing 'primaryColor'")
	}
}

func TestResolvedThemeHealthClass(t *testing.T) {
	rt := DefaultTheme().Resolve(ThemeNatural)
	tests := []struct {
		health Health
		want   string
	}{
		{Healthy, "healthy"},
		{Sick, "sick"},
		{Fatal, "fatal"},
	}
	for _, tt := range tests {
		if got := rt.HealthClass(tt.health); got != tt.want {
			t.Errorf("HealthClass(%d) = %q, want %q", tt.health, got, tt.want)
		}
	}
}

func TestResolvedThemeNodeSuffix(t *testing.T) {
	rt := DefaultTheme().Resolve(ThemeNatural)
	suffix := rt.NodeSuffix(Fatal)
	if suffix != ":::fatal" {
		t.Errorf("NodeSuffix(Fatal) = %q, want %q", suffix, ":::fatal")
	}
}

func TestResolvedThemeColorHex(t *testing.T) {
	rt := DefaultTheme().Resolve(ThemeNatural)
	hex := rt.ColorHex("green")
	if hex == "" {
		t.Error("ColorHex('green') returned empty")
	}
	if !strings.HasPrefix(hex, "#") {
		t.Errorf("ColorHex('green') = %q, want hex starting with #", hex)
	}
}
