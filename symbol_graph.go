package oculus

import "strings"

// normalizeReceiverName rewrites Go method names so pointer/value receivers
// collapse to Type.Method: (*T).M, *T.M, T.M → T.M.
func normalizeReceiverName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	// (*Type).Method
	if strings.HasPrefix(name, "(*") {
		if end := strings.Index(name, ")."); end > 2 {
			return name[2:end] + "." + name[end+2:]
		}
	}
	// *Type.Method (no parens)
	if strings.HasPrefix(name, "*") {
		name = name[1:]
	}
	return name
}

// methodOfType reports whether methodFQN is a method of typeFQN
// (pkg.Type), accepting Go receiver spellings.
func methodOfType(typeFQN, methodFQN string) bool {
	if typeFQN == "" || methodFQN == "" {
		return false
	}
	if strings.HasPrefix(methodFQN, typeFQN+".") {
		return true
	}
	// pkg.Type → try pkg.(*Type).Method and pkg.*Type.Method
	pkg, typ, ok := splitPkgType(typeFQN)
	if !ok {
		return false
	}
	candidates := []string{
		pkg + ".(*" + typ + ").",
		pkg + ".*" + typ + ".",
	}
	for _, prefix := range candidates {
		if strings.HasPrefix(methodFQN, prefix) {
			return true
		}
	}
	// Compare canonical forms when packages match.
	canonType := pkg + "." + normalizeReceiverName(typ)
	canonMethod := canonicalizeFQN(methodFQN)
	return strings.HasPrefix(canonMethod, canonType+".")
}

func splitPkgType(typeFQN string) (pkg, typ string, ok bool) {
	i := strings.LastIndex(typeFQN, ".")
	if i <= 0 || i == len(typeFQN)-1 {
		return "", "", false
	}
	return typeFQN[:i], typeFQN[i+1:], true
}

// canonicalizeFQN normalizes receiver spellings inside a full FQN.
func canonicalizeFQN(fqn string) string {
	pkg, name, ok := splitPkgType(fqn)
	if !ok {
		return normalizeReceiverName(fqn)
	}
	// Name may itself be (*T).M — normalize the whole name part.
	if strings.Contains(name, "(*") || strings.HasPrefix(name, "*") {
		return pkg + "." + normalizeReceiverName(name)
	}
	// pkg.(*Type).Method — splitPkgType only took last dot; recover.
	if idx := strings.Index(fqn, ".(*"); idx > 0 {
		pkg = fqn[:idx]
		rest := fqn[idx+1:] // (*Type).Method
		return pkg + "." + normalizeReceiverName(rest)
	}
	if idx := strings.Index(fqn, ".*"); idx > 0 {
		pkg = fqn[:idx]
		rest := fqn[idx+1:] // *Type.Method
		return pkg + "." + normalizeReceiverName(rest)
	}
	return fqn
}

// MergeSymbolGraph builds a unified SymbolGraph from call graph, type,
// and reference data. Deduplicates nodes by FQN and edges by
// (source, target, kind) triple.
func MergeSymbolGraph(cg *CallGraph, classes []ClassInfo, impls []ImplEdge, refs []FieldRef) *SymbolGraph {
	nodeMap := make(map[string]Symbol)
	type edgeKey struct{ source, target, kind string }
	edgeSet := make(map[edgeKey]SymbolEdge)

	fqn := func(pkg, name string) string {
		name = normalizeReceiverName(name)
		if pkg == "" {
			return name
		}
		return pkg + "." + name
	}

	// Nodes + edges from CallGraph
	if cg != nil {
		for _, n := range cg.Nodes {
			normName := normalizeReceiverName(n.Name)
			key := fqn(n.Package, n.Name)
			if existing, exists := nodeMap[key]; !exists || existing.Kind == "" {
				kind := n.Kind
				if kind == "" {
					kind = "function"
					if strings.Contains(normName, ".") {
						kind = "method"
					}
				}
				nodeMap[key] = Symbol{
					Name: normName, Package: n.Package, Kind: kind,
					File: n.File, Line: n.Line, EndLine: n.EndLine,
					Exported: isUpper(strings.TrimPrefix(strings.TrimPrefix(normName, "*"), "(")),
				}
			}
		}
		for _, e := range cg.Edges {
			src := fqn(e.CallerPkg, e.Caller)
			tgt := fqn(e.CalleePkg, e.Callee)
			kind := e.Kind
			if kind == "" {
				kind = "call"
			}
			ek := edgeKey{src, tgt, kind}
			if _, exists := edgeSet[ek]; !exists {
				edgeSet[ek] = SymbolEdge{
					SourceFQN: src, TargetFQN: tgt, Kind: kind,
					File: e.File, Line: e.Line, EndLine: e.EndLine,
					ParamTypes: e.ParamTypes, ReturnTypes: e.ReturnTypes,
					Args: e.Args, Weight: e.Confidence, Layer: cg.Layer,
				}
			}
		}
	}

	// Nodes from ClassInfo (types + methods)
	for _, ci := range classes {
		key := fqn(ci.Package, ci.Name)
		if _, exists := nodeMap[key]; !exists {
			nodeMap[key] = Symbol{
				Name: ci.Name, Package: ci.Package, Kind: ci.Kind,
				File: ci.File, Line: ci.Line, EndLine: ci.EndLine,
				Exported: ci.Exported,
			}
		}
		for _, m := range ci.Methods {
			mKey := fqn(ci.Package, ci.Name+"."+m.Name)
			if _, exists := nodeMap[mKey]; !exists {
				nodeMap[mKey] = Symbol{
					Name: ci.Name + "." + m.Name, Package: ci.Package, Kind: "method",
					File: m.File, Line: m.Line, EndLine: m.EndLine,
					Exported: m.Exported,
				}
			}
		}
	}

	// Edges from ImplEdge
	for _, impl := range impls {
		ek := edgeKey{impl.From, impl.To, impl.Kind}
		if _, exists := edgeSet[ek]; !exists {
			edgeSet[ek] = SymbolEdge{
				SourceFQN: impl.From, TargetFQN: impl.To, Kind: impl.Kind,
			}
		}
	}

	// Edges from FieldRef
	for _, ref := range refs {
		ek := edgeKey{ref.Owner, ref.RefType, "field_ref"}
		if _, exists := edgeSet[ek]; !exists {
			edgeSet[ek] = SymbolEdge{
				SourceFQN: ref.Owner, TargetFQN: ref.RefType, Kind: "field_ref",
			}
		}
	}

	nodes := make([]Symbol, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	edges := make([]SymbolEdge, 0, len(edgeSet))
	for _, e := range edgeSet {
		edges = append(edges, e)
	}

	return &SymbolGraph{Nodes: nodes, Edges: edges}
}

func isUpper(s string) bool {
	if s == "" {
		return false
	}
	return s[0] >= 'A' && s[0] <= 'Z'
}
