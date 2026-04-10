// Package ts provides a tree-sitter abstraction layer.
// Parsers import ts.Parser/ts.Node instead of a specific binding.
// The backend (smacker, official, etc.) is swapped by changing one file.
package ts

// Language identifies a tree-sitter grammar.
type Language interface{}

// Parser parses source code into a syntax tree.
type Parser interface {
	SetLanguage(lang Language)
	Parse(src []byte) (Tree, error)
	Close()
}

// Tree is the parsed syntax tree.
type Tree interface {
	RootNode() Node
	Close()
}

// Node is a single node in the syntax tree.
type Node interface {
	Type() string
	Content(src []byte) string
	ChildCount() int
	Child(i int) Node
	ChildByFieldName(name string) Node
	StartPoint() Point
	EndPoint() Point
	StartByte() int
	EndByte() int
	IsNamed() bool
}

// Point is a row/column position in source code.
type Point struct {
	Row    int
	Column int
}
