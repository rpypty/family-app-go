package handler

import (
	analyticsdomain "family-app-go/internal/domain/analytics"
	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	gymdomain "family-app-go/internal/domain/gym"
	syncdomain "family-app-go/internal/domain/sync"
	todosdomain "family-app-go/internal/domain/todos"
	"family-app-go/pkg/logger"
)

type Handlers struct {
	Analytics *analyticsdomain.Service
	Families  *familydomain.Service
	Expenses  *expensesdomain.Service
	Todos     *todosdomain.Service
	Sync      *syncdomain.Service
	Gym       *gymdomain.Service
	log       logger.Logger
}

func New(analytics *analyticsdomain.Service, families *familydomain.Service, expenses *expensesdomain.Service, todos *todosdomain.Service, sync *syncdomain.Service, gym *gymdomain.Service, log logger.Logger) *Handlers {
	return &Handlers{
		Analytics: analytics,
		Families:  families,
		Expenses:  expenses,
		Todos:     todos,
		Sync:      sync,
		Gym:       gym,
		log:       log,
	}
}
