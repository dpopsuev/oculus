package analyzer

import (
	"sync"

	gogit "github.com/go-git/go-git/v5"

	"github.com/dpopsuev/oculus/v3/lsp"
)

var deepCache sync.Map // key: "path@sha" → *DeepFallbackAnalyzer

// CachedDeepFallback returns a cached DeepFallbackAnalyzer for the given path.
// The cache key is (path, HEAD SHA, poolPresence) — a new commit or switching
// between Quick (nil pool) and full (pooled) must not reuse the wrong analyzer.
func CachedDeepFallback(path string, pool ...lsp.Pool) *DeepFallbackAnalyzer {
	sha := resolveHead(path)
	var p lsp.Pool
	if len(pool) > 0 {
		p = pool[0]
	}
	poolTag := "nopool"
	if p != nil {
		poolTag = "pool"
	}
	key := path + "@" + sha + "-" + poolTag

	if cached, ok := deepCache.Load(key); ok {
		return cached.(*DeepFallbackAnalyzer)
	}

	da := NewDeepFallback(path, p)
	deepCache.Store(key, da)
	return da
}

func resolveHead(path string) string {
	repo, err := gogit.PlainOpen(path)
	if err != nil {
		return "unknown"
	}
	ref, err := repo.Head()
	if err != nil {
		return "unknown"
	}
	return ref.Hash().String()
}
