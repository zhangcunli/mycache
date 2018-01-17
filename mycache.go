package mycache

import (
	"errors"
	"sync"
)

const (
	TYPE_SIMPLE = "simple"
	TYPE_LRU    = "lru"
	TYPE_LFU    = "lfu"
)

var (
	KeyNotFound = errors.New("Key not found.")
)

type Cache interface {
	Get(interface{}) (interface{}, error)
	Set(interface{}, interface{}) error
	Del(interface{}) error
	Expire(interface{}, int64) error
	TTL(interface{}) (int64, error)
	SetWithExpire(interface{}, interface{}, int64) error

	GetALL() map[interface{}]interface{}
	Keys() []interface{}
	Len() int
	Clear()

	String() string

	statsAccessor

	/*
	   Mget([]interface{}) []interface{}
	   Mset(map[interface{}]interface{}) int
	   Mdel(key []interface{}) (delnum int)
	*/
}

type baseCache struct {
	size  int
	clock Clock
	mutex sync.RWMutex
	*stats
}

type CacheBuilder struct {
	clock Clock
	tp    string
	size  int
}

func New(size int) *CacheBuilder {
	return &CacheBuilder{
		clock: NewRealClock(),
		tp:    TYPE_SIMPLE,
		size:  size,
	}
}

func buildCache(c *baseCache, cb *CacheBuilder) {
	c.size = cb.size
	c.clock = cb.clock
	c.stats = &stats{}
}

func (cb *CacheBuilder) EvictType(tp string) *CacheBuilder {
	cb.tp = tp
	return cb
}

func (cb *CacheBuilder) Simple() *CacheBuilder {
	return cb.EvictType(TYPE_SIMPLE)
}

func (cb *CacheBuilder) LRU() *CacheBuilder {
	return cb.EvictType(TYPE_LRU)
}

func (cb *CacheBuilder) LFU() *CacheBuilder {
	return cb.EvictType(TYPE_LFU)
}

func (cb *CacheBuilder) Build() Cache {
	if cb.size <= 0 && cb.tp != TYPE_SIMPLE {
		panic("gcache: Cache size <= 0")
	}

	return cb.build()
}

func (cb *CacheBuilder) build() Cache {
	switch cb.tp {
	case TYPE_SIMPLE:
		return newSimpleCache(cb)
	case TYPE_LRU:
		return newLruCache(cb)
	case TYPE_LFU:
		return newLfuCache(cb)
	default:
		panic("gcache: Unknown type " + cb.tp)
	}
}
