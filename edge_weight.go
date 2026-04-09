package oculus

import "strings"

// Edge weight constants for the classification model.
const (
	WeightInternalCross = 1.0  // cross-component call — architectural signal
	WeightInternalSame  = 0.5  // same-component call — internal coupling
	WeightExternalMeaningful = 0.3  // infrastructure libs (net/http, database/sql)
	WeightStdlibPlumbing     = 0.01 // ubiquitous stdlib (fmt, strings, errors)
)

// stdlibPlumbing contains packages that appear in nearly every Go file.
// Calls to these carry almost no architectural information.
var stdlibPlumbing = map[string]bool{
	"fmt": true, "strings": true, "errors": true, "strconv": true,
	"sort": true, "bytes": true, "math": true, "unicode": true,
	"log": true, "log/slog": true, "slices": true, "maps": true,
	"sync": true, "context": true, "time": true,
	"os": true, "io": true, "path": true, "path/filepath": true,
}

// stdlibMeaningful contains packages that signal real infrastructure choices.
var stdlibMeaningful = map[string]bool{
	"net/http": true, "net": true, "net/rpc": true,
	"database/sql": true,
	"encoding/json": true, "encoding/xml": true, "encoding/csv": true,
	"os/exec": true, "os/signal": true,
	"crypto": true, "crypto/tls": true,
	"io/fs": true,
	"html/template": true, "text/template": true,
	"reflect": true, "unsafe": true,
	"go/ast": true, "go/parser": true, "go/token": true, "go/types": true,
}

// ClassifyEdgeWeight returns a weight for a symbol edge based on whether
// the source and target are in the same component, different components,
// or external to the workspace.
func ClassifyEdgeWeight(source, target string, components []string) float64 {
	srcComp := resolveComponent(source, components)
	tgtComp := resolveComponent(target, components)

	// Both internal
	if srcComp != "" && tgtComp != "" {
		if srcComp != tgtComp {
			return WeightInternalCross
		}
		return WeightInternalSame
	}

	// Target is external — classify by package
	tgtPkg := extractPackage(target)
	if stdlibPlumbing[tgtPkg] {
		return WeightStdlibPlumbing
	}
	if stdlibMeaningful[tgtPkg] {
		return WeightExternalMeaningful
	}

	// Unknown external — default to meaningful (third-party libs, etc.)
	return WeightExternalMeaningful
}

// resolveComponent finds which component a FQN belongs to.
func resolveComponent(fqn string, components []string) string {
	// FQN format: "package.Symbol" or "package/subpkg.Symbol"
	pkg := extractPackage(fqn)
	for _, c := range components {
		if pkg == c || strings.HasPrefix(pkg, c+"/") {
			return c
		}
	}
	return ""
}

// extractPackage returns the package portion of a FQN ("pkg.Func" → "pkg").
// Normalizes GOROOT/GOPATH absolute paths to stdlib package names:
// "../../../../root/go/pkg/mod/golang.org/toolchain@.../src/sort.Strings" → "sort"
func extractPackage(fqn string) string {
	// Strip function/method name
	pkg := fqn
	if dot := strings.LastIndex(pkg, "."); dot >= 0 {
		pkg = pkg[:dot]
	}

	// Normalize external paths: find the stdlib package name after /src/
	if idx := strings.LastIndex(pkg, "/src/"); idx >= 0 {
		pkg = pkg[idx+5:]
	}

	return pkg
}
