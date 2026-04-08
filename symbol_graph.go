package oculus

// MergeSymbolGraph builds a unified SymbolGraph from call graph, type,
// and reference data. Deduplicates nodes by FQN and edges by
// (source, target, kind) triple.
func MergeSymbolGraph(cg *CallGraph, classes []ClassInfo, impls []ImplEdge, refs []FieldRef) *SymbolGraph {
	nodeMap := make(map[string]SymbolNode)
	type edgeKey struct{ source, target, kind string }
	edgeSet := make(map[edgeKey]SymbolEdge)

	fqn := func(pkg, name string) string {
		if pkg == "" {
			return name
		}
		return pkg + "." + name
	}

	// Nodes + edges from CallGraph
	if cg != nil {
		for _, n := range cg.Nodes {
			key := fqn(n.Package, n.Name)
			if _, exists := nodeMap[key]; !exists {
				nodeMap[key] = SymbolNode{
					Name: n.Name, Package: n.Package, Kind: "function",
					File: n.File, Line: n.Line, EndLine: n.EndLine,
					Exported: isUpper(n.Name),
				}
			}
		}
		for _, e := range cg.Edges {
			src := fqn(e.CallerPkg, e.Caller)
			tgt := fqn(e.CalleePkg, e.Callee)
			ek := edgeKey{src, tgt, "call"}
			if _, exists := edgeSet[ek]; !exists {
				edgeSet[ek] = SymbolEdge{
					SourceFQN: src, TargetFQN: tgt, Kind: "call",
					File: e.File, Line: e.Line, EndLine: e.EndLine,
					ParamTypes: e.ParamTypes, ReturnTypes: e.ReturnTypes,
				}
			}
		}
	}

	// Nodes from ClassInfo (types + methods)
	for _, ci := range classes {
		key := fqn(ci.Package, ci.Name)
		if _, exists := nodeMap[key]; !exists {
			nodeMap[key] = SymbolNode{
				Name: ci.Name, Package: ci.Package, Kind: ci.Kind,
				File: ci.File, Line: ci.Line, EndLine: ci.EndLine,
				Exported: ci.Exported,
			}
		}
		for _, m := range ci.Methods {
			mKey := fqn(ci.Package, ci.Name+"."+m.Name)
			if _, exists := nodeMap[mKey]; !exists {
				nodeMap[mKey] = SymbolNode{
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

	nodes := make([]SymbolNode, 0, len(nodeMap))
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
