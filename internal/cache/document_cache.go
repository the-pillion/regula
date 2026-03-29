package cache

import (
	"sync"
	"time"

	"github.com/pillion/regula/internal/store"
)

type latestDocumentEntry struct {
	value     store.GetLatestPublishedDocumentVersionRow
	expiresAt time.Time
}

type LatestDocumentCache struct {
	mu       sync.RWMutex
	ttl      time.Duration
	maxItems int
	items    map[string]latestDocumentEntry
}

func NewLatestDocumentCache(ttl time.Duration, maxItems int) *LatestDocumentCache {
	if ttl <= 0 || maxItems <= 0 {
		return nil
	}

	return &LatestDocumentCache{
		ttl:      ttl,
		maxItems: maxItems,
		items:    make(map[string]latestDocumentEntry, maxItems),
	}
}

func (c *LatestDocumentCache) Get(key string) (store.GetLatestPublishedDocumentVersionRow, bool) {
	if c == nil {
		return store.GetLatestPublishedDocumentVersionRow{}, false
	}

	now := time.Now()

	c.mu.RLock()
	entry, ok := c.items[key]
	c.mu.RUnlock()
	if !ok {
		return store.GetLatestPublishedDocumentVersionRow{}, false
	}
	if now.After(entry.expiresAt) {
		c.mu.Lock()
		delete(c.items, key)
		c.mu.Unlock()
		return store.GetLatestPublishedDocumentVersionRow{}, false
	}

	return entry.value, true
}

func (c *LatestDocumentCache) Set(key string, value store.GetLatestPublishedDocumentVersionRow) {
	if c == nil {
		return
	}

	now := time.Now()

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.items) >= c.maxItems {
		for itemKey, entry := range c.items {
			if now.After(entry.expiresAt) {
				delete(c.items, itemKey)
			}
		}
	}
	if len(c.items) >= c.maxItems {
		for itemKey := range c.items {
			delete(c.items, itemKey)
			break
		}
	}

	c.items[key] = latestDocumentEntry{
		value:     value,
		expiresAt: now.Add(c.ttl),
	}
}

func (c *LatestDocumentCache) InvalidatePrefix(prefix string) {
	if c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for key := range c.items {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			delete(c.items, key)
		}
	}
}
