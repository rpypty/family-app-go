package common

import (
	familydomain "family-app-go/internal/domain/family"
	syncdomain "family-app-go/internal/domain/sync"
	"family-app-go/pkg/logger"
)

type Handlers struct {
	Families *familydomain.Service
	Sync     *syncdomain.Service
	log      logger.Logger
}

func New(families *familydomain.Service, sync *syncdomain.Service, log logger.Logger) *Handlers {
	return &Handlers{
		Families: families,
		Sync:     sync,
		log:      log,
	}
}
