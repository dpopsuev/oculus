package arch

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dpopsuev/oculus/v3/graph"
	"github.com/dpopsuev/oculus/v3/model"
)

// ArchService, ArchEdge, ArchForbidden, ArchModel — moved to root package.
// Type aliases in arch/compat.go provide backward compatibility.

const (
	protoImport   = "import"
	protoExternal = "external"
)

// ComponentGroup maps a logical component name to a set of package import paths.
// When groups are provided, packages within the same group are collapsed into
// a single node and only inter-group edges are retained.
type ComponentGroup struct {
	Name     string
	Packages []string
}

// SyncOptions controls how ProjectToArchModel converts survey data.
type SyncOptions struct {
	Groups          []ComponentGroup
	ModulePath      string
	ExcludeTests    bool
	IncludeExternal bool
	ChurnData       map[string]int
}

// ProjectToArchModel converts a survey model.Project into an ArchModel.
// Without groups, each internal package becomes a service node.
// With groups, packages are collapsed into named component nodes.
func ProjectToArchModel(proj *model.Project, opts SyncOptions) ArchModel {
	modPath := opts.ModulePath
	if modPath == "" {
		modPath = proj.Path
	}

	m := ArchModel{Title: filepath.Base(modPath)}

	if len(opts.Groups) == 0 {
		return projectToArchPackageLevel(proj, modPath, m, opts)
	}
	return projectToArchGroupLevel(proj, modPath, m, opts)
}

func projectToArchPackageLevel(proj *model.Project, modPath string, m ArchModel, opts SyncOptions) ArchModel {
	nsSet := make(map[string]bool, len(proj.Namespaces))
	for _, ns := range proj.Namespaces {
		nsSet[ns.ImportPath] = true
	}

	for _, ns := range proj.Namespaces {
		rel := shortImportPath(modPath, ns.ImportPath)
		if opts.ExcludeTests && strings.HasPrefix(rel, "testkit/") {
			continue
		}
		svc := ArchService{
			Name:     rel,
			Package:  ns.ImportPath,
			Language: proj.Language,
			Churn:    opts.ChurnData[rel],
		}
		for _, sym := range ns.Symbols {
			if sym.Exported {
				svc.Symbols = append(svc.Symbols, *sym)
			}
		}
		m.Services = append(m.Services, svc)
	}

	m = buildPackageEdges(proj, modPath, m, opts, nsSet)

	sort.Slice(m.Services, func(i, j int) bool { return m.Services[i].Name < m.Services[j].Name })
	sort.Slice(m.Edges, func(i, j int) bool {
		if m.Edges[i].From != m.Edges[j].From {
			return m.Edges[i].From < m.Edges[j].From
		}
		return m.Edges[i].To < m.Edges[j].To
	})
	return m
}

func buildPackageEdges(proj *model.Project, modPath string, m ArchModel, opts SyncOptions, nsSet map[string]bool) ArchModel {
	if proj.DependencyGraph == nil {
		return m
	}
	for _, e := range proj.DependencyGraph.Edges {
		if e.External && !opts.IncludeExternal {
			continue
		}
		if !e.External && (!nsSet[e.From] || !nsSet[e.To]) {
			continue
		}
		fromRel := shortImportPath(modPath, e.From)
		toRel := shortImportPath(modPath, e.To)
		if e.External {
			toRel = e.To
		}
		if opts.ExcludeTests && (strings.HasPrefix(fromRel, "testkit/") || strings.HasPrefix(toRel, "testkit/")) {
			continue
		}
		proto := protoImport
		if e.External {
			proto = protoExternal
		}
		m.Edges = append(m.Edges, ArchEdge{
			From:     fromRel,
			To:       toRel,
			Protocol: proto,
			Weight:   e.Weight,
		})
	}
	return m
}

