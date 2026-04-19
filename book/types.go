package book

// BookNode is a knowledge entry in the graph.
type BookNode struct {
	ID       string   `json:"id" yaml:"id"`
	Path     string   `json:"path" yaml:"path"`
	Kind     string   `json:"kind" yaml:"kind"`
	Keywords []string `json:"keywords" yaml:"keywords"`
	About    string   `json:"about" yaml:"about"`
	Content  string   `json:"content,omitempty" yaml:"-"`
}

// BookEdge is a typed relationship between knowledge entries.
// Satisfies graph.Edge via Source()/Target().
type BookEdge struct {
	From string  `json:"from" yaml:"from"`
	To   string  `json:"to" yaml:"to"`
	Kind string  `json:"kind" yaml:"kind"`
	Weight float64 `json:"weight,omitempty" yaml:"weight,omitempty"`
}

func (e BookEdge) Source() string { return e.From }
func (e BookEdge) Target() string { return e.To }

// BookGraph is the knowledge mesh.
type BookGraph struct {
	Nodes   map[string]BookNode `json:"nodes"`
	Edges   []BookEdge          `json:"edges"`
	bookDir string
}

// BookResult is the query response — a subgraph neighborhood.
type BookResult struct {
	Entries []BookNode `json:"entries"`
	Edges   []BookEdge `json:"edges"`
	Roots   []string   `json:"roots"`
}
