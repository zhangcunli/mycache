package mycache

import (
	"container/list"
	"fmt"
)

//LFU O(1): http://dhruvbird.com/lfu.pdf
type LfuCache struct {
	baseCache
	items    map[interface{}]*lfuItem
	freqList *list.List // list, 访问频率, 每个节点的值为 freqEntry
}

type freqEntry struct {
	freq  uint                  //访问频率，增加一个节点时，freq = 0 ???
	items map[*lfuItem]struct{} //map，访问频率为 freq 的节点集合
}

type lfuItem struct {
	clock       Clock
	expireAt    int64
	key         interface{}
	value       interface{}
	freqElement *list.Element //指向 freqList 头部吗??? 还是该节点访问频率的节点
}

func newLfuCache(cb *CacheBuilder) *LfuCache {
	lf := &LfuCache{}
	buildCache(&lf.baseCache, cb)

	lf.init()
	return lf
}

func (lf *LfuCache) init() {
	lf.freqList = list.New()
	lf.items = make(map[interface{}]*lfuItem, lf.size+1)
	lf.freqList.PushFront(&freqEntry{
		freq:  0,
		items: make(map[*lfuItem]struct{}),
	})
}

func (lf *LfuCache) String() string {
	//str := fmt.Sprintf("LfuCache:[size:%d, count:%d]", lf.size, len(sc.items))
	fmt.Printf("\n======>1. LfuCache:[size:%d, count:%d], freqListLen:[%d]\n", lf.size, len(lf.items), lf.freqList.Len())

	for k, v := range lf.items {
		fmt.Printf("-------->2. key:[%v], value:[%v]\n", k, v.value)
	}

	head := lf.freqList.Front()
	for head != nil {
		fe := head.Value.(*freqEntry)
		fmt.Printf("----------->3. freq:[%d]\n", fe.freq)
		for k, _ := range fe.items {
			fmt.Printf("----------------->. key:[%v], value:[%v]\n", k.key, k.value)
		}
		head = head.Next()
	}

	return ""
}

func (lf *LfuCache) Get(key interface{}) (interface{}, error) {
	v, err := lf.getValue(key)
	if err != nil {
		lf.stats.IncrMissCount()
		return nil, err
	}

	lf.stats.IncrHitCount()
	return v, nil
}

func (lf *LfuCache) Set(key interface{}, value interface{}) error {
	lf.mutex.Lock()
	defer lf.mutex.Unlock()
	return lf.set(key, value, 0)
}

func (lf *LfuCache) Del(key interface{}) error {
	lf.mutex.Lock()
	defer lf.mutex.Unlock()
	return lf.remove(key)
}

func (lf *LfuCache) Expire(key interface{}, expireSeconds int64) error {
	value, err := lf.Get(key)
	if err != nil {
		return err
	}

	lf.mutex.Lock()
	defer lf.mutex.Unlock()
	return lf.set(key, value, expireSeconds)
}

func (lf *LfuCache) TTL(key interface{}) (int64, error) {
	lf.mutex.RLock()
	defer lf.mutex.RUnlock()

	if item, ok := lf.items[key]; ok {
		now := item.clock.Now()
		if !item.IsExpired(item.clock.Now()) {
			var ttl int64
			if item.expireAt == 0 {
				ttl = -1
			} else {
				ttl = item.expireAt - now
			}
			lf.stats.IncrHitCount()
			lf.increment(item)
			return ttl, nil
		}
		lf.removeItem(item)
	}

	lf.stats.IncrMissCount()
	return -2, KeyNotFound
}

func (lf *LfuCache) SetWithExpire(key interface{}, value interface{}, expireSeconds int64) error {
	lf.mutex.Lock()
	defer lf.mutex.Unlock()
	return lf.set(key, value, expireSeconds)
}

func (lf *LfuCache) GetALL() map[interface{}]interface{} {
	keyvalues := make(map[interface{}]interface{})
	for _, k := range lf.keys() {
		v, err := lf.Get(k)
		if err == nil {
			keyvalues[k] = v
		}
	}
	return keyvalues
}

