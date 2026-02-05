package handler

import (
	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
)

type Handlers struct {
	Families *familydomain.Service
	Expenses *expensesdomain.Service
}

func New(families *familydomain.Service, expenses *expensesdomain.Service) *Handlers {
	return &Handlers{
		Families: families,
		Expenses: expenses,
	}
}
