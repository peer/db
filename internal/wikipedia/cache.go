package wikipedia

import (
	"sync/atomic"

	lru "github.com/hashicorp/golang-lru"
)

type Cache struct {
	*lru.Cache
	missCount uint64
}

func NewCache(size int) (*Cache, error) {
	cache, err := lru.New(size)
	if err != nil {
		return nil, err
	}
	return &Cache{
		Cache:     cache,
		missCount: 0,
	}, nil
}

func (c *Cache) Get(key interface{}) (interface{}, bool) {
	value, ok := c.Cache.Get(key)
	if !ok {
		atomic.AddUint64(&c.missCount, 1)
	}
	return value, ok
}

func (c *Cache) MissCount() uint64 {
	return atomic.SwapUint64(&c.missCount, 0)
}
