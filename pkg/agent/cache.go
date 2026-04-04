package agent

import (
	"context"
	"sync"
)

// cacheEntry holds a cached result for a single input key.
type cacheEntry[O any] struct {
	value O
	err   error
}

// cacheAgent wraps an Agent[I, O] with a memoization layer.
// Identical inputs (as determined by keyFunc) are served from cache on subsequent calls.
type cacheAgent[I, O any] struct {
	inner   Agent[I, O]
	keyFunc func(I) string
	store   sync.Map // map[string]*cacheEntry[O]
}

// Name returns the name of the underlying agent, prefixed with "cache:".
func (c *cacheAgent[I, O]) Name() string {
	return "cache:" + c.inner.Name()
}

// Run checks the cache for a stored result matching keyFunc(input).
// On a cache hit, the stored value (or error) is returned immediately without
// calling the underlying agent. On a miss, the agent is called and the result
// is stored in the cache for future calls.
//
// Note: errors are also cached. If you do not want to cache errors, wrap the
// agent with WithRetry before applying WithCache.
func (c *cacheAgent[I, O]) Run(ctx context.Context, input I) (O, error) {
	key := c.keyFunc(input)

	// Fast path: check the cache.
	if v, ok := c.store.Load(key); ok {
		entry := v.(*cacheEntry[O])
		return entry.value, entry.err
	}

	// Cache miss: call the underlying agent.
	result, err := c.inner.Run(ctx, input)

	// Store the result (including errors) to avoid repeated failed calls for the same input.
	c.store.Store(key, &cacheEntry[O]{value: result, err: err})

	return result, err
}

// WithCache wraps an agent with a memoization layer.
//
// keyFunc maps an input value to a string cache key. Two inputs that produce the
// same key will share the cached result. The caller is responsible for choosing
// an appropriate key function.
//
// The cache is unbounded and lives for the lifetime of the returned agent.
// For production use with large input spaces, consider implementing a bounded cache.
//
// Example:
//
//	// Cache by URL string
//	cached := agent.WithCache(fetchAgent, func(url string) string { return url })
//
//	// Cache by struct field
//	cached := agent.WithCache(processAgent, func(req Request) string { return req.ID })
func WithCache[I, O any](a Agent[I, O], keyFunc func(I) string) Agent[I, O] {
	return &cacheAgent[I, O]{
		inner:   a,
		keyFunc: keyFunc,
	}
}
