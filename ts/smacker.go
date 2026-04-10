package ts

import (
	"context"

	sitter "github.com/smacker/go-tree-sitter"
)

// SmackerParser wraps smacker/go-tree-sitter as a ts.Parser.
type SmackerParser struct {
	inner *sitter.Parser
}

// NewParser creates a ts.Parser backed by smacker/go-tree-sitter.
func NewParser() Parser {
	return &SmackerParser{inner: sitter.NewParser()}
}

func (p *SmackerParser) SetLanguage(lang Language) {
	p.inner.SetLanguage(lang.(*sitter.Language))
}

func (p *SmackerParser) Parse(src []byte) (Tree, error) {
	tree, err := p.inner.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return nil, err
	}
	return &smackerTree{inner: tree}, nil
}

func (p *SmackerParser) Close() {}

// smackerTree wraps *sitter.Tree.
type smackerTree struct {
	inner *sitter.Tree
}

func (t *smackerTree) RootNode() Node {
	return &smackerNode{inner: t.inner.RootNode()}
}

func (t *smackerTree) Close() {}

// smackerNode wraps *sitter.Node.
type smackerNode struct {
	inner *sitter.Node
}

func (n *smackerNode) Type() string {
	if n.inner == nil {
		return ""
	}
	return n.inner.Type()
}

func (n *smackerNode) Content(src []byte) string {
	if n.inner == nil {
		return ""
	}
	return n.inner.Content(src)
}

func (n *smackerNode) ChildCount() int {
	if n.inner == nil {
		return 0
	}
	return int(n.inner.ChildCount())
}

func (n *smackerNode) Child(i int) Node {
	if n.inner == nil {
		return nil
	}
	child := n.inner.Child(i)
	if child == nil {
		return nil
	}
	return &smackerNode{inner: child}
}

func (n *smackerNode) ChildByFieldName(name string) Node {
	if n.inner == nil {
		return nil
	}
	child := n.inner.ChildByFieldName(name)
	if child == nil {
		return nil
	}
	return &smackerNode{inner: child}
}

func (n *smackerNode) StartPoint() Point {
	if n.inner == nil {
		return Point{}
	}
	p := n.inner.StartPoint()
	return Point{Row: int(p.Row), Column: int(p.Column)}
}

func (n *smackerNode) EndPoint() Point {
	if n.inner == nil {
		return Point{}
	}
	p := n.inner.EndPoint()
	return Point{Row: int(p.Row), Column: int(p.Column)}
}

func (n *smackerNode) StartByte() int {
	if n.inner == nil {
		return 0
	}
	return int(n.inner.StartByte())
}

func (n *smackerNode) EndByte() int {
	if n.inner == nil {
		return 0
	}
	return int(n.inner.EndByte())
}

func (n *smackerNode) IsNamed() bool {
	if n.inner == nil {
		return false
	}
	return n.inner.IsNamed()
}

