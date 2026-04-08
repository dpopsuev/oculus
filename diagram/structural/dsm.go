package structural

import (
	"fmt"
	"strings"

	"github.com/dpopsuev/oculus/diagram/core"
	"github.com/dpopsuev/oculus/graph"
)

// DSM renders a Dependency Structure Matrix from the architecture report.
func DSM(in core.Input, _ core.Options) string {
	return DSMFromEdges(in.Report.Architecture.Edges, "Dependency Structure Matrix")
}

// SymbolDSM renders a DSM at symbol granularity from the unified symbol graph.
func SymbolDSM(in core.Input, _ core.Options) (string, error) {
	if in.SymbolGraph == nil || len(in.SymbolGraph.Edges) == 0 {
		return "", core.ErrSymbolGraphRequired
	}
	return DSMFromEdges(in.SymbolGraph.Edges, "Symbol Dependency Structure Matrix"), nil
}

// DSMFromEdges renders a Dependency Structure Matrix from any edge slice.
// Components are reordered by RCM partitioning to reveal clusters.
func DSMFromEdges[E graph.Edge](edges []E, title string) string {
	order := graph.Partition(edges)
	if len(order) == 0 {
		return fmt.Sprintf("# %s\n\nNo components to display.", title)
	}

	// Build edge lookup.
	hasEdge := make(map[string]map[string]bool)
	for _, e := range edges {
		src, tgt := e.Source(), e.Target()
		if hasEdge[src] == nil {
			hasEdge[src] = make(map[string]bool)
		}
		hasEdge[src][tgt] = true
	}

	// Abbreviate names for readability.
	abbrev := make([]string, len(order))
	for i, name := range order {
		parts := strings.Split(name, "/")
		short := parts[len(parts)-1]
		// For FQN names like "pkg.Symbol", use the symbol part
		if dotIdx := strings.LastIndex(short, "."); dotIdx >= 0 {
			short = short[dotIdx+1:]
		}
		abbrev[i] = short
	}

	// Render matrix.
	var b strings.Builder
	b.WriteString(fmt.Sprintf("# %s\n\n", title))
	b.WriteString(fmt.Sprintf("%d components, RCM-partitioned\n\n", len(order)))

	maxName := 0
	for _, a := range abbrev {
		if len(a) > maxName {
			maxName = len(a)
		}
	}
	if maxName > 20 {
		maxName = 20
	}

	// Column header indices.
	b.WriteString(strings.Repeat(" ", maxName+2))
	for i := range order {
		b.WriteString(fmt.Sprintf("%2d ", i+1))
	}
	b.WriteByte('\n')

	// Separator.
	b.WriteString(strings.Repeat(" ", maxName+2))
	b.WriteString(strings.Repeat("---", len(order)))
	b.WriteByte('\n')

	// Data rows.
	for i, from := range order {
		name := abbrev[i]
		if len(name) > maxName {
			name = name[:maxName]
		}
		fmt.Fprintf(&b, "%*s: ", maxName, name)
		for j, to := range order {
			switch {
			case i == j:
				b.WriteString(" \u25a0 ")
			case hasEdge[from][to]:
				b.WriteString(" \u00d7 ")
			default:
				b.WriteString(" \u00b7 ")
			}
		}
		fmt.Fprintf(&b, " [%d]\n", i+1)
	}

	b.WriteString("\n\u25a0 = self  \u00d7 = depends on  \u00b7 = no dependency\n")
	return b.String()
}
