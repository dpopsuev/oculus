package book

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type graphDef struct {
	Nodes []BookNode `yaml:"nodes"`
	Edges []BookEdge `yaml:"edges"`
}

// Load reads graph.yaml from bookDir on the filesystem.
func Load(bookDir string) (*BookGraph, error) {
	data, err := os.ReadFile(filepath.Join(bookDir, "graph.yaml"))
	if err != nil {
		return nil, fmt.Errorf("read graph.yaml: %w", err)
	}
	return parse(data, os.DirFS(bookDir))
}

// LoadEmbedded loads the Book from the compiled-in embed.FS.
func LoadEmbedded() (*BookGraph, error) {
	data, err := fs.ReadFile(content, "graph.yaml")
	if err != nil {
		return nil, fmt.Errorf("read embedded graph.yaml: %w", err)
	}
	return parse(data, content)
}

func parse(data []byte, fsys fs.FS) (*BookGraph, error) {
	var def graphDef
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parse graph.yaml: %w", err)
	}

	g := &BookGraph{
		Nodes: make(map[string]BookNode, len(def.Nodes)),
		Edges: def.Edges,
		fsys:  fsys,
	}
	for _, n := range def.Nodes {
		g.Nodes[n.ID] = n
	}
	return g, nil
}

// LoadContent reads the markdown file for a node and caches it.
func (g *BookGraph) LoadContent(nodeID string) (string, error) {
	n, ok := g.Nodes[nodeID]
	if !ok {
		return "", fmt.Errorf("node %q not found", nodeID)
	}
	if n.Content != "" {
		return n.Content, nil
	}
	if g.fsys == nil {
		return "", fmt.Errorf("no filesystem for content loading")
	}
	data, err := fs.ReadFile(g.fsys, n.Path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", n.Path, err)
	}
	n.Content = string(data)
	g.Nodes[nodeID] = n
	return n.Content, nil
}
