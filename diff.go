package oculus

import "sort"

// SymbolDiff reports symbol-level changes between two SymbolGraphs.
type SymbolDiff struct {
	Added    []string `json:"added"`
	Removed  []string `json:"removed"`
	Modified []string `json:"modified"`
}

// DiffSymbolGraphs compares two SymbolGraphs and reports symbol-level changes.
// Added: in after but not before. Removed: in before but not after.
// Modified: in both but Signature differs.
func DiffSymbolGraphs(before, after *SymbolGraph) *SymbolDiff {
	beforeMap := make(map[string]Symbol, len(before.Nodes))
	for _, s := range before.Nodes {
		beforeMap[s.FQN()] = s
	}

	afterMap := make(map[string]Symbol, len(after.Nodes))
	for _, s := range after.Nodes {
		afterMap[s.FQN()] = s
	}

	var added, removed, modified []string

	for fqn, afterSym := range afterMap {
		beforeSym, ok := beforeMap[fqn]
		if !ok {
			added = append(added, fqn)
			continue
		}
		if beforeSym.Signature != afterSym.Signature {
			modified = append(modified, fqn)
		}
	}

	for fqn := range beforeMap {
		if _, ok := afterMap[fqn]; !ok {
			removed = append(removed, fqn)
		}
	}

	sort.Strings(added)
	sort.Strings(removed)
	sort.Strings(modified)

	return &SymbolDiff{
		Added:    added,
		Removed:  removed,
		Modified: modified,
	}
}
