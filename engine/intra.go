package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/dpopsuev/oculus/v3/arch"
)

// IntraPackageDepsReport holds file-to-file dependency edges within a component.
type IntraPackageDepsReport struct {
	Component string              `json:"component"`
	Edges     []arch.ArchEdge     `json:"edges"`
	Files     []string            `json:"files"`
	Summary   string              `json:"summary"`
}

// IntraCouplingReport holds file-to-file coupling edges within a component.
type IntraCouplingReport struct {
	Component string          `json:"component"`
	Edges     []arch.ArchEdge `json:"edges"`
	Summary   string          `json:"summary"`
}

// TypeUsageFile describes one file that references a named type.
type TypeUsageFile struct {
	File      string `json:"file"`
	Component string `json:"component"`
	Symbols   []string `json:"symbols,omitempty"` // symbols in this file that reference the type
}

// TypeUsageReport holds files that reference a named type.
type TypeUsageReport struct {
	TypeName string          `json:"type_name"`
	Files    []TypeUsageFile `json:"files"`
	Summary  string          `json:"summary"`
}

// GetIntraPackageDeps returns file-to-file dependency edges within a single
// component. Requires a file-granularity scan (each service is a file).
// Returns ErrComponentNotFound when no file-level services match component.
func (p *Engine) GetIntraPackageDeps(ctx context.Context, path, component string, cacheKey ...string) (*IntraPackageDepsReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(ctx, path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return intraPackageDeps(report, component)
}

func intraPackageDeps(report *arch.ContextReport, component string) (*IntraPackageDepsReport, error) {
	prefix := component + "/"
	// Collect file-level services within this component.
	fileSet := make(map[string]bool)
	for _, svc := range report.Architecture.Services {
		if strings.HasPrefix(svc.Name, prefix) || svc.Name == component {
			fileSet[svc.Name] = true
		}
	}
	if len(fileSet) == 0 {
		return nil, fmt.Errorf("%w: %q — run scan with file_granularity=true to see file-level nodes", ErrComponentNotFound, component)
	}

	// Collect intra-component edges (both From and To within the component).
	var edges []arch.ArchEdge
	for _, e := range report.Architecture.Edges {
		if fileSet[e.From] && fileSet[e.To] {
			edges = append(edges, e)
		}
	}

	files := make([]string, 0, len(fileSet))
	for f := range fileSet {
		files = append(files, f)
	}

	return &IntraPackageDepsReport{
		Component: component,
		Edges:     edges,
		Files:     files,
		Summary:   fmt.Sprintf("%d file(s) in %q, %d intra-component edge(s)", len(files), component, len(edges)),
	}, nil
}

// GetIntraCoupling returns file-to-file coupling edges within a single
// component, ordered by weight descending. Complements GetCouplingTable
// which only shows cross-component coupling.
func (p *Engine) GetIntraCoupling(ctx context.Context, path, component string, cacheKey ...string) (*IntraCouplingReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(ctx, path, cacheKey...)
	if err != nil {
		return nil, err
	}
	r, err := intraPackageDeps(report, component)
	if err != nil {
		return nil, err
	}
	return &IntraCouplingReport{
		Component: component,
		Edges:     r.Edges,
		Summary:   fmt.Sprintf("%d intra-component coupling edge(s) within %q", len(r.Edges), component),
	}, nil
}

// GetTypeUsages returns the files and components that reference a named type.
// It searches the symbol index for symbols whose name matches the type, then
// reports every component that declares or uses that type name.
//
// This is a structural search — it does not require LSP. For cross-reference
// depth (e.g. parameter types, return types), LSP-based analysis is needed.
func (p *Engine) GetTypeUsages(ctx context.Context, path, typeName string, cacheKey ...string) (*TypeUsageReport, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(ctx, path, cacheKey...)
	if err != nil {
		return nil, err
	}
	return typeUsages(report, typeName), nil
}

func typeUsages(report *arch.ContextReport, typeName string) *TypeUsageReport {
	lower := strings.ToLower(typeName)
	var files []TypeUsageFile
	seen := make(map[string]bool)

	for i := range report.Architecture.Services {
		svc := &report.Architecture.Services[i]
		var matching []string
		for _, sym := range svc.Symbols {
			if strings.ToLower(sym.Name) == lower {
				matching = append(matching, sym.Name)
			}
		}
		if len(matching) > 0 && !seen[svc.Name] {
			seen[svc.Name] = true
			files = append(files, TypeUsageFile{
				File:      svc.Name,
				Component: svc.Name,
				Symbols:   matching,
			})
		}
	}

	return &TypeUsageReport{
		TypeName: typeName,
		Files:    files,
		Summary:  fmt.Sprintf("%d file(s) reference type %q", len(files), typeName),
	}
}
