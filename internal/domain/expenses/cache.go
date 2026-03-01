package expenses

import "time"

type CategoriesCache interface {
	GetByFamilyID(familyID string) ([]Category, bool)
	SetByFamilyID(familyID string, categories []Category, ttl time.Duration)
	DeleteByFamilyID(familyID string)
}

type noopCategoriesCache struct{}

func (noopCategoriesCache) GetByFamilyID(string) ([]Category, bool) {
	return nil, false
}

func (noopCategoriesCache) SetByFamilyID(string, []Category, time.Duration) {}

func (noopCategoriesCache) DeleteByFamilyID(string) {}
