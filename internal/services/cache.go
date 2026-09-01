package services

import (
	"container/list"
	"sync"
	"time"

	"github.com/kaicorplabs/linkup/internal/models"
)

type cacheEntry struct {
	key       string
	link      models.Link
	cachedAt  time.Time
}

// LinkCache is a thread-safe in-memory LRU cache for ultra-fast slug resolution.
type LinkCache struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*list.Element
	evictList *list.List
	ttl      time.Duration
}

func NewLinkCache(capacity int, ttl time.Duration) *LinkCache {
	if capacity <= 0 {
		capacity = 5000
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &LinkCache{
		capacity:  capacity,
		items:     make(map[string]*list.Element),
		evictList: list.New(),
		ttl:       ttl,
	}
}

// Get retrieves a link from cache if present and unexpired.
func (c *LinkCache) Get(slug string) (*models.Link, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.items[slug]
	if !found {
		return nil, false
	}

	entry := elem.Value.(*cacheEntry)

	// Check cache TTL
	if time.Since(entry.cachedAt) > c.ttl {
		c.removeElement(elem)
		return nil, false
	}

	// Move to front (most recently used)
	c.evictList.MoveToFront(elem)

	// Return a copy to avoid race conditions
	linkCopy := entry.link
	return &linkCopy, true
}

// Set stores or updates a link in cache.
func (c *LinkCache) Set(slug string, link *models.Link) {
	if link == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	// If already in cache, update value and move to front
	if elem, found := c.items[slug]; found {
		c.evictList.MoveToFront(elem)
		entry := elem.Value.(*cacheEntry)
		entry.link = *link
		entry.cachedAt = time.Now()
		return
	}

	// If at capacity, evict least recently used (back)
	if c.evictList.Len() >= c.capacity {
		c.evictOldest()
	}

	entry := &cacheEntry{
		key:      slug,
		link:     *link,
		cachedAt: time.Now(),
	}
	elem := c.evictList.PushFront(entry)
	c.items[slug] = elem
}

// Delete invalidates a slug in cache.
func (c *LinkCache) Delete(slug string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, found := c.items[slug]; found {
		c.removeElement(elem)
	}
}

// Clear flushes all entries.
func (c *LinkCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.evictList.Init()
}

// Len returns the current number of cached items.
func (c *LinkCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.evictList.Len()
}

func (c *LinkCache) removeElement(elem *list.Element) {
	c.evictList.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(c.items, entry.key)
}

func (c *LinkCache) evictOldest() {
	elem := c.evictList.Back()
	if elem != nil {
		c.removeElement(elem)
	}
}
