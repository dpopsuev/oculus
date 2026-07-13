package locator

import "testing"

func TestParse_Forms(t *testing.T) {
	tests := []struct {
		in      string
		path    string
		line    int
		name    string
		wantErr bool
	}{
		{"WarmLSP", "", 0, "WarmLSP", false},
		{"Engine.WarmLSP", "", 0, "Engine.WarmLSP", false},
		{"engine/protocol.go:WarmLSP", "engine/protocol.go", 0, "WarmLSP", false},
		{"engine/protocol.go:235:WarmLSP", "engine/protocol.go", 235, "WarmLSP", false},
		{"engine/protocol.go:235:Engine.WarmLSP", "engine/protocol.go", 235, "Engine.WarmLSP", false},
		{"Foo::bar", "", 0, "Foo.bar", false},
		{"pkg/foo.go:Foo::bar", "pkg/foo.go", 0, "Foo.bar", false},
		{"", "", 0, "", true},
		{":WarmLSP", "", 0, "", true},
		{"file.go:", "", 0, "", true},
		{"file.go:abc:WarmLSP", "", 0, "", true},
		{"a:b:c:d", "", 0, "", true},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Parse(%q) err=nil, want error", tt.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if got.Path != tt.path || got.Line != tt.line || got.Name != tt.name {
			t.Errorf("Parse(%q) = {Path:%q Line:%d Name:%q}, want {Path:%q Line:%d Name:%q}",
				tt.in, got.Path, got.Line, got.Name, tt.path, tt.line, tt.name)
		}
		if got.Raw != tt.in && tt.in != "Foo::bar" && tt.in != "pkg/foo.go:Foo::bar" {
			// Raw preserves input; :: forms keep original raw
		}
		if got.Raw != tt.in {
			t.Errorf("Parse(%q).Raw=%q, want %q", tt.in, got.Raw, tt.in)
		}
	}
}

func TestParsed_LeafParent(t *testing.T) {
	p, err := Parse("Engine.WarmLSP")
	if err != nil {
		t.Fatal(err)
	}
	if p.Leaf() != "WarmLSP" || p.Parent() != "Engine" {
		t.Errorf("Leaf=%q Parent=%q", p.Leaf(), p.Parent())
	}
	bare, _ := Parse("Run")
	if bare.Leaf() != "Run" || bare.Parent() != "" {
		t.Errorf("bare Leaf=%q Parent=%q", bare.Leaf(), bare.Parent())
	}
}
