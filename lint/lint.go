// Package lint orchestrates all Locus violation detectors into a single pass.
// It unifies hexa, SOLID, pattern, symbol quality, layer, boundary, and budget
// checks into a common Violation/Report structure with incremental filtering.
package lint

import (
	"context"
	"github.com/dpopsuev/oculus/v3/analyzer"
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/clinic"
	clinichexa "github.com/dpopsuev/oculus/v3/clinic/hexa"
	clinicnaming "github.com/dpopsuev/oculus/v3/clinic/naming"
	clinicsolid "github.com/dpopsuev/oculus/v3/clinic/solid"
	"github.com/dpopsuev/oculus/v3/constraint"
	"github.com/dpopsuev/oculus/v3/graph"
	"github.com/dpopsuev/oculus/v3/port"
	"github.com/dpopsuev/oculus/v3/survey"
	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
)

// Category identifies which linter produced a violation.
type Category string

const (
	CategoryHexa    Category = "hexa"
	CategorySOLID   Category = "solid"
	CategoryPattern Category = "pattern"
	CategorySymbol  Category = "symbol"
	CategoryLayer   Category = "layer"
	CategoryBudget  Category = "budget"
)

// allDefaultCategories lists the linters enabled by default.
var allDefaultCategories = []Category{
	CategoryHexa,
	CategorySOLID,
	CategoryPattern,
	CategorySymbol,
}

// Violation is the unified representation of any detected issue.
type Violation struct {
	Category   Category `json:"category"`
	Component  string   `json:"component"`
	Rule       string   `json:"rule"`
	Severity   string   `json:"severity"` // "error", "warning", "critical", "info"
	Detail string `json:"detail"`
}

// Report is the unified lint output.
type Report struct {
	Violations []Violation      `json:"violations"`
	ByCategory map[Category]int `json:"by_category"`
	Clean      bool             `json:"clean"`
	Summary    string           `json:"summary"`
}

// RunOpts controls which linters to run and how.
type RunOpts struct {
	// EnabledLinters selects which categories to run. Nil/empty means default
	// (hexa, solid, pattern, symbol). Layer, boundary, and budget require
	// explicit opt-in because they depend on DesiredState configuration.
	EnabledLinters []Category

	// DesiredState provides architecture rules (layers, boundaries, constraints,
	// role overrides, accepted violations). Nil is safe -- config-gated linters
	// are skipped and accepted list is empty.
	DesiredState *port.DesiredState

	// Root is the filesystem path for OCP checks that need file access.
	Root string

	// ChangedComponents limits output to violations involving these components.
	// When empty, all violations are returned.
	ChangedComponents []string
}

