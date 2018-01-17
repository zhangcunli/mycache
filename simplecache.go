package mycache

import (
	"fmt"
)

// SimpleCache has no clear priority for evict cache. It depends on key-value map order.
type SimpleCache struct {
	baseCache
	items map[interface{}]*simpleItem
}

type simpleItem struct {
	clock    Clock
	expireAt int64
	value    interface{}
}

func newSimpleCache(cb *CacheBuilder) *SimpleCache {
	sc := &SimpleCache{}
	buildCache(&sc.baseCache, cb)

	sc.init()
	return sc
}

func (sc *SimpleCache) init() {
	if sc.size <= 0 {
		sc.items = make(map[interface{}]*simpleItem)
	} else {
		sc.items = make(map[interface{}]*simpleItem, sc.size)
	}
}

func (sc *SimpleCache) String() string {
	return fmt.Sprintf("SimpleCache:[size:%d, count:%d]", sc.size, len(sc.items))
}

func (sc *SimpleCache) Get(key interface{}) (interface{}, error) {
	v, err := sc.getValue(key)
	if err != nil {
		sc.stats.IncrMissCount()
		return nil, err
	}

	sc.stats.IncrHitCount()
	return v, nil
}

func (sc *SimpleCache) Set(key, value interface{}) error {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	return sc.set(key, value, 0)
}

func (sc *SimpleCache) Del(key interface{}) error {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	return sc.remove(key)
}

func (sc *SimpleCache) Expire(key interface{}, expireSeconds int64) error {
	value, err := sc.Get(key)
	if err != nil {
		return err
	}

	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	err = sc.set(key, value, expireSeconds)
	return err
}

func (sc *SimpleCache) TTL(key interface{}) (int64, error) {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	item, ok := sc.items[key]
	if ok {
		now := item.clock.Now()
		if !item.IsExpired(now) {
			var ttl int64
			if item.expireAt == 0 {
				ttl = -1
			} else {
				ttl = item.expireAt - now
			}
			sc.stats.IncrHitCount()
			return ttl, nil
		}
		sc.remove(key)
	}

	sc.stats.IncrMissCount()
	return -2, KeyNotFound
}

func (sc *SimpleCache) SetWithExpire(key interface{}, value interface{}, expireSeconds int64) error {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	return sc.set(key, value, expireSeconds)
}

func (sc *SimpleCache) GetALL() map[interface{}]interface{} {
	keys := sc.keys()
	m := make(map[interface{}]interface{})
	for _, k := range keys {
		if v, err := sc.Get(k); err != nil {
			continue
		} else {
			m[k] = v
		}
	}
	return m
}

func (sc *SimpleCache) Keys() []interface{} {
	keys := sc.keys()
	validkeys := []interface{}{}
	for _, k := range keys {
		if _, err := sc.getValue(k); err != nil {
			continue
		}
		validkeys = append(validkeys, k)
	}
	return validkeys
}

func (sc *SimpleCache) Len() int {
	return len(sc.Keys())
}

func (sc *SimpleCache) Clear() {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	sc.init()
}

func (sc *simpleItem) IsExpired(now int64) bool {
	if sc.expireAt == 0 {
		return false
	}

	if sc.expireAt < now {
		return true
	}
	return false
}

func (sc *SimpleCache) getValue(key interface{}) (interface{}, error) {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	if item, ok := sc.items[key]; ok {
		if !item.IsExpired(item.clock.Now()) {
			v := item.value
			return v, nil
		}
		sc.remove(key)
	}
	return nil, KeyNotFound
}

func (sc *SimpleCache) set(key, value interface{}, expireSeconds int64) error {
	expireAt := sc.clock.ExpireAt(expireSeconds)

	// Check for existing item
	item, ok := sc.items[key]
	if ok {
		item.value = value
		item.expireAt = expireAt
	} else {
		if (len(sc.items) >= sc.size) && sc.size > 0 {
			sc.evict(1)
		}

		item = &simpleItem{
			clock:    sc.clock,
			value:    value,
			expireAt: expireAt,
		}
		sc.items[key] = item
		sc.stats.IncrKeyCount()
	}
	return nil
}

func (sc *SimpleCache) evict(count int) {
	now := sc.clock.Now()
	current := 0
	for key, item := range sc.items {
		if current >= count {
			return
		}

		//TODO
		if item.expireAt == 0 || item.expireAt < now {
			sc.stats.IncrEvictCount()
			defer sc.remove(key)
			current++
		}
	}
}

func (sc *SimpleCache) remove(key interface{}) error {
	if _, ok := sc.items[key]; ok {
		delete(sc.items, key)
		sc.stats.DecrKeyCount()
		return nil
	}
	return KeyNotFound
}

func (sc *SimpleCache) keys() []interface{} {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	keys := make([]interface{}, len(sc.items))
	var i = 0
	for k := range sc.items {
		keys[i] = k
		i++
	}
	return keys
}
