package family

import "time"

type Cache interface {
	GetByUserID(userID string) (*Family, bool)
	SetByUserID(userID string, family *Family, ttl time.Duration)
	DeleteByUserID(userID string)
	Clear()
}

type noopCache struct{}

func (noopCache) GetByUserID(string) (*Family, bool) {
	return nil, false
}

func (noopCache) SetByUserID(string, *Family, time.Duration) {}

func (noopCache) DeleteByUserID(string) {}

func (noopCache) Clear() {}
