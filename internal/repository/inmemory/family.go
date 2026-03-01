package inmemory

import (
	"sync"
	"time"

	familydomain "family-app-go/internal/domain/family"
)

type InMemoryFamilyCache struct {
	mu    sync.RWMutex
	items map[string]familyItem
}

type familyItem struct {
	value     familydomain.Family
	expiresAt time.Time
}

func NewInMemoryFamilyCache() *InMemoryFamilyCache {
	return &InMemoryFamilyCache{
		items: make(map[string]familyItem),
	}
}

func (c *InMemoryFamilyCache) GetByUserID(userID string) (*familydomain.Family, bool) {
	now := time.Now()

	c.mu.RLock()
	item, ok := c.items[userID]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}

	if !item.expiresAt.After(now) {
		c.mu.Lock()
		item, ok = c.items[userID]
		if ok && !item.expiresAt.After(now) {
			delete(c.items, userID)
		}
		c.mu.Unlock()
		return nil, false
	}

	value := item.value
	return &value, true
}

func (c *InMemoryFamilyCache) SetByUserID(userID string, family *familydomain.Family, ttl time.Duration) {
	if family == nil || ttl <= 0 {
		c.DeleteByUserID(userID)
		return
	}

	c.mu.Lock()
	c.items[userID] = familyItem{
		value:     *family,
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

func (c *InMemoryFamilyCache) DeleteByUserID(userID string) {
	c.mu.Lock()
	delete(c.items, userID)
	c.mu.Unlock()
}

func (c *InMemoryFamilyCache) Clear() {
	c.mu.Lock()
	c.items = make(map[string]familyItem)
	c.mu.Unlock()
}
