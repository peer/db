package es

import (
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru/v2"
	"gitlab.com/tozd/go/errors"

	"gitlab.com/peerdb/peerdb/document"
)

// Cache is a LRU cache which counts cache misses.
type Cache struct {
	*lru.Cache[any, *document.D]

	missCount uint64
}

// NewCache creates a new LRU cache for document storage with the specified size.
func NewCache(size int) (*Cache, errors.E) {
	cache, err := lru.New[any, *document.D](size)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &Cache{
		Cache:     cache,
		missCount: 0,
	}, nil
}

// Get retrieves a document from the cache and tracks cache misses.
func (c *Cache) Get(key interface{}) (*document.D, bool) {
	value, ok := c.Cache.Get(key)
	if !ok {
		atomic.AddUint64(&c.missCount, 1)
	}
	return value, ok
}

// MissCount returns the number of cache misses since the last call
// of MissCount (or since the initialization of the cache).
func (c *Cache) MissCount() uint64 {
	return atomic.SwapUint64(&c.missCount, 0)
}
