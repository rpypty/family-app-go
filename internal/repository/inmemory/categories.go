package inmemory

import (
	"sync"
	"time"

	expensesdomain "family-app-go/internal/domain/expenses"
)

type InMemoryCategoriesCache struct {
	mu    sync.RWMutex
	items map[string]categoriesItem
}

type categoriesItem struct {
	value     []expensesdomain.Category
	expiresAt time.Time
}

func NewInMemoryCategoriesCache() *InMemoryCategoriesCache {
	return &InMemoryCategoriesCache{
		items: make(map[string]categoriesItem),
	}
}

func (c *InMemoryCategoriesCache) GetByFamilyID(familyID string) ([]expensesdomain.Category, bool) {
	now := time.Now()

	c.mu.RLock()
	item, ok := c.items[familyID]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}

	if !item.expiresAt.After(now) {
		c.mu.Lock()
		item, ok = c.items[familyID]
		if ok && !item.expiresAt.After(now) {
			delete(c.items, familyID)
		}
		c.mu.Unlock()
		return nil, false
	}

	return cloneCategories(item.value), true
}

func (c *InMemoryCategoriesCache) SetByFamilyID(familyID string, categories []expensesdomain.Category, ttl time.Duration) {
	if ttl <= 0 {
		c.DeleteByFamilyID(familyID)
		return
	}

	c.mu.Lock()
	c.items[familyID] = categoriesItem{
		value:     cloneCategories(categories),
		expiresAt: time.Now().Add(ttl),
	}
	c.mu.Unlock()
}

func (c *InMemoryCategoriesCache) DeleteByFamilyID(familyID string) {
	c.mu.Lock()
	delete(c.items, familyID)
	c.mu.Unlock()
}

func cloneCategories(categories []expensesdomain.Category) []expensesdomain.Category {
	if categories == nil {
		return nil
	}
	cloned := make([]expensesdomain.Category, len(categories))
	for i := range categories {
		cloned[i] = categories[i]
		if categories[i].Color != nil {
			color := *categories[i].Color
			cloned[i].Color = &color
		}
		if categories[i].Emoji != nil {
			emoji := *categories[i].Emoji
			cloned[i].Emoji = &emoji
		}
	}
	return cloned
}
