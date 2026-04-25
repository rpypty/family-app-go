package common

import (
	"context"

	"family-app-go/internal/devseed"
	familydomain "family-app-go/internal/domain/family"
	syncdomain "family-app-go/internal/domain/sync"
	"family-app-go/pkg/logger"
)

type FamilySeeder interface {
	SeedFamily(ctx context.Context, input devseed.SeedFamilyInput) (devseed.SeedFamilyResult, error)
}

type Handlers struct {
	Families     *familydomain.Service
	Sync         *syncdomain.Service
	FamilySeeder FamilySeeder
	log          logger.Logger
}

func New(families *familydomain.Service, sync *syncdomain.Service, log logger.Logger, seeders ...FamilySeeder) *Handlers {
	var familySeeder FamilySeeder
	if len(seeders) > 0 {
		familySeeder = seeders[0]
	}
	return &Handlers{
		Families:     families,
		Sync:         sync,
		FamilySeeder: familySeeder,
		log:          log,
	}
}
