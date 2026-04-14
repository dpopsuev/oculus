package oculus

import (
	archanchors "github.com/dpopsuev/oculus/v3/arch/anchors"
	archgit "github.com/dpopsuev/oculus/v3/arch/git"
	"github.com/dpopsuev/oculus/v3/graph"
	"github.com/dpopsuev/oculus/v3/model"
)

// ArchService represents a service or component node in an architecture artifact.
type ArchService struct {
	Name       string
	Package    string
	Language   model.Language `json:"language,omitempty"`
	TrustZone  string
	Symbols    []model.Symbol
	Churn      int
	LOC        int     `json:"loc,omitempty"`
	MaxNesting int     `json:"max_nesting,omitempty"`
	AvgNesting float64 `json:"avg_nesting,omitempty"`
	Changed    bool    `json:"changed,omitempty"`
}

// ArchEdge represents a dependency edge in an architecture artifact.
type ArchEdge struct {
	Name       string
	From       string
	To         string
	Protocol   string
	Weight     int
	CallSites  int
	LOCSurface int
}

// Source implements graph.Edge for ArchEdge.
func (e ArchEdge) Source() string { return e.From }

// Target implements graph.Edge for ArchEdge.
func (e ArchEdge) Target() string { return e.To }

// ArchForbidden represents a forbidden dependency in an architecture artifact.
type ArchForbidden struct {
	Name          string
	From          string
	To            string
	FromTrustZone string
	ToTrustZone   string
	Reason        string
}

// ArchModel is the parsed representation of an architecture artifact's structure.
type ArchModel struct {
	Title      string
	Resolution string
	Implements string
	Services   []ArchService
	Edges      []ArchEdge
	Forbidden  []ArchForbidden
}

// HotSpot identifies a component with high fan-in, high churn, and/or deep nesting.
type HotSpot struct {
	Component string `json:"component"`
	FanIn     int    `json:"fan_in"`
	Churn     int    `json:"churn"`
	Nesting   int    `json:"nesting,omitempty"`
}

// APISurface measures the public API size of a component.
type APISurface struct {
	Component     string `json:"component"`
	ExportedCount int    `json:"exported_count"`
}

// BoundaryCrossing flags an edge that crosses trust zone boundaries.
type BoundaryCrossing struct {
	From     string `json:"from"`
	To       string `json:"to"`
	FromZone string `json:"from_zone"`
	ToZone   string `json:"to_zone"`
}

// ScanCore holds the primary scan output — project model, architecture, and metadata.
type ScanCore struct {
	Project        *model.Project `json:"project"`
	Architecture   ArchModel      `json:"architecture"`
	ModulePath     string         `json:"module_path"`
	Scanner        string         `json:"scanner"`
	SuggestedDepth int            `json:"suggested_depth,omitempty"`
}

// GraphMetrics holds graph-derived analysis: hot spots, cycles, depths, and violations.
type GraphMetrics struct {
	HotSpots          []HotSpot              `json:"hot_spots,omitempty"`
	Cycles            []graph.Cycle          `json:"cycles,omitempty"`
	ImportDepth       graph.DepthMap         `json:"import_depth,omitempty"`
	LayerViolations   []graph.LayerViolation `json:"layer_violations,omitempty"`
	APISurfaces       []APISurface           `json:"api_surfaces,omitempty"`
	BoundaryCrossings []BoundaryCrossing     `json:"boundary_crossings,omitempty"`
	FanIn             graph.CountMap         `json:"fan_in,omitempty"`
	FanOut            graph.CountMap         `json:"fan_out,omitempty"`
}

// GitContext holds git-derived data: coverage, commits, authors, file hotspots.
type GitContext struct {
	Coverage      []archgit.CoverageResult    `json:"coverage,omitempty"`
	RecentCommits []archgit.PackageCommit     `json:"recent_commits,omitempty"`
	Authors       map[string][]archgit.Author `json:"authors,omitempty"`
	FileHotSpots  []archgit.HotFile           `json:"file_hot_spots,omitempty"`
}

// DeepContext holds deep analysis data requiring AST/LSP inspection.
type DeepContext struct {
	Anchors []archanchors.SemanticAnchor `json:"anchors,omitempty"`
}

// ContextReport is the full output of a ScanAndBuild invocation.
// Embeds sub-structs for SRP; field access is unchanged via Go promotion.
type ContextReport struct {
	ScanCore
	GraphMetrics
	GitContext
	DeepContext
}

// ArchDrift reports the delta between a desired and actual architecture.
type ArchDrift struct {
	MissingComponents []string   `json:"missing_components,omitempty"`
	ExtraComponents   []string   `json:"extra_components,omitempty"`
	MissingEdges      []ArchEdge `json:"missing_edges,omitempty"`
	ExtraEdges        []ArchEdge `json:"extra_edges,omitempty"`
	Summary           string     `json:"summary"`
}
