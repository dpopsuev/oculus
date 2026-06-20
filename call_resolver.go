package oculus

import "strings"

// Resolution confidence scores — short-circuit chain tries highest first.
const (
	ConfImportMap  = 0.95 // callee found in caller's import map
	ConfSamePkg    = 0.90 // callee in same package as caller
	ConfUniqueName = 0.75 // only one symbol with this bare name project-wide
	ConfSuffixMatch = 0.55 // multiple candidates, filtered by import reachability
)

// Resolution holds the result of resolving a callee name to a symbol.
type Resolution struct {
	Symbol     *Symbol
	Confidence float64
	Strategy   string
}

// CallResolver resolves bare callee names to fully-qualified symbols using a
// 4-strategy short-circuit chain: import_map → same_pkg → unique_name → suffix_match.
type CallResolver struct {
	byName map[string][]*Symbol // bare name → all candidates
	byFQN  map[string]*Symbol  // "pkg.Name" → symbol
}

// NewCallResolver builds resolution indexes from a symbol list.
func NewCallResolver(symbols []Symbol) *CallResolver {
	r := &CallResolver{
		byName: make(map[string][]*Symbol),
		byFQN:  make(map[string]*Symbol),
	}
	for i := range symbols {
		s := &symbols[i]
		r.byName[s.Name] = append(r.byName[s.Name], s)
		fqn := s.Package + "." + s.Name
		r.byFQN[fqn] = s
	}
	return r
}

// Resolve resolves a bare callee name given the caller's context.
// Returns the best match with confidence, or nil if unresolved.
func (r *CallResolver) Resolve(calleeName, callerPkg, callerFile string, fileImports []string) Resolution {
	// Strategy 1: import map — callee name matches a symbol reachable via imports.
	if len(fileImports) > 0 {
		if res := r.resolveImportMap(calleeName, fileImports); res.Symbol != nil {
			return res
		}
	}

	// Strategy 2: same package — callee exists in caller's package.
	if res := r.resolveSamePkg(calleeName, callerPkg); res.Symbol != nil {
		return res
	}

	// Strategy 3: unique name — exactly one symbol with this bare name.
	candidates := r.byName[calleeName]
	if len(candidates) == 1 {
		return Resolution{
			Symbol:     candidates[0],
			Confidence: ConfUniqueName,
			Strategy:   "unique_name",
		}
	}

	// Strategy 4: suffix match with import filtering.
	if len(candidates) > 1 {
		if res := r.resolveSuffixMatch(calleeName, candidates, fileImports); res.Symbol != nil {
			return res
		}
	}

	return Resolution{}
}

func (r *CallResolver) resolveImportMap(calleeName string, fileImports []string) Resolution {
	candidates := r.byName[calleeName]
	if len(candidates) == 0 {
		return Resolution{}
	}
	for _, s := range candidates {
		if isImportReachable(s.Package, fileImports) {
			return Resolution{
				Symbol:     s,
				Confidence: ConfImportMap,
				Strategy:   "import_map",
			}
		}
	}
	return Resolution{}
}

func (r *CallResolver) resolveSamePkg(calleeName, callerPkg string) Resolution {
	fqn := callerPkg + "." + calleeName
	if s, ok := r.byFQN[fqn]; ok {
		return Resolution{
			Symbol:     s,
			Confidence: ConfSamePkg,
			Strategy:   "same_pkg",
		}
	}
	return Resolution{}
}

func (r *CallResolver) resolveSuffixMatch(calleeName string, candidates []*Symbol, fileImports []string) Resolution {
	if len(fileImports) > 0 {
		var reachable []*Symbol
		for _, s := range candidates {
			if isImportReachable(s.Package, fileImports) {
				reachable = append(reachable, s)
			}
		}
		if len(reachable) == 1 {
			return Resolution{
				Symbol:     reachable[0],
				Confidence: ConfSuffixMatch,
				Strategy:   "suffix_match",
			}
		}
		if len(reachable) > 1 {
			// Multiple import-reachable candidates — pick first, penalize.
			penalty := min(1.0, 3.0/float64(len(reachable)))
			return Resolution{
				Symbol:     reachable[0],
				Confidence: ConfSuffixMatch * penalty,
				Strategy:   "suffix_match",
			}
		}
	}
	// No import info or no reachable candidates — use all, heavily penalized.
	penalty := min(1.0, 3.0/float64(len(candidates)))
	return Resolution{
		Symbol:     candidates[0],
		Confidence: ConfSuffixMatch * 0.5 * penalty,
		Strategy:   "suffix_match",
	}
}

// isImportReachable checks if a package is reachable from the given imports.
func isImportReachable(pkg string, imports []string) bool {
	for _, imp := range imports {
		if imp == pkg || strings.HasPrefix(pkg, imp+"/") || strings.HasSuffix(imp, "/"+pkg) {
			return true
		}
		// Match last path segment: import "github.com/foo/bar" reaches package "bar"
		if idx := strings.LastIndex(imp, "/"); idx >= 0 {
			if imp[idx+1:] == pkg || strings.HasPrefix(pkg, imp[idx+1:]+"/") {
				return true
			}
		}
		// Match tail: import "foo.bar.baz" reaches "baz" or "bar.baz"
		if strings.HasSuffix(imp, "."+pkg) || strings.HasSuffix(imp, "/"+pkg) {
			return true
		}
	}
	return false
}
