package survey

import (
	"os/exec"
	"path/filepath"

	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/model"
)

// AutoScanner selects the best available scanner for a project root.
//
// When Override is empty/"auto" (the default agent path), Scan runs a
// language inventory first pass (root markers + extension census). Multi-language
// roots use CompositeScanner; single-language roots use the registry scanner.
// Explicit Override values skip inventory and force that backend (escape hatch).
type AutoScanner struct {
	// Override forces a specific scanner backend. Valid values:
	// "auto" (default), "go", "packages", "lsp", "ctags", "rust",
	// "typescript", "python", "composite".
	Override string
	// LSPCmd overrides the LSP server command (e.g. "rust-analyzer").
	LSPCmd string
	// TSFileGranularity makes TypeScript and Rust scanners emit per-file
	// components instead of package/crate aggregates (MCP file_granularity).
	TSFileGranularity bool
}

func (s *AutoScanner) Scan(root string) (*model.Project, error) {
	if s.Override != "" && s.Override != "auto" {
		return s.resolve(root).Scan(root)
	}

	absRoot, _ := filepath.Abs(root)
	inv := lang.InventoryLanguages(absRoot)
	subs := discoverSubProjects(absRoot)

	// Multi-language (inventory) or multi-subproject layout → composite.
	if inv.IsMultiLanguage() || len(subs) > 1 {
		return (&CompositeScanner{TSFileGranularity: s.TSFileGranularity}).Scan(root)
	}

	if primary := inv.Primary(); primary != lang.Unknown {
		return s.scannerForLang(ToModelLanguage(primary), root).Scan(root)
	}

	return s.resolve(root).Scan(root)
}

func (s *AutoScanner) resolve(root string) Scanner {
	switch s.Override {
	case "go":
		return &GoScanner{}
	case "packages":
		return &PackagesScanner{Fallback: &GoScanner{}}
	case "lsp":
		cmd := s.LSPCmd
		if cmd == "" {
			lang := DetectLanguage(root)
			cmd = DefaultLSPServer(lang)
		}
		return &LSPScanner{ServerCmd: cmd}
	case "ctags":
		return &CtagsScanner{}
	case "rust":
		return &RustScanner{Granularity: s.granularity()}
	case "typescript":
		return &TypeScriptScanner{Granularity: s.granularity()}
	case "python":
		return &PythonScanner{}
	case "composite":
		return &CompositeScanner{TSFileGranularity: s.TSFileGranularity}
	}

	detected := DetectLanguage(root)

	// For languages with dedicated scanners, use the shared registry.
	if detected != model.LangUnknown {
		return s.scannerForLang(detected, root)
	}

	// Unknown language: try LSP, fall back to ctags.
	cmd := s.LSPCmd
	if cmd == "" {
		cmd = DefaultLSPServer(detected)
	}
	if cmd != "" {
		if _, err := exec.LookPath(splitFirst(cmd)); err == nil {
			return &LSPScanner{ServerCmd: cmd}
		}
	}
	return &CtagsScanner{}
}

func (s *AutoScanner) scannerForLang(l model.Language, root string) Scanner {
	if l == model.LangTypeScript && s.TSFileGranularity {
		return &TypeScriptScanner{Granularity: FileLevel}
	}
	if l == model.LangRust && s.TSFileGranularity {
		return &RustScanner{Granularity: FileLevel}
	}
	return ScannerFromRegistry(l, root)
}

// DetectLanguage inspects marker files in root to determine the project language.
func DetectLanguage(root string) model.Language {
	return ToModelLanguage(lang.DetectLanguage(root))
}

// DefaultLSPServer returns the conventional LSP server command for a language.
func DefaultLSPServer(l model.Language) string {
	return DefaultLSPServers[l]
}

func (s *AutoScanner) granularity() Granularity {
	if s.TSFileGranularity {
		return FileLevel
	}
	return DirLevel
}

func splitFirst(cmd string) string {
	for i, c := range cmd {
		if c == ' ' {
			return cmd[:i]
		}
	}
	return cmd
}