// Run executes enabled linters against a pre-scanned ContextReport and returns
// a unified Report. The ContextReport must be non-nil.
func Run(ctx context.Context, report *arch.ContextReport, opts RunOpts) *Report {
	enabled := resolveEnabled(opts.EnabledLinters)
	ds := opts.DesiredState
	if ds == nil {
		ds = &port.DesiredState{}
	}

	services := report.Architecture.Services
	edges := report.Architecture.Edges
	cycles := report.Cycles

	// Resolve hexagonal roles (needed by hexa, SOLID, pattern).
	var hexaClassification *clinichexa.HexaClassificationReport
	var roles map[string]clinichexa.HexaRole

	if enabled[CategoryHexa] || enabled[CategorySOLID] || enabled[CategoryPattern] {
		// Obtain class analysis for hexa classification.
		classes := safeClasses(ctx, opts.Root)
		hexaClassification = clinichexa.ComputeHexaClassification(services, edges, classes)
		roles = clinichexa.ResolveRoles(hexaClassification, ds.Roles)
	}

	var violations []Violation

	if enabled[CategoryHexa] {
		violations = append(violations, runHexa(ctx, services, edges, opts.Root)...)
	}
	if enabled[CategorySOLID] {
		violations = append(violations, runSOLID(ctx, services, edges, hexaClassification, roles, ds, opts.Root)...)
	}
	if enabled[CategoryPattern] {
		violations = append(violations, runPattern(ctx, services, edges, cycles, roles, ds, opts.Root)...)
	}
	if enabled[CategorySymbol] {
		violations = append(violations, runSymbol(services, edges)...)
	}
	if enabled[CategoryLayer] {
		violations = append(violations, runLayer(edges, ds)...)
	}
	if enabled[CategoryBudget] {
		violations = append(violations, runBudget(services, edges, ds)...)
	}

	// Incremental filtering: keep only violations touching changed components.
	if len(opts.ChangedComponents) > 0 {
		violations = filterChanged(violations, opts.ChangedComponents)
	}

	// Sort: severity rank ascending (critical first), then category, then component.
	sort.Slice(violations, func(i, j int) bool {
		ri, rj := severityRank(violations[i].Severity), severityRank(violations[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if violations[i].Category != violations[j].Category {
			return violations[i].Category < violations[j].Category
		}
		return violations[i].Component < violations[j].Component
	})

	byCategory := make(map[Category]int)
	for _, v := range violations {
		byCategory[v.Category]++
	}

	clean := len(violations) == 0

	summary := buildSummary(violations, byCategory)

	return &Report{
		Violations: violations,
		ByCategory: byCategory,
		Clean:      clean,
		Summary:    summary,
	}
}

// resolveEnabled converts the user's category list into a lookup set.
func resolveEnabled(linters []Category) map[Category]bool {
	if len(linters) == 0 {
		linters = allDefaultCategories
	}
	m := make(map[Category]bool, len(linters))
	for _, c := range linters {
		m[c] = true
	}
	return m
}

// safeClasses obtains class analysis without panicking.
// Returns nil if root is empty or analysis fails.
func safeClasses(ctx context.Context, root string) []oculus.ClassInfo {
	if root == "" {
		return nil
	}
	fb := analyzer.NewFallback(root, nil)
	classes, err := fb.Classes(ctx, root)
	if err != nil {
		return nil
	}
	return classes
}

// safeImpls obtains implementation edges without panicking.
func safeImpls(ctx context.Context, root string) []oculus.ImplEdge {
	if root == "" {
		return nil
	}
	fb := analyzer.NewFallback(root, nil)
	impls, err := fb.Implements(ctx, root)
	if err != nil {
		return nil
	}
	return impls
}

// --- Linter runners ---

func runHexa(ctx context.Context, services []arch.ArchService, edges []arch.ArchEdge, root string) []Violation {
	classes := safeClasses(ctx, root)
	hr := clinichexa.ComputeHexaViolations(services, edges, classes)
	if hr == nil {
		return nil
	}

	violations := make([]Violation, 0, len(hr.Violations))
	for _, hv := range hr.Violations {
		violations = append(violations, Violation{
			Category:  CategoryHexa,
			Component: hv.From,
			Rule:      hv.Rule,
			Severity:  string(hv.Severity),
			Detail:    fmt.Sprintf("%s (%s) -> %s (%s)", hv.From, hv.FromRole, hv.To, hv.ToRole),
		})
	}
	return violations
}

func runSOLID(
	ctx context.Context,
	services []arch.ArchService,
	edges []arch.ArchEdge,
	hexaClassification *clinichexa.HexaClassificationReport,
	roles map[string]clinichexa.HexaRole,
	ds *port.DesiredState,
	root string,
) []Violation {
	classes := safeClasses(ctx, root)
	impls := safeImpls(ctx, root)

	sr := clinicsolid.ComputeSOLIDScan(
		services, edges,
		classes, impls,
		hexaClassification,
		root,
		roles,
		ds.Accepted,
	)
	if sr == nil {
		return nil
	}

	violations := make([]Violation, 0, len(sr.Violations))
	for _, sv := range sr.Violations {
		violations = append(violations, Violation{
			Category:   CategorySOLID,
			Component:  sv.Component,
			Rule:       string(sv.Principle),
			Severity:   string(sv.Severity),
			Detail:     sv.Detail,
		})
	}
	return violations
}

func runPattern(
	ctx context.Context,
	services []arch.ArchService,
	edges []arch.ArchEdge,
	cycles []graph.Cycle,
	roles map[string]clinichexa.HexaRole,
	ds *port.DesiredState,
	root string,
) []Violation {
	classes := safeClasses(ctx, root)
	impls := safeImpls(ctx, root)

	pr := clinic.ComputePatternScan(
		services, edges, cycles,
		classes, impls,
		roles,
		ds.Accepted,
	)
	if pr == nil {
		return nil
	}

	violations := make([]Violation, 0, pr.SmellsFound)
	for i := range pr.Detections {
		d := &pr.Detections[i]
		// Only report smells as violations. Positive patterns are informational
		// and do not indicate problems.
		if d.Kind != clinic.PatternKindSmell {
			continue
		}
		violations = append(violations, Violation{
			Category:  CategoryPattern,
			Component: d.Component,
			Rule:      d.PatternID,
			Severity:  string(d.Severity),
			Detail:    fmt.Sprintf("%s (confidence: %.0f%%): %s", d.PatternName, float64(d.Confidence)*100, strings.Join(d.Evidence, "; ")),
		})
	}
	return violations
}

func runSymbol(services []arch.ArchService, edges []arch.ArchEdge) []Violation {
	// Resolve language-specific rules from the first service's language.
	var rules lang.Rules
	if len(services) > 0 {
		if ls := survey.GetLanguageSupport(services[0].Language); ls != nil && ls.Rules != nil {
			rules = ls.Rules
		}
	}

	sr := clinicnaming.ComputeSymbolQuality(services, edges, rules)
	if sr == nil {
		return nil
	}

	violations := make([]Violation, 0, len(sr.Issues))
	for _, si := range sr.Issues {
		violations = append(violations, Violation{
			Category:   CategorySymbol,
			Component:  si.Package,
			Rule:       si.Issue,
			Severity:   string(si.Severity),
			Detail:     fmt.Sprintf("symbol %q: %s", si.Symbol, si.Issue),
		})
	}
	return violations
}

func runLayer(edges []arch.ArchEdge, ds *port.DesiredState) []Violation {
	if len(ds.Layers) == 0 {
		return nil
	}

	lvs := graph.CheckLayerPurity(edges, ds.Layers)

	violations := make([]Violation, 0, len(lvs))
	for _, lv := range lvs {
		violations = append(violations, Violation{
			Category:  CategoryLayer,
			Component: lv.From,
			Rule:      "layer_purity",
			Severity:  string(port.SeverityError),
			Detail:    fmt.Sprintf("%s (%s) imports %s (%s)", lv.From, lv.FromLayer, lv.To, lv.ToLayer),
		})
	}
	return violations
}

func runBudget(services []arch.ArchService, edges []arch.ArchEdge, ds *port.DesiredState) []Violation {
	if len(ds.Constraints) == 0 {
		return nil
	}

	br := constraint.ComputeBudgetViolations(services, edges, ds.Constraints)
	if br == nil {
		return nil
	}

	violations := make([]Violation, 0, len(br.Violations))
	for _, bv := range br.Violations {
		violations = append(violations, Violation{
			Category:  CategoryBudget,
			Component: bv.Component,
			Rule:      bv.Metric,
			Severity:  string(bv.Severity),
			Detail:    fmt.Sprintf("%s %s: actual=%.0f, budget=%.0f", bv.Component, bv.Metric, bv.Actual, bv.Budget),
		})
	}
	return violations
}

// --- Helpers ---

// filterChanged keeps only violations whose Component matches a changed component.
func filterChanged(violations []Violation, changed []string) []Violation {
	set := make(map[string]bool, len(changed))
	for _, c := range changed {
		set[c] = true
	}
	var filtered []Violation
	for _, v := range violations {
		if set[v.Component] {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// severityRank returns a numeric sort rank: critical=0, error=1, warning=2, info=3.
func severityRank(severity string) int {
	switch port.Severity(severity) {
	case port.SeverityCritical:
		return 0
	case port.SeverityError:
		return 1
	case port.SeverityWarning:
		return 2
	case port.SeverityInfo:
		return 3
	default:
		return 4
	}
}

func buildSummary(violations []Violation, byCategory map[Category]int) string {
	if len(violations) == 0 {
		return "Lint: clean, no violations"
	}

	parts := make([]string, 0, len(byCategory))
	// Stable order.
	for _, cat := range []Category{CategoryHexa, CategorySOLID, CategoryPattern, CategorySymbol, CategoryLayer, CategoryBudget} {
		if n, ok := byCategory[cat]; ok && n > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", cat, n))
		}
	}

	return fmt.Sprintf("Lint: %d violation(s): %s", len(violations), strings.Join(parts, ", "))
}