func (lf *LfuCache) Keys() []interface{} {
	keys := []interface{}{}
	for _, k := range lf.keys() {
		_, err := lf.Get(k)
		if err == nil {
			keys = append(keys, k)
		}
	}
	return keys
}

func (lf *LfuCache) Len() int {
	return len(lf.Keys())
}

func (lf *LfuCache) Clear() {
	lf.mutex.Lock()
	defer lf.mutex.Unlock()
	lf.init()
}

func (lf *LfuCache) set(key, value interface{}, expireSeconds int64) error {
	expireAt := lf.clock.ExpireAt(expireSeconds)

	item, ok := lf.items[key]
	if ok {
		item.value = value
		item.expireAt = expireAt
	} else {
		if len(lf.items) >= lf.size {
			lf.evict(1)
		}

		item = &lfuItem{
			clock:       lf.clock,
			key:         key,
			value:       value,
			expireAt:    expireAt,
			freqElement: nil,
		}

		//当增加一个节点时，将这个节点放入 freqList 头部
		//新增节点的访问频率是 0
		el := lf.freqList.Front()
		fe := el.Value.(*freqEntry)
		fe.items[item] = struct{}{} //将新加点加入访问频率为 0 的集合中

		//节点的 freqElement list 指向 freqList 头部
		item.freqElement = el
		lf.items[key] = item

		lf.stats.IncrKeyCount()
	}

	return nil
}

func (lf *LfuCache) getValue(key interface{}) (interface{}, error) {
	lf.mutex.RLock()
	defer lf.mutex.RUnlock()
	item, ok := lf.items[key]
	if ok {
		if !item.IsExpired(item.clock.Now()) {
			lf.increment(item)
			v := item.value
			return v, nil
		}
		lf.removeItem(item)
	}

	return nil, KeyNotFound
}

func (lf *LfuCache) remove(key interface{}) error {
	if ent, ok := lf.items[key]; ok {
		lf.removeItem(ent)
		return nil
	}
	return KeyNotFound
}

func (lf *LfuCache) removeItem(item *lfuItem) {
	delete(lf.items, item.key)
	delete(item.freqElement.Value.(*freqEntry).items, item)
}

func (lf *LfuCache) evict(count int) {
	entry := lf.freqList.Front()
	for i := 0; i < count; {
		if entry == nil {
			return
		} else {
			for item, _ := range entry.Value.(*freqEntry).items {
				if i >= count {
					return
				}
				lf.removeItem(item)
				i++
			}
			entry = entry.Next()
		}
	}
}

func (lf *LfuCache) keys() []interface{} {
	lf.mutex.RLock()
	defer lf.mutex.RUnlock()

	keys := make([]interface{}, len(lf.items))
	var i = 0
	for k := range lf.items {
		keys[i] = k
		i++
	}
	return keys
}

////LFU 最关键的函数
func (lf *LfuCache) increment(item *lfuItem) {
	currentFreqElement := item.freqElement
	currentFreqEntry := currentFreqElement.Value.(*freqEntry)
	nextFreq := currentFreqEntry.freq + 1
	delete(currentFreqEntry.items, item)

	nextFreqElement := currentFreqElement.Next()
	if nextFreqElement == nil || nextFreqElement.Value.(*freqEntry).freq > nextFreq {
		nextFreqElement = lf.freqList.InsertAfter(&freqEntry{
			freq:  nextFreq,
			items: make(map[*lfuItem]struct{}),
		}, currentFreqElement)
	}
	nextFreqElement.Value.(*freqEntry).items[item] = struct{}{}
	item.freqElement = nextFreqElement

	//如果 freqList 链表中，某个 freq 中的元素个数为 0，则删除该节点；但为了代码简单，保留了 freq = 0 节点;
	if len(currentFreqEntry.items) == 0 && currentFreqEntry.freq != 0 {
		lf.freqList.Remove(currentFreqElement)
	}
}

func (lt *lfuItem) IsExpired(now int64) bool {
	if lt.expireAt == 0 {
		return false
	}

	if lt.expireAt < now {
		return true
	}
	return false
}
