package analyzer

import (
	"testing"
)

func TestParseSignatureTypes_Go(t *testing.T) {
	tests := []struct {
		sig     string
		params  []string
		returns []string
	}{
		{"func(x int, y string) (*Result, error)", []string{"int", "string"}, []string{"*Result", "error"}},
		{"func LoadConfig(path string) *Config", []string{"string"}, []string{"*Config"}},
		{"func main()", nil, nil},
	}
	for _, tt := range tests {
		params, returns := parseSignatureTypes(tt.sig)
		assertTypes(t, "Go", tt.sig, "params", tt.params, params)
		assertTypes(t, "Go", tt.sig, "returns", tt.returns, returns)
	}
}

func TestParseSignatureTypes_Python(t *testing.T) {
	tests := []struct {
		sig     string
		params  []string
		returns []string
	}{
		{"def load_config(path: str) -> dict", []string{"str"}, []string{"dict"}},
		{"def transform(cfg: dict) -> list", []string{"dict"}, []string{"list"}},
		{"def main()", nil, nil},
		{"def process(self, data: bytes) -> None", []string{"bytes"}, nil},
	}
	for _, tt := range tests {
		params, returns := parseSignatureTypes(tt.sig)
		assertTypes(t, "Python", tt.sig, "params", tt.params, params)
		assertTypes(t, "Python", tt.sig, "returns", tt.returns, returns)
	}
}

func TestParseSignatureTypes_TypeScript(t *testing.T) {
	tests := []struct {
		sig     string
		params  []string
		returns []string
	}{
		{"function loadConfig(path: string): Config", []string{"string"}, []string{"Config"}},
		{"function transform(cfg: Config): string", []string{"Config"}, []string{"string"}},
		{"function main(): void", nil, nil},
	}
	for _, tt := range tests {
		params, returns := parseSignatureTypes(tt.sig)
		assertTypes(t, "TypeScript", tt.sig, "params", tt.params, params)
		assertTypes(t, "TypeScript", tt.sig, "returns", tt.returns, returns)
	}
}

func TestParseSignatureTypes_Rust(t *testing.T) {
	tests := []struct {
		sig     string
		params  []string
		returns []string
	}{
		{"fn load_config(path: &str) -> Config", []string{"&str"}, []string{"Config"}},
		{"pub fn transform(cfg: &Config) -> String", []string{"&Config"}, []string{"String"}},
		{"fn main()", nil, nil},
		{"fn process(&self, data: Vec<u8>) -> Result<(), Error>", []string{"Vec<u8>"}, []string{"Result<(), Error>"}},
	}
	for _, tt := range tests {
		params, returns := parseSignatureTypes(tt.sig)
		assertTypes(t, "Rust", tt.sig, "params", tt.params, params)
		assertTypes(t, "Rust", tt.sig, "returns", tt.returns, returns)
	}
}

func TestParseSignatureTypes_CCpp(t *testing.T) {
	tests := []struct {
		sig     string
		params  []string
		returns []string
	}{
		{"Config loadConfig(const char* path)", []string{"const char*"}, []string{"Config"}},
		{"int transform(Config* cfg)", []string{"Config*"}, []string{"int"}},
		{"void main()", nil, nil},
	}
	for _, tt := range tests {
		params, returns := parseSignatureTypes(tt.sig)
		assertTypes(t, "C/C++", tt.sig, "params", tt.params, params)
		assertTypes(t, "C/C++", tt.sig, "returns", tt.returns, returns)
	}
}

func TestExtractSignatureFromHover_MultiLang(t *testing.T) {
	tests := []struct {
		name  string
		hover string
		want  string
	}{
		{
			"Go fenced",
			"```go\nfunc LoadConfig(path string) *Config\n```",
			"func LoadConfig(path string) *Config",
		},
		{
			"Python fenced",
			"```python\ndef load_config(path: str) -> dict\n```",
			"def load_config(path: str) -> dict",
		},
		{
			"TypeScript fenced",
			"```typescript\nfunction loadConfig(path: string): Config\n```",
			"function loadConfig(path: string): Config",
		},
		{
			"Rust fenced",
			"```rust\nfn load_config(path: &str) -> Config\n```",
			"fn load_config(path: &str) -> Config",
		},
		{
			"Rust pub fn",
			"```rust\npub fn load_config(path: &str) -> Config\n```",
			"pub fn load_config(path: &str) -> Config",
		},
		{
			"C fenced",
			"```c\nConfig load_config(const char* path)\n```",
			"Config load_config(const char* path)",
		},
		{
			"Pyright format",
			"(function) def load_config(path: str) -> dict",
			"def load_config(path: str) -> dict",
		},
		{
			"Plain Go",
			"func LoadConfig(path string) *Config",
			"func LoadConfig(path string) *Config",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractSignatureFromHover(tt.hover)
			if got != tt.want {
				t.Errorf("extractSignatureFromHover(%q)\n  got  %q\n  want %q", tt.hover, got, tt.want)
			}
		})
	}
}

func assertTypes(t *testing.T, lang, sig, kind string, want, got []string) {
	t.Helper()
	if len(want) == 0 && len(got) == 0 {
		return
	}
	if len(want) != len(got) {
		t.Errorf("[%s] parseSignatureTypes(%q) %s: got %v, want %v", lang, sig, kind, got, want)
		return
	}
	for i := range want {
		if want[i] != got[i] {
			t.Errorf("[%s] parseSignatureTypes(%q) %s[%d]: got %q, want %q", lang, sig, kind, i, got[i], want[i])
		}
	}
}
