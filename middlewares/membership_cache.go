package middlewares

import (
	"container/list"
	"sync"
	"time"
)

// ttlEntry stores the cached value and its expiration time.
type ttlEntry struct {
	key      string
	value    []string
	expires  time.Time
	listElem *list.Element
}

// TTLCache is a bounded LRU cache with per-entry TTL. It is used by
// WorkspaceMembershipResolver to amortize Keto ListWorkspacesForUser
// calls across requests from the same user.
//
// NOT safe for concurrent use by callers; internally synchronized.
type TTLCache struct {
	mu      sync.Mutex
	entries map[string]*ttlEntry
	order   *list.List
	maxSize int
	ttl     time.Duration
}

// NewTTLCache returns a cache with maxSize entries and a fixed TTL.
func NewTTLCache(maxSize int, ttl time.Duration) *TTLCache {
	return &TTLCache{
		entries: make(map[string]*ttlEntry, maxSize),
		order:   list.New(),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get returns a cached value if present and not expired. If expired or
// missing, the second return value is false.
func (c *TTLCache) Get(key string) ([]string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	ent, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(ent.expires) {
		c.deleteEntry(ent)
		return nil, false
	}
	c.touch(ent)
	return ent.value, true
}

// Set stores the value for key with the configured TTL. If the cache is
// over capacity, the oldest unused entry is evicted.
func (c *TTLCache) Set(key string, value []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ent, ok := c.entries[key]; ok {
		ent.value = value
		ent.expires = time.Now().Add(c.ttl)
		c.touch(ent)
		return
	}

	if c.maxSize > 0 && len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	ent := &ttlEntry{
		key:     key,
		value:   value,
		expires: time.Now().Add(c.ttl),
	}
	ent.listElem = c.order.PushFront(ent)
	c.entries[key] = ent
}

// touch moves the entry to the front of the recency list.
func (c *TTLCache) touch(ent *ttlEntry) {
	c.order.MoveToFront(ent.listElem)
}

// deleteEntry removes a single entry from the cache maps.
func (c *TTLCache) deleteEntry(ent *ttlEntry) {
	c.order.Remove(ent.listElem)
	delete(c.entries, ent.key)
}

// evictOldest removes the least recently used entry.
func (c *TTLCache) evictOldest() {
	if elem := c.order.Back(); elem != nil {
		c.deleteEntry(elem.Value.(*ttlEntry))
	}
}
