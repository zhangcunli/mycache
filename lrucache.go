package mycache

import (
	"container/list"
	"fmt"
)

type LruCache struct {
	baseCache
	items     map[interface{}]*list.Element
	evictList *list.List
}

type lruItem struct {
	clock    Clock
	expireAt int64
	key      interface{}
	value    interface{}
}

func newLruCache(cb *CacheBuilder) *LruCache {
	lc := &LruCache{}
	buildCache(&lc.baseCache, cb)

	lc.init()
	return lc
}

func (lc *LruCache) init() {
	lc.evictList = list.New()
	lc.items = make(map[interface{}]*list.Element, lc.size+1) //?????
}

func (lc *LruCache) String() string {
	return fmt.Sprintf("LruCache:[size:%d, count:%d, evictList:%d]", lc.size, len(lc.items), lc.evictList.Len())
}

func (lc *LruCache) Get(key interface{}) (interface{}, error) {
	v, err := lc.getValue(key)
	if err != nil {
		lc.stats.IncrMissCount()
		return nil, err
	}

	lc.stats.IncrHitCount()
	return v, nil
}

func (lc *LruCache) Set(key interface{}, value interface{}) error {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	return lc.set(key, value, 0)
}

func (lc *LruCache) Del(key interface{}) error {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	return lc.remove(key)
}

func (lc *LruCache) Expire(key interface{}, expireSeconds int64) error {
	value, err := lc.Get(key)
	if err != nil {
		return err
	}

	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	return lc.set(key, value, expireSeconds)
}

func (lc *LruCache) TTL(key interface{}) (int64, error) {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	if item, ok := lc.items[key]; ok {
		it := item.Value.(*lruItem)
		now := it.clock.Now()
		if !it.IsExpired(it.clock.Now()) {
			var ttl int64
			if it.expireAt == 0 {
				ttl = -1
			} else {
				ttl = it.expireAt - now
			}
			lc.stats.IncrHitCount()
			lc.evictList.MoveToFront(item)
			return ttl, nil
		}
		lc.removeElement(item)
	}

	lc.stats.IncrMissCount()
	return -2, KeyNotFound
}

func (lc *LruCache) SetWithExpire(key interface{}, value interface{}, expireSeconds int64) error {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	return lc.set(key, value, expireSeconds)
}

func (lc *LruCache) GetALL() map[interface{}]interface{} {
	keyvalues := make(map[interface{}]interface{})
	for _, k := range lc.keys() {
		v, err := lc.Get(k)
		if err == nil {
			keyvalues[k] = v
		}
	}
	return keyvalues
}

func (lc *LruCache) Keys() []interface{} {
	keys := []interface{}{}
	for _, k := range lc.keys() {
		_, err := lc.Get(k)
		if err == nil {
			keys = append(keys, k)
		}
	}
	return keys
}

func (lc *LruCache) Len() int {
	return len(lc.Keys())
}

func (lc *LruCache) Clear() {
	lc.mutex.Lock()
	defer lc.mutex.Unlock()
	lc.init()
}

//////////////////////////////////////////////////////
func (lc *LruCache) getValue(key interface{}) (interface{}, error) {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()

	if item, ok := lc.items[key]; ok {
		it := item.Value.(*lruItem)
		if !it.IsExpired(it.clock.Now()) {
			lc.evictList.MoveToFront(item)
			return it.value, nil
		}
		lc.removeElement(item)
	}
	return nil, KeyNotFound
}

func (lc *LruCache) set(key, value interface{}, expireSeconds int64) error {
	var item *lruItem
	if element, ok := lc.items[key]; ok {
		item := element.Value.(*lruItem)
		lc.evictList.MoveToFront(element)
		item.value = value
		item.expireAt = item.clock.ExpireAt(expireSeconds)
	} else {
		if lc.evictList.Len() >= lc.size {
			lc.evict(1)
			lc.stats.IncrEvictCount()
		}
		item = &lruItem{
			clock: lc.clock,
			key:   key,
			value: value,
		}
		item.expireAt = item.clock.ExpireAt(expireSeconds)
		lc.items[key] = lc.evictList.PushFront(item)

		lc.stats.IncrKeyCount()
	}
	return nil
}

func (lc *LruCache) remove(key interface{}) error {
	if ent, ok := lc.items[key]; ok {
		lc.removeElement(ent)
		return nil
	}
	return KeyNotFound
}

func (lc *LruCache) removeElement(e *list.Element) {
	lc.evictList.Remove(e)
	entry := e.Value.(*lruItem)
	delete(lc.items, entry.key)
	lc.stats.DecrKeyCount()
}

func (lc *LruCache) evict(num int) {
	for i := 0; i < num; i++ {
		ent := lc.evictList.Back()
		if ent == nil {
			return
		} else {
			lc.removeElement(ent)
		}
	}
}

func (lc *LruCache) keys() []interface{} {
	lc.mutex.RLock()
	defer lc.mutex.RUnlock()
	keys := make([]interface{}, len(lc.items))
	var i = 0
	for k := range lc.items {
		keys[i] = k
		i++
	}
	return keys
}

func (lc *lruItem) IsExpired(now int64) bool {
	if lc.expireAt == 0 {
		return false
	}

	if lc.expireAt < now {
		return true
	}
	return false
}
