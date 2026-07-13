package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/arch"
)

// ComplexityHintsResult is the MCP/engine payload for complexity_hints.
type ComplexityHintsResult struct {
	HotSpots   []arch.HotSpot `json:"hot_spots"`
	Disclaimer string         `json:"disclaimer"`
}

// GetComplexityHints returns hotspot list annotated with AST algo-pattern heuristics.
func (p *Engine) GetComplexityHints(ctx context.Context, path string, topN int, cacheKey ...string) (*ComplexityHintsResult, error) {
	path = p.resolvePath(path)
	report, err := p.getOrScan(ctx, path, cacheKey...)
	if err != nil {
		return nil, err
	}
	spots := make([]arch.HotSpot, len(report.HotSpots))
	copy(spots, report.HotSpots)
	sort.Slice(spots, func(i, j int) bool {
		if spots[i].Churn != spots[j].Churn {
			return spots[i].Churn > spots[j].Churn
		}
		return spots[i].FanIn > spots[j].FanIn
	})
	if topN <= 0 {
		topN = 10
	}
	enriched := arch.EnrichHotSpotsComplexity(path, spots, topN)
	return &ComplexityHintsResult{
		HotSpots:   enriched,
		Disclaimer: arch.ComplexityDisclaimer,
	}, nil
}

// TaintDisclaimer documents the non-CPG nature of the built-in path finder.
const TaintDisclaimer = "heuristic call-graph BFS on symbol edges — not a sound CPG/PDG taint analysis; set LOCUS_TAINT_CMD for Joern/CodeQL federation"

// TaintResult is the response for Engine.TaintQuery.
type TaintResult struct {
	Source     string   `json:"source"`
	Sink       string   `json:"sink"`
	Engine     string   `json:"engine"` // "heuristic" | "federated"
	Found      bool     `json:"found"`
	Path       []string `json:"path,omitempty"`
	Disclaimer string   `json:"disclaimer"`
	Federated  string   `json:"federated,omitempty"`
}

// TaintQuery walks symbol-graph call edges source→sink (BFS). If LOCUS_TAINT_CMD
// is set (template with {source} {sink} {path}), runs it and attaches stdout.
func (p *Engine) TaintQuery(ctx context.Context, path, source, sink string, cacheKey ...string) (*TaintResult, error) {
	path = p.resolvePath(path)
	if source == "" || sink == "" {
		return nil, fmt.Errorf("source and sink are required")
	}
	out := &TaintResult{
		Source:     source,
		Sink:       sink,
		Engine:     "heuristic",
		Disclaimer: TaintDisclaimer,
	}

	sg, err := p.GetSymbolGraph(ctx, path, SymbolGraphOpts{Quick: true})
	if err == nil && sg != nil {
		srcFQN := matchSymbol(sg, source)
		dstFQN := matchSymbol(sg, sink)
		if srcFQN != "" && dstFQN != "" {
			if hops := bfsCallPath(sg.Edges, srcFQN, dstFQN); len(hops) > 0 {
				out.Found = true
				out.Path = hops
			}
		}
	} else if err != nil && strings.TrimSpace(os.Getenv("LOCUS_TAINT_CMD")) == "" {
		return nil, err
	}

	if cmdTmpl := strings.TrimSpace(os.Getenv("LOCUS_TAINT_CMD")); cmdTmpl != "" {
		fed, ferr := runTaintFederation(ctx, cmdTmpl, path, source, sink)
		if ferr != nil {
			out.Federated = "error: " + ferr.Error()
		} else {
			out.Federated = fed
			out.Engine = "federated"
			if strings.TrimSpace(fed) != "" && !out.Found {
				out.Found = true
			}
		}
	}

	_ = cacheKey // reserved: future path-bound SG lookup
	return out, nil
}

func matchSymbol(sg *oculus.SymbolGraph, needle string) string {
	if sg == nil || needle == "" {
		return ""
	}
	needle = strings.TrimSpace(needle)
	var best string
	for _, n := range sg.Nodes {
		fqn := n.FQN()
		if fqn == needle || n.Name == needle {
			return fqn
		}
		if strings.HasSuffix(fqn, "."+needle) {
			best = fqn
		}
		if best == "" && strings.Contains(fqn, needle) {
			best = fqn
		}
	}
	for _, e := range sg.Edges {
		for _, fqn := range []string{e.SourceFQN, e.TargetFQN} {
			if fqn == needle {
				return fqn
			}
			if best == "" && (strings.HasSuffix(fqn, "."+needle) || strings.Contains(fqn, needle)) {
				best = fqn
			}
		}
	}
	return best
}

func bfsCallPath(edges []oculus.SymbolEdge, src, dst string) []string {
	adj := map[string][]string{}
	for _, e := range edges {
		kind := e.Kind
		if kind != "" && kind != "call" && !strings.Contains(kind, "call") && kind != "data_flow" && kind != "DATA_FLOWS" {
			continue
		}
		adj[e.SourceFQN] = append(adj[e.SourceFQN], e.TargetFQN)
	}
	if src == dst {
		return []string{src}
	}
	type item struct {
		node string
		path []string
	}
	q := []item{{src, []string{src}}}
	seen := map[string]bool{src: true}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		for _, next := range adj[cur.node] {
			if seen[next] {
				continue
			}
			np := append(append([]string{}, cur.path...), next)
			if next == dst {
				return np
			}
			seen[next] = true
			q = append(q, item{next, np})
		}
	}
	return nil
}

func runTaintFederation(ctx context.Context, tmpl, path, source, sink string) (string, error) {
	cmdStr := tmpl
	cmdStr = strings.ReplaceAll(cmdStr, "{source}", source)
	cmdStr = strings.ReplaceAll(cmdStr, "{sink}", sink)
	cmdStr = strings.ReplaceAll(cmdStr, "{path}", path)
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
	cmd.Dir = path
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%w: %s", err, truncate(string(out), 400))
	}
	return string(out), nil
}
