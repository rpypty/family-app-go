package handler

import (
	analyticsdomain "family-app-go/internal/domain/analytics"
	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	gymdomain "family-app-go/internal/domain/gym"
	ratesdomain "family-app-go/internal/domain/rates"
	syncdomain "family-app-go/internal/domain/sync"
	todosdomain "family-app-go/internal/domain/todos"
	commonhandler "family-app-go/internal/transport/httpserver/handler/common"
	expenseshandler "family-app-go/internal/transport/httpserver/handler/expenses"
	gymhandler "family-app-go/internal/transport/httpserver/handler/gym"
	todoshandler "family-app-go/internal/transport/httpserver/handler/todos"
	"family-app-go/pkg/logger"
)

type Handlers struct {
	Common   *commonhandler.Handlers
	Expenses *expenseshandler.Handlers
	Todos    *todoshandler.Handlers
	Gym      *gymhandler.Handlers
}

func New(analytics *analyticsdomain.Service, families *familydomain.Service, expenses *expensesdomain.Service, rates *ratesdomain.Service, todos *todosdomain.Service, sync *syncdomain.Service, gym *gymdomain.Service, log logger.Logger, seeders ...commonhandler.FamilySeeder) *Handlers {
	return &Handlers{
		Common:   commonhandler.New(families, sync, log, seeders...),
		Expenses: expenseshandler.New(analytics, families, expenses, rates, log),
		Todos:    todoshandler.New(families, todos, log),
		Gym:      gymhandler.New(gym, log),
	}
}
