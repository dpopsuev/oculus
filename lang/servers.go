package lang

// DefaultLSPServers maps languages to their conventional LSP server command
// strings for backward compatibility. Populated from the embedded registry at
// startup. Use DefaultServerEntry for structured metadata (args, root markers).
var DefaultLSPServers map[Language]string

func init() {
	DefaultLSPServers = make(map[Language]string, len(LanguageServers))
	for lang := range LanguageServers {
		if s := DefaultLSPServer(lang); s != "" {
			DefaultLSPServers[lang] = s
		}
	}
}

// DefaultLSPServer returns the conventional LSP server command string for a
// language. The string is suitable for splitting on whitespace and passing to
// exec.Command. Delegates to the embedded registry when available.
//
// Prefer DefaultServerEntry when you need structured metadata (args, root
// markers, settings).
func DefaultLSPServer(l Language) string {
	entry := DefaultServerEntry(l)
	if entry == nil {
		return ""
	}
	cmd := entry.Command
	for _, arg := range entry.Args {
		cmd += " " + arg
	}
	return cmd
}