func projectToArchGroupLevel(proj *model.Project, modPath string, m ArchModel, opts SyncOptions) ArchModel {
	pkgToGroup := make(map[string]string)
	for _, g := range opts.Groups {
		for _, pkg := range g.Packages {
			pkgToGroup[pkg] = g.Name
		}
	}

	groupChurn := make(map[string]int)
	groupSet := make(map[string]bool)
	for _, ns := range proj.Namespaces {
		rel := shortImportPath(modPath, ns.ImportPath)
		if opts.ExcludeTests && strings.HasPrefix(rel, "testkit/") {
			continue
		}
		gName := pkgToGroup[rel]
		if gName == "" {
			gName = rel
		}
		groupChurn[gName] += opts.ChurnData[rel]
		if !groupSet[gName] {
			groupSet[gName] = true
			m.Services = append(m.Services, ArchService{Name: gName, Language: proj.Language})
		}
	}

	for i := range m.Services {
		m.Services[i].Churn = groupChurn[m.Services[i].Name]
	}

	m = buildGroupEdges(proj, modPath, m, opts, pkgToGroup, groupSet)

	sort.Slice(m.Services, func(i, j int) bool { return m.Services[i].Name < m.Services[j].Name })
	sort.Slice(m.Edges, func(i, j int) bool {
		if m.Edges[i].From != m.Edges[j].From {
			return m.Edges[i].From < m.Edges[j].From
		}
		return m.Edges[i].To < m.Edges[j].To
	})
	return m
}

func buildGroupEdges(proj *model.Project, modPath string, m ArchModel, opts SyncOptions, pkgToGroup map[string]string, groupSet map[string]bool) ArchModel {
	if proj.DependencyGraph == nil {
		return m
	}

	edgeWeights := make(map[[2]string]int)
	edgeProto := make(map[[2]string]string)
	for _, e := range proj.DependencyGraph.Edges {
		if e.External && !opts.IncludeExternal {
			continue
		}
		fromRel := shortImportPath(modPath, e.From)
		toRel := shortImportPath(modPath, e.To)
		if e.External {
			toRel = e.To
		}
		if opts.ExcludeTests && (strings.HasPrefix(fromRel, "testkit/") || strings.HasPrefix(toRel, "testkit/")) {
			continue
		}
		fromGroup := pkgToGroup[fromRel]
		if fromGroup == "" {
			fromGroup = fromRel
		}
		var toGroup string
		if e.External {
			toGroup = toRel
			if !groupSet[toGroup] {
				groupSet[toGroup] = true
				m.Services = append(m.Services, ArchService{Name: toGroup, Language: proj.Language})
			}
		} else {
			toGroup = pkgToGroup[toRel]
			if toGroup == "" {
				toGroup = toRel
			}
		}
		if fromGroup == toGroup {
			continue
		}
		key := [2]string{fromGroup, toGroup}
		edgeWeights[key] += e.Weight
		if e.External {
			edgeProto[key] = protoExternal
		} else if edgeProto[key] == "" {
			edgeProto[key] = protoImport
		}
	}

	for key, w := range edgeWeights {
		proto := edgeProto[key]
		if proto == "" {
			proto = protoImport
		}
		m.Edges = append(m.Edges, ArchEdge{From: key[0], To: key[1], Protocol: proto, Weight: w})
	}
	return m
}

func shortImportPath(modPath, importPath string) string {
	if importPath == modPath {
		return "."
	}
	if strings.HasPrefix(importPath, modPath+"/") {
		return strings.TrimPrefix(importPath, modPath+"/")
	}
	return importPath
}


