package analyzer

import (
	"sort"

	"github.com/dpopsuev/oculus/v3"
	"github.com/dpopsuev/oculus/v3/lang"
	"github.com/dpopsuev/oculus/v3/lsp"
)

// DeepAnalyzerFactory creates a DeepAnalyzer for a given root and optional LSP pool.
// Returns nil if the analyzer is not applicable (wrong language, missing tools).
type DeepAnalyzerFactory func(root string, pool lsp.Pool) oculus.DeepAnalyzer

// TypeAnalyzerFactory creates a TypeAnalyzer for a given root and optional LSP pool.
type TypeAnalyzerFactory func(root string, pool lsp.Pool) oculus.TypeAnalyzer

// SymbolSourceFactory creates a SymbolSource for a given root and optional LSP pool.
// Returns nil if the source is not applicable (wrong language, missing tools).
type SymbolSourceFactory func(root string, pool lsp.Pool) oculus.SymbolSource

type analyzerEntry struct {
	language lang.Language // lang.Unknown = matches any language
	priority int           // higher wins
	deep     DeepAnalyzerFactory
	typeA    TypeAnalyzerFactory
}

type sourceEntry struct {
	language       lang.Language
	priority       int
	maxGranularity oculus.Granularity // highest detail level this source provides
	factory        SymbolSourceFactory
}

var registry []analyzerEntry
var sourceRegistry []sourceEntry

// Register adds an analyzer factory to the global registry.
// Language lang.Unknown matches any detected language (for LSP, TreeSitter, Regex).
// Higher priority analyzers are tried first in the fallback chain.
func Register(language lang.Language, priority int, deep DeepAnalyzerFactory, typeA TypeAnalyzerFactory) {
	registry = append(registry, analyzerEntry{
		language: language,
		priority: priority,
		deep:     deep,
		typeA:    typeA,
	})
	sort.Slice(registry, func(i, j int) bool {
		return registry[i].priority > registry[j].priority
	})
}

// RegisterSource adds a SymbolSource factory to the global registry.
// SymbolSources are used by SymbolPipeline to provide bounded concurrent
// graph walks. Higher priority sources are tried first.
// maxGranularity declares the highest detail level this source provides.
func RegisterSource(language lang.Language, priority int, factory SymbolSourceFactory, maxGranularity ...oculus.Granularity) {
	mg := oculus.GranularityTypedCallGraph // default
	if len(maxGranularity) > 0 {
		mg = maxGranularity[0]
	}
	sourceRegistry = append(sourceRegistry, sourceEntry{
		language:       language,
		priority:       priority,
		maxGranularity: mg,
		factory:        factory,
	})
	sort.Slice(sourceRegistry, func(i, j int) bool {
		return sourceRegistry[i].priority > sourceRegistry[j].priority
	})
}

// resolveDeepAnalyzers returns all applicable DeepAnalyzers for a root, ordered by priority.
func resolveDeepAnalyzers(root string, pool lsp.Pool) []oculus.DeepAnalyzer {
	detected := lang.DetectLanguage(root)
	var result []oculus.DeepAnalyzer
	for _, entry := range registry {
		if entry.deep == nil {
			continue
		}
		if entry.language != lang.Unknown && entry.language != detected {
			continue
		}
		if a := entry.deep(root, pool); a != nil {
			result = append(result, a)
		}
	}
	return result
}

// resolveSymbolSources returns all applicable SymbolSources for a root, ordered by priority.
// If granularity is specified (non-zero), only sources that can satisfy it are returned.
// Sources are sorted cheapest-first when granularity filtering is active.
func resolveSymbolSources(root string, pool lsp.Pool, granularity ...oculus.Granularity) []oculus.SymbolSource {
	detected := lang.DetectLanguage(root)
	requested := oculus.GranularityDefault
	if len(granularity) > 0 {
		requested = granularity[0]
	}
	if requested == oculus.GranularityDefault {
		requested = oculus.GranularityTypedCallGraph
	}

	var result []oculus.SymbolSource
	for _, entry := range sourceRegistry {
		if entry.language != lang.Unknown && entry.language != detected {
			continue
		}
		// Skip sources that can't satisfy the requested granularity.
		if entry.maxGranularity < requested {
			continue
		}
		if src := entry.factory(root, pool); src != nil {
			result = append(result, src)
		}
	}
	return result
}

// resolveTypeAnalyzers returns all applicable TypeAnalyzers for a root, ordered by priority.
func resolveTypeAnalyzers(root string, pool lsp.Pool) []oculus.TypeAnalyzer {
	detected := lang.DetectLanguage(root)
	var result []oculus.TypeAnalyzer
	for _, entry := range registry {
		if entry.typeA == nil {
			continue
		}
		if entry.language != lang.Unknown && entry.language != detected {
			continue
		}
		if a := entry.typeA(root, pool); a != nil {
			result = append(result, a)
		}
	}
	return result
}
