package book

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type graphDef struct {
	Nodes []BookNode `yaml:"nodes"`
	Edges []BookEdge `yaml:"edges"`
}

// Load reads graph.yaml from bookDir and builds a BookGraph.
func Load(bookDir string) (*BookGraph, error) {
	data, err := os.ReadFile(filepath.Join(bookDir, "graph.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read graph.yaml: %w", err)
	}

	var def graphDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse graph.yaml: %w", err)
	}

	g := &BookGraph{
		Nodes:   make(map[string]BookNode, len(def.Nodes)),
		Edges:   def.Edges,
		bookDir: bookDir,
	}
	for _, n := range def.Nodes {
		g.Nodes[n.ID] = n
	}
	return g, nil
}

// LoadContent reads the markdown file for a node and returns it.
func (g *BookGraph) LoadContent(bookDir, nodeID string) (string, error) {
	n, ok := g.Nodes[nodeID]
	if !ok {
		return "", fmt.Errorf("node %q not found", nodeID)
	}
	if n.Content != "" {
		return n.Content, nil
	}
	data, err := os.ReadFile(filepath.Join(bookDir, n.Path))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", n.Path, err)
	}
	n.Content = string(data)
	g.Nodes[nodeID] = n
	return n.Content, nil
}
