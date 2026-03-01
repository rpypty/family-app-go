package todos

import (
	familydomain "family-app-go/internal/domain/family"
	todosdomain "family-app-go/internal/domain/todos"
	"family-app-go/pkg/logger"
)

type Handlers struct {
	Families *familydomain.Service
	Todos    *todosdomain.Service
	log      logger.Logger
}

func New(families *familydomain.Service, todos *todosdomain.Service, log logger.Logger) *Handlers {
	return &Handlers{
		Families: families,
		Todos:    todos,
		log:      log,
	}
}
