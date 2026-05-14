package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	oculus "github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/ts"
)

// asyncContextAt returns the CallEdge* kind for the call site at (line, col)
// in callerFile, or "" if the call is synchronous.
//
// It parses callerFile with the tree-sitter grammar matching its extension,
// finds the deepest node that contains the call site, then walks ancestors
// looking for async wrapper constructs. Results are not cached — callerFile
// is small relative to the rest of the analysis and parse is fast.
//
// line and col are 1-based (as stored in CallEdge.SiteLine/SiteCol).
// Returns "" when line/col are zero (position unknown) or the file cannot
// be read.
func asyncContextAt(root, callerFile string, line, col int) string {
	if line == 0 {
		return ""
	}

	// Resolve to absolute path.
	abs := callerFile
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, callerFile)
	}

	src, err := srcCache.get(abs)
	if err != nil {
		return ""
	}

	ext := filepath.Ext(abs)
	lang, grammar := grammarForExt(ext)
	if grammar == nil {
		return ""
	}

	parser := ts.NewParser()
	parser.SetLanguage(grammar())
	tree, err := parser.Parse(src)
	if err != nil {
		return ""
	}

	// Convert 1-based line/col → 0-based byte offset.
	offset := lineColToOffset(src, line-1, col-1)
	if offset < 0 {
		return ""
	}

	// Walk the tree to find all ancestors of the byte at offset.
	// The ts.Node interface has no Parent() or descendant-by-byte method,
	// so we collect the ancestor chain by recursive descent.
	var ancestors []ts.Node
	gatherAncestors(tree.RootNode(), offset, &ancestors)

	// Walk from deepest ancestor outward.
	for i := len(ancestors) - 1; i >= 0; i-- {
		if kind := nodeAsyncKind(ancestors[i], src, lang); kind != "" {
			return kind
		}
	}
	return ""
}

// gatherAncestors does a recursive descent and appends every ancestor of
// the node containing byteOffset to out, root first.
func gatherAncestors(n ts.Node, byteOffset int, out *[]ts.Node) bool {
	if byteOffset < n.StartByte() || byteOffset >= n.EndByte() {
		return false
	}
	*out = append(*out, n)
	for i := 0; i < int(n.ChildCount()); i++ {
		if gatherAncestors(n.Child(i), byteOffset, out) {
			return true
		}
	}
	return true
}

// nodeAsyncKind returns the async edge kind for a single AST node, or "".
func nodeAsyncKind(n ts.Node, src []byte, lang string) string {
	t := n.Type()
	switch lang {
	case "go":
		switch t {
		case "go_statement":
			return oculus.CallEdgeGoroutine
		case "send_statement":
			return oculus.CallEdgeChanSend
		case "unary_expression":
			// <- operator
			for i := 0; i < int(n.ChildCount()); i++ {
				if n.Child(i).Content(src) == "<-" {
					return oculus.CallEdgeChanRecv
				}
			}
		}

	case "typescript", "javascript":
		switch t {
		case "await_expression":
			return oculus.CallEdgeAwait
		case "call_expression":
			// .then/.catch/.finally
			if fn := n.ChildByFieldName("function"); fn != nil && fn.Type() == "member_expression" {
				if prop := fn.ChildByFieldName("property"); prop != nil {
					switch prop.Content(src) {
					case "then", "catch", "finally":
						return oculus.CallEdgePromise
					}
				}
			}
		}

	case "python":
		if t == "await" {
			return oculus.CallEdgeAwait
		}

	case "rust":
		switch t {
		case "await_expression":
			return oculus.CallEdgeAwait
		case "call_expression":
			if fn := n.ChildByFieldName("function"); fn != nil {
				if extractSimpleName(fn, src) == "spawn" {
					return oculus.CallEdgeTaskSpawn
				}
			} else if n.ChildCount() > 0 {
				if extractSimpleName(n.Child(0), src) == "spawn" {
					return oculus.CallEdgeTaskSpawn
				}
			}
		}

	case "kotlin":
		if t == "call_expression" {
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				if child.Type() == "simple_identifier" {
					switch child.Content(src) {
					case "launch", "async", "runBlocking", "withContext":
						return oculus.CallEdgeTaskSpawn
					}
				}
			}
		}

	case "csharp":
		if t == "await_expression" {
			return oculus.CallEdgeAwait
		}

	case "swift":
		if t == "await_expression" {
			return oculus.CallEdgeAwait
		}

	case "c":
		if t == "call_expression" {
			if fn := n.ChildByFieldName("function"); fn != nil {
				switch fn.Content(src) {
				case "pthread_create", "thrd_create":
					return oculus.CallEdgeGoroutine
				}
			}
		}

	case "cpp":
		switch t {
		case "call_expression":
			var fnName string
			if fn := n.ChildByFieldName("function"); fn != nil {
				fnName = extractSimpleName(fn, src)
			} else if n.ChildCount() > 0 {
				fnName = extractSimpleName(n.Child(0), src)
			}
			if fnName == "async" {
				return oculus.CallEdgeTaskSpawn
			}
		case "co_await_expression":
			return oculus.CallEdgeAwait
		case "declaration":
			for i := 0; i < int(n.ChildCount()); i++ {
				child := n.Child(i)
				if (child.Type() == "type_identifier" || child.Type() == "qualified_identifier") &&
					extractSimpleName(child, src) == "thread" {
					return oculus.CallEdgeGoroutine
				}
			}
		}
	}

	return ""
}

// grammarForExt returns the language string and tree-sitter grammar factory
// for the given file extension, or ("", nil) for unknown extensions.
func grammarForExt(ext string) (string, func() ts.Language) {
	switch ext {
	case extGo:
		return "go", ts.Go
	case extRust:
		return "rust", ts.Rust
	case extPy:
		return "python", ts.Python
	case extTS, extTSX:
		return "typescript", ts.TypeScript
	case extJS, extJSX:
		return "javascript", ts.TypeScript // TS grammar handles JS
	case extJava:
		return "java", ts.Java
	case extKt:
		return "kotlin", ts.Kotlin
	case extCS:
		return "csharp", ts.CSharp
	case extSwift:
		return "swift", ts.Swift
	case extC, extH:
		return "c", ts.C
	case extCpp, extHpp:
		return "cpp", ts.Cpp
	}
	return "", nil
}

// lineColToOffset converts 0-based line/col to a byte offset in src.
// Returns -1 if line is out of range.
func lineColToOffset(src []byte, line, col int) int {
	cur := 0
	for l := 0; l < line; l++ {
		idx := strings.IndexByte(string(src[cur:]), '\n')
		if idx < 0 {
			return -1 // line out of range
		}
		cur += idx + 1
	}
	cur += col
	if cur > len(src) {
		return len(src) - 1
	}
	return cur
}

// srcCache is a simple process-lifetime read cache for source files.
// Avoids re-reading the same file for every outgoing call edge.
var srcCache = &fileCache{m: make(map[string][]byte)}

type fileCache struct {
	mu sync.RWMutex
	m  map[string][]byte
}

func (c *fileCache) get(path string) ([]byte, error) {
	c.mu.RLock()
	if src, ok := c.m[path]; ok {
		c.mu.RUnlock()
		return src, nil
	}
	c.mu.RUnlock()

	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c.mu.Lock()
	c.m[path] = src
	c.mu.Unlock()
	return src, nil
}
