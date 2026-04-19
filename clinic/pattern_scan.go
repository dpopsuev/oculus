package clinic

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dpopsuev/oculus/v3/arch"
	"github.com/dpopsuev/oculus/v3/graph"
	"github.com/dpopsuev/oculus/v3/port"
	oculus "github.com/dpopsuev/oculus/v3"
)

// PatternKind distinguishes design patterns from code smells.
type PatternKind string

const (
	PatternKindPattern PatternKind = "pattern"
	PatternKindSmell   PatternKind = "smell"
)

// CatalogEntry describes a known pattern or smell in the catalog.
type CatalogEntry struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Kind        PatternKind `json:"kind"`
	Category    string      `json:"category"`
	Description string      `json:"description"`
	Indicators  []string    `json:"indicators"`
}

// PatternDetection records a single detected pattern or smell in a component.
type PatternDetection struct {
	PatternID   string          `json:"pattern_id"`
	PatternName string          `json:"pattern_name"`
	Kind        PatternKind     `json:"kind"`
	Component   string          `json:"component"`
	Confidence  port.Confidence `json:"confidence"`
	Evidence    []string        `json:"evidence"`
	Severity    port.Severity   `json:"severity"`
}

// PatternScanReport is the result of scanning an architecture for patterns and smells.
type PatternScanReport struct {
	Detections    []PatternDetection `json:"detections"`
	PatternsFound int                `json:"patterns_found"`
	SmellsFound   int                `json:"smells_found"`
	Summary       string             `json:"summary"`
}

// PatternCatalogReport lists catalog entries, optionally filtered.
type PatternCatalogReport struct {
	Entries []CatalogEntry `json:"entries"`
	Summary string         `json:"summary"`
}

// patternCatalog is the compile-time catalog of known patterns and smells.
// Kept as reference metadata — the Book provides diagnostic reasoning.
var patternCatalog = []CatalogEntry{
	{ID: "circular_dependency", Name: "Circular Dependency", Kind: PatternKindSmell, Category: "smell", Description: "Mutual dependency cycle", Indicators: []string{"cycle in dependency graph"}},
	{ID: "inappropriate_intimacy", Name: "Inappropriate Intimacy", Kind: PatternKindSmell, Category: "smell", Description: "Bidirectional coupling between components", Indicators: []string{"mutual edges between 2 components"}},
}

// catalogByID provides O(1) lookup into the catalog.
var catalogByID map[string]*CatalogEntry

func init() {
	catalogByID = make(map[string]*CatalogEntry, len(patternCatalog))
	for i := range patternCatalog {
		catalogByID[patternCatalog[i].ID] = &patternCatalog[i]
	}
}

// ComputePatternScan performs only deterministic structural checks:
// cycle participation and bidirectional edges. All heuristic detection
// is delegated to the agent via the Book.
func ComputePatternScan(
	services []arch.ArchService,
	edges []arch.ArchEdge,
	cycles []graph.Cycle,
	classes []oculus.ClassInfo,
	impls []oculus.ImplEdge,
	_ any,
	accepted []port.AcceptedViolation,
) *PatternScanReport {
	var detections []PatternDetection

	// Deterministic: cycle participation.
	cycleEntry := catalogByID["circular_dependency"]
	if cycleEntry != nil {
		cycleMembers := make(map[string][]string)
		for _, cycle := range cycles {
			for _, node := range cycle {
				cycleMembers[node] = cycle
			}
		}
		for comp, cycle := range cycleMembers {
			if !isAccepted(accepted, comp, "circular_dependency") {
				detections = append(detections, PatternDetection{
					PatternID:   cycleEntry.ID,
					PatternName: cycleEntry.Name,
					Kind:        PatternKindSmell,
					Component:   comp,
					Confidence:  1.0,
					Evidence:    []string{fmt.Sprintf("participates in cycle: %s", strings.Join(cycle, " → "))},
					Severity:    port.SeverityWarning,
				})
			}
		}
	}

	// Deterministic: bidirectional edges.
	intimacyEntry := catalogByID["inappropriate_intimacy"]
	if intimacyEntry != nil {
		outgoing := make(map[string]map[string]bool)
		for _, e := range edges {
			if outgoing[e.From] == nil {
				outgoing[e.From] = make(map[string]bool)
			}
			outgoing[e.From][e.To] = true
		}
		seen := make(map[string]bool)
		for _, e := range edges {
			if outgoing[e.To] != nil && outgoing[e.To][e.From] {
				key := e.From + "↔" + e.To
				if e.From > e.To {
					key = e.To + "↔" + e.From
				}
				if seen[key] {
					continue
				}
				seen[key] = true
				if !isAccepted(accepted, e.From, "inappropriate_intimacy") {
					detections = append(detections, PatternDetection{
						PatternID:   intimacyEntry.ID,
						PatternName: intimacyEntry.Name,
						Kind:        PatternKindSmell,
						Component:   e.From,
						Confidence:  0.9,
						Evidence:    []string{fmt.Sprintf("bidirectional edge: %s ↔ %s", e.From, e.To)},
						Severity:    port.SeverityWarning,
					})
				}
			}
		}
	}

	sort.Slice(detections, func(i, j int) bool {
		return detections[i].Confidence > detections[j].Confidence
	})

	smellsFound := len(detections)
	summary := "No structural issues detected"
	if smellsFound > 0 {
		summary = fmt.Sprintf("%d structural issue(s) detected", smellsFound)
	}

	return &PatternScanReport{
		Detections:  detections,
		SmellsFound: smellsFound,
		Summary:     summary,
	}
}

func isAccepted(accepted []port.AcceptedViolation, component, principle string) bool {
	for _, a := range accepted {
		if a.Component == component && a.Principle == principle {
			return true
		}
	}
	return false
}

// GetPatternCatalog returns catalog entries, optionally filtered.
func GetPatternCatalog(filter string) *PatternCatalogReport {
	var entries []CatalogEntry

	switch {
	case filter == "":
		entries = make([]CatalogEntry, len(patternCatalog))
		copy(entries, patternCatalog)
	case filter == "pattern" || filter == "smell":
		kind := PatternKind(filter)
		for i := range patternCatalog {
			if patternCatalog[i].Kind == kind {
				entries = append(entries, patternCatalog[i])
			}
		}
	default:
		lower := strings.ToLower(filter)
		for i := range patternCatalog {
			e := &patternCatalog[i]
			if strings.Contains(strings.ToLower(e.ID), lower) ||
				strings.Contains(strings.ToLower(e.Name), lower) ||
				strings.Contains(strings.ToLower(e.Category), lower) ||
				strings.Contains(strings.ToLower(e.Description), lower) {
				entries = append(entries, *e)
			}
		}
	}

	summary := fmt.Sprintf("%d catalog entries", len(entries))
	if filter != "" {
		summary = fmt.Sprintf("%d entries matching '%s'", len(entries), filter)
	}

	return &PatternCatalogReport{
		Entries: entries,
		Summary: summary,
	}
}
