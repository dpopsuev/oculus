package oculus

// ProbeResult contains all vitals for a single symbol — zero traversal.
type ProbeResult struct {
	FQN         string   `json:"fqn"`
	Package     string   `json:"package"`
	File        string   `json:"file,omitempty"`
	Line        int      `json:"line,omitempty"`
	EndLine     int      `json:"end_line,omitempty"`
	Kind        string   `json:"kind"`
	Exported    bool     `json:"exported"`
	Params      []string `json:"params,omitempty"`
	Returns     []string `json:"returns,omitempty"`
	FanIn       int      `json:"fan_in"`
	FanOut      int      `json:"fan_out"`
	Instability float64  `json:"instability"`
	CrossPkg    int      `json:"cross_pkg_callees"`
	Centrality  float64  `json:"centrality,omitempty"`
	Circuits    int      `json:"circuits"`
	Boundaries  []string `json:"boundaries,omitempty"`
}

// ScenarioNode is a symbol in a scenario trace with depth from the pivot.
type ScenarioNode struct {
	FQN             string `json:"fqn"`
	Package         string `json:"package,omitempty"`
	Depth           int    `json:"depth"`
	Kind            string `json:"kind"`
	FanOut          int    `json:"fan_out,omitempty"`
	DownstreamCount int    `json:"downstream_count,omitempty"`
}

// ScenarioResult is the bidirectional trace from a pivot symbol to system boundaries.
type ScenarioResult struct {
	Symbol     string         `json:"symbol"`
	Upstream   []ScenarioNode `json:"upstream"`
	Downstream []ScenarioNode `json:"downstream"`
	Edges      []SymbolEdge   `json:"edges"`
	Circuits   [][]string     `json:"circuits,omitempty"`
	Boundaries []string       `json:"boundaries,omitempty"`
}

// ConvergenceNode is a shared dependency reached by multiple input symbols.
type ConvergenceNode struct {
	FQN       string   `json:"fqn"`
	Converges int      `json:"converges"`
	Sources   []string `json:"sources"`
}

// ConvergenceResult shows where N symbols' downstream call trees overlap.
type ConvergenceResult struct {
	Symbols []string          `json:"symbols"`
	Nodes   []ConvergenceNode `json:"nodes"`
	Edges   []SymbolEdge      `json:"edges"`
}

// IsolateResult shows what disconnects when a symbol is removed from the graph.
type IsolateResult struct {
	Symbol           string     `json:"symbol"`
	ComponentsBefore int        `json:"components_before"`
	ComponentsAfter  int        `json:"components_after"`
	Disconnected     [][]string `json:"disconnected,omitempty"`
	OrphanedSymbols  int        `json:"orphaned_symbols"`
}
