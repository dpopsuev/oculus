package survey

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/model"
)

// LanguageSupport describes how to scan a particular language.
type LanguageSupport struct {
	Language       model.Language
	Markers        []string                  // project marker files (go.mod, Cargo.toml, etc.)
	ScannerFactory func(root string) Scanner // creates a scanner for this language
	SkipDirs       map[string]bool           // language-specific directories to skip
	Rules          lang.Rules                // per-language naming rules for symbol quality
}

var (
	langRegistry   = make(map[model.Language]*LanguageSupport)
	langRegistryMu sync.RWMutex
)

// Register adds a language to the scanner registry.
func Register(ls *LanguageSupport) {
	langRegistryMu.Lock()
	defer langRegistryMu.Unlock()
	langRegistry[ls.Language] = ls
}

// GetLanguageSupport returns the registered support for a language, or nil.
func GetLanguageSupport(lang model.Language) *LanguageSupport {
	langRegistryMu.RLock()
	defer langRegistryMu.RUnlock()
	return langRegistry[lang]
}

// DetectFromMarkers checks for project marker files and returns the detected language.
// It checks markers in registration order — first match wins.
func DetectFromMarkers(root string) model.Language {
	langRegistryMu.RLock()
	defer langRegistryMu.RUnlock()

	// Check in a deterministic order: Go first (most common in this project).
	order := []model.Language{
		model.LangGo, model.LangRust, model.LangPython, model.LangTypeScript,
		model.LangJava, model.LangKotlin, model.LangSwift, model.LangCSharp,
		model.LangZig, model.LangLua, model.LangC, model.LangCpp,
		model.LangJavaScript, model.LangProto, model.LangShell,
	}
	for _, lang := range order {
		ls, ok := langRegistry[lang]
		if !ok {
			continue
		}
		for _, marker := range ls.Markers {
			if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
				return lang
			}
		}
	}
	return model.LangUnknown
}

// ScannerFromRegistry creates a scanner for the given language using the registry.
// Falls back to CtagsScanner for unregistered languages.
func ScannerFromRegistry(lang model.Language, root string) Scanner {
	ls := GetLanguageSupport(lang)
	if ls == nil || ls.ScannerFactory == nil {
		return &CtagsScanner{}
	}
	return ls.ScannerFactory(root)
}

// lspWithFallback returns a scanner factory that prefers the LSP scanner
// when the named server binary is on PATH, falling back to the given scanner.
func lspWithFallback(serverCmd string, fallback Scanner) func(string) Scanner {
	return func(_ string) Scanner {
		if _, err := exec.LookPath(serverCmd); err == nil {
			return &LSPScanner{ServerCmd: serverCmd}
		}
		return fallback
	}
}

// builtinLanguages is the default language registry. Each entry is registered
// at startup via init(). External callers may call Register() to add more.
var builtinLanguages = []LanguageSupport{
	{
		Language: model.LangGo,
		Markers:  []string{"go.mod"},
		ScannerFactory: func(_ string) Scanner {
			return &PackagesScanner{Fallback: &GoScanner{}}
		},
		Rules: &lang.GoRules{},
	},
	{
		Language: model.LangRust,
		Markers:  []string{"Cargo.toml"},
		ScannerFactory: func(_ string) Scanner { return &RustScanner{} },
		Rules:    &lang.RustRules{},
	},
	{
		Language: model.LangPython,
		Markers:  []string{"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt"},
		ScannerFactory: func(_ string) Scanner { return &PythonScanner{} },
		SkipDirs: PythonSkipDirs,
		Rules:    &lang.PythonRules{},
	},
	{
		Language: model.LangTypeScript,
		Markers:  []string{"tsconfig.json"},
		ScannerFactory: func(_ string) Scanner { return &TypeScriptScanner{} },
		SkipDirs: TSSkipDirs,
		Rules:    &lang.TypeScriptRules{},
	},
	{
		Language:       model.LangC,
		Markers:        []string{"CMakeLists.txt"},
		ScannerFactory: lspWithFallback("clangd", &CtagsScanner{}),
		Rules:          &lang.GenericRules{},
	},
	{
		Language:       model.LangCpp,
		Markers:        []string{"CMakeLists.txt"},
		ScannerFactory: lspWithFallback("clangd", &CtagsScanner{}),
		Rules:          &lang.GenericRules{},
	},
	{
		Language:       model.LangJava,
		Markers:        []string{"pom.xml", "build.gradle"},
		ScannerFactory: lspWithFallback("jdtls", &CtagsScanner{}),
		Rules:          &lang.GenericRules{},
	},
	{
		Language: model.LangJavaScript,
		Markers:  []string{"package.json"},
		ScannerFactory: func(_ string) Scanner { return &TypeScriptScanner{} },
		SkipDirs: TSSkipDirs,
		Rules:    &lang.TypeScriptRules{},
	},
	{
		Language:       model.LangKotlin,
		Markers:        []string{"build.gradle.kts"},
		ScannerFactory: func(_ string) Scanner { return &CtagsScanner{} },
		Rules:          &lang.GenericRules{},
	},
	{
		Language:       model.LangZig,
		Markers:        []string{"build.zig"},
		ScannerFactory: func(_ string) Scanner { return &CtagsScanner{} },
		Rules:          &lang.GenericRules{},
	},
	{
		Language:       model.LangCSharp,
		Markers:        []string{"global.json", "Directory.Build.props"},
		ScannerFactory: func(_ string) Scanner { return &CtagsScanner{} },
		Rules:          &lang.GenericRules{},
	},
	{
		Language:       model.LangSwift,
		Markers:        []string{"Package.swift"},
		ScannerFactory: func(_ string) Scanner { return &CtagsScanner{} },
		Rules:          &lang.GenericRules{},
	},
	{
		Language:       model.LangLua,
		Markers:        []string{".luarc.json", ".luacheckrc", "lua"},
		ScannerFactory: func(_ string) Scanner { return &CtagsScanner{} },
		Rules:          &lang.GenericRules{},
	},
}

func init() {
	for i := range builtinLanguages {
		Register(&builtinLanguages[i])
	}
}
