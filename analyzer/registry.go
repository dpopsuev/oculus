package analyzer

import (
	"sort"

	"github.com/dpopsuev/oculus"
	"github.com/dpopsuev/oculus/lang"
	"github.com/dpopsuev/oculus/lsp"
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
	language lang.Language
	priority int
	factory  SymbolSourceFactory
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
func RegisterSource(language lang.Language, priority int, factory SymbolSourceFactory) {
	sourceRegistry = append(sourceRegistry, sourceEntry{
		language: language,
		priority: priority,
		factory:  factory,
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
func resolveSymbolSources(root string, pool lsp.Pool) []oculus.SymbolSource {
	detected := lang.DetectLanguage(root)
	var result []oculus.SymbolSource
	for _, entry := range sourceRegistry {
		if entry.language != lang.Unknown && entry.language != detected {
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
