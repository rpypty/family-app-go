package expenses

import (
	analyticsdomain "family-app-go/internal/domain/analytics"
	expensesdomain "family-app-go/internal/domain/expenses"
	familydomain "family-app-go/internal/domain/family"
	ratesdomain "family-app-go/internal/domain/rates"
	"family-app-go/pkg/logger"
)

type Handlers struct {
	Analytics *analyticsdomain.Service
	Families  *familydomain.Service
	Expenses  *expensesdomain.Service
	Rates     *ratesdomain.Service
	log       logger.Logger
}

func New(analytics *analyticsdomain.Service, families *familydomain.Service, expenses *expensesdomain.Service, rates *ratesdomain.Service, log logger.Logger) *Handlers {
	return &Handlers{
		Analytics: analytics,
		Families:  families,
		Expenses:  expenses,
		Rates:     rates,
		log:       log,
	}
}
