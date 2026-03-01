package gym

import (
	gymdomain "family-app-go/internal/domain/gym"
	"family-app-go/pkg/logger"
)

type Handlers struct {
	Gym *gymdomain.Service
	log logger.Logger
}

func New(gym *gymdomain.Service, log logger.Logger) *Handlers {
	return &Handlers{
		Gym: gym,
		log: log,
	}
}
