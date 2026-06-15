package arch

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/oculus/v3/graph"
	"github.com/dpopsuev/oculus/v3/model"
)

const (
	maxEdgesMarkdown       = 20
	maxSymbolsPerComponent = 5
)

// RenderMarkdown produces a human-readable markdown summary of a ContextReport.
// Ordered for LLM consumption: summary → hot spots → coupling table →
// key symbols (with signatures when available) → dependency adjacency list (capped).
func RenderMarkdown(report *ContextReport) string {
	fanIn := graph.FanIn(report.Architecture.Edges)

	var b strings.Builder

	lang := ""
	if report.Project != nil && report.Project.Language != model.LangUnknown {
		lang = report.Project.Language.String()
	}

	fmt.Fprintf(&b, "# %s\n\n", report.ModulePath)
	if lang != "" {
		fmt.Fprintf(&b, "%s | Scanner: %s | Components: %d | Edges: %d\n\n",
			lang, report.Scanner, len(report.Architecture.Services), len(report.Architecture.Edges))
	} else {
		fmt.Fprintf(&b, "Scanner: %s | Components: %d | Edges: %d\n\n",
			report.Scanner, len(report.Architecture.Services), len(report.Architecture.Edges))
	}

	if len(report.HotSpots) > 0 {
		b.WriteString("## Hot Spots\n\n")
		spots := make([]HotSpot, len(report.HotSpots))
		copy(spots, report.HotSpots)
		sort.Slice(spots, func(i, j int) bool { return spots[i].Churn > spots[j].Churn })
		n := len(spots)
		if n > MaxHotSpotsMarkdown {
			n = MaxHotSpotsMarkdown
		}
		for _, s := range spots[:n] {
			nest := ""
			if s.Nesting > 0 {
				nest = fmt.Sprintf("  nesting=%d", s.Nesting)
			}
			fmt.Fprintf(&b, "- %s  churn=%d  fan_in=%d%s\n", s.Component, s.Churn, fanIn[s.Component], nest)
		}
		b.WriteByte('\n')
	}

	b.WriteString(RenderCouplingTable(report, "fan_in", 0))
	b.WriteByte('\n')
	b.WriteString(renderKeySymbols(report))
	b.WriteString(RenderEdgeList(report, ""))
	b.WriteByte('\n')

	return b.String()
}

// RenderCouplingTable produces a markdown table of components with fan-in, fan-out,
// churn, nesting, and symbol count. sortBy is "fan_in", "fan_out", "churn", or "nesting".
// topN=0 means all.
func RenderCouplingTable(report *ContextReport, sortBy string, topN int) string {
	fanIn := graph.FanIn(report.Architecture.Edges)
	fanOut := graph.FanOut(report.Architecture.Edges)

	type row struct {
		Name       string
		FanIn      int
		FanOut     int
		LOC        int
		Churn      int
		Symbols    int
		MaxNesting int
	}

	rows := make([]row, 0, len(report.Architecture.Services))
	for i := range report.Architecture.Services {
		svc := &report.Architecture.Services[i]
		fi := fanIn[svc.Name]
		fo := fanOut[svc.Name]
		if fi > 0 || fo > 0 {
			rows = append(rows, row{
				Name:       svc.Name,
				FanIn:      fi,
				FanOut:     fo,
				LOC:        svc.LOC,
				Churn:      svc.Churn,
				Symbols:    len(svc.Symbols),
				MaxNesting: svc.MaxNesting,
			})
		}
	}

	switch sortBy {
	case "fan_out":
		sort.Slice(rows, func(i, j int) bool { return rows[i].FanOut > rows[j].FanOut })
	case "churn":
		sort.Slice(rows, func(i, j int) bool { return rows[i].Churn > rows[j].Churn })
	case "nesting":
		sort.Slice(rows, func(i, j int) bool { return rows[i].MaxNesting > rows[j].MaxNesting })
	case "loc":
		sort.Slice(rows, func(i, j int) bool { return rows[i].LOC > rows[j].LOC })
	default:
		sort.Slice(rows, func(i, j int) bool { return rows[i].FanIn > rows[j].FanIn })
	}

	if topN > 0 && topN < len(rows) {
		rows = rows[:topN]
	}

	var b strings.Builder
	b.WriteString("## Package Coupling\n\n")

	nameW := len("Package")
	for _, r := range rows {
		if len(r.Name) > nameW {
			nameW = len(r.Name)
		}
	}

	fmt.Fprintf(&b, "%-*s  %6s  %7s  %5s  %5s  %7s  %7s\n", nameW, "Package", "Fan-In", "Fan-Out", "LOC", "Churn", "MaxNest", "Symbols")
	fmt.Fprintf(&b, "%s  %s  %s  %s  %s  %s  %s\n",
		strings.Repeat("-", nameW),
		strings.Repeat("-", 6),
		strings.Repeat("-", 7),
		strings.Repeat("-", 5),
		strings.Repeat("-", 5),
		strings.Repeat("-", 7),
		strings.Repeat("-", 7))

	for _, r := range rows {
		fmt.Fprintf(&b, "%-*s  %6d  %7d  %5d  %5d  %7d  %7d\n", nameW, r.Name, r.FanIn, r.FanOut, r.LOC, r.Churn, r.MaxNesting, r.Symbols)
	}

	return b.String()
}

