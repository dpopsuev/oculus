package arch

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	errUnsupportedFormat = errors.New("unsupported format")
	errNoMermaidContent  = errors.New("no components or edges found in mermaid input")
)

// ArchDrift — moved to root package.
// Type alias in arch/compat.go provides backward compatibility.

// ValidateArchitecture computes the drift between a desired and actual ArchModel.
// Component and edge endpoints match after normalizing `_` ↔ `/` so mermaid
// node IDs like internal_mcp align with package paths internal/mcp.
func ValidateArchitecture(desired, actual ArchModel) *ArchDrift {
	desiredComps := make(map[string]bool, len(desired.Services))
	for i := range desired.Services {
		desiredComps[normalizeComponentName(desired.Services[i].Name)] = true
	}
	actualComps := make(map[string]bool, len(actual.Services))
	actualDisplay := make(map[string]string, len(actual.Services)) // norm → first seen display name
	for i := range actual.Services {
		n := normalizeComponentName(actual.Services[i].Name)
		actualComps[n] = true
		if _, ok := actualDisplay[n]; !ok {
			actualDisplay[n] = actual.Services[i].Name
		}
	}
	desiredDisplay := make(map[string]string, len(desired.Services))
	for i := range desired.Services {
		n := normalizeComponentName(desired.Services[i].Name)
		if _, ok := desiredDisplay[n]; !ok {
			desiredDisplay[n] = desired.Services[i].Name
		}
	}

	var missing, extra []string
	for c := range desiredComps {
		if !actualComps[c] {
			missing = append(missing, desiredDisplay[c])
		}
	}
	for c := range actualComps {
		if !desiredComps[c] {
			extra = append(extra, actualDisplay[c])
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)

	type edgeKey struct{ from, to string }
	desiredEdges := make(map[edgeKey]ArchEdge, len(desired.Edges))
	for _, e := range desired.Edges {
		k := edgeKey{normalizeComponentName(e.From), normalizeComponentName(e.To)}
		desiredEdges[k] = e
	}
	actualEdges := make(map[edgeKey]ArchEdge, len(actual.Edges))
	for _, e := range actual.Edges {
		k := edgeKey{normalizeComponentName(e.From), normalizeComponentName(e.To)}
		actualEdges[k] = e
	}

	var missingEdges, extraEdges []ArchEdge
	for k, e := range desiredEdges {
		if _, ok := actualEdges[k]; !ok {
			missingEdges = append(missingEdges, e)
		}
	}
	for k, e := range actualEdges {
		if _, ok := desiredEdges[k]; !ok {
			extraEdges = append(extraEdges, e)
		}
	}
	sortEdges(missingEdges)
	sortEdges(extraEdges)

	summary := fmt.Sprintf("components: %d missing, %d extra; edges: %d missing, %d extra",
		len(missing), len(extra), len(missingEdges), len(extraEdges))

	return &ArchDrift{
		MissingComponents: missing,
		ExtraComponents:   extra,
		MissingEdges:      missingEdges,
		ExtraEdges:        extraEdges,
		Summary:           summary,
	}
}

// ParseDesiredState parses a desired architecture from mermaid or JSON input.
func ParseDesiredState(input, format string) (*ArchModel, error) {
	switch strings.ToLower(format) {
	case "json":
		return parseDesiredJSON(input)
	case "mermaid", "":
		return parseDesiredMermaid(input)
	default:
		return nil, fmt.Errorf("%w: %s (use json or mermaid)", errUnsupportedFormat, format)
	}
}

func parseDesiredJSON(input string) (*ArchModel, error) {
	var m ArchModel
	if err := json.Unmarshal([]byte(input), &m); err != nil {
		return nil, fmt.Errorf("parse JSON architecture: %w", err)
	}
	return &m, nil
}

var (
	mermaidNodeRe = regexp.MustCompile(`^\s+(\w+)\[["']?([^"'\]]+)["']?\]`)
	mermaidEdgeRe = regexp.MustCompile(`^\s+(\w+)\s+--[->]+(?:\|[^|]*\|)?\s*(\w+)`)
)

func parseDesiredMermaid(input string) (*ArchModel, error) {
	m := &ArchModel{}
	nodeLabels := make(map[string]string)

	for _, line := range strings.Split(input, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "graph") || strings.HasPrefix(trimmed, "%%") {
			continue
		}

		if matches := mermaidNodeRe.FindStringSubmatch(line); matches != nil {
			id, label := matches[1], matches[2]
			nodeLabels[id] = label
			continue
		}

		if matches := mermaidEdgeRe.FindStringSubmatch(line); matches != nil {
			from, to := matches[1], matches[2]
			fromName := nodeLabels[from]
			if fromName == "" {
				fromName = from
			}
			toName := nodeLabels[to]
			if toName == "" {
				toName = to
			}
			m.Edges = append(m.Edges, ArchEdge{From: fromName, To: toName})

			if !hasService(m.Services, fromName) {
				m.Services = append(m.Services, ArchService{Name: fromName})
			}
			if !hasService(m.Services, toName) {
				m.Services = append(m.Services, ArchService{Name: toName})
			}
			continue
		}

		// Bare node reference (just an ID on a line, possibly as part of an edge)
	}

	if len(m.Services) == 0 && len(m.Edges) == 0 {
		return nil, errNoMermaidContent
	}
	return m, nil
}

func hasService(services []ArchService, name string) bool {
	for i := range services {
		if services[i].Name == name {
			return true
		}
	}
	return false
}

// normalizeComponentName maps mermaid-safe IDs and package paths onto one key:
// internal_mcp, internal/mcp, and Internal/MCP all become "internal/mcp".
func normalizeComponentName(name string) string {
	n := strings.TrimSpace(name)
	n = strings.ReplaceAll(n, "_", "/")
	n = strings.ReplaceAll(n, "\\", "/")
	for strings.Contains(n, "//") {
		n = strings.ReplaceAll(n, "//", "/")
	}
	return strings.ToLower(n)
}

func sortEdges(edges []ArchEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From != edges[j].From {
			return edges[i].From < edges[j].From
		}
		return edges[i].To < edges[j].To
	})
}
