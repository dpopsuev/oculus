package lang

import (
	_ "embed"
	"encoding/json"
	"log/slog"
	"path/filepath"
)

//go:embed lsps.json
var lspsJSON []byte

// ServerEntry describes a Language Server Protocol server.
// Fields mirror the powernap lsps.json schema so the embedded registry
// can be consumed directly.
type ServerEntry struct {
	// Command is the executable name (looked up via PATH).
	Command string `json:"command"`
	// Args are the command-line arguments to pass to Command.
	Args []string `json:"args,omitempty"`
	// FileTypes lists the LSP languageId values this server handles.
	FileTypes []string `json:"filetypes,omitempty"`
	// RootMarkers are glob patterns checked in the workspace root before
	// spawning. An empty list means always spawn (or use SingleFileSupport).
	RootMarkers []string `json:"root_markers,omitempty"`
	// Settings are the server-specific workspace/configuration settings.
	Settings map[string]any `json:"settings,omitempty"`
	// InitOptions are passed in the initialize request's initializationOptions.
	InitOptions map[string]any `json:"init_options,omitempty"`
	// SingleFileSupport indicates the server works without a project root.
	// When true, RootMarkers are not required before spawning.
	SingleFileSupport bool `json:"-"`
	// SkipAutoStart prevents the pool from auto-starting this server.
	// Set for servers whose command name is ambiguous (e.g. python, node)
	// or requires complex multi-step invocation (e.g. omnisharp).
	SkipAutoStart bool `json:"-"`
}

// registry is the full server name → entry map loaded from lsps.json at startup.
var registry map[string]*ServerEntry

// LanguageServers maps each Language constant to the canonical server name
// in the registry. The first entry that is installed wins when multiple
// alternatives exist (see DefaultServerEntry).
var LanguageServers = map[Language][]string{
	Go:         {"gopls"},
	Rust:       {"rust_analyzer"},
	Python:     {"pyright", "basedpyright"},
	TypeScript: {"typescript-language-server"},
	JavaScript: {"typescript-language-server"},
	C:          {"clangd"},
	Cpp:        {"clangd"},
	Java:       {"jdtls"},
	Kotlin:     {"kotlin_language_server"},
	CSharp:     {"omnisharp"},
	Swift:      {"sourcekit"},
	Zig:        {"zls"},
	Lua:        {"lua_ls"},
}

// singleFileServers lists servers that can work without a project root.
var singleFileServers = map[string]bool{
	"gopls":                    true,
	"clangd":                   true,
	"zls":                      true,
	"lua_ls":                   true,
	"pyright":                  true,
	"basedpyright":             true,
	"typescript-language-server": true,
	"sourcekit":                true,
}

// skipAutoStartServers lists servers that should not be auto-started.
// Their command names are either too generic or require complex invocation.
var skipAutoStartServers = map[string]bool{
	"omnisharp": true, // complex multi-arg invocation
	"jdtls":     true, // requires Java + data-dir setup
	// clangd spawns parallel clang processes for background indexing (defaults
	// to all available CPUs). Auto-starting it on any deep analysis call caused
	// 88 GB committed memory and load average 60 on a 16-core machine
	// (OCL-BUG-11). Require an explicit WarmLSP call to activate clangd.
	"clangd": true,
}

func init() {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(lspsJSON, &raw); err != nil {
		slog.Error("lsp registry: failed to parse lsps.json", "error", err)
		registry = make(map[string]*ServerEntry)
		return
	}

	registry = make(map[string]*ServerEntry, len(raw))
	for name, data := range raw {
		var entry ServerEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			slog.Warn("lsp registry: skipping malformed entry", "name", name, "error", err)
			continue
		}
		entry.SingleFileSupport = singleFileServers[name]
		entry.SkipAutoStart = skipAutoStartServers[name]
		registry[name] = &entry
	}

	// Inject the typescript-language-server entry (not in lsps.json by that
	// name — powernap prefers vtsls, but Oculus's e2e tests use tsserver).
	if _, ok := registry["typescript-language-server"]; !ok {
		registry["typescript-language-server"] = &ServerEntry{
			Command:           "typescript-language-server",
			Args:              []string{"--stdio"},
			FileTypes:         []string{"typescript", "typescriptreact", "javascript", "javascriptreact"},
			RootMarkers:       []string{"tsconfig.json", "jsconfig.json", "package.json", ".git"},
			SingleFileSupport: true,
		}
	}
}

// LookupServer returns the registry entry for the given server name, or nil.
func LookupServer(name string) *ServerEntry {
	return registry[name]
}

// DefaultServerEntry returns the first available (installed) ServerEntry for
// the given Language, or nil if none can be found.
func DefaultServerEntry(l Language) *ServerEntry {
	names, ok := LanguageServers[l]
	if !ok {
		return nil
	}
	for _, name := range names {
		entry := registry[name]
		if entry == nil {
			continue
		}
		return entry
	}
	return nil
}

// HasRootMarkers reports whether any of the given glob patterns matches a file
// directly inside dir. Uses filepath.Glob — non-recursive — to avoid walking
// large directories such as node_modules.
func HasRootMarkers(dir string, markers []string) bool {
	if len(markers) == 0 {
		return true
	}
	for _, pattern := range markers {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		if err == nil && len(matches) > 0 {
			return true
		}
	}
	return false
}
