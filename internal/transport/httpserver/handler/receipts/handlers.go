package receipts

import (
	familydomain "family-app-go/internal/domain/family"
	receiptsdomain "family-app-go/internal/domain/receipts"
	"family-app-go/pkg/logger"
)

type Handlers struct {
	Families *familydomain.Service
	Receipts *receiptsdomain.Service
	log      logger.Logger
}

func New(families *familydomain.Service, receipts *receiptsdomain.Service, log logger.Logger) *Handlers {
	return &Handlers{
		Families: families,
		Receipts: receipts,
		log:      log,
	}
}
