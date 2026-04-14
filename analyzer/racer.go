package analyzer

import (
	"context"
	"sync"
	"time"
)

// QualityTier ranks analyzer output quality. Higher = more accurate.
type QualityTier int

const (
	QualityRegex      QualityTier = 10  // pattern matching, ~50% accurate
	QualityTreeSitter QualityTier = 50  // AST parsing, ~80% accurate
	QualityGoAST      QualityTier = 90  // native Go parser, ~95% accurate
	QualityLSP        QualityTier = 100 // semantic analysis, ~99% accurate
)

// Attempt represents one analyzer's entry in a race.
type Attempt[T any] struct {
	Name    string
	Quality QualityTier
	Fn      func(ctx context.Context) (T, error)
}

// RaceResult is the outcome of a race.
type RaceResult[T any] struct {
	Value   T
	Winner  string
	Quality QualityTier
	Elapsed time.Duration
	Cached  bool
}

// Racer races multiple analyzers in parallel. The first non-empty result
// wins and returns immediately. Losers continue in background and cache
// their results with quality tiers. Next call returns the highest-quality
// cached result in O(1).
type Racer[T any] struct {
	attempts   []Attempt[T]
	isEmpty    func(T) bool
	minQuality QualityTier // results below this quality are treated as empty

	mu    sync.RWMutex
	cache *RaceResult[T]
}

// NewRacer creates a racer with the given attempts and emptiness check.
func NewRacer[T any](isEmpty func(T) bool, attempts ...Attempt[T]) *Racer[T] {
	return &Racer[T]{
		attempts: attempts,
		isEmpty:  isEmpty,
	}
}

// WithMinQuality sets the minimum quality threshold. Results from attempts
// below this quality are treated as empty — the racer keeps waiting for
// a higher-quality result.
func (r *Racer[T]) WithMinQuality(q QualityTier) *Racer[T] {
	r.minQuality = q
	return r
}

// Race runs all attempts in parallel. Returns the first non-empty result.
// Losers continue in background and upgrade the cache if they produce
// higher-quality results.
func (r *Racer[T]) Race(ctx context.Context) (*RaceResult[T], error) {
	// Check cache first — return highest-quality cached result.
	r.mu.RLock()
	if r.cache != nil {
		cached := *r.cache
		cached.Cached = true
		r.mu.RUnlock()
		return &cached, nil
	}
	r.mu.RUnlock()

	// Race all attempts in parallel.
	type result struct {
		value   T
		name    string
		quality QualityTier
		elapsed time.Duration
		err     error
	}

	ch := make(chan result, len(r.attempts))
	start := time.Now()

	for _, a := range r.attempts {
		go func(attempt Attempt[T]) {
			v, err := attempt.Fn(ctx)
			ch <- result{
				value:   v,
				name:    attempt.Name,
				quality: attempt.Quality,
				elapsed: time.Since(start),
				err:     err,
			}
		}(a)
	}

	// Wait for first non-empty result.
	var winner *RaceResult[T]
	remaining := len(r.attempts)

	for remaining > 0 {
		select {
		case <-ctx.Done():
			var zero T
			return &RaceResult[T]{Value: zero}, ctx.Err()
		case res := <-ch:
			remaining--

			if res.err != nil || r.isEmpty(res.value) || res.quality < r.minQuality {
				continue
			}

			rr := &RaceResult[T]{
				Value:   res.value,
				Winner:  res.name,
				Quality: res.quality,
				Elapsed: res.elapsed,
			}

			if winner == nil {
				winner = rr
				// Cache this result.
				r.mu.Lock()
				r.cache = rr
				r.mu.Unlock()

				// Don't return yet — drain remaining results in background
				// to potentially upgrade cache with higher quality.
				go func() {
					for remaining > 0 {
						select {
						case <-ctx.Done():
							return
						case bg := <-ch:
							remaining--
							if bg.err != nil || r.isEmpty(bg.value) {
								continue
							}
							r.mu.Lock()
							if r.cache == nil || bg.quality > r.cache.Quality {
								r.cache = &RaceResult[T]{
									Value:   bg.value,
									Winner:  bg.name,
									Quality: bg.quality,
									Elapsed: bg.elapsed,
								}
							}
							r.mu.Unlock()
						}
					}
				}()

				return winner, nil
			}
		}
	}

	// All attempts empty or errored.
	if winner == nil {
		var zero T
		return &RaceResult[T]{Value: zero}, nil
	}
	return winner, nil
}

// Invalidate clears the cached result. Next Race() runs fresh.
func (r *Racer[T]) Invalidate() {
	r.mu.Lock()
	r.cache = nil
	r.mu.Unlock()
}

// CachedQuality returns the quality tier of the cached result, or 0 if uncached.
func (r *Racer[T]) CachedQuality() QualityTier {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.cache == nil {
		return 0
	}
	return r.cache.Quality
}
