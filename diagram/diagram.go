package diagram

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/diagram/core"
)

// Render dispatches to the appropriate renderer by type name.
func Render(in core.Input, opts core.Options) (string, error) {
	if in.ResolvedTheme == nil {
		theme := core.DefaultTheme()
		mode := opts.Theme
		if mode == "" {
			mode = core.ThemeNatural
		}
		in.ResolvedTheme = theme.Resolve(mode)
	}
	switch opts.Type {
	case "dependency":
		return Dependency(in, opts), nil
	case "c4":
		return C4(in, opts), nil
	case "tree":
		return Tree(in, opts), nil
	case "layers":
		return Layers(in, opts), nil
	case "zones":
		return Zones(in, opts), nil
	case "dsm":
		return DSM(in, opts), nil
	case "symbol_dsm":
		return SymbolDSM(in, opts)
	case "sequence":
		return Sequence(in, opts)
	case "callgraph":
		return CallGraph(in, opts)
	case "dataflow":
		return Dataflow(in, opts)
	case "state":
		return State(in, opts)
	case "classes":
		return Classes(in, opts)
	case "interfaces":
		return Interfaces(in, opts)
	case "er":
		return ER(in, opts)
	case "coupling":
		return Coupling(in, opts), nil
	case "churn":
		return Churn(in, opts), nil
	case "hexa":
		return Hexa(in, opts)
	default:
		return "", fmt.Errorf("%w %q (use: %s)", core.ErrUnknownDiagramType, opts.Type, strings.Join(Types(), ", "))
	}
}

// RenderFacts returns plain-text machine-readable assertions.
func RenderFacts(report *arch.ContextReport) string {
	return Facts(report)
}

// Types returns the list of supported diagram type names.
func Types() []string {
	return []string{
		"dependency", "c4", "coupling", "churn", "layers", "tree",
		"classes", "interfaces", "sequence", "er",
		"dataflow", "callgraph", "state", "zones", "hexa", "dsm", "symbol_dsm",
	}
}
