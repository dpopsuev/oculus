package analyzer

import (
	"sync"

	gogit "github.com/go-git/go-git/v5"

	"github.com/dpopsuev/oculus/v3/lsp"
)

var deepCache sync.Map // key: "path@sha" → *DeepFallbackAnalyzer

// CachedDeepFallback returns a cached DeepFallbackAnalyzer for the given path.
// The cache key is (path, HEAD SHA) — a new commit invalidates the cache.
func CachedDeepFallback(path string, pool ...lsp.Pool) *DeepFallbackAnalyzer {
	sha := resolveHead(path)
	key := path + "@" + sha

	if cached, ok := deepCache.Load(key); ok {
		return cached.(*DeepFallbackAnalyzer)
	}

	var p lsp.Pool
	if len(pool) > 0 {
		p = pool[0]
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
