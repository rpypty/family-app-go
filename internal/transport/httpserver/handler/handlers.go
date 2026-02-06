package handler

import (
	analyticsdomain "family-app-go/internal/domain/analytics"
	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	todosdomain "family-app-go/internal/domain/todos"
)

type Handlers struct {
	Analytics *analyticsdomain.Service
	Families  *familydomain.Service
	Expenses  *expensesdomain.Service
	Todos     *todosdomain.Service
}

func New(analytics *analyticsdomain.Service, families *familydomain.Service, expenses *expensesdomain.Service, todos *todosdomain.Service) *Handlers {
	return &Handlers{
		Analytics: analytics,
		Families:  families,
		Expenses:  expenses,
		Todos:     todos,
	}
}
