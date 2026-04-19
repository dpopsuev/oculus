package book

import "embed"

//go:embed graph.yaml metrics smells principles patterns
var content embed.FS