// RenderEdgeList produces a natural language adjacency list of dependencies.
// Groups edges by source component and lists targets with weights.
// Caps output at maxEdgesMarkdown source components with a hint for the remainder.
// If component is non-empty, only edges involving that component are shown (uncapped).
func RenderEdgeList(report *ContextReport, component string) string {
	var b strings.Builder
	b.WriteString("## Dependencies\n\n")

	edges := report.Architecture.Edges

	adjacency := make(map[string][]adjTarget)
	for _, e := range edges {
		if component != "" && e.From != component && e.To != component {
			continue
		}
		if e.From == e.To {
			continue
		}
		adjacency[e.From] = append(adjacency[e.From], adjTarget{name: e.To, weight: e.Weight})
	}

	sources := make([]string, 0, len(adjacency))
	for src := range adjacency {
		sources = append(sources, src)
	}
	sort.Slice(sources, func(i, j int) bool {
		return totalWeight(adjacency[sources[i]]) > totalWeight(adjacency[sources[j]])
	})

	cap := len(sources)
	truncated := false
	if component == "" && cap > maxEdgesMarkdown {
		cap = maxEdgesMarkdown
		truncated = true
	}

	for _, src := range sources[:cap] {
		tgts := adjacency[src]
		sort.Slice(tgts, func(i, j int) bool { return tgts[i].weight > tgts[j].weight })
		parts := make([]string, 0, len(tgts))
		for _, t := range tgts {
			parts = append(parts, fmt.Sprintf("%s(%d)", t.name, t.weight))
		}
		fmt.Fprintf(&b, "%s depends on: %s\n", src, strings.Join(parts, ", "))
	}

	if truncated {
		fmt.Fprintf(&b, "\n(%d more components — use `analysis deps` to explore)\n", len(sources)-cap)
	}

	return b.String()
}

type adjTarget struct {
	name   string
	weight int
}

func totalWeight(targets []adjTarget) int {
	sum := 0
	for _, t := range targets {
		sum += t.weight
	}
	return sum
}

var symbolKindPriority = map[model.SymbolKind]int{
	model.SymbolInterface:   0,
	model.SymbolFunction:    1,
	model.SymbolMethod:      2,
	model.SymbolStruct:      3,
	model.SymbolClass:       4,
	model.SymbolEnum:        5,
	model.SymbolConstant:    6,
	model.SymbolVariable:    7,
	model.SymbolConstructor: 8,
}

var symbolKindPrefix = map[model.SymbolKind]string{
	model.SymbolInterface:   "interface",
	model.SymbolFunction:    "func",
	model.SymbolMethod:      "method",
	model.SymbolStruct:      "struct",
	model.SymbolClass:       "class",
	model.SymbolEnum:        "enum",
	model.SymbolConstant:    "const",
	model.SymbolVariable:    "var",
	model.SymbolConstructor: "new",
}

func renderKeySymbols(report *ContextReport) string {
	type entry struct {
		component string
		kind      model.SymbolKind
		name      string
		signature string
	}

	var entries []entry
	for _, svc := range report.Architecture.Services {
		exported := make([]model.Symbol, 0, len(svc.Symbols))
		for _, s := range svc.Symbols {
			if s.Exported {
				exported = append(exported, s)
			}
		}
		sort.Slice(exported, func(i, j int) bool {
			pi := symbolKindPriority[exported[i].Kind]
			pj := symbolKindPriority[exported[j].Kind]
			if pi != pj {
				return pi < pj
			}
			return exported[i].Name < exported[j].Name
		})
		n := len(exported)
		if n > maxSymbolsPerComponent {
			n = maxSymbolsPerComponent
		}
		for _, s := range exported[:n] {
			entries = append(entries, entry{
				component: svc.Name,
				kind:      s.Kind,
				name:      s.Name,
				signature: s.Signature,
			})
		}
	}

	if len(entries) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Key Symbols\n\n")

	current := ""
	for _, e := range entries {
		if e.component != current {
			if current != "" {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "**%s**\n", e.component)
			current = e.component
		}
		if e.signature != "" {
			fmt.Fprintf(&b, "  %s\n", e.signature)
		} else {
			prefix := symbolKindPrefix[e.kind]
			if prefix == "" {
				prefix = e.kind.String()
			}
			fmt.Fprintf(&b, "  %s %s\n", prefix, e.name)
		}
	}
	b.WriteByte('\n')

	return b.String()
}
