package oculus

import (
	"fmt"
	"sort"
	"strings"
)

// DetectPipelines finds linear call chains where each function's return
// types overlap with the next function's parameter types. These represent
// data transformation pipelines.
type funcSig struct {
	paramTypes  []string
	returnTypes []string
}

func DetectPipelines(sg *SymbolGraph, minLength int) *PipelineReport {
	if sg == nil || len(sg.Edges) == 0 {
		return &PipelineReport{Summary: "no edges"}
	}
	if minLength <= 0 {
		minLength = 3
	}

	// Build adjacency and type lookup from "call" edges with type info.
	adj := make(map[string][]SymbolEdge)     // source → outgoing call edges
	inDeg := make(map[string]int)             // target → incoming edge count
	calleeSig := make(map[string]funcSig)     // FQN → callee's signature

	for _, e := range sg.Edges {
		if e.Kind != "call" {
			continue
		}
		adj[e.SourceFQN] = append(adj[e.SourceFQN], e)
		inDeg[e.TargetFQN]++
		if len(e.ParamTypes) > 0 || len(e.ReturnTypes) > 0 {
			calleeSig[e.TargetFQN] = funcSig{paramTypes: e.ParamTypes, returnTypes: e.ReturnTypes}
		}
	}

	// Also capture caller signatures from edges where they appear as callees.
	// The caller's own signature comes from edges where it IS the callee.
	// Additionally, for roots (no incoming), we look at their outgoing edges.

	// Find chain starting points: nodes with call edges but no incoming calls,
	// or with multiple incoming calls (branching breaks linearity).
	visited := make(map[string]bool)
	var pipelines []Pipeline

	// Walk from each node that has outgoing edges
	var roots []string
	for src := range adj {
		roots = append(roots, src)
	}
	sort.Strings(roots)

	for _, start := range roots {
		if visited[start] {
			continue
		}
		chain := buildChain(start, adj, calleeSig, visited)
		if len(chain.Steps) >= minLength {
			pipelines = append(pipelines, chain)
		}
	}

	// Sort by length descending
	sort.Slice(pipelines, func(i, j int) bool {
		return pipelines[i].Length > pipelines[j].Length
	})

	summary := fmt.Sprintf("%d pipeline(s) detected (min length %d)", len(pipelines), minLength)
	return &PipelineReport{Pipelines: pipelines, Summary: summary}
}

// buildChain walks forward from start, following single outgoing call edges
// where return types overlap with the next function's param types.
func buildChain(start string, adj map[string][]SymbolEdge, sigs map[string]funcSig, visited map[string]bool) Pipeline {
	var steps []PipelineStep
	var typeChain []string

	current := start
	for {
		if visited[current] {
			break
		}
		visited[current] = true

		sig := sigs[current]
		steps = append(steps, PipelineStep{
			FQN:         current,
			ParamTypes:  sig.paramTypes,
			ReturnTypes: sig.returnTypes,
		})

		outs := adj[current]
		if len(outs) != 1 {
			break // no outgoing or fork — end of chain
		}

		next := outs[0]
		nextSig := sigs[next.TargetFQN]

		// Check type overlap: current's return types vs next's param types
		overlap, found := typesOverlap(sig.returnTypes, nextSig.paramTypes)
		if !found {
			break
		}
		typeChain = append(typeChain, overlap)
		current = next.TargetFQN
	}

	return Pipeline{Steps: steps, TypeChain: typeChain, Length: len(steps)}
}

// typesOverlap checks if any non-error return type appears in the param types.
// Returns the first matching type and true, or empty and false.
func typesOverlap(returnTypes, paramTypes []string) (string, bool) {
	if len(returnTypes) == 0 || len(paramTypes) == 0 {
		return "", false
	}
	paramSet := make(map[string]bool, len(paramTypes))
	for _, p := range paramTypes {
		paramSet[normalizeType(p)] = true
	}
	for _, r := range returnTypes {
		nr := normalizeType(r)
		if nr == "error" {
			continue // skip error to avoid false positives
		}
		if paramSet[nr] {
			return r, true
		}
	}
	return "", false
}

// normalizeType strips pointer prefix for fuzzy matching.
func normalizeType(t string) string {
	return strings.TrimPrefix(t, "*")
}