// RenderArchMarkdown generates an ARCHITECTURE.md from an ArchModel.
func RenderArchMarkdown(m ArchModel) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Architecture: %s\n\n", m.Title)
	b.WriteString("> Auto-generated by `locus scan`. Do not edit manually.\n\n")

	hasChurn := false
	for i := range m.Services {
		if m.Services[i].Churn > 0 {
			hasChurn = true
			break
		}
	}

	b.WriteString("## Components\n\n")
	if hasChurn {
		b.WriteString("| Component | Package | Churn |\n")
		b.WriteString("|-----------|----------|-------|\n")
	} else {
		b.WriteString("| Component | Package |\n")
		b.WriteString("|-----------|----------|\n")
	}
	for i := range m.Services {
		s := &m.Services[i]
		pkg := s.Package
		if pkg == "" {
			pkg = "-"
		}
		if hasChurn {
			fmt.Fprintf(&b, "| %s | `%s` | %d |\n", s.Name, pkg, s.Churn)
		} else {
			fmt.Fprintf(&b, "| %s | `%s` |\n", s.Name, pkg)
		}
	}
	b.WriteString("\n")

	b.WriteString("## Dependency Graph\n\n")
	b.WriteString("```mermaid\n")
	b.WriteString(RenderMermaid(m))
	b.WriteString("```\n\n")

	if hasChurn {
		fanIn := graph.FanIn(m.Edges)
		type hotSpot struct {
			Name  string
			FanIn int
			Churn int
		}
		var spots []hotSpot
		for i := range m.Services {
			s := &m.Services[i]
			fi := fanIn[s.Name]
			if fi >= 3 && s.Churn >= 5 {
				spots = append(spots, hotSpot{s.Name, fi, s.Churn})
			}
		}
		if len(spots) > 0 {
			sort.Slice(spots, func(i, j int) bool { return spots[i].Churn > spots[j].Churn })
			b.WriteString("## Hot Spots\n\n")
			b.WriteString("| Component | Fan-In | Churn |\n")
			b.WriteString("|-----------|--------|-------|\n")
			for _, h := range spots {
				fmt.Fprintf(&b, "| %s | %d | %d |\n", h.Name, h.FanIn, h.Churn)
			}
			b.WriteString("\n")
		}
	}

	if len(m.Forbidden) > 0 {
		b.WriteString("## Forbidden Dependencies\n\n")
		b.WriteString("| From | To | Reason |\n")
		b.WriteString("|------|-----|--------|\n")
		for _, f := range m.Forbidden {
			reason := f.Reason
			if reason == "" {
				reason = "-"
			}
			fmt.Fprintf(&b, "| %s | %s | %s |\n", f.From, f.To, reason)
		}
		b.WriteString("\n")
	}

	return b.String()
}

// RenderMermaid generates a Mermaid graph from an architecture model.
func RenderMermaid(m ArchModel) string {
	var b strings.Builder
	b.WriteString("graph TD\n")

	for i := range m.Services {
		s := &m.Services[i]
		id := mermaidID(s.Name)
		label := s.Name
		if s.Package != "" {
			label += fmt.Sprintf("\\n(%s)", s.Package)
		}
		if s.TrustZone != "" {
			label += fmt.Sprintf("\\n[%s]", s.TrustZone)
		}
		fmt.Fprintf(&b, "    %s[\"%s\"]\n", id, label)
	}

	for _, e := range m.Edges {
		fromID := mermaidID(e.From)
		toID := mermaidID(e.To)
		edgeLabel := e.Protocol
		if e.Weight > 0 {
			edgeLabel = fmt.Sprintf("%s(%d)", edgeLabel, e.Weight)
		}
		renderMermaidEdge(&b, fromID, toID, edgeLabel, e.Protocol == protoExternal)
	}

	for _, f := range m.Forbidden {
		if f.From == "" || f.To == "" {
			continue
		}
		fromID := mermaidID(f.From)
		toID := mermaidID(f.To)
		label := "FORBIDDEN"
		if f.Reason != "" {
			label = f.Reason
		}
		fmt.Fprintf(&b, "    %s -.-x|\"%s\"| %s\n", fromID, label, toID)
	}

	return b.String()
}

func renderMermaidEdge(b *strings.Builder, fromID, toID, label string, external bool) {
	switch {
	case label != "" && external:
		fmt.Fprintf(b, "    %s -.->|\"%s\"| %s\n", fromID, label, toID)
	case label != "":
		fmt.Fprintf(b, "    %s -->|\"%s\"| %s\n", fromID, label, toID)
	case external:
		fmt.Fprintf(b, "    %s -.-> %s\n", fromID, toID)
	default:
		fmt.Fprintf(b, "    %s --> %s\n", fromID, toID)
	}
}

func mermaidID(name string) string {
	r := strings.NewReplacer(" ", "_", "-", "_", ".", "_", "/", "_")
	return r.Replace(name)
}
